package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

const (
	liquibaseRepository     = "liquibase/liquibase"
	registryURL             = "https://registry-1.docker.io"
	ociIndexMediaType       = "application/vnd.oci.image.index.v1+json"
	dockerIndexMediaType    = "application/vnd.docker.distribution.manifest.list.v2+json"
	ociManifestMediaType    = "application/vnd.oci.image.manifest.v1+json"
	dockerManifestMediaType = "application/vnd.docker.distribution.manifest.v2+json"
	maxRunnerIndexBytes     = 4 << 20
)

var runnerTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)
var runnerDigestPattern = regexp.MustCompile(`^sha256:[a-f0-9]{64}$`)
var bearerParamPattern = regexp.MustCompile(`([A-Za-z]+)="([^"]*)"`)

// RunnerPlatform is the exact target platform selected by the engine, not
// the platform inferred from a locally cached Docker tag.
type RunnerPlatform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant,omitempty"`
}

// RunnerImageBinding is the immutable image identity required before any
// Liquibase plan, cache lookup, or execution. See
// docs/architecture/liquibase-container-component-structure.md.
type RunnerImageBinding struct {
	RequestedTag   string
	Platform       RunnerPlatform
	IndexDigest    string
	ManifestDigest string
	DigestRef      string
}

// RunnerImageResolver obtains a fresh trusted-repository index for every
// Resolve call. It deliberately has no local-tag or previous-binding cache.
type RunnerImageResolver struct {
	baseURL string
	client  *http.Client
}

// RunnerPlatform reads the Docker daemon platform. It must not use
// runtime.GOOS/GOARCH: a Windows engine can address a Linux Docker daemon.
// Unknown architectures fail closed before a tag is resolved.
func (r *DockerRuntime) RunnerPlatform(ctx context.Context) (RunnerPlatform, error) {
	if r == nil {
		return RunnerPlatform{}, fmt.Errorf("Docker runtime is unavailable")
	}
	output, err := r.run(ctx, []string{"info", "--format", "{{json .}}"}, nil)
	if err != nil {
		return RunnerPlatform{}, fmt.Errorf("cannot inspect Docker platform: %w", err)
	}
	var info struct {
		OSType       string `json:"OSType"`
		Architecture string `json:"Architecture"`
	}
	if err := json.Unmarshal([]byte(output), &info); err != nil {
		return RunnerPlatform{}, fmt.Errorf("invalid Docker platform response: %w", err)
	}
	osName := strings.ToLower(strings.TrimSpace(info.OSType))
	arch := strings.ToLower(strings.TrimSpace(info.Architecture))
	switch arch {
	case "x86_64", "x86-64":
		arch = "amd64"
	case "aarch64":
		arch = "arm64"
	}
	if osName != "linux" && osName != "windows" || arch != "amd64" && arch != "arm64" {
		return RunnerPlatform{}, fmt.Errorf("unsupported Docker platform %q/%q", info.OSType, info.Architecture)
	}
	return RunnerPlatform{OS: osName, Architecture: arch}, nil
}

// NewRunnerImageResolver constructs the production Docker Hub resolver.
func NewRunnerImageResolver(client *http.Client) *RunnerImageResolver {
	return newRunnerImageResolver(registryURL, client)
}

func newRunnerImageResolver(baseURL string, client *http.Client) *RunnerImageResolver {
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	return &RunnerImageResolver{baseURL: strings.TrimRight(baseURL, "/"), client: client}
}

type runnerIndex struct {
	SchemaVersion int    `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Manifests     []struct {
		MediaType string         `json:"mediaType"`
		Digest    string         `json:"digest"`
		Platform  RunnerPlatform `json:"platform"`
	} `json:"manifests"`
}

// Resolve refreshes a tag and chooses exactly one platform-specific manifest.
// Neither Docker's local RepoDigests list nor an earlier resolution is used.
func (r *RunnerImageResolver) Resolve(ctx context.Context, tag string, platform RunnerPlatform) (RunnerImageBinding, error) {
	if r == nil || r.client == nil || r.baseURL == "" {
		return RunnerImageBinding{}, fmt.Errorf("runner image resolver is unavailable")
	}
	if !runnerTagPattern.MatchString(tag) || platform.OS == "" || platform.Architecture == "" {
		return RunnerImageBinding{}, fmt.Errorf("invalid Liquibase tag or platform")
	}
	manifestURL := r.baseURL + "/v2/" + liquibaseRepository + "/manifests/" + tag
	response, err := r.getManifest(ctx, manifestURL)
	if err != nil {
		return RunnerImageBinding{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return RunnerImageBinding{}, fmt.Errorf("Liquibase registry returned %s", response.Status)
	}
	mediaType, _, err := mime.ParseMediaType(response.Header.Get("Content-Type"))
	if err != nil || mediaType != ociIndexMediaType && mediaType != dockerIndexMediaType {
		return RunnerImageBinding{}, fmt.Errorf("unsupported Liquibase registry manifest type: %q", response.Header.Get("Content-Type"))
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxRunnerIndexBytes+1))
	if err != nil || len(body) > maxRunnerIndexBytes {
		return RunnerImageBinding{}, fmt.Errorf("cannot read Liquibase image index: %v", err)
	}
	sum := sha256.Sum256(body)
	indexDigest := "sha256:" + hex.EncodeToString(sum[:])
	if header := strings.TrimSpace(response.Header.Get("Docker-Content-Digest")); header != "" && header != indexDigest {
		return RunnerImageBinding{}, fmt.Errorf("Liquibase registry index digest does not match content")
	}
	var index runnerIndex
	if err := json.Unmarshal(body, &index); err != nil || index.SchemaVersion != 2 || len(index.Manifests) == 0 {
		return RunnerImageBinding{}, fmt.Errorf("invalid Liquibase image index")
	}
	if index.MediaType != "" && index.MediaType != mediaType {
		return RunnerImageBinding{}, fmt.Errorf("Liquibase image index media type mismatch")
	}
	selected := ""
	for _, descriptor := range index.Manifests {
		if descriptor.Platform != platform {
			continue
		}
		if descriptor.MediaType != ociManifestMediaType && descriptor.MediaType != dockerManifestMediaType || !runnerDigestPattern.MatchString(descriptor.Digest) {
			return RunnerImageBinding{}, fmt.Errorf("invalid platform manifest descriptor")
		}
		if selected != "" {
			return RunnerImageBinding{}, fmt.Errorf("ambiguous Liquibase manifest for platform %+v", platform)
		}
		selected = descriptor.Digest
	}
	if selected == "" {
		return RunnerImageBinding{}, fmt.Errorf("Liquibase image has no manifest for platform %+v", platform)
	}
	return RunnerImageBinding{RequestedTag: tag, Platform: platform, IndexDigest: indexDigest, ManifestDigest: selected, DigestRef: liquibaseRepository + "@" + selected}, nil
}

func (r *RunnerImageResolver) getManifest(ctx context.Context, manifestURL string) (*http.Response, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", strings.Join([]string{ociIndexMediaType, dockerIndexMediaType}, ", "))
	response, err := r.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Liquibase registry unavailable: %w", err)
	}
	if response.StatusCode != http.StatusUnauthorized {
		return response, nil
	}
	challenge := response.Header.Get("Www-Authenticate")
	response.Body.Close()
	token, err := r.fetchBearerToken(ctx, challenge)
	if err != nil {
		return nil, err
	}
	request, err = http.NewRequestWithContext(ctx, http.MethodGet, manifestURL, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("Accept", strings.Join([]string{ociIndexMediaType, dockerIndexMediaType}, ", "))
	request.Header.Set("Authorization", "Bearer "+token)
	response, err = r.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("Liquibase registry unavailable: %w", err)
	}
	return response, nil
}

func (r *RunnerImageResolver) fetchBearerToken(ctx context.Context, challenge string) (string, error) {
	if !strings.HasPrefix(strings.ToLower(challenge), "bearer ") {
		return "", fmt.Errorf("unsupported Liquibase registry authentication")
	}
	params := map[string]string{}
	for _, match := range bearerParamPattern.FindAllStringSubmatch(challenge, -1) {
		params[strings.ToLower(match[1])] = match[2]
	}
	realm, err := url.Parse(params["realm"])
	if err != nil || realm == nil || !realm.IsAbs() {
		return "", fmt.Errorf("invalid Liquibase registry authentication realm")
	}
	if strings.HasPrefix(r.baseURL, "https://") && realm.Scheme != "https" {
		return "", fmt.Errorf("insecure Liquibase registry authentication realm")
	}
	query := realm.Query()
	if params["service"] != "" {
		query.Set("service", params["service"])
	}
	scope := params["scope"]
	if scope == "" {
		scope = "repository:" + liquibaseRepository + ":pull"
	}
	if scope != "repository:"+liquibaseRepository+":pull" {
		return "", fmt.Errorf("unexpected Liquibase registry authentication scope")
	}
	query.Set("scope", scope)
	realm.RawQuery = query.Encode()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, realm.String(), nil)
	if err != nil {
		return "", err
	}
	response, err := r.client.Do(request)
	if err != nil {
		return "", fmt.Errorf("Liquibase registry token unavailable: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return "", fmt.Errorf("Liquibase registry token request returned %s", response.Status)
	}
	var payload struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&payload); err != nil {
		return "", err
	}
	if payload.Token != "" {
		return payload.Token, nil
	}
	if payload.AccessToken != "" {
		return payload.AccessToken, nil
	}
	return "", fmt.Errorf("Liquibase registry returned no bearer token")
}

// EnsureRunnerImage checks and, if needed, pulls only a previously resolved
// immutable manifest. It returns the local Docker image ID so an integration
// check can compare that ID with the container actually created from DigestRef.
func (r *DockerRuntime) EnsureRunnerImage(ctx context.Context, binding RunnerImageBinding) (string, error) {
	if !runnerDigestPattern.MatchString(binding.ManifestDigest) || binding.DigestRef != liquibaseRepository+"@"+binding.ManifestDigest {
		return "", fmt.Errorf("invalid trusted Liquibase image binding")
	}
	return r.ensurePinnedImage(ctx, binding.DigestRef, binding.ManifestDigest)
}

// ensurePinnedImage is the digest-only Docker operation shared with the
// controlled-registry test. The public entry point above alone enforces the
// trusted Liquibase repository; see liquibase-container-component-structure.md.
func (r *DockerRuntime) ensurePinnedImage(ctx context.Context, digestRef, manifestDigest string) (string, error) {
	if !runnerDigestPattern.MatchString(manifestDigest) ||
		!strings.HasSuffix(digestRef, "@"+manifestDigest) || strings.HasPrefix(digestRef, "@") {
		return "", fmt.Errorf("invalid pinned image digest reference")
	}
	inspect := func() (string, error) {
		output, err := r.run(ctx, []string{"image", "inspect", "--format", "{{.Id}}", digestRef}, nil)
		if err != nil {
			return "", err
		}
		imageID := strings.TrimSpace(output)
		if !runnerDigestPattern.MatchString(imageID) {
			return "", fmt.Errorf("invalid Docker image ID for Liquibase binding")
		}
		return imageID, nil
	}
	if imageID, err := inspect(); err == nil {
		return imageID, nil
	}
	if _, err := r.run(ctx, []string{"pull", digestRef}, nil); err != nil {
		return "", fmt.Errorf("cannot pull pinned Liquibase image %s: %w", digestRef, err)
	}
	imageID, err := inspect()
	if err != nil {
		return "", fmt.Errorf("cannot inspect pinned Liquibase image %s: %w", digestRef, err)
	}
	return imageID, nil
}

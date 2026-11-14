package runtime

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func testImageDigest(body string) string {
	sum := sha256.Sum256([]byte(body))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// The Docker daemon can run on another OS/architecture than the engine
// process (notably Windows clients of a Linux Docker daemon).
func TestRunnerPlatformUsesDaemonArchitecture(t *testing.T) {
	for _, tc := range []struct {
		info string
		want RunnerPlatform
	}{
		{`{"OSType":"linux","Architecture":"x86_64"}`, RunnerPlatform{OS: "linux", Architecture: "amd64"}},
		{`{"OSType":"linux","Architecture":"aarch64"}`, RunnerPlatform{OS: "linux", Architecture: "arm64"}},
		{`{"OSType":"windows","Architecture":"amd64"}`, RunnerPlatform{OS: "windows", Architecture: "amd64"}},
	} {
		runner := &fakeRunner{responses: []runResponse{{output: tc.info}}}
		platform, err := NewDocker(Options{Binary: "docker", Runner: runner}).RunnerPlatform(context.Background())
		if err != nil || platform != tc.want {
			t.Fatalf("info=%s platform=%+v error=%v", tc.info, platform, err)
		}
	}
	for _, info := range []string{`{}`, `{"OSType":"linux","Architecture":"mips"}`, `not json`} {
		runner := &fakeRunner{responses: []runResponse{{output: info}}}
		if _, err := NewDocker(Options{Binary: "docker", Runner: runner}).RunnerPlatform(context.Background()); err == nil {
			t.Fatalf("accepted unsupported daemon info %q", info)
		}
	}
}

// I02a: the selected reference is a platform manifest from the registry,
// never an index digest, an unrelated RepoDigests entry, or a mutable tag.
func TestRunnerImageResolverSelectsPlatformManifest(t *testing.T) {
	linuxAMD64 := "sha256:" + strings.Repeat("a", 64)
	linuxARM64 := "sha256:" + strings.Repeat("b", 64)
	windowsAMD64 := "sha256:" + strings.Repeat("c", 64)
	linuxV3 := "sha256:" + strings.Repeat("d", 64)
	index := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"windows","architecture":"amd64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"linux","architecture":"arm64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"linux","architecture":"amd64","variant":"v3"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"linux","architecture":"amd64"}}]}`, windowsAMD64, linuxARM64, linuxV3, linuxAMD64)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v2/liquibase/liquibase/manifests/5.0.3" {
			t.Errorf("unexpected registry path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		w.Header().Set("Docker-Content-Digest", testImageDigest(index))
		_, _ = w.Write([]byte(index))
	}))
	t.Cleanup(server.Close)
	resolver := newRunnerImageResolver(server.URL, server.Client())
	for _, tc := range []struct {
		platform RunnerPlatform
		want     string
	}{
		{platform: RunnerPlatform{OS: "linux", Architecture: "amd64"}, want: linuxAMD64},
		{platform: RunnerPlatform{OS: "linux", Architecture: "amd64", Variant: "v3"}, want: linuxV3},
		{platform: RunnerPlatform{OS: "windows", Architecture: "amd64"}, want: windowsAMD64},
	} {
		binding, err := resolver.Resolve(context.Background(), "5.0.3", tc.platform)
		if err != nil {
			t.Fatal(err)
		}
		if binding.ManifestDigest != tc.want || binding.IndexDigest != testImageDigest(index) || binding.DigestRef != "liquibase/liquibase@"+tc.want || binding.RequestedTag != "5.0.3" {
			t.Fatalf("binding=%+v, want platform digest %s", binding, tc.want)
		}
	}
}

// I01a: both numerical releases and latest are refreshed for each request;
// a registry failure cannot fall back to an earlier successful resolution.
func TestRunnerImageResolverRefreshesTagAndFailsClosed(t *testing.T) {
	for _, tag := range []string{"latest", "5.0.3"} {
		t.Run(tag, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				call := calls.Add(1)
				if call == 3 {
					http.Error(w, "registry offline", http.StatusServiceUnavailable)
					return
				}
				digest := "sha256:" + strings.Repeat(string(rune('a'+call)), 64)
				index := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"linux","architecture":"amd64"}}]}`, digest)
				w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
				w.Header().Set("Docker-Content-Digest", testImageDigest(index))
				_, _ = w.Write([]byte(index))
			}))
			t.Cleanup(server.Close)
			resolver := newRunnerImageResolver(server.URL, server.Client())
			platform := RunnerPlatform{OS: "linux", Architecture: "amd64"}
			first, err := resolver.Resolve(context.Background(), tag, platform)
			if err != nil {
				t.Fatal(err)
			}
			second, err := resolver.Resolve(context.Background(), tag, platform)
			if err != nil {
				t.Fatal(err)
			}
			if first.ManifestDigest == second.ManifestDigest {
				t.Fatalf("mutable tag did not refresh: first=%+v second=%+v", first, second)
			}
			if _, err := resolver.Resolve(context.Background(), tag, platform); err == nil {
				t.Fatal("registry error reused an earlier digest")
			}
		})
	}
}

// The Docker Hub bearer challenge is part of the natural registry path; the
// resolver must authenticate for the trusted repository and retry the lookup.
func TestRunnerImageResolverFollowsScopedBearerChallenge(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	index := fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"linux","architecture":"amd64"}}]}`, digest)
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/token" {
			if r.URL.Query().Get("scope") != "repository:liquibase/liquibase:pull" {
				t.Errorf("token scope=%q", r.URL.Query().Get("scope"))
			}
			_, _ = w.Write([]byte(`{"token":"scoped-token"}`))
			return
		}
		if r.Header.Get("Authorization") != "Bearer scoped-token" {
			w.Header().Set("Www-Authenticate", `Bearer realm="`+server.URL+`/token",service="registry.test",scope="repository:liquibase/liquibase:pull"`)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
		w.Header().Set("Docker-Content-Digest", testImageDigest(index))
		_, _ = w.Write([]byte(index))
	}))
	t.Cleanup(server.Close)
	resolver := newRunnerImageResolver(server.URL, server.Client())
	if binding, err := resolver.Resolve(context.Background(), "latest", RunnerPlatform{OS: "linux", Architecture: "amd64"}); err != nil || binding.ManifestDigest != digest {
		t.Fatalf("bearer resolution: binding=%+v error=%v", binding, err)
	}
}

func TestRunnerImageResolverRejectsUnsafeIndex(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	for _, tc := range []struct {
		name   string
		body   string
		header string
	}{
		{name: "wrong digest", body: fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"linux","architecture":"amd64"}}]}`, digest), header: "sha256:" + strings.Repeat("f", 64)},
		{name: "ambiguous platform", body: fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"linux","architecture":"amd64"}},{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"linux","architecture":"amd64"}}]}`, digest, digest)},
		{name: "missing platform", body: fmt.Sprintf(`{"schemaVersion":2,"mediaType":"application/vnd.oci.image.index.v1+json","manifests":[{"mediaType":"application/vnd.oci.image.manifest.v1+json","digest":%q,"platform":{"os":"linux","architecture":"arm64"}}]}`, digest)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/vnd.oci.image.index.v1+json")
				if tc.header != "" {
					w.Header().Set("Docker-Content-Digest", tc.header)
				}
				_, _ = w.Write([]byte(tc.body))
			}))
			t.Cleanup(server.Close)
			resolver := newRunnerImageResolver(server.URL, server.Client())
			if _, err := resolver.Resolve(context.Background(), "latest", RunnerPlatform{OS: "linux", Architecture: "amd64"}); err == nil {
				t.Fatal("unsafe image index was accepted")
			}
		})
	}
}

// I03a: a recovered binding checks and pulls the immutable reference only;
// Docker's local tag and first RepoDigests entry cannot redirect the run.
func TestDockerEnsuresRunnerImageByDigestOnly(t *testing.T) {
	digest := "sha256:" + strings.Repeat("a", 64)
	binding := RunnerImageBinding{RequestedTag: "latest", ManifestDigest: digest, DigestRef: "liquibase/liquibase@" + digest}
	imageID := "sha256:" + strings.Repeat("e", 64)
	for _, tc := range []struct {
		name      string
		responses []runResponse
		wantCalls int
		wantError bool
	}{
		{name: "already present", responses: []runResponse{{output: imageID + "\n"}}, wantCalls: 1},
		{name: "pull absent digest", responses: []runResponse{{err: errors.New("not found")}, {output: "pulled"}, {output: imageID + "\n"}}, wantCalls: 3},
		{name: "pull fails", responses: []runResponse{{err: errors.New("not found")}, {err: errors.New("registry offline")}}, wantCalls: 2, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			runner := &fakeRunner{responses: tc.responses}
			rt := NewDocker(Options{Binary: "docker", Runner: runner})
			gotImageID, err := rt.EnsureRunnerImage(context.Background(), binding)
			if (err != nil) != tc.wantError {
				t.Fatalf("imageID=%q error=%v", gotImageID, err)
			}
			if !tc.wantError && gotImageID != imageID {
				t.Fatalf("imageID=%q", gotImageID)
			}
			if len(runner.calls) != tc.wantCalls {
				t.Fatalf("calls=%+v", runner.calls)
			}
			for _, call := range runner.calls {
				joined := strings.Join(call.args, " ")
				if !strings.Contains(joined, binding.DigestRef) || strings.Contains(joined, "liquibase/liquibase:latest") {
					t.Fatalf("mutable or unrelated image reference used: %+v", call)
				}
			}
		})
	}
}

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

var pushedDigest = regexp.MustCompile(`digest: (sha256:[a-f0-9]{64})`)

type recordingDockerRunner struct{ calls [][]string }

func (r *recordingDockerRunner) Run(ctx context.Context, name string, args []string, stdin *string) (string, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	return execRunner{}.Run(ctx, name, args, stdin)
}

// I02b: a local registry can move latest after resolution, but image pulling
// and container creation must use the saved manifest digest. See
// docs/architecture/liquibase-container-tests.md.
func TestRunnerImagePinnedDigestSurvivesMovingRegistryTag(t *testing.T) {
	if os.Getenv("SQLRS_LIQUIBASE_REGISTRY_E2E") != "1" {
		t.Skip("set SQLRS_LIQUIBASE_REGISTRY_E2E=1 for the Docker registry test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Minute)
	defer cancel()
	docker := func(args ...string) string {
		t.Helper()
		command := exec.CommandContext(ctx, "docker", args...)
		output, err := command.CombinedOutput()
		if err != nil {
			t.Fatalf("docker %s: %v: %s", strings.Join(args, " "), err, output)
		}
		return strings.TrimSpace(string(output))
	}
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	listener.Close()
	name := fmt.Sprintf("sqlrs-i02b-%d", port)
	host := fmt.Sprintf("127.0.0.1:%d", port)
	repository := host + "/liquibase/liquibase"
	latest := repository + ":latest"
	baseURL := "http://" + host
	docker("run", "--rm", "-d", "--name", name, "-p", fmt.Sprintf("127.0.0.1:%d:5000", port), "registry:3")
	t.Cleanup(func() {
		_ = exec.Command("docker", "stop", name).Run()
		_ = exec.Command("docker", "image", "rm", latest).Run()
	})
	client := &http.Client{Timeout: 5 * time.Second}
	ready := false
	for attempt := 0; attempt < 40; attempt++ {
		response, err := client.Get(baseURL + "/v2/")
		if err == nil {
			response.Body.Close()
			if response.StatusCode == http.StatusOK {
				ready = true
				break
			}
		}
		time.Sleep(250 * time.Millisecond)
	}
	if !ready {
		t.Fatal("temporary Docker registry did not become ready")
	}
	pushIndex := func(source string) string {
		t.Helper()
		docker("tag", source, latest)
		output := docker("push", latest)
		match := pushedDigest.FindStringSubmatch(output)
		if len(match) != 2 {
			t.Fatalf("Docker push omitted manifest digest: %s", output)
		}
		digest := match[1]
		request, _ := http.NewRequestWithContext(ctx, http.MethodGet, baseURL+"/v2/liquibase/liquibase/manifests/"+digest, nil)
		request.Header.Set("Accept", dockerManifestMediaType)
		response, err := client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		body, err := io.ReadAll(response.Body)
		response.Body.Close()
		if err != nil || response.StatusCode != http.StatusOK {
			t.Fatalf("registry manifest status=%s error=%v", response.Status, err)
		}
		index, err := json.Marshal(map[string]any{
			"schemaVersion": 2,
			"mediaType":     dockerIndexMediaType,
			"manifests": []any{map[string]any{
				"mediaType": dockerManifestMediaType, "size": len(body), "digest": digest,
				"platform": map[string]string{"os": "linux", "architecture": "amd64"},
			}},
		})
		if err != nil {
			t.Fatal(err)
		}
		request, _ = http.NewRequestWithContext(ctx, http.MethodPut, baseURL+"/v2/liquibase/liquibase/manifests/latest", bytes.NewReader(index))
		request.Header.Set("Content-Type", dockerIndexMediaType)
		response, err = client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		response.Body.Close()
		if response.StatusCode != http.StatusCreated {
			t.Fatalf("registry index status=%s", response.Status)
		}
		return digest
	}
	oldDigest := pushIndex("alpine:3.23")
	resolver := newRunnerImageResolver(baseURL, client)
	platform := RunnerPlatform{OS: "linux", Architecture: "amd64"}
	oldBinding, err := resolver.Resolve(ctx, "latest", platform)
	if err != nil || oldBinding.ManifestDigest != oldDigest {
		t.Fatalf("old binding=%+v error=%v", oldBinding, err)
	}
	newDigest := pushIndex("registry:3")
	newBinding, err := resolver.Resolve(ctx, "latest", platform)
	if err != nil || newBinding.ManifestDigest != newDigest || newDigest == oldDigest {
		t.Fatalf("moved binding=%+v error=%v", newBinding, err)
	}
	oldRef := repository + "@" + oldDigest
	t.Cleanup(func() { _ = exec.Command("docker", "image", "rm", oldRef).Run() })
	if _, err := exec.CommandContext(ctx, "docker", "image", "inspect", oldRef).CombinedOutput(); err == nil {
		docker("image", "rm", oldRef)
	}
	if _, err := exec.CommandContext(ctx, "docker", "image", "inspect", oldRef).CombinedOutput(); err == nil {
		t.Fatal("old digest is still locally available before the required pull")
	}
	runner := &recordingDockerRunner{}
	runtime := NewDocker(Options{Binary: "docker", Runner: runner})
	imageID, err := runtime.ensurePinnedImage(ctx, oldRef, oldDigest)
	if err != nil {
		t.Fatal(err)
	}
	if imageID != docker("image", "inspect", "--format", "{{.Id}}", "alpine:3.23") {
		t.Fatal("pinned digest pulled a different image")
	}
	containerName := fmt.Sprintf("sqlrs-lb-i02b-%d", port)
	containerID, err := runtime.CreateContainer(ctx, RunRequest{ImageID: oldRef, Name: containerName, Args: []string{"true"}})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = exec.Command("docker", "rm", containerID).Run() })
	if got := docker("container", "inspect", "--format", "{{.Image}}", containerID); got != imageID {
		t.Fatalf("container image=%s, pinned image=%s", got, imageID)
	}
	pulled := false
	created := false
	for _, call := range runner.calls {
		if len(call) >= 2 && call[0] == "pull" && call[1] == oldRef {
			pulled = true
		}
		if len(call) >= 2 && call[0] == "create" && call[len(call)-2] == oldRef {
			created = true
		}
	}
	if !pulled || !created {
		t.Fatalf("digest pull/create not observed: %+v", runner.calls)
	}
}

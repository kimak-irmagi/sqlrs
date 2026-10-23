package app

import (
	"bytes"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type providerInitCommitWriter struct {
	t          *testing.T
	configPath string
	bytes.Buffer
}

func (w *providerInitCommitWriter) Write(p []byte) (int, error) {
	if _, err := os.Stat(w.configPath); err != nil {
		w.t.Fatalf("init wrote output before config commit: %v", err)
	}
	return w.Buffer.Write(p)
}

func TestInitRemoteShowsProvidersAfterCommit(t *testing.T) {
	workspace := t.TempDir()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/connection-info" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"installation_id":"test","endpoints":{"control":%q,"current":%q},"auth_providers":[{"id":"yandex","display_name":"Yandex ID","adapter":"oidc"},{"id":"google","display_name":"Google","adapter":"oidc"}]}`, server.URL, server.URL)
	}))
	defer server.Close()
	out := &providerInitCommitWriter{t: t, configPath: filepath.Join(workspace, ".sqlrs", "config.yaml")}
	if err := runInit(out, workspace, "", []string{"remote", server.URL}, false); err != nil {
		t.Fatal(err)
	}
	if strings.Index(out.String(), "google") > strings.Index(out.String(), "yandex") || !strings.Contains(out.String(), "Yandex ID") || !strings.Contains(out.String(), "sqlrs auth login") {
		t.Fatalf("init output = %q", out.String())
	}
	config, err := os.ReadFile(out.configPath)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(config, []byte("yandex")) || bytes.Contains(config, []byte("google")) {
		t.Fatalf("provider catalogue was persisted")
	}
}

func TestInitRemoteUpdateShowsOnlyNewProviders(t *testing.T) {
	workspace := t.TempDir()
	newServer := func(id string) *httptest.Server {
		var server *httptest.Server
		server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprintf(w, `{"installation_id":"test","endpoints":{"control":%q,"current":%q},"auth_providers":[{"id":%q,"display_name":%q,"adapter":"oidc"}]}`, server.URL, server.URL, id, id)
		}))
		return server
	}
	first := newServer("alpha")
	defer first.Close()
	second := newServer("beta")
	defer second.Close()
	if err := runInit(io.Discard, workspace, "", []string{"remote", first.URL}, false); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"remote", second.URL, "--update"}, false); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out.String(), "beta") || strings.Contains(out.String(), "alpha") {
		t.Fatalf("update output = %q", out.String())
	}
}

func TestInitRemoteDoesNotSuggestProvidersOnDryRunOrLegacy(t *testing.T) {
	workspace := t.TempDir()
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"installation_id":"test","endpoints":{"control":%q,"current":%q},"auth_providers":[{"id":"google","display_name":"Google","adapter":"oidc"}]}`, server.URL, server.URL)
	}))
	defer server.Close()
	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"remote", server.URL, "--dry-run"}, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "sqlrs auth login") || strings.Contains(out.String(), "Google") {
		t.Fatalf("dry run output = %q", out.String())
	}
	out.Reset()
	if err := runInit(&out, workspace, "", []string{"remote", server.URL, "--token", "test-only"}, false); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(out.String(), "sqlrs auth login") || strings.Contains(out.String(), "Google") {
		t.Fatalf("legacy output = %q", out.String())
	}
}

func TestInitRemoteDoesNotSuggestProvidersOnDiscoveryOrCommitFailure(t *testing.T) {
	workspace := t.TempDir()
	var out bytes.Buffer
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer bad.Close()
	if err := runInit(&out, workspace, "", []string{"remote", bad.URL}, false); err == nil {
		t.Fatal("discovery failure accepted")
	}
	if strings.Contains(out.String(), "sqlrs auth login") {
		t.Fatalf("suggested provider after discovery failure: %q", out.String())
	}

	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprintf(w, `{"installation_id":"test","endpoints":{"control":%q,"current":%q},"auth_providers":[{"id":"google","display_name":"Google","adapter":"oidc"}]}`, server.URL, server.URL)
	}))
	defer server.Close()
	if err := os.Mkdir(filepath.Join(workspace, ".sqlrs"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(workspace, ".sqlrs", "config.yaml"), 0o700); err != nil {
		t.Fatal(err)
	}
	out.Reset()
	if err := runInit(&out, workspace, "", []string{"remote", server.URL, "--update"}, false); err == nil {
		t.Fatal("config commit failure accepted")
	}
	if strings.Contains(out.String(), "sqlrs auth login") {
		t.Fatalf("suggested provider after commit failure: %q", out.String())
	}
}

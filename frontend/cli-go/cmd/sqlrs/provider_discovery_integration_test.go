package main

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
)

func TestProvidersCanBeRecoveredInNewProcessAfterInit(t *testing.T) {
	workspace := t.TempDir()
	bin := filepath.Join(t.TempDir(), "sqlrs")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}
	build := exec.Command("go", "build", "-o", bin, ".")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build CLI: %v\n%s", err, output)
	}

	var mu sync.Mutex
	provider := "alpha"
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		id := provider
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/connection-info":
			fmt.Fprintf(w, `{"installation_id":"test","endpoints":{"control":%q,"current":%q},"auth_providers":[{"id":%q,"display_name":%q,"adapter":"oidc"}]}`, server.URL, server.URL, id, id)
		case "/v1/auth/providers":
			fmt.Fprintf(w, `{"providers":[{"id":%q,"display_name":%q,"adapter":"oidc"}]}`, id, id)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	run := func(args ...string) string {
		t.Helper()
		cmd := exec.Command(bin, args...)
		cmd.Dir = workspace
		output, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("sqlrs %v: %v\n%s", args, err, output)
		}
		return string(output)
	}
	_ = run("init", "remote", server.URL)
	configPath := filepath.Join(workspace, ".sqlrs", "config.yaml")
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if got := run("auth", "providers"); !strings.Contains(got, "alpha") {
		t.Fatalf("first recovered list = %q", got)
	}
	mu.Lock()
	provider = "beta"
	mu.Unlock()
	if got := run("auth", "providers"); !strings.Contains(got, "beta") || strings.Contains(got, "alpha") {
		t.Fatalf("updated list = %q", got)
	}
	after, err := os.ReadFile(configPath)
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("provider listing changed config: %v", err)
	}
}

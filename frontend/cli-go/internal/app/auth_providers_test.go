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

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
)

func TestAuthProvidersRendersFreshPublicList(t *testing.T) {
	originalManagerFactory := authManagerFactory
	authManagerFactory = func() authManager { panic("auth providers opened credential manager") }
	t.Cleanup(func() { authManagerFactory = originalManagerFactory })
	workspace := t.TempDir()
	setTestDirs(t, workspace)
	response := `{"providers":[{"id":"yandex","display_name":"Yandex ID","adapter":"oidc"},{"id":"google","display_name":"Google","adapter":"oidc"}]}`
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/providers" {
			t.Errorf("path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, response)
	}))
	defer server.Close()
	writeProjectConfig(t, workspace, authTestConfig("remote", server.URL, "remoteSession"))
	before, err := os.ReadFile(filepath.Join(workspace, ".sqlrs", "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}

	var human bytes.Buffer
	if err := runAuth(&human, io.Discard, workspace, cli.GlobalOptions{Workspace: workspace}, []string{"providers"}); err != nil {
		t.Fatal(err)
	}
	if strings.Index(human.String(), "google") > strings.Index(human.String(), "yandex") || !strings.Contains(human.String(), "Yandex ID") || !strings.Contains(human.String(), "sqlrs auth login") {
		t.Fatalf("human list = %q", human.String())
	}

	var jsonOut bytes.Buffer
	if err := runAuth(&jsonOut, io.Discard, workspace, cli.GlobalOptions{Workspace: workspace, Output: "json"}, []string{"providers"}); err != nil {
		t.Fatal(err)
	}
	want := `{"providers":[{"id":"google","display_name":"Google","adapter":"oidc"},{"id":"yandex","display_name":"Yandex ID","adapter":"oidc"}]}`
	if strings.TrimSpace(jsonOut.String()) != want {
		t.Fatalf("JSON = %q, want %q", jsonOut.String(), want)
	}
	after, err := os.ReadFile(filepath.Join(workspace, ".sqlrs", "config.yaml"))
	if err != nil || !bytes.Equal(before, after) {
		t.Fatalf("auth providers changed config: %v", err)
	}
}

func TestAuthProvidersUsesEffectiveProfileAndEndpoint(t *testing.T) {
	workspace := t.TempDir()
	setTestDirs(t, workspace)
	serverA := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/providers" {
			t.Errorf("server A path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"providers":[{"id":"alpha","display_name":"Alpha","adapter":"oidc"}]}`)
	}))
	defer serverA.Close()
	serverB := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/school/v1/auth/providers" {
			t.Errorf("server B path = %q", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"providers":[{"id":"beta","display_name":"Beta","adapter":"oidc"}]}`)
	}))
	defer serverB.Close()
	writeProjectConfig(t, workspace, fmt.Sprintf("defaultProfile: first\nprofiles:\n  first:\n    mode: remote\n    endpoint: %s\n    auth:\n      mode: remoteSession\n  second:\n    mode: remote\n    endpoint: %s/school\n    auth:\n      mode: remoteSession\n", serverA.URL, serverB.URL))
	for _, tc := range []struct {
		name string
		opts cli.GlobalOptions
		want string
	}{
		{name: "default", opts: cli.GlobalOptions{Workspace: workspace}, want: "alpha"},
		{name: "selected", opts: cli.GlobalOptions{Workspace: workspace, Profile: "second"}, want: "beta"},
		{name: "override", opts: cli.GlobalOptions{Workspace: workspace, Profile: "first", Endpoint: serverB.URL + "/school"}, want: "beta"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := runAuth(&out, io.Discard, workspace, tc.opts, []string{"providers"}); err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(out.String(), tc.want) {
				t.Fatalf("output = %q, want %s", out.String(), tc.want)
			}
		})
	}
}

func TestAuthProvidersFailuresDoNotRenderPartialList(t *testing.T) {
	for _, tc := range []struct {
		name   string
		body   string
		status int
		want   string
	}{
		{name: "empty", body: `{"providers":[]}`, want: "no enabled login providers"},
		{name: "invalid", body: `{"providers":null}`, want: "invalid provider catalogue"},
		{name: "unavailable", status: http.StatusServiceUnavailable, want: "cannot fetch provider catalogue"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			setTestDirs(t, workspace)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				if tc.status != 0 {
					w.WriteHeader(tc.status)
					return
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			writeProjectConfig(t, workspace, authTestConfig("remote", server.URL, "remoteSession"))
			var out bytes.Buffer
			err := runAuth(&out, io.Discard, workspace, cli.GlobalOptions{Workspace: workspace}, []string{"providers"})
			if err == nil || !strings.Contains(err.Error(), tc.want) || out.Len() != 0 {
				t.Fatalf("output = %q, error = %v", out.String(), err)
			}
		})
	}
}

func TestAuthProvidersRejectsLocalOrInvalidEndpoint(t *testing.T) {
	for _, tc := range []struct {
		name     string
		mode     string
		endpoint string
	}{
		{name: "local", mode: "local", endpoint: "https://example.org"},
		{name: "missing", mode: "remote", endpoint: "auto"},
		{name: "invalid", mode: "remote", endpoint: "http://example.org"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			workspace := t.TempDir()
			setTestDirs(t, workspace)
			writeProjectConfig(t, workspace, authTestConfig(tc.mode, tc.endpoint, "remoteSession"))
			var out bytes.Buffer
			if err := runAuth(&out, io.Discard, workspace, cli.GlobalOptions{Workspace: workspace}, []string{"providers"}); err == nil || out.Len() != 0 {
				t.Fatalf("output = %q, error = %v", out.String(), err)
			}
		})
	}
}

func TestAuthProvidersSanitizesHumanName(t *testing.T) {
	workspace := t.TempDir()
	setTestDirs(t, workspace)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"providers":[{"id":"google","display_name":"Google\nInjected\u001b[31m","adapter":"oidc"}]}`)
	}))
	defer server.Close()
	writeProjectConfig(t, workspace, authTestConfig("remote", server.URL, "remoteSession"))
	var out bytes.Buffer
	if err := runAuth(&out, io.Discard, workspace, cli.GlobalOptions{Workspace: workspace}, []string{"providers"}); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out.String(), "\r\x1b") || strings.Contains(out.String(), "Google\nInjected") {
		t.Fatalf("unsafe human output = %q", out.String())
	}
}

func TestAuthProvidersSanitizesUnicodeLayoutControls(t *testing.T) {
	list := client.AuthProviderList{Providers: []client.AuthProviderSummary{{
		ID: "google", DisplayName: "Go\u202eogle\u2028Injected", Adapter: "oidc",
	}}}
	var out bytes.Buffer
	if err := writeAuthProviders(&out, list, "human"); err != nil {
		t.Fatal(err)
	}
	if strings.ContainsAny(out.String(), "\u202e\u2028") || !strings.Contains(out.String(), "Go ogle Injected") {
		t.Fatalf("unsafe human output = %q", out.String())
	}
}

func TestAuthProvidersParserAndHelp(t *testing.T) {
	for _, tc := range []struct {
		args     []string
		wantErr  bool
		wantHelp bool
	}{
		{args: []string{"providers"}},
		{args: []string{"providers", "--help"}, wantHelp: true},
		{args: []string{"providers", "extra"}, wantErr: true},
		{args: []string{"providers", "--bad"}, wantErr: true},
	} {
		got, help, err := parseAuthArgs(tc.args)
		if (err != nil) != tc.wantErr || help != tc.wantHelp || (err == nil && !help && got.action != "providers") {
			t.Fatalf("parse %v: action=%q help=%t err=%v", tc.args, got.action, help, err)
		}
	}
	_, _, err := parseAuthArgs([]string{"login"})
	if err == nil || !strings.Contains(err.Error(), "sqlrs auth providers") {
		t.Fatalf("missing login provider error = %v", err)
	}
	var out bytes.Buffer
	printAuthUsage(&out)
	if !strings.Contains(out.String(), "sqlrs auth providers") {
		t.Fatalf("usage = %q", out.String())
	}
}

func TestAuthProvidersReportsOutputFailure(t *testing.T) {
	list := client.AuthProviderList{Providers: []client.AuthProviderSummary{{ID: "google", DisplayName: "Google", Adapter: "oidc"}}}
	for _, output := range []string{"human", "json"} {
		t.Run(output, func(t *testing.T) {
			if err := writeAuthProviders(failingWriter{}, list, output); err == nil || !strings.Contains(err.Error(), "write failed") {
				t.Fatalf("output failure = %v", err)
			}
		})
	}
	var writes int
	writeSecond := writerFunc(func(p []byte) (int, error) {
		writes++
		if writes == 2 {
			return 0, fmt.Errorf("provider row write failed")
		}
		return len(p), nil
	})
	if err := writeAuthProviders(writeSecond, list, "human"); err == nil || !strings.Contains(err.Error(), "provider row write failed") {
		t.Fatalf("provider row failure = %v", err)
	}
}

type writerFunc func([]byte) (int, error)

func (fn writerFunc) Write(p []byte) (int, error) { return fn(p) }

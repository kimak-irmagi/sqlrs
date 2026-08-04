package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/config"
)

func TestParseAuthArgs(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		action      string
		provider    string
		loginHint   string
		noBrowser   bool
		noRevoke    bool
		help        bool
		errContains string
	}{
		{name: "missing command", errContains: "Missing auth command"},
		{name: "top-level help", args: []string{"--help"}, help: true},
		{name: "status", args: []string{"status"}, action: "status"},
		{name: "status help", args: []string{"status", "-h"}, help: true},
		{name: "status arguments", args: []string{"status", "extra"}, errContains: "does not accept arguments"},
		{name: "unknown command", args: []string{"frobnicate"}, errContains: "Unknown auth command"},
		{name: "login missing provider", args: []string{"login"}, errContains: "provider is required"},
		{name: "login help", args: []string{"login", "-h"}, action: "login", help: true},
		{name: "login provider help", args: []string{"login", "google", "--help"}, action: "login", help: true},
		{name: "unsupported provider", args: []string{"login", "github"}, errContains: "unsupported auth provider"},
		{name: "login flags", args: []string{"login", "google", "--login-hint", " user@example.org ", "--no-browser"}, action: "login", provider: "google", loginHint: "user@example.org", noBrowser: true},
		{name: "login invalid flag", args: []string{"login", "google", "--wat"}, errContains: "Invalid arguments"},
		{name: "login positional", args: []string{"login", "google", "extra"}, errContains: "does not accept positional arguments"},
		{name: "logout", args: []string{"logout", "--no-revoke"}, action: "logout", provider: "google", noRevoke: true},
		{name: "logout help", args: []string{"logout", "--help"}, action: "logout", provider: "google", help: true},
		{name: "logout invalid flag", args: []string{"logout", "--wat"}, errContains: "Invalid arguments"},
		{name: "logout positional", args: []string{"logout", "extra"}, errContains: "does not accept positional arguments"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, help, err := parseAuthArgs(tt.args)
			if tt.errContains != "" {
				if err == nil || !strings.Contains(err.Error(), tt.errContains) {
					t.Fatalf("error = %v, want containing %q", err, tt.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("parseAuthArgs: %v", err)
			}
			if help != tt.help || got.action != tt.action || got.provider != tt.provider || got.loginHint != tt.loginHint || got.noBrowser != tt.noBrowser || got.noRevoke != tt.noRevoke {
				t.Fatalf("result = %#v, help=%t", got, help)
			}
		})
	}
}

func TestAuthResultRenderers(t *testing.T) {
	expiry := time.Date(2026, time.August, 4, 12, 0, 0, 0, time.UTC)
	login := authLoginResult{Provider: "google", Email: "user@example.org", Issuer: "issuer", Audience: "client", TokenExpiry: expiry, Profile: "remote", Endpoint: "https://example.org", AuthorizationURL: "https://accounts.example.org"}
	status := authStatusResult{LoggedIn: true, Provider: "google", Email: "user@example.org", Issuer: "issuer", Audience: "client", TokenExpiry: expiry, Profile: "remote", Endpoint: "https://example.org", Override: "SQLRS_TOKEN"}
	logout := authLogoutResult{Provider: "google", Profile: "remote", Endpoint: "https://example.org", Revoked: false, RevocationFailed: "offline"}

	tests := []struct {
		name     string
		write    func(*bytes.Buffer) error
		contains []string
	}{
		{name: "login human", write: func(w *bytes.Buffer) error { return writeAuthLoginResult(w, login, "human") }, contains: []string{"authorizationURL:", "logged in", "email: user@example.org", "tokenExpiry: 2026-08-04T12:00:00Z", "override: none"}},
		{name: "status human", write: func(w *bytes.Buffer) error { return writeAuthStatusResult(w, status, "human") }, contains: []string{"status: logged in", "override: SQLRS_TOKEN"}},
		{name: "status logged out", write: func(w *bytes.Buffer) error { status.LoggedIn = false; return writeAuthStatusResult(w, status, "human") }, contains: []string{"status: not logged in"}},
		{name: "logout human", write: func(w *bytes.Buffer) error { return writeAuthLogoutResult(w, logout, "human") }, contains: []string{"logged out", "provider: google", "revoked: false", "revocationWarning: offline"}},
		{name: "login json", write: func(w *bytes.Buffer) error { return writeAuthLoginResult(w, login, "json") }, contains: []string{"\"provider\":\"google\""}},
		{name: "status json", write: func(w *bytes.Buffer) error { return writeAuthStatusResult(w, status, "json") }, contains: []string{"\"logged_in\":false"}},
		{name: "logout json", write: func(w *bytes.Buffer) error { return writeAuthLogoutResult(w, logout, "json") }, contains: []string{"\"revoked\":false"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var out bytes.Buffer
			if err := tt.write(&out); err != nil {
				t.Fatalf("render: %v", err)
			}
			for _, want := range tt.contains {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("output %q does not contain %q", out.String(), want)
				}
			}
		})
	}

	var usage bytes.Buffer
	printAuthUsage(&usage)
	if !strings.Contains(usage.String(), "sqlrs auth logout") {
		t.Fatalf("usage = %q", usage.String())
	}
}

func TestResolveEffectiveAuthTokenBranches(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	tests := []struct {
		name      string
		ctx       commandContext
		manager   fakeAuthManager
		wantToken string
		wantErr   string
	}{
		{name: "local remains unchanged", ctx: commandContext{mode: "local"}},
		{name: "configured bearer", ctx: commandContext{mode: "remote", profile: config.ProfileConfig{Auth: config.AuthConfig{Mode: "bearer", Token: " configured "}}}, wantToken: "configured"},
		{name: "remote without oidc", ctx: commandContext{mode: "remote", profile: config.ProfileConfig{Auth: config.AuthConfig{Mode: "none"}}}},
		{name: "oidc manager error", ctx: commandContext{mode: "remote", profile: config.ProfileConfig{Auth: config.AuthConfig{Mode: "oidcSession"}}}, manager: fakeAuthManager{resolveErr: errors.New("refresh failed")}, wantErr: "refresh failed"},
		{name: "oidc trims resolved token", ctx: commandContext{mode: "remote", profile: config.ProfileConfig{Auth: config.AuthConfig{Mode: "oidcSession"}}}, manager: fakeAuthManager{}, wantToken: "fresh-id-token"},
	}

	oldFactory := authManagerFactory
	t.Cleanup(func() { authManagerFactory = oldFactory })
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			authManagerFactory = func() authManager { return tt.manager }
			got, err := resolveEffectiveAuthToken(context.Background(), tt.ctx)
			if tt.wantErr != "" {
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("error = %v, want containing %q", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveEffectiveAuthToken: %v", err)
			}
			if got.authToken != tt.wantToken {
				t.Fatalf("auth token = %q, want %q", got.authToken, tt.wantToken)
			}
		})
	}
}

func TestResolveAuthTokenUsesEnv(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "env-token")
	got := resolveAuthToken(config.AuthConfig{TokenEnv: "SQLRS_TOKEN", Token: "fallback"})
	if got != "env-token" {
		t.Fatalf("expected env token, got %q", got)
	}
}

func TestResolveAuthTokenFallsBackToToken(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	got := resolveAuthToken(config.AuthConfig{Mode: "bearer", TokenEnv: "SQLRS_TOKEN", Token: " token "})
	if got != "token" {
		t.Fatalf("expected fallback token, got %q", got)
	}
}

func TestResolveAuthTokenDefaultsOIDCSessionToSQLRSToken(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "debug-token")
	got := resolveAuthToken(config.AuthConfig{Mode: "oidcSession"})
	if got != "debug-token" {
		t.Fatalf("expected SQLRS_TOKEN override, got %q", got)
	}
}

func TestResolveAuthTokenIgnoresStaticTokenForOIDCSession(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	got := resolveAuthToken(config.AuthConfig{Mode: "oidcSession", Token: " stale-static-token "})
	if got != "" {
		t.Fatalf("expected oidcSession to ignore static token, got %q", got)
	}
}

func TestResolveEffectiveAuthTokenUsesStoredSessionBeforeOIDCStaticToken(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	oldFactory := authManagerFactory
	var gotOptions authResolveOptions
	authManagerFactory = func() authManager {
		return fakeAuthManager{onResolve: func(opts authResolveOptions) {
			gotOptions = opts
		}}
	}
	t.Cleanup(func() { authManagerFactory = oldFactory })

	ctx := commandContext{
		mode:        "remote",
		profileName: "remote",
		profile: config.ProfileConfig{
			Mode:     "remote",
			Endpoint: "https://sqlrs.example.org",
			Auth: config.AuthConfig{
				Mode:         "oidcSession",
				Token:        "stale-static-token",
				ClientID:     "client-id",
				ClientSecret: "client-secret",
			},
		},
	}

	resolved, err := resolveEffectiveAuthToken(context.Background(), ctx)
	if err != nil {
		t.Fatalf("resolveEffectiveAuthToken: %v", err)
	}
	if resolved.authToken != "fresh-id-token" {
		t.Fatalf("auth token = %q, want refreshed session token", resolved.authToken)
	}
	if gotOptions.ClientSecret != "client-secret" {
		t.Fatalf("client secret = %q, want client-secret", gotOptions.ClientSecret)
	}
}

func TestStartCleanupSpinnerVerboseWritesLabel(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
		_ = r.Close()
		_ = w.Close()
	})

	stop := startCleanupSpinner("inst-1", true)
	stop()
	_ = w.Close()
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	if !strings.Contains(string(data), "Deleting instance inst-1") {
		t.Fatalf("expected label, got %q", string(data))
	}
}

func TestStartCleanupSpinnerNonTerminalWritesLabel(t *testing.T) {
	oldStdout := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	t.Cleanup(func() {
		os.Stdout = oldStdout
		_ = r.Close()
		_ = w.Close()
	})

	stop := startCleanupSpinner("inst-2", false)
	stop()
	_ = w.Close()
	data, readErr := io.ReadAll(r)
	if readErr != nil {
		t.Fatalf("read stdout: %v", readErr)
	}
	if !strings.Contains(string(data), "Deleting instance inst-2") {
		t.Fatalf("expected label, got %q", string(data))
	}
}

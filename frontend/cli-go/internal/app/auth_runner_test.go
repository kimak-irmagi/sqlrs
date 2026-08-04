package app

import (
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/config"
)

func TestRunnerRoutesAuthWithoutProtectedTokenResolution(t *testing.T) {
	cwd := t.TempDir()
	var authArgs []string
	resolveContextCalls := 0
	resolveTokenCalls := 0

	err := runWithParsedCommands(t, cli.GlobalOptions{}, []cli.Command{{Name: "auth", Args: []string{"status"}}}, func(deps *runnerDeps) {
		deps.getwd = func() (string, error) {
			return cwd, nil
		}
		deps.resolveCommandContext = func(string, cli.GlobalOptions) (commandContext, error) {
			resolveContextCalls++
			return testCommandContext(cwd, "human", false), nil
		}
		deps.resolveEffectiveAuthToken = func(context.Context, commandContext) (commandContext, error) {
			resolveTokenCalls++
			return commandContext{}, nil
		}
		deps.runAuth = func(stdout, stderr io.Writer, gotCwd string, opts cli.GlobalOptions, args []string) error {
			if gotCwd != cwd {
				t.Fatalf("cwd = %q, want %q", gotCwd, cwd)
			}
			authArgs = append([]string(nil), args...)
			return nil
		}
	})
	if err != nil {
		t.Fatalf("runner.run: %v", err)
	}
	if got := strings.Join(authArgs, "|"); got != "status" {
		t.Fatalf("auth args = %q, want status", got)
	}
	if resolveContextCalls != 0 {
		t.Fatalf("resolveCommandContext calls = %d, want 0", resolveContextCalls)
	}
	if resolveTokenCalls != 0 {
		t.Fatalf("resolveEffectiveAuthToken calls = %d, want 0", resolveTokenCalls)
	}
}

func TestRunnerRejectsCompositeAuth(t *testing.T) {
	err := runWithParsedCommands(t, cli.GlobalOptions{}, []cli.Command{
		{Name: "auth", Args: []string{"status"}},
		{Name: "status"},
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "auth cannot be combined") {
		t.Fatalf("expected composite auth error, got %v", err)
	}
}

func TestRunnerResolvesEffectiveTokenForProtectedRemoteCommand(t *testing.T) {
	cwd := t.TempDir()
	var gotToken string
	resolveTokenCalls := 0

	err := runWithParsedCommands(t, cli.GlobalOptions{}, []cli.Command{{Name: "user", Args: []string{"me"}}}, func(deps *runnerDeps) {
		deps.getwd = func() (string, error) {
			return cwd, nil
		}
		deps.resolveCommandContext = func(string, cli.GlobalOptions) (commandContext, error) {
			ctx := testCommandContext(cwd, "human", false)
			ctx.mode = "remote"
			ctx.profileName = "remote-dev"
			ctx.profile = config.ProfileConfig{
				Mode:     "remote",
				Endpoint: "https://sqlrs.example.org",
				Auth: config.AuthConfig{
					Mode:     "oidcSession",
					ClientID: "client-id",
					Issuer:   "https://accounts.google.com",
				},
			}
			ctx.timeout = time.Second
			return ctx, nil
		}
		deps.resolveEffectiveAuthToken = func(_ context.Context, ctx commandContext) (commandContext, error) {
			resolveTokenCalls++
			ctx.authToken = "fresh-id-token"
			return ctx, nil
		}
		deps.runUser = func(_ io.Writer, ctx commandContext, args []string, output string) error {
			gotToken = ctx.userOrgOptions().AuthToken
			return nil
		}
	})
	if err != nil {
		t.Fatalf("runner.run: %v", err)
	}
	if resolveTokenCalls != 1 {
		t.Fatalf("resolveEffectiveAuthToken calls = %d, want 1", resolveTokenCalls)
	}
	if gotToken != "fresh-id-token" {
		t.Fatalf("AuthToken = %q, want fresh-id-token", gotToken)
	}
}

func TestRunnerDoesNotResolveEffectiveTokenForLocalOnlyCommands(t *testing.T) {
	cwd := t.TempDir()
	resolveTokenCalls := 0

	err := runWithParsedCommands(t, cli.GlobalOptions{}, []cli.Command{{Name: "alias", Args: []string{"list"}}}, func(deps *runnerDeps) {
		deps.getwd = func() (string, error) {
			return cwd, nil
		}
		deps.resolveCommandContext = func(string, cli.GlobalOptions) (commandContext, error) {
			return testCommandContext(cwd, "human", false), nil
		}
		deps.resolveEffectiveAuthToken = func(context.Context, commandContext) (commandContext, error) {
			resolveTokenCalls++
			return commandContext{}, nil
		}
		deps.runAlias = func(io.Writer, commandContext, []string) error {
			return nil
		}
	})
	if err != nil {
		t.Fatalf("runner.run: %v", err)
	}
	if resolveTokenCalls != 0 {
		t.Fatalf("resolveEffectiveAuthToken calls = %d, want 0", resolveTokenCalls)
	}
}

func TestRunnerDoesNotResolveEffectiveTokenForConfigCommand(t *testing.T) {
	cwd := t.TempDir()
	resolveTokenCalls := 0
	runConfigCalls := 0

	err := runWithParsedCommands(t, cli.GlobalOptions{}, []cli.Command{{Name: "config", Args: []string{"get", "features.flag"}}}, func(deps *runnerDeps) {
		deps.getwd = func() (string, error) {
			return cwd, nil
		}
		deps.resolveCommandContext = func(string, cli.GlobalOptions) (commandContext, error) {
			ctx := testCommandContext(cwd, "human", false)
			ctx.mode = "remote"
			ctx.profile = config.ProfileConfig{
				Mode:     "remote",
				Endpoint: "https://sqlrs.example.org",
				Auth:     config.AuthConfig{Mode: "oidcSession", ClientID: "client-id"},
			}
			return ctx, nil
		}
		deps.resolveEffectiveAuthToken = func(context.Context, commandContext) (commandContext, error) {
			resolveTokenCalls++
			return commandContext{}, nil
		}
		deps.runConfig = func(io.Writer, cli.ConfigOptions, []string, string) error {
			runConfigCalls++
			return nil
		}
	})
	if err != nil {
		t.Fatalf("runner.run: %v", err)
	}
	if resolveTokenCalls != 0 {
		t.Fatalf("resolveEffectiveAuthToken calls = %d, want 0", resolveTokenCalls)
	}
	if runConfigCalls != 1 {
		t.Fatalf("runConfig calls = %d, want 1", runConfigCalls)
	}
}

func TestRunAuthLoginNoBrowserPrintsAuthorizationURLBeforeFinalOutput(t *testing.T) {
	cwd := t.TempDir()
	setTestDirs(t, cwd)
	writeProjectConfig(t, cwd,
		"defaultProfile: remote\n"+
			"profiles:\n"+
			"  remote:\n"+
			"    mode: remote\n"+
			"    endpoint: https://sqlrs.example.org\n"+
			"    auth:\n"+
			"      mode: oidcSession\n"+
			"      clientID: client-id\n"+
			"      clientSecret: client-secret\n")

	var stdout bytes.Buffer
	oldFactory := authManagerFactory
	var gotOptions authLoginOptions
	authManagerFactory = func() authManager {
		return fakeAuthManager{login: authLoginResult{
			LoggedIn:    true,
			Provider:    "google",
			Email:       "alice@example.com",
			Issuer:      "https://accounts.google.com",
			Audience:    "client-id",
			Profile:     "remote",
			Endpoint:    "https://sqlrs.example.org",
			TokenExpiry: time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC),
		}, onLogin: func(opts authLoginOptions) {
			gotOptions = opts
		}}
	}
	t.Cleanup(func() { authManagerFactory = oldFactory })

	if err := runAuth(&stdout, io.Discard, cwd, cli.GlobalOptions{Workspace: cwd}, []string{"login", "google", "--no-browser"}); err != nil {
		t.Fatalf("runAuth: %v", err)
	}
	out := stdout.String()
	urlIndex := strings.Index(out, "authorizationURL: https://accounts.google.com/o/oauth2/v2/auth")
	loggedInIndex := strings.Index(out, "logged in")
	if urlIndex < 0 || loggedInIndex < 0 || urlIndex > loggedInIndex {
		t.Fatalf("stdout should print URL before final login output, got %q", out)
	}
	if strings.Count(out, "authorizationURL:") != 1 {
		t.Fatalf("stdout should print authorization URL once, got %q", out)
	}
	if gotOptions.ClientSecret != "client-secret" {
		t.Fatalf("client secret = %q, want client-secret", gotOptions.ClientSecret)
	}
}

func TestRunAuthStatusRendersNoRawTokens(t *testing.T) {
	cwd := t.TempDir()
	setTestDirs(t, cwd)
	writeProjectConfig(t, cwd,
		"defaultProfile: remote\n"+
			"profiles:\n"+
			"  remote:\n"+
			"    mode: remote\n"+
			"    endpoint: https://sqlrs.example.org\n"+
			"    auth:\n"+
			"      mode: oidcSession\n"+
			"      clientID: client-id\n")

	var stdout bytes.Buffer
	oldFactory := authManagerFactory
	authManagerFactory = func() authManager {
		return fakeAuthManager{status: authStatusResult{
			LoggedIn:    true,
			Provider:    "google",
			Email:       "alice@example.com",
			Issuer:      "https://accounts.google.com",
			Audience:    "client-id",
			TokenExpiry: time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC),
			Profile:     "remote",
			Endpoint:    "https://sqlrs.example.org",
		}}
	}
	t.Cleanup(func() { authManagerFactory = oldFactory })

	if err := runAuth(&stdout, io.Discard, cwd, cli.GlobalOptions{Workspace: cwd}, []string{"status"}); err != nil {
		t.Fatalf("runAuth: %v", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "status: logged in") || !strings.Contains(out, "email: alice@example.com") {
		t.Fatalf("stdout = %q, want status metadata", out)
	}
	if strings.Contains(out, "refresh-token") || strings.Contains(out, "id-token") {
		t.Fatalf("stdout leaked token: %q", out)
	}
}

func writeProjectConfig(t *testing.T, workspace string, content string) {
	t.Helper()
	configDir := filepath.Join(workspace, ".sqlrs")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatalf("mkdir project config dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(configDir, "config.yaml"), []byte(content), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}
}

type fakeAuthManager struct {
	login      authLoginResult
	status     authStatusResult
	logout     authLogoutResult
	loginErr   error
	statusErr  error
	logoutErr  error
	resolveErr error
	onLogin    func(authLoginOptions)
	onResolve  func(authResolveOptions)
}

func (m fakeAuthManager) LoginGoogle(_ context.Context, opts authLoginOptions) (authLoginResult, error) {
	if m.loginErr != nil {
		return authLoginResult{}, m.loginErr
	}
	if m.onLogin != nil {
		m.onLogin(opts)
	}
	authURL := m.login.AuthorizationURL
	if authURL == "" {
		authURL = "https://accounts.google.com/o/oauth2/v2/auth"
	}
	if opts.AuthorizationURLReady != nil {
		if err := opts.AuthorizationURLReady(authURL); err != nil {
			return authLoginResult{}, err
		}
	}
	m.login.AuthorizationURL = authURL
	return m.login, nil
}

func (m fakeAuthManager) Status(context.Context, authStatusOptions) (authStatusResult, error) {
	return m.status, m.statusErr
}

func (m fakeAuthManager) Logout(context.Context, authLogoutOptions) (authLogoutResult, error) {
	return m.logout, m.logoutErr
}

func (m fakeAuthManager) ResolveBearerToken(_ context.Context, opts authResolveOptions) (authResolvedBearerToken, error) {
	if m.resolveErr != nil {
		return authResolvedBearerToken{}, m.resolveErr
	}
	if m.onResolve != nil {
		m.onResolve(opts)
	}
	return authResolvedBearerToken{Token: "fresh-id-token"}, nil
}

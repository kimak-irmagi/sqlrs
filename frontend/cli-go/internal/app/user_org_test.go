package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/authsession"
	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
)

func TestReconcileCreatedOrganizationSwitchesStoredRemoteSession(t *testing.T) {
	workspace := t.TempDir()
	writeProjectConfig(t, workspace, "defaultProfile: remote\nprofiles:\n  remote:\n    mode: remote\n    endpoint: https://api.example.test\n    installationID: installation-1\n    installationEndpoint: https://api.example.test\n    auth:\n      mode: remoteSession\n")
	loaded, err := config.Load(config.LoadOptions{WorkingDir: workspace})
	if err != nil {
		t.Fatal(err)
	}
	ctx := commandContext{profileName: "remote", profile: loaded.Config.Profiles["remote"], cfgResult: loaded, authTokenSource: authsession.TokenSourceStoredRemoteSession}
	var stderr bytes.Buffer
	if err := reconcileCreatedOrganization(ctx, client.Organization{Slug: "nsu", Endpoint: "https://api.example.test/nsu"}, &stderr); err != nil {
		t.Fatalf("reconcileCreatedOrganization: %v", err)
	}
	if !strings.Contains(stderr.String(), "switched to organization \"nsu\"") {
		t.Fatalf("stderr = %q", stderr.String())
	}
	raw := loadConfigMap(t, filepath.Join(workspace, ".sqlrs", "config.yaml"))
	if got := nestedString(raw, "profiles", "remote", "endpoint"); got != "https://api.example.test/nsu" {
		t.Fatalf("endpoint = %q", got)
	}
}

func TestParseUserArgsRegisterRejectsExplicitIdentity(t *testing.T) {
	_, _, err := parseUserArgs([]string{"register", "--identity-issuer", "https://issuer.example.test"})
	if err == nil || !strings.Contains(err.Error(), "identity is derived from the authenticated token") {
		t.Fatalf("expected explicit identity rejection, got %v", err)
	}
}

func TestParseUserArgsCreateRequiresIdentityKey(t *testing.T) {
	_, _, err := parseUserArgs([]string{"create", "--identity-provider", "google", "--identity-issuer", "https://issuer.example.test"})
	if err == nil || !strings.Contains(err.Error(), "identity subject is required") {
		t.Fatalf("expected missing subject error, got %v", err)
	}
}

func TestParseUserArgsCreateRequiresExplicitProvider(t *testing.T) {
	cmd, showHelp, err := parseUserArgs([]string{
		"create",
		"--identity-provider", "google",
		"--identity-issuer", "https://issuer.example.test",
		"--identity-subject", "sub-1",
		"--display-name", "New User",
	})
	if err != nil {
		t.Fatalf("parseUserArgs: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help")
	}
	if cmd.action != "create" {
		t.Fatalf("action = %q, want create", cmd.action)
	}
	if cmd.identity.Provider != "google" || cmd.identity.Issuer != "https://issuer.example.test" || cmd.identity.Subject != "sub-1" {
		t.Fatalf("unexpected identity: %+v", cmd.identity)
	}
	if cmd.profile.DisplayName != "New User" {
		t.Fatalf("unexpected profile request: %+v", cmd.profile)
	}
}

func TestParseOrgArgsCreateAndGetRequireRefs(t *testing.T) {
	_, _, err := parseOrgArgs([]string{"create"})
	if err == nil || !strings.Contains(err.Error(), "organization slug is required") {
		t.Fatalf("expected missing slug error, got %v", err)
	}

	_, _, err = parseOrgArgs([]string{"get"})
	if err == nil || !strings.Contains(err.Error(), "organization reference is required") {
		t.Fatalf("expected missing ref error, got %v", err)
	}

	cmd, showHelp, err := parseOrgArgs([]string{"create", "--name", "Evil Lab", "evil-lab"})
	if err != nil {
		t.Fatalf("parseOrgArgs: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help")
	}
	if cmd.action != "create" || cmd.slug != "evil-lab" || cmd.displayName != "Evil Lab" {
		t.Fatalf("unexpected org command: %+v", cmd)
	}

	cmd, showHelp, err = parseOrgArgs([]string{"create", "evil-lab", "--name", "Evil Lab"})
	if err != nil {
		t.Fatalf("parseOrgArgs interspersed: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help")
	}
	if cmd.action != "create" || cmd.slug != "evil-lab" || cmd.displayName != "Evil Lab" {
		t.Fatalf("unexpected interspersed org command: %+v", cmd)
	}
}

func TestParseUserOrgArgsHelpAndValidation(t *testing.T) {
	userCases := []struct {
		name string
		args []string
		want string
		help bool
	}{
		{name: "user help", args: []string{"--help"}, help: true},
		{name: "user short help", args: []string{"-h"}, help: true},
		{name: "user me help", args: []string{"me", "--help"}, help: true},
		{name: "user register help", args: []string{"register", "--help"}, help: true},
		{name: "user create help", args: []string{"create", "--help"}, help: true},
		{name: "missing user command", args: nil, want: "Missing user command"},
		{name: "unknown user command", args: []string{"bad"}, want: "Unknown user command"},
		{name: "user me extra", args: []string{"me", "extra"}, want: "user me does not accept arguments"},
		{name: "register positional", args: []string{"register", "extra"}, want: "does not accept positional arguments"},
		{name: "create positional", args: []string{"create", "--identity-issuer", "iss", "--identity-subject", "sub", "extra"}, want: "does not accept positional arguments"},
		{name: "create missing issuer", args: []string{"create", "--identity-provider", "google", "--identity-subject", "sub"}, want: "identity issuer is required"},
	}
	for _, tc := range userCases {
		t.Run(tc.name, func(t *testing.T) {
			_, showHelp, err := parseUserArgs(tc.args)
			if tc.help {
				if err != nil || !showHelp {
					t.Fatalf("expected help, showHelp=%v err=%v", showHelp, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}

	orgCases := []struct {
		name string
		args []string
		want string
		help bool
	}{
		{name: "org help", args: []string{"--help"}, help: true},
		{name: "org short help", args: []string{"-h"}, help: true},
		{name: "org create help", args: []string{"create", "--help"}, help: true},
		{name: "org ls help", args: []string{"ls", "--help"}, help: true},
		{name: "org get help", args: []string{"get", "--help"}, help: true},
		{name: "missing org command", args: nil, want: "Missing organization command"},
		{name: "unknown org command", args: []string{"bad"}, want: "Unknown organization command"},
		{name: "org ls extra", args: []string{"ls", "extra"}, want: "org ls does not accept arguments"},
		{name: "org get too many", args: []string{"get", "one", "two"}, want: "too many org get arguments"},
		{name: "org create too many", args: []string{"create", "one", "two"}, want: "too many org create arguments"},
	}
	for _, tc := range orgCases {
		t.Run(tc.name, func(t *testing.T) {
			_, showHelp, err := parseOrgArgs(tc.args)
			if tc.help {
				if err != nil || !showHelp {
					t.Fatalf("expected help, showHelp=%v err=%v", showHelp, err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestRunUserCommandsRemoteOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/users/me":
			_, _ = w.Write([]byte(`{"status":"existing","user":{"id":"usr_1","display_name":"Evil Guest"},"identities":[],"memberships":[]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/users/me":
			if r.Header.Get("If-None-Match") != "*" {
				t.Fatalf("expected create-only self-registration")
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"created","user":{"id":"usr_1","display_name":"Evil Guest"},"identities":[],"memberships":[]}`))
		case r.Method == http.MethodPut && r.URL.Path == "/v1/users/by-identity":
			if r.URL.Query().Get("issuer") != "https://issuer.example.test" || r.URL.Query().Get("subject") != "sub-1" {
				t.Fatalf("unexpected identity query: %s", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"status":"created","user":{"id":"usr_2","display_name":"New User"},"identities":[],"memberships":[]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx := commandContext{
		profileName: "remote",
		profile:     config.ProfileConfig{Endpoint: server.URL},
		mode:        "remote",
		output:      "human",
	}

	var out strings.Builder
	if err := runUser(&out, ctx, []string{"me"}, "json"); err != nil {
		t.Fatalf("runUser me: %v", err)
	}
	if !strings.Contains(out.String(), `"id":"usr_1"`) {
		t.Fatalf("unexpected user me json: %q", out.String())
	}

	out.Reset()
	if err := runUser(&out, ctx, []string{"register", "--display-name", "Evil Guest"}, "human"); err != nil {
		t.Fatalf("runUser register: %v", err)
	}
	if !strings.Contains(out.String(), "status: created") {
		t.Fatalf("unexpected register output: %q", out.String())
	}

	out.Reset()
	err := runUser(&out, ctx, []string{
		"create",
		"--identity-provider", "google",
		"--identity-issuer", "https://issuer.example.test",
		"--identity-subject", "sub-1",
		"--display-name", "New User",
	}, "human")
	if err != nil {
		t.Fatalf("runUser create: %v", err)
	}
	if !strings.Contains(out.String(), "user: usr_2") {
		t.Fatalf("unexpected create output: %q", out.String())
	}
}

func TestRunOrgCommandsRemoteOutput(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/organizations":
			w.WriteHeader(http.StatusCreated)
			_, _ = w.Write([]byte(`{"organization":{"id":"org_1","slug":"evil-lab","display_name":"Evil Lab"},"membership":{"user_id":"usr_1","organization_id":"org_1","role":"admin"}}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/organizations":
			_, _ = w.Write([]byte(`[{"organization":{"id":"org_1","slug":"evil-lab","display_name":"Evil Lab"},"membership":{"user_id":"usr_1","organization_id":"org_1","role":"admin"}}]`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/organizations/evil-lab":
			_, _ = w.Write([]byte(`{"organization":{"id":"org_1","slug":"evil-lab","display_name":"Evil Lab"},"membership":{"user_id":"usr_1","organization_id":"org_1","role":"admin"}}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	ctx := commandContext{
		profileName: "remote",
		profile:     config.ProfileConfig{Endpoint: server.URL},
		mode:        "remote",
		output:      "human",
	}

	var out strings.Builder
	if err := runOrg(&out, ctx, []string{"create", "evil-lab", "--name", "Evil Lab"}, "human"); err != nil {
		t.Fatalf("runOrg create: %v", err)
	}
	if !strings.Contains(out.String(), "organization: evil-lab") || !strings.Contains(out.String(), "id: org_1") || !strings.Contains(out.String(), "role: admin") {
		t.Fatalf("unexpected create output: %q", out.String())
	}

	out.Reset()
	if err := runOrg(&out, ctx, []string{"ls"}, "json"); err != nil {
		t.Fatalf("runOrg ls: %v", err)
	}
	if !strings.Contains(out.String(), `"slug":"evil-lab"`) {
		t.Fatalf("unexpected list json: %q", out.String())
	}

	out.Reset()
	if err := runOrg(&out, ctx, []string{"get", "evil-lab"}, "human"); err != nil {
		t.Fatalf("runOrg get: %v", err)
	}
	if !strings.Contains(out.String(), "organization: evil-lab") || !strings.Contains(out.String(), "id: org_1") {
		t.Fatalf("unexpected get output: %q", out.String())
	}
}

func TestRunUserOrgRejectLocalModeBeforeRemoteCall(t *testing.T) {
	ctx := commandContext{
		profileName: "local",
		profile:     config.ProfileConfig{Endpoint: "auto"},
		mode:        "local",
		output:      "human",
	}

	var out strings.Builder
	err := runUser(&out, ctx, []string{"me"}, "human")
	if err == nil || !strings.Contains(err.Error(), "require remote mode") {
		t.Fatalf("expected remote-only user error, got %v", err)
	}

	err = runOrg(&out, ctx, []string{"ls"}, "human")
	if err == nil || !strings.Contains(err.Error(), "require remote mode") {
		t.Fatalf("expected remote-only org error, got %v", err)
	}
}

func TestRunnerRoutesUserOrgThroughInjectedHandlers(t *testing.T) {
	cwd := t.TempDir()

	t.Run("user", func(t *testing.T) {
		userCalls := 0
		orgCalls := 0
		err := runWithParsedCommands(t, cli.GlobalOptions{}, []cli.Command{{Name: "user", Args: []string{"me"}}}, func(deps *runnerDeps) {
			deps.getwd = func() (string, error) { return cwd, nil }
			deps.resolveCommandContext = func(string, cli.GlobalOptions) (commandContext, error) {
				return testCommandContext(cwd, "human", false), nil
			}
			deps.runUser = func(io.Writer, commandContext, []string, string) error {
				userCalls++
				return nil
			}
			deps.runOrg = func(io.Writer, commandContext, []string, string) error {
				orgCalls++
				return nil
			}
		})
		if err != nil {
			t.Fatalf("runner.run: %v", err)
		}
		if userCalls != 1 || orgCalls != 0 {
			t.Fatalf("unexpected handler calls: user=%d org=%d", userCalls, orgCalls)
		}
	})

	t.Run("org", func(t *testing.T) {
		userCalls := 0
		orgCalls := 0
		err := runWithParsedCommands(t, cli.GlobalOptions{}, []cli.Command{{Name: "org", Args: []string{"ls"}}}, func(deps *runnerDeps) {
			deps.getwd = func() (string, error) { return cwd, nil }
			deps.resolveCommandContext = func(string, cli.GlobalOptions) (commandContext, error) {
				return testCommandContext(cwd, "human", false), nil
			}
			deps.runUser = func(io.Writer, commandContext, []string, string) error {
				userCalls++
				return nil
			}
			deps.runOrg = func(io.Writer, commandContext, []string, string) error {
				orgCalls++
				return nil
			}
		})
		if err != nil {
			t.Fatalf("runner.run: %v", err)
		}
		if userCalls != 0 || orgCalls != 1 {
			t.Fatalf("unexpected handler calls: user=%d org=%d", userCalls, orgCalls)
		}
	})
}

func TestRunnerRejectsCompositeUserOrgCommands(t *testing.T) {
	err := runWithParsedCommands(t, cli.GlobalOptions{}, []cli.Command{
		{Name: "user", Args: []string{"me"}},
		{Name: "org", Args: []string{"ls"}},
	}, func(deps *runnerDeps) {
		deps.resolveCommandContext = func(string, cli.GlobalOptions) (commandContext, error) {
			return testCommandContext(t.TempDir(), "human", false), nil
		}
	})
	if err == nil || !strings.Contains(err.Error(), "user cannot be combined") {
		t.Fatalf("expected composite user rejection, got %v", err)
	}
}

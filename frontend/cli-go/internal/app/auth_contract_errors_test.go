package app

import (
	"context"
	"errors"
	"github.com/sqlrs/cli/internal/authsession"
	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Q01/D06/L09: discovery refusals stop login before callback/browser ownership passes.
func TestLoginInstallationBoundariesBeforeManager(t *testing.T) {
	oldLoader, oldFactory := authProviderLoader, authManagerFactory
	t.Cleanup(func() { authProviderLoader = oldLoader; authManagerFactory = oldFactory })
	for _, mode := range []string{"legacy", "discovery-failure", "substitution", "foreign-exchange", "foreign-refresh"} {
		t.Run(mode, func(t *testing.T) {
			cwd := t.TempDir()
			setTestDirs(t, cwd)
			authMode := "remoteSession"
			if mode == "legacy" {
				authMode = "oidcSession"
			}
			writeProjectConfig(t, cwd, authTestConfig("remote", "https://api.example", authMode))
			authManagerFactory = func() authManager {
				return fakeAuthManager{onLogin: func(authLoginOptions) { t.Fatal("unsafe login reached manager") }}
			}
			authProviderLoader = func(context.Context, string, string) (client.AuthProviderConfiguration, error) {
				p := client.AuthProviderConfiguration{ID: "google", ExchangeEndpoint: "https://api.example/exchange", RefreshEndpoint: "https://api.example/refresh"}
				switch mode {
				case "discovery-failure":
					return p, errors.New("discovery unavailable")
				case "substitution":
					p.ID = "other"
				case "foreign-exchange":
					p.ExchangeEndpoint = "https://other.example/exchange"
				case "foreign-refresh":
					p.RefreshEndpoint = "https://other.example/refresh"
				}
				return p, nil
			}
			if err := runAuth(io.Discard, io.Discard, cwd, cli.GlobalOptions{Workspace: cwd}, []string{"login", "google"}); err == nil {
				t.Fatal("unsafe discovery accepted")
			}
		})
	}
}

// Q01: safe installation refusals retain invalid_grant semantics without remote text.
func TestInstallationAdapterSafeRefusalMapping(t *testing.T) {
	_, err := sessionTokenResult(client.AuthTokens{}, &client.ErrorResponseError{StatusCode: 400, Code: "auth_invalid_grant", Message: "PRIVATE-CANARY"})
	var refusal *authsession.OAuthError
	if !errors.As(err, &refusal) || refusal.Code != "invalid_grant" || strings.Contains(err.Error(), "PRIVATE-CANARY") {
		t.Fatalf("unsafe refusal: %v", err)
	}
}

// Q01: provider discovery is anonymous and occurs only through the selected profile.
func TestAuthRefreshAndLogoutResolveCurrentProvider(t *testing.T) {
	oldLoader, oldFactory := authProviderLoader, authManagerFactory
	t.Cleanup(func() { authProviderLoader = oldLoader; authManagerFactory = oldFactory })
	for _, failure := range []bool{false, true} {
		authProviderLoader = func(context.Context, string, string) (client.AuthProviderConfiguration, error) {
			if failure {
				return client.AuthProviderConfiguration{}, errors.New("unavailable")
			}
			return client.AuthProviderConfiguration{ID: "university", Issuer: "https://issuer.example", ClientID: "client", RefreshEndpoint: "https://api.example/refresh", RevocationEndpoint: "https://issuer.example/revoke"}, nil
		}
		check := func(p authsession.ProviderConfiguration, err error) {
			if failure {
				if err == nil {
					t.Fatal("discovery refusal lost")
				}
			} else if err != nil || p.ID != "university" || p.RefreshEndpoint == "" {
				t.Fatalf("binding not forwarded: %+v %v", p, err)
			}
		}
		authManagerFactory = func() authManager {
			return fakeAuthManager{onResolve: func(o authResolveOptions) {
				p, err := o.ProviderResolver(context.Background(), "university")
				check(p, err)
			}}
		}
		ctx := remoteSessionCommandContext(t, "https://api.example", "https://api.example")
		ctx.mode = "remote"
		if _, err := resolveEffectiveAuthToken(context.Background(), ctx); err != nil {
			t.Fatal(err)
		}
		cwd := t.TempDir()
		setTestDirs(t, cwd)
		writeProjectConfig(t, cwd, authTestConfig("remote", "https://api.example", "remoteSession"))
		authManagerFactory = func() authManager { return discoveryLogoutManager{check: check} }
		if err := runAuth(io.Discard, io.Discard, cwd, cli.GlobalOptions{Workspace: cwd}, []string{"logout"}); err != nil {
			t.Fatal(err)
		}
	}
}

type discoveryLogoutManager struct {
	fakeAuthManager
	check func(authsession.ProviderConfiguration, error)
}

func (m discoveryLogoutManager) Logout(ctx context.Context, o authLogoutOptions) (authLogoutResult, error) {
	p, err := o.ProviderResolver(ctx, "university")
	m.check(p, err)
	return authLogoutResult{}, nil
}

// Q01/L08: output and post-login routing errors do not silently report full success.
func TestAuthLoginOutputAndRoutingErrors(t *testing.T) {
	oldFactory := authManagerFactory
	t.Cleanup(func() { authManagerFactory = oldFactory })
	for _, mode := range []string{"output", "authorization-url-output", "routing"} {
		t.Run(mode, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(503) }))
			defer server.Close()
			installAuthProviderFixture(t, server.URL)
			cwd := t.TempDir()
			setTestDirs(t, cwd)
			writeProjectConfig(t, cwd, authTestConfig("remote", server.URL, "remoteSession"))
			var out, errout io.Writer = io.Discard, io.Discard
			args := []string{"login", "google"}
			opts := cli.GlobalOptions{Workspace: cwd, Output: "json"}
			result := authLoginResult{}
			if mode == "output" {
				out = failingWriter{}
			}
			if mode == "authorization-url-output" {
				errout = failingWriter{}
				args = append(args, "--no-browser")
			}
			if mode == "routing" {
				result.BearerToken = "cached"
			}
			authManagerFactory = func() authManager { return fakeAuthManager{login: result} }
			if err := runAuth(out, errout, cwd, opts, args); err == nil {
				t.Fatal("failure reported success")
			}
		})
	}
}

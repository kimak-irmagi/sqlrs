package client

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetConnectionInfoIsPublicAndDecodesBootstrap(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/nsu/v1/connection-info" {
			t.Fatalf("request = %s %s", r.Method, r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("public discovery sent Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"installation_id":"taidon-production","endpoints":{"control":"https://api.taidon.dev","current":"https://api.taidon.dev/nsu"},"auth_providers":[{"id":"google","display_name":"Google","adapter":"oidc"}]}`))
	}))
	defer server.Close()

	got, err := New(server.URL+"/nsu", Options{Timeout: time.Second, AuthToken: "must-not-leak"}).GetConnectionInfo(context.Background())
	if err != nil {
		t.Fatalf("GetConnectionInfo: %v", err)
	}
	if got.InstallationID != "taidon-production" || got.Endpoints.Control != "https://api.taidon.dev" || got.Endpoints.Current != "https://api.taidon.dev/nsu" {
		t.Fatalf("connection info = %+v", got)
	}
	if len(got.AuthProviders) != 1 || got.AuthProviders[0].ID != "google" || got.AuthProviders[0].Adapter != "oidc" {
		t.Fatalf("providers = %+v", got.AuthProviders)
	}
}

func TestGetAuthProviderUsesAdvertisedGenericOIDCConfiguration(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/auth/providers/google" {
			t.Fatalf("path = %q", r.URL.Path)
		}
		if got := r.Header.Get("Authorization"); got != "" {
			t.Fatalf("public provider request sent Authorization = %q", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"google","display_name":"Google","adapter":"oidc","configuration_version":1,"flow":"authorizationCodePKCE","issuer":"https://accounts.example.test","client_id":"public-client","authorization_endpoint":"https://accounts.example.test/auth","token_endpoint":"https://accounts.example.test/token","token_endpoint_auth_method":"none","authorization_response_iss_parameter_supported":true,"scopes":["openid","email"],"authorization_parameters":{"prompt":"consent"}}`))
	}))
	defer server.Close()

	got, err := New(server.URL, Options{Timeout: time.Second}).GetAuthProvider(context.Background(), "google")
	if err != nil {
		t.Fatalf("GetAuthProvider: %v", err)
	}
	if got.Adapter != "oidc" || got.ClientID != "public-client" || got.AuthorizationEndpoint != "https://accounts.example.test/auth" || got.AuthorizationParameters["prompt"] != "consent" {
		t.Fatalf("provider = %+v", got)
	}
}

func TestValidateOIDCProviderRejectsReservedAuthorizationParameters(t *testing.T) {
	provider := validTestOIDCProvider()
	for _, key := range []string{"client_id", "redirect_uri", "scope", "state", "nonce", "code_challenge", "code_challenge_method", "login_hint", "response_mode", "request", "request_uri"} {
		provider.AuthorizationParameters = map[string]string{key: "collision"}
		if err := ValidateOIDCProvider("google", provider); err == nil || !strings.Contains(err.Error(), key) {
			t.Fatalf("reserved parameter %q: err = %v", key, err)
		}
	}
}

func validTestOIDCProvider() AuthProviderConfiguration {
	return AuthProviderConfiguration{
		ID: "google", DisplayName: "Google", Adapter: "oidc", ConfigurationVersion: 1,
		Flow: "authorizationCodePKCE", Issuer: "https://accounts.example.test", ClientID: "public-client",
		AuthorizationEndpoint: "https://accounts.example.test/auth", TokenEndpoint: "https://accounts.example.test/token",
		TokenEndpointAuthMethod: "none", AuthorizationResponseISSParameterSupported: true, Scopes: []string{"openid", "email"},
	}
}

func TestValidateConnectionInfoRejectsTrustBoundaryViolations(t *testing.T) {
	valid := ConnectionInfo{InstallationID: "installation-1", Endpoints: ConnectionEndpoints{Control: "https://api.example.test", Current: "https://api.example.test/nsu"}}
	cases := []struct {
		name string
		edit func(*ConnectionInfo)
	}{
		{name: "missing installation", edit: func(v *ConnectionInfo) { v.InstallationID = "" }},
		{name: "relative control", edit: func(v *ConnectionInfo) { v.Endpoints.Control = "/api" }},
		{name: "insecure control", edit: func(v *ConnectionInfo) { v.Endpoints.Control = "http://api.example.test" }},
		{name: "control query", edit: func(v *ConnectionInfo) { v.Endpoints.Control += "?tenant=x" }},
		{name: "control trailing slash", edit: func(v *ConnectionInfo) { v.Endpoints.Control += "/" }},
		{name: "current origin", edit: func(v *ConnectionInfo) { v.Endpoints.Current = "https://other.example.test/nsu" }},
		{name: "nested current", edit: func(v *ConnectionInfo) { v.Endpoints.Current = "https://api.example.test/a/b" }},
		{name: "invalid slug", edit: func(v *ConnectionInfo) { v.Endpoints.Current = "https://api.example.test/NO" }},
		{name: "invalid provider", edit: func(v *ConnectionInfo) {
			v.AuthProviders = []AuthProviderSummary{{ID: "Google", DisplayName: "Google", Adapter: "oidc"}}
		}},
		{name: "duplicate provider", edit: func(v *ConnectionInfo) {
			v.AuthProviders = []AuthProviderSummary{{ID: "google", DisplayName: "Google", Adapter: "oidc"}, {ID: "google", DisplayName: "Google 2", Adapter: "oidc"}}
		}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := valid
			tc.edit(&got)
			if err := ValidateConnectionInfo(got); err == nil {
				t.Fatalf("expected validation error for %+v", got)
			}
		})
	}
	loopback := valid
	loopback.Endpoints = ConnectionEndpoints{Control: "http://127.0.0.1:8080", Current: "http://127.0.0.1:8080"}
	if err := ValidateConnectionInfo(loopback); err != nil {
		t.Fatalf("loopback HTTP should be valid: %v", err)
	}
}

func TestValidateOIDCProviderRejectsUnsupportedConfigurations(t *testing.T) {
	cases := []struct {
		name string
		edit func(*AuthProviderConfiguration)
	}{
		{name: "id mismatch", edit: func(v *AuthProviderConfiguration) { v.ID = "other" }},
		{name: "adapter", edit: func(v *AuthProviderConfiguration) { v.Adapter = "googleOidc" }},
		{name: "version", edit: func(v *AuthProviderConfiguration) { v.ConfigurationVersion = 2 }},
		{name: "flow", edit: func(v *AuthProviderConfiguration) { v.Flow = "deviceCode" }},
		{name: "client auth", edit: func(v *AuthProviderConfiguration) { v.TokenEndpointAuthMethod = "client_secret_post" }},
		{name: "no response issuer", edit: func(v *AuthProviderConfiguration) { v.AuthorizationResponseISSParameterSupported = false }},
		{name: "missing client", edit: func(v *AuthProviderConfiguration) { v.ClientID = "" }},
		{name: "bad issuer", edit: func(v *AuthProviderConfiguration) { v.Issuer = "http://issuer.example.test" }},
		{name: "issuer query", edit: func(v *AuthProviderConfiguration) { v.Issuer += "?x=1" }},
		{name: "bad authorize", edit: func(v *AuthProviderConfiguration) { v.AuthorizationEndpoint = "/authorize" }},
		{name: "authorize reserved query", edit: func(v *AuthProviderConfiguration) { v.AuthorizationEndpoint += "?state=fixed" }},
		{name: "bad token", edit: func(v *AuthProviderConfiguration) { v.TokenEndpoint = "http://issuer.example.test/token" }},
		{name: "bad revoke", edit: func(v *AuthProviderConfiguration) { v.RevocationEndpoint = "http://issuer.example.test/revoke" }},
		{name: "empty scope", edit: func(v *AuthProviderConfiguration) { v.Scopes = []string{"openid", " "} }},
		{name: "duplicate scope", edit: func(v *AuthProviderConfiguration) { v.Scopes = []string{"openid", "openid"} }},
		{name: "no openid", edit: func(v *AuthProviderConfiguration) { v.Scopes = []string{"email"} }},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := validTestOIDCProvider()
			tc.edit(&got)
			if err := ValidateOIDCProvider("google", got); err == nil {
				t.Fatalf("expected validation error for %+v", got)
			}
		})
	}
}

func TestPublicBootstrapRejectsMalformedResponsesAndRedirects(t *testing.T) {
	cases := []struct {
		name        string
		contentType string
		body        string
		status      int
	}{
		{name: "wrong content type", contentType: "text/html", body: `{}`, status: http.StatusOK},
		{name: "unknown field", contentType: "application/json", body: `{"installation_id":"x","endpoints":{"control":"https://api.example.test","current":"https://api.example.test"},"auth_providers":[],"extra":true}`, status: http.StatusOK},
		{name: "trailing json", contentType: "application/json", body: `{"installation_id":"x","endpoints":{"control":"https://api.example.test","current":"https://api.example.test"},"auth_providers":[]} {}`, status: http.StatusOK},
		{name: "server error", contentType: "application/json", body: `{"code":"unavailable","message":"later"}`, status: http.StatusServiceUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", tc.contentType)
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer server.Close()
			if _, err := New(server.URL, Options{Timeout: time.Second}).GetConnectionInfo(context.Background()); err == nil {
				t.Fatalf("expected bootstrap error")
			}
		})
	}
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { t.Fatalf("redirect target must not be contacted") }))
	defer target.Close()
	redirect := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, http.StatusFound) }))
	defer redirect.Close()
	if _, err := New(redirect.URL, Options{Timeout: time.Second}).GetConnectionInfo(context.Background()); err == nil {
		t.Fatalf("expected redirect rejection")
	}
	if _, err := New(redirect.URL, Options{}).GetAuthProvider(context.Background(), "Invalid!"); err == nil {
		t.Fatalf("expected provider id rejection")
	}
}

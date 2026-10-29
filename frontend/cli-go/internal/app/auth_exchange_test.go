package app

import (
	"context"
	"encoding/json"
	"github.com/sqlrs/cli/internal/authsession"
	"github.com/sqlrs/cli/internal/client"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestInstallationTokenAdapterUsesProviderBinding(t *testing.T) {
	var seen map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			t.Error("cached bearer attached")
		}
		json.NewDecoder(r.Body).Decode(&seen)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]string{"id_token": "id", "refresh_token": "refresh"})
	}))
	defer server.Close()
	provider := authsession.LoginProvider{Binding: authsession.InstallationBinding{InstallationID: "installation", ControlEndpoint: server.URL, ProviderID: "university", Issuer: "https://issuer.example", ClientID: "client"}, ExchangeEndpoint: server.URL + "/exchange", RefreshEndpoint: server.URL + "/refresh"}
	adapter := installationTokenExchanger{}
	_, err := adapter.ExchangeCode(context.Background(), provider, authsession.CodeExchangeRequest{Code: "code", CodeVerifier: "verifier", RedirectURI: "http://127.0.0.1:1234/", ExpectedNonce: "nonce"})
	if err != nil {
		t.Fatal(err)
	}
	if seen["expected_installation_id"] != "installation" || seen["expected_issuer"] != provider.Binding.Issuer || seen["expected_nonce"] != "nonce" {
		t.Fatal("code binding lost")
	}
	_, err = adapter.Refresh(context.Background(), provider, authsession.RefreshRequest{RefreshToken: "old", ExpectedSubject: "subject", ExpectedNonce: "nonce"})
	if err != nil || seen["expected_subject"] != "subject" || seen["refresh_token"] != "old" {
		t.Fatal("refresh binding lost", err)
	}
}

func installAuthProviderFixture(t *testing.T, control string) {
	t.Helper()
	previous := authProviderLoader
	t.Cleanup(func() { authProviderLoader = previous })
	authProviderLoader = func(context.Context, string, string) (client.AuthProviderConfiguration, error) {
		return client.AuthProviderConfiguration{ID: "google", DisplayName: "Google", Adapter: "oidc", ConfigurationVersion: 2, Flow: "authorizationCodePKCE", Issuer: "https://issuer.example", ClientID: "client", AuthorizationEndpoint: "https://issuer.example/authorize", AuthorizationResponseISSParameterSupported: true, Scopes: []string{"openid"}, ExchangeEndpoint: control + "/v1/auth/providers/google/exchange", RefreshEndpoint: control + "/v1/auth/providers/google/refresh"}, nil
	}
}

// L08: the command timeout applies to both installation token operations.
func TestInstallationTokenAdapterHonorsTimeout(t *testing.T) {
	release := make(chan struct{})
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-r.Context().Done():
		case <-release:
		}
	}))
	defer s.Close()
	defer close(release)
	p := authsession.LoginProvider{Binding: authsession.InstallationBinding{ControlEndpoint: s.URL}, ExchangeEndpoint: s.URL + "/exchange", RefreshEndpoint: s.URL + "/refresh", Timeout: 20 * time.Millisecond}
	a := installationTokenExchanger{}
	started := time.Now()
	if _, err := a.ExchangeCode(context.Background(), p, authsession.CodeExchangeRequest{}); err == nil {
		t.Fatal("exchange ignored timeout")
	}
	if _, err := a.Refresh(context.Background(), p, authsession.RefreshRequest{}); err == nil {
		t.Fatal("refresh ignored timeout")
	}
	if time.Since(started) > time.Second {
		t.Fatal("configured timeout was not applied")
	}
}

package client

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestInstallationExchangeUsesAnonymousBoundJSON(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.Header.Get("Authorization") != "" || r.Header.Get("Content-Type") != "application/json" {
			t.Error("wrong credential transport")
		}
		var body map[string]any
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body["expected_installation_id"] != "installation" || body["expected_issuer"] != "https://issuer.example" || body["expected_client_id"] != "client" {
			t.Error("binding lost")
		}
		if _, ok := body["client_secret"]; ok {
			t.Error("secret in CLI request")
		}
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/exchange") {
			if body["code_verifier"] != "verifier" {
				t.Error("PKCE lost")
			}
			io.WriteString(w, `{"id_token":"id","refresh_token":"refresh"}`)
		} else {
			if body["expected_subject"] != "subject" || body["refresh_token"] != "old" {
				t.Error("refresh binding lost")
			}
			io.WriteString(w, `{"id_token":"new-id"}`)
		}
	}))
	defer server.Close()
	c := New(server.URL, Options{AuthToken: "unrelated", Timeout: time.Second})
	_, err := c.ExchangeCode(context.Background(), server.URL+"/v1/auth/providers/university/exchange", CodeExchangeRequest{InstallationID: "installation", Issuer: "https://issuer.example", ClientID: "client", Code: "code", CodeVerifier: "verifier", RedirectURI: "http://127.0.0.1:1234/", Nonce: "nonce"})
	if err != nil {
		t.Fatal(err)
	}
	result, err := c.RefreshTokens(context.Background(), server.URL+"/v1/auth/providers/university/refresh", RefreshTokensRequest{InstallationID: "installation", Issuer: "https://issuer.example", ClientID: "client", RefreshToken: "old", Subject: "subject", Nonce: "nonce"})
	if err != nil || result.RefreshToken != "" {
		t.Fatalf("refresh: %v", err)
	}
}

func TestInstallationExchangeRefusesRedirectsAndUntrustedEndpoints(t *testing.T) {
	var hits atomic.Int32
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer target.Close()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Redirect(w, r, target.URL, 307) }))
	defer s.Close()
	c := New(s.URL, Options{})
	if _, err := c.RefreshTokens(context.Background(), s.URL+"/v1/auth/providers/a/refresh", RefreshTokensRequest{}); err == nil {
		t.Fatal("redirect accepted")
	}
	if hits.Load() != 0 {
		t.Fatal("credentials leaked through redirect")
	}
	for _, raw := range []string{target.URL + "/refresh", s.URL + "/refresh?token=x", s.URL + "/refresh#fragment", s.URL + "/base/../refresh", s.URL + "/base/%2e%2e/refresh"} {
		if err := ValidateOperationEndpoint(s.URL+"/base", raw); err == nil {
			t.Fatalf("untrusted endpoint accepted: %s", raw)
		}
	}
	if err := ValidateOperationEndpoint("https://api.example/control/base", "https://api.example/control/base/v1/auth/providers/a/refresh"); err != nil {
		t.Fatal(err)
	}
}

func TestInstallationTokenResponseValidationAndSafeFailures(t *testing.T) {
	for _, body := range []string{`{"access_token":"only-access"}`, `{"id_token":""}`, `{"id_token":"id","refresh_token":""}`, `{"id_token":"id","refresh_token":null}`, `{"id_token":"id","extra":true}`, `{"id_token":"id"} {}`, "CANARY-PRIVATE", strings.Repeat(" ", (1<<20)+1)} {
		s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, body)
		}))
		_, err := New(s.URL, Options{}).RefreshTokens(context.Background(), s.URL+"/refresh", RefreshTokensRequest{})
		s.Close()
		if err == nil || strings.Contains(err.Error(), "CANARY-PRIVATE") {
			t.Fatalf("invalid or unsafe response error: %v", err)
		}
	}
}

// Q01/Q05: real HTTP failures preserve anonymous, bounded installation transport.
func TestInstallationExchangeFailureBoundaries(t *testing.T) {
	for _, mode := range []string{"no-refresh", "media", "disconnect", "cancelled", "invalid-control", "invalid-endpoint", "base-escape"} {
		t.Run(mode, func(t *testing.T) {
			hits := 0
			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				hits++
				if mode == "disconnect" {
					conn, _, _ := w.(http.Hijacker).Hijack()
					conn.Close()
					return
				}
				if mode != "media" {
					w.Header().Set("Content-Type", "application/json")
				}
				io.WriteString(w, `{"id_token":"id"}`)
			}))
			defer s.Close()
			base, endpoint := s.URL, s.URL+"/exchange"
			if mode == "invalid-control" {
				base = "invalid"
			}
			if mode == "invalid-endpoint" {
				endpoint = s.URL + "/bad?secret=x"
			}
			if mode == "base-escape" {
				base = s.URL + "/base"
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			_, err := New(base, Options{UserAgent: "sqlrs-test"}).ExchangeCode(ctx, endpoint, CodeExchangeRequest{})
			if err == nil {
				t.Fatal("invalid exchange accepted")
			}
			if (mode == "invalid-control" || mode == "invalid-endpoint" || mode == "base-escape" || mode == "cancelled") && hits != 0 {
				t.Fatal("invalid request reached transport")
			}
		})
	}
}

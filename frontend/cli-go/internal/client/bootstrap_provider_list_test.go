package client

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestGetAuthProvidersUsesSelectedPublicEndpoint(t *testing.T) {
	for _, prefix := range []string{"", "/school"} {
		t.Run(prefix, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodGet || r.URL.Path != prefix+"/v1/auth/providers" {
					t.Errorf("request = %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "" || r.Header.Get("Cookie") != "" {
					t.Errorf("public request carried credentials")
				}
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, `{"providers":[{"id":"yandex","display_name":"Yandex ID","adapter":"oidc"},{"id":"google","display_name":"Google","adapter":"oidc"}]}`)
			}))
			defer server.Close()

			got, err := New(server.URL+prefix, Options{AuthToken: "must-not-leak"}).GetAuthProviders(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if len(got.Providers) != 2 || got.Providers[0].ID != "yandex" || got.Providers[1].DisplayName != "Google" {
				t.Fatalf("providers = %+v", got.Providers)
			}
		})
	}
}

func TestGetAuthProvidersRejectsInvalidCatalogue(t *testing.T) {
	cases := []struct {
		name        string
		body        string
		contentType string
	}{
		{name: "missing array", body: `{}`},
		{name: "null array", body: `{"providers":null}`},
		{name: "unknown field", body: `{"providers":[],"secret":"x"}`},
		{name: "malformed", body: `{"providers":`},
		{name: "trailing", body: `{"providers":[]} {}`},
		{name: "invalid id", body: `{"providers":[{"id":"Google!","display_name":"Google","adapter":"oidc"}]}`},
		{name: "duplicate id", body: `{"providers":[{"id":"google","display_name":"Google","adapter":"oidc"},{"id":"google","display_name":"Other","adapter":"oidc"}]}`},
		{name: "blank name", body: `{"providers":[{"id":"google","display_name":" ","adapter":"oidc"}]}`},
		{name: "blank adapter", body: `{"providers":[{"id":"google","display_name":"Google","adapter":" "}]}`},
		{name: "wrong content type", body: `{"providers":[]}`, contentType: "text/plain"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				contentType := tc.contentType
				if contentType == "" {
					contentType = "application/json"
				}
				w.Header().Set("Content-Type", contentType)
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			if _, err := New(server.URL, Options{}).GetAuthProviders(context.Background()); err == nil {
				t.Fatal("invalid catalogue was accepted")
			}
		})
	}
}

func TestGetAuthProvidersAcceptsEmptyListAndBoundsBody(t *testing.T) {
	const limit = 1 << 20
	const before = `{"providers":[{"id":"google","display_name":"`
	const after = `","adapter":"oidc"}]}`
	for _, tc := range []struct {
		name    string
		body    string
		wantErr bool
	}{
		{name: "empty", body: `{"providers":[]}`},
		{name: "at limit", body: before + strings.Repeat("a", limit-len(before)-len(after)) + after},
		{name: "over limit", body: before + strings.Repeat("a", limit-len(before)-len(after)+1) + after, wantErr: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				fmt.Fprint(w, tc.body)
			}))
			defer server.Close()
			got, err := New(server.URL, Options{}).GetAuthProviders(context.Background())
			if tc.wantErr {
				if err == nil || len(got.Providers) != 0 {
					t.Fatalf("oversized catalogue: got %+v, err %v", got, err)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestGetAuthProvidersRedirectAndUnavailable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/auth/providers" {
			http.Redirect(w, r, "/listed", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"providers":[]}`)
	}))
	defer server.Close()
	if _, err := New(server.URL, Options{}).GetAuthProviders(context.Background()); err != nil {
		t.Fatalf("same-origin redirect: %v", err)
	}

	foreign := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		fmt.Fprint(w, `{"providers":[]}`)
	}))
	defer foreign.Close()
	crossOrigin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL, http.StatusFound)
	}))
	defer crossOrigin.Close()
	if _, err := New(crossOrigin.URL, Options{AuthToken: "secret"}).GetAuthProviders(context.Background()); err == nil {
		t.Fatal("cross-origin redirect accepted")
	}
	tlsRedirect := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, foreign.URL, http.StatusFound)
	}))
	defer tlsRedirect.Close()
	tlsClient := New(tlsRedirect.URL, Options{})
	tlsClient.http = tlsRedirect.Client()
	if _, err := tlsClient.GetAuthProviders(context.Background()); err == nil {
		t.Fatal("HTTPS-to-HTTP redirect accepted")
	}

	unavailable := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "unavailable", http.StatusServiceUnavailable)
	}))
	defer unavailable.Close()
	if _, err := New(unavailable.URL, Options{}).GetAuthProviders(context.Background()); err == nil {
		t.Fatal("503 accepted")
	}

	slow := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		time.Sleep(30 * time.Millisecond)
		fmt.Fprint(w, `{"providers":[]}`)
	}))
	defer slow.Close()
	if _, err := New(slow.URL, Options{Timeout: time.Millisecond}).GetAuthProviders(context.Background()); err == nil {
		t.Fatal("timeout accepted")
	}
}

type providerRoundTripFunc func(*http.Request) (*http.Response, error)

func (fn providerRoundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return fn(req)
}

type providerReadErrorBody struct{}

func (providerReadErrorBody) Read([]byte) (int, error) { return 0, errors.New("response read failed") }
func (providerReadErrorBody) Close() error             { return nil }

func TestGetAuthProvidersPropagatesBodyReadFailure(t *testing.T) {
	c := New("https://example.org", Options{})
	c.http.Transport = providerRoundTripFunc(func(req *http.Request) (*http.Response, error) {
		if req.URL.Path != "/v1/auth/providers" || req.Header.Get("Authorization") != "" {
			t.Fatalf("unexpected public request: %s", req.URL)
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			Header:     http.Header{"Content-Type": []string{"application/json"}},
			Body:       io.NopCloser(providerReadErrorBody{}),
			Request:    req,
		}, nil
	})
	if _, err := c.GetAuthProviders(context.Background()); err == nil || !strings.Contains(err.Error(), "response read failed") {
		t.Fatalf("read error = %v", err)
	}
}

func TestGetAuthProvidersBoundsErrorResponse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
		fmt.Fprint(w, strings.Repeat("x", maxAuthProviderListBytes+1))
	}))
	defer server.Close()
	if _, err := New(server.URL, Options{}).GetAuthProviders(context.Background()); !errors.Is(err, ErrInvalidPublicResponse) {
		t.Fatalf("oversized error response = %v", err)
	}
}

package authsession

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"io"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestGeneratePKCEPairUsesS256(t *testing.T) {
	entropy := bytes.Repeat([]byte{0x42}, pkceEntropyBytes)

	pair, err := GeneratePKCEPair(bytes.NewReader(entropy))
	if err != nil {
		t.Fatalf("GeneratePKCEPair: %v", err)
	}

	wantVerifier := base64.RawURLEncoding.EncodeToString(entropy)
	sum := sha256.Sum256([]byte(wantVerifier))
	wantChallenge := base64.RawURLEncoding.EncodeToString(sum[:])
	if pair.Verifier != wantVerifier {
		t.Fatalf("verifier = %q, want %q", pair.Verifier, wantVerifier)
	}
	if pair.Challenge != wantChallenge {
		t.Fatalf("challenge = %q, want %q", pair.Challenge, wantChallenge)
	}
	if pair.Method != "S256" {
		t.Fatalf("method = %q, want S256", pair.Method)
	}
}

func TestGeneratePKCEPairReportsEntropyError(t *testing.T) {
	if _, err := GeneratePKCEPair(errorReader{}); err == nil {
		t.Fatalf("expected entropy error")
	}
	if _, err := generateOpaqueToken(errorReader{}); err == nil {
		t.Fatalf("expected opaque token entropy error")
	}
}

func TestGeneratePKCEPairAndOpaqueTokenUseCryptoRandByDefault(t *testing.T) {
	pair, err := GeneratePKCEPair(nil)
	if err != nil {
		t.Fatalf("GeneratePKCEPair(nil): %v", err)
	}
	if pair.Verifier == "" || pair.Challenge == "" || pair.Method != "S256" {
		t.Fatalf("pair = %+v, want populated S256 PKCE pair", pair)
	}

	token, err := generateOpaqueToken(nil)
	if err != nil {
		t.Fatalf("generateOpaqueToken(nil): %v", err)
	}
	if token == "" {
		t.Fatalf("expected opaque token")
	}
}

func TestBuildGoogleAuthURLIncludesOIDCOfflinePKCEParameters(t *testing.T) {
	raw, err := BuildGoogleAuthURL(AuthURLOptions{
		ClientID:      "client.apps.googleusercontent.com",
		RedirectURI:   "http://127.0.0.1:49152",
		State:         "state-1",
		Nonce:         "nonce-1",
		CodeChallenge: "challenge-1",
		LoginHint:     "alice@example.com",
	})
	if err != nil {
		t.Fatalf("BuildGoogleAuthURL: %v", err)
	}

	parsed, err := url.Parse(raw)
	if err != nil {
		t.Fatalf("parse URL: %v", err)
	}
	if got := parsed.Scheme + "://" + parsed.Host + parsed.Path; got != googleAuthorizationEndpoint {
		t.Fatalf("endpoint = %q, want %q", got, googleAuthorizationEndpoint)
	}
	query := parsed.Query()
	assertQueryValue(t, query, "response_type", "code")
	assertQueryValue(t, query, "client_id", "client.apps.googleusercontent.com")
	assertQueryValue(t, query, "redirect_uri", "http://127.0.0.1:49152")
	assertQueryValue(t, query, "scope", strings.Join(googleOIDCScopes, " "))
	assertQueryValue(t, query, "state", "state-1")
	assertQueryValue(t, query, "nonce", "nonce-1")
	assertQueryValue(t, query, "code_challenge", "challenge-1")
	assertQueryValue(t, query, "code_challenge_method", "S256")
	assertQueryValue(t, query, "access_type", "offline")
	assertQueryValue(t, query, "prompt", "consent")
	assertQueryValue(t, query, "login_hint", "alice@example.com")
}

func TestBuildGoogleAuthURLUsesAdvertisedEndpointAndParameters(t *testing.T) {
	raw, err := BuildGoogleAuthURL(AuthURLOptions{
		ClientID: "public-client", RedirectURI: "http://127.0.0.1:49152",
		State: "state-canary", Nonce: "nonce-canary", CodeChallenge: "challenge-canary",
		AuthorizationEndpoint: "https://issuer.example.test/authorize?tenant=shared",
		Scopes:                []string{"openid", "email"}, AuthorizationParameters: map[string]string{"prompt": "consent"},
	})
	if err != nil {
		t.Fatalf("BuildGoogleAuthURL: %v", err)
	}
	parsed, _ := url.Parse(raw)
	if parsed.Host != "issuer.example.test" || parsed.Query().Get("tenant") != "shared" || parsed.Query().Get("prompt") != "consent" {
		t.Fatalf("authorization URL = %q", raw)
	}
	if strings.Contains(raw, "verifier") {
		t.Fatalf("authorization URL leaked PKCE verifier: %q", raw)
	}
	bad := AuthURLOptions{ClientID: "c", RedirectURI: "http://127.0.0.1:1", State: "s", Nonce: "n", CodeChallenge: "ch", AuthorizationEndpoint: "https://issuer.example.test/auth", Scopes: []string{"openid"}, AuthorizationParameters: map[string]string{"nonce": "collision"}}
	if _, err := BuildGoogleAuthURL(bad); err == nil {
		t.Fatalf("expected reserved parameter collision")
	}
}

func TestBuildGoogleAuthURLValidationErrors(t *testing.T) {
	valid := AuthURLOptions{
		ClientID:      "client-id",
		RedirectURI:   "http://127.0.0.1:1",
		State:         "state",
		Nonce:         "nonce",
		CodeChallenge: "challenge",
	}
	cases := []AuthURLOptions{
		withAuthURLClientID(valid, ""),
		withAuthURLRedirect(valid, ""),
		withAuthURLState(valid, ""),
		withAuthURLNonce(valid, ""),
		withAuthURLChallenge(valid, ""),
		withAuthURLEndpoint(valid, "http://%zz"),
	}
	for _, opts := range cases {
		if _, err := BuildGoogleAuthURL(opts); err == nil {
			t.Fatalf("expected validation error for %+v", opts)
		}
	}
}

func TestValidateCallback(t *testing.T) {
	t.Run("valid", func(t *testing.T) {
		got, err := ValidateCallback(url.Values{"state": {"s"}, "code": {"c"}}, "s")
		if err != nil {
			t.Fatalf("ValidateCallback: %v", err)
		}
		if got.Code != "c" {
			t.Fatalf("code = %q, want c", got.Code)
		}
	})

	t.Run("state mismatch", func(t *testing.T) {
		if _, err := ValidateCallback(url.Values{"state": {"bad"}, "code": {"c"}}, "s"); err == nil || !strings.Contains(err.Error(), "state") {
			t.Fatalf("expected state error, got %v", err)
		}
	})

	t.Run("duplicate state and code", func(t *testing.T) {
		if _, err := ValidateCallback(url.Values{"state": {"s", "s"}, "code": {"c"}}, "s"); err == nil {
			t.Fatalf("expected duplicate state error")
		}
		if _, err := ValidateCallback(url.Values{"state": {"s"}, "code": {"c", "c2"}}, "s"); err == nil {
			t.Fatalf("expected duplicate code error")
		}
	})

	t.Run("oauth error", func(t *testing.T) {
		values := url.Values{"state": {"s"}, "error": {"access_denied"}, "error_description": {"denied"}}
		if _, err := ValidateCallback(values, "s"); err == nil || !strings.Contains(err.Error(), "access_denied") {
			t.Fatalf("expected OAuth error, got %v", err)
		}
	})

	t.Run("oauth error without description", func(t *testing.T) {
		values := url.Values{"state": {"s"}, "error": {"access_denied"}}
		if _, err := ValidateCallback(values, "s"); err == nil || !strings.Contains(err.Error(), "access_denied") {
			t.Fatalf("expected OAuth error, got %v", err)
		}
	})

	t.Run("missing code", func(t *testing.T) {
		if _, err := ValidateCallback(url.Values{"state": {"s"}}, "s"); err == nil || !strings.Contains(err.Error(), "code") {
			t.Fatalf("expected missing code error, got %v", err)
		}
	})

	t.Run("rfc9207 issuer", func(t *testing.T) {
		values := url.Values{"state": {"s"}, "code": {"c"}, "iss": {"https://issuer.example.test"}}
		if _, err := ValidateCallbackIssuer(values, "s", "https://issuer.example.test", true); err != nil {
			t.Fatalf("ValidateCallbackIssuer: %v", err)
		}
		values.Del("iss")
		if _, err := ValidateCallbackIssuer(values, "s", "https://issuer.example.test", true); err == nil || !strings.Contains(err.Error(), "issuer") {
			t.Fatalf("expected issuer error, got %v", err)
		}
		values["iss"] = []string{"https://issuer.example.test", "https://issuer.example.test"}
		if _, err := ValidateCallbackIssuer(values, "s", "https://issuer.example.test", true); err == nil {
			t.Fatalf("expected duplicate issuer error")
		}
	})
}

func TestShouldRefresh(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	window := 5 * time.Minute

	if !ShouldRefresh(time.Time{}, now, window) {
		t.Fatal("zero expiry should refresh")
	}
	if !ShouldRefresh(now.Add(window), now, window) {
		t.Fatal("expiry at refresh window should refresh")
	}
	if ShouldRefresh(now.Add(window+time.Second), now, window) {
		t.Fatal("expiry after refresh window should not refresh")
	}
}

func assertQueryValue(t *testing.T, query url.Values, key string, want string) {
	t.Helper()
	if got := query.Get(key); got != want {
		t.Fatalf("%s = %q, want %q", key, got, want)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("entropy unavailable")
}

var _ io.Reader = errorReader{}

func withAuthURLClientID(opts AuthURLOptions, value string) AuthURLOptions {
	opts.ClientID = value
	return opts
}

func withAuthURLRedirect(opts AuthURLOptions, value string) AuthURLOptions {
	opts.RedirectURI = value
	return opts
}

func withAuthURLState(opts AuthURLOptions, value string) AuthURLOptions {
	opts.State = value
	return opts
}

func withAuthURLNonce(opts AuthURLOptions, value string) AuthURLOptions {
	opts.Nonce = value
	return opts
}

func withAuthURLChallenge(opts AuthURLOptions, value string) AuthURLOptions {
	opts.CodeChallenge = value
	return opts
}

func withAuthURLEndpoint(opts AuthURLOptions, value string) AuthURLOptions {
	opts.AuthorizationEndpoint = value
	return opts
}

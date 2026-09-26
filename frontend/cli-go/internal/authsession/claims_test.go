package authsession

import (
	"encoding/base64"
	"encoding/json"
	"strings"
	"testing"
	"time"
)

func TestDecodeAndValidateIDTokenClaims(t *testing.T) {
	expiry := time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC)
	token := testIDToken(t, map[string]any{
		"iss":   "https://accounts.google.com",
		"aud":   "client-id",
		"sub":   "subject-1",
		"email": "alice@example.com",
		"exp":   expiry.Unix(),
		"nonce": "nonce-1",
	})

	claims, err := DecodeIDTokenClaims(token)
	if err != nil {
		t.Fatalf("DecodeIDTokenClaims: %v", err)
	}
	if claims.Issuer != "https://accounts.google.com" || claims.Email != "alice@example.com" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if len(claims.Audience) != 1 || claims.Audience[0] != "client-id" {
		t.Fatalf("audience = %+v, want client-id", claims.Audience)
	}
	if !claims.Expiry.Equal(expiry) {
		t.Fatalf("expiry = %s, want %s", claims.Expiry, expiry)
	}
	if err := ValidateIDTokenClaims(claims, "https://accounts.google.com", "client-id", "nonce-1", expiry.Add(-time.Minute)); err != nil {
		t.Fatalf("ValidateIDTokenClaims: %v", err)
	}
}

func TestValidateIDTokenClaimsRejectsIssuerAudienceExpiryAndNonce(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	base := IDTokenClaims{
		Issuer:   "https://accounts.google.com",
		Audience: []string{"client-id"},
		Subject:  "subject-1",
		Expiry:   now.Add(time.Hour),
		IssuedAt: now.Add(-time.Minute),
		Nonce:    "nonce-1",
	}

	cases := []struct {
		name   string
		claims IDTokenClaims
		want   string
	}{
		{name: "issuer", claims: withIssuer(base, "https://issuer.example"), want: "issuer"},
		{name: "audience", claims: withAudience(base, []string{"other"}), want: "audience"},
		{name: "multiple audiences", claims: withAudience(base, []string{"client-id", "other"}), want: "audience"},
		{name: "subject", claims: withSubject(base, ""), want: "subject"},
		{name: "authorized party", claims: withAuthorizedParty(base, "other"), want: "authorized party"},
		{name: "missing issued at", claims: withIssuedAt(base, time.Time{}), want: "issued-at"},
		{name: "future issued at", claims: withIssuedAt(base, now.Add(5*time.Minute+time.Second)), want: "future"},
		{name: "expiry", claims: withExpiry(base, now), want: "expired"},
		{name: "nonce", claims: withNonce(base, "other"), want: "nonce"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateIDTokenClaims(tc.claims, "https://accounts.google.com", "client-id", "nonce-1", now)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
		})
	}
}

func TestDecodeIDTokenClaimsSupportsAudienceArray(t *testing.T) {
	token := testIDToken(t, map[string]any{
		"iss": "https://accounts.google.com",
		"aud": []string{"client-1", "client-2"},
		"sub": "subject-1",
		"exp": time.Date(2026, 7, 2, 12, 30, 0, 0, time.UTC).Unix(),
	})

	claims, err := DecodeIDTokenClaims(token)
	if err != nil {
		t.Fatalf("DecodeIDTokenClaims: %v", err)
	}
	if len(claims.Audience) != 2 || claims.Audience[1] != "client-2" {
		t.Fatalf("audience = %+v, want array", claims.Audience)
	}
}

func TestDecodeIDTokenClaimsRejectsMalformedTokens(t *testing.T) {
	cases := []string{
		"not-a-jwt",
		"header.%%%bad%%%.sig",
		"header." + base64.RawURLEncoding.EncodeToString([]byte("{")) + ".sig",
		"header." + base64.RawURLEncoding.EncodeToString([]byte(`{"aud": 42}`)) + ".sig",
	}
	for _, token := range cases {
		if _, err := DecodeIDTokenClaims(token); err == nil {
			t.Fatalf("expected decode error for %q", token)
		}
	}
}

func TestMaskSubject(t *testing.T) {
	if got := maskSubject("abcd1234"); got != "abcd1234" {
		t.Fatalf("short mask = %q", got)
	}
	if got := maskSubject("abcdefghijklmnopqrstuvwxyz"); got != "abcd...wxyz" {
		t.Fatalf("long mask = %q", got)
	}
}

func testIDToken(t *testing.T, payload map[string]any) string {
	t.Helper()
	if _, ok := payload["iat"]; !ok {
		if exp, ok := payload["exp"].(int64); ok {
			payload["iat"] = exp - 3600
		}
	}
	header := map[string]any{"alg": "none"}
	return encodeJWTPart(t, header) + "." + encodeJWTPart(t, payload) + ".sig"
}

func withSubject(claims IDTokenClaims, subject string) IDTokenClaims {
	claims.Subject = subject
	return claims
}
func withAuthorizedParty(claims IDTokenClaims, azp string) IDTokenClaims {
	claims.AuthorizedParty = azp
	return claims
}
func withIssuedAt(claims IDTokenClaims, issuedAt time.Time) IDTokenClaims {
	claims.IssuedAt = issuedAt
	return claims
}

func encodeJWTPart(t *testing.T, value any) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal JWT part: %v", err)
	}
	return base64.RawURLEncoding.EncodeToString(data)
}

func withIssuer(claims IDTokenClaims, issuer string) IDTokenClaims {
	claims.Issuer = issuer
	return claims
}

func withAudience(claims IDTokenClaims, audience []string) IDTokenClaims {
	claims.Audience = audience
	return claims
}

func withExpiry(claims IDTokenClaims, expiry time.Time) IDTokenClaims {
	claims.Expiry = expiry
	return claims
}

func withNonce(claims IDTokenClaims, nonce string) IDTokenClaims {
	claims.Nonce = nonce
	return claims
}

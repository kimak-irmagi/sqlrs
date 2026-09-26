package authsession

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// IDTokenClaims is the safe subset of Google ID-token claims used locally for
// expiry checks and diagnostics. Gateway signature verification remains server-side.
type IDTokenClaims struct {
	Issuer          string
	Audience        []string
	Subject         string
	Email           string
	Expiry          time.Time
	IssuedAt        time.Time
	AuthorizedParty string
	Nonce           string
}

type idTokenClaimsJSON struct {
	Issuer          string        `json:"iss"`
	Audience        audienceClaim `json:"aud"`
	Subject         string        `json:"sub"`
	Email           string        `json:"email"`
	Expiry          int64         `json:"exp"`
	IssuedAt        int64         `json:"iat"`
	AuthorizedParty string        `json:"azp"`
	Nonce           string        `json:"nonce"`
}

type audienceClaim []string

func (a *audienceClaim) UnmarshalJSON(data []byte) error {
	var single string
	if err := json.Unmarshal(data, &single); err == nil {
		*a = []string{single}
		return nil
	}
	var many []string
	if err := json.Unmarshal(data, &many); err != nil {
		return err
	}
	*a = many
	return nil
}

// DecodeIDTokenClaims decodes the unsigned JWT payload for local metadata and
// expiry decisions. It deliberately does not replace gateway signature checks.
func DecodeIDTokenClaims(idToken string) (IDTokenClaims, error) {
	parts := strings.Split(strings.TrimSpace(idToken), ".")
	if len(parts) < 2 {
		return IDTokenClaims{}, fmt.Errorf("invalid ID token format")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return IDTokenClaims{}, fmt.Errorf("decode ID token payload: %w", err)
	}
	var raw idTokenClaimsJSON
	if err := json.Unmarshal(payload, &raw); err != nil {
		return IDTokenClaims{}, fmt.Errorf("parse ID token claims: %w", err)
	}
	return IDTokenClaims{
		Issuer:          strings.TrimSpace(raw.Issuer),
		Audience:        append([]string(nil), raw.Audience...),
		Subject:         strings.TrimSpace(raw.Subject),
		Email:           strings.TrimSpace(raw.Email),
		Expiry:          time.Unix(raw.Expiry, 0).UTC(),
		IssuedAt:        time.Unix(raw.IssuedAt, 0).UTC(),
		AuthorizedParty: strings.TrimSpace(raw.AuthorizedParty),
		Nonce:           strings.TrimSpace(raw.Nonce),
	}, nil
}

// ValidateIDTokenClaims validates the local OIDC invariants needed before the
// CLI caches login metadata or reuses a refreshed token.
func ValidateIDTokenClaims(claims IDTokenClaims, issuer string, clientID string, nonce string, now time.Time) error {
	if claims.Issuer != defaultIssuer(issuer) {
		return fmt.Errorf("ID token issuer mismatch: %s", claims.Issuer)
	}
	if strings.TrimSpace(claims.Subject) == "" {
		return fmt.Errorf("ID token subject is required")
	}
	if len(claims.Audience) != 1 || strings.TrimSpace(claims.Audience[0]) != strings.TrimSpace(clientID) {
		return fmt.Errorf("ID token audience mismatch")
	}
	if claims.AuthorizedParty != "" && claims.AuthorizedParty != strings.TrimSpace(clientID) {
		return fmt.Errorf("ID token authorized party mismatch")
	}
	if claims.IssuedAt.Unix() <= 0 {
		return fmt.Errorf("ID token issued-at claim is required")
	}
	if claims.IssuedAt.After(now.UTC().Add(5 * time.Minute)) {
		return fmt.Errorf("ID token issued-at claim is too far in the future")
	}
	if !claims.Expiry.After(now.UTC()) {
		return fmt.Errorf("ID token expired")
	}
	if strings.TrimSpace(nonce) != "" && claims.Nonce != strings.TrimSpace(nonce) {
		return fmt.Errorf("ID token nonce mismatch")
	}
	return nil
}

func containsString(values []string, want string) bool {
	for _, value := range values {
		if strings.TrimSpace(value) == want {
			return true
		}
	}
	return false
}

// ShouldRefresh reports whether a cached ID token is missing, expired, or close
// enough to expiry that the CLI should refresh it before a protected API call.
func ShouldRefresh(expiry, now time.Time, window time.Duration) bool {
	if expiry.IsZero() {
		return true
	}
	return !expiry.After(now.UTC().Add(window))
}

func maskSubject(subject string) string {
	subject = strings.TrimSpace(subject)
	if len(subject) <= 8 {
		return subject
	}
	return subject[:4] + "..." + subject[len(subject)-4:]
}

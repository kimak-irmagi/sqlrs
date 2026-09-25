package authsession

import (
	"fmt"
	"net/url"
	"strings"
)

// CallbackResult contains the validated authorization code callback data.
type CallbackResult struct {
	Code string
}

// ValidateCallback enforces the loopback callback checks from
// docs/architecture/cli-auth-flow.md before any token endpoint exchange.
func ValidateCallback(values url.Values, expectedState string) (CallbackResult, error) {
	return ValidateCallbackIssuer(values, expectedState, "", false)
}

// ValidateCallbackIssuer additionally enforces RFC 9207 issuer binding for a
// service-discovered multi-provider login attempt.
func ValidateCallbackIssuer(values url.Values, expectedState, expectedIssuer string, requireIssuer bool) (CallbackResult, error) {
	if len(values["state"]) != 1 {
		return CallbackResult{}, fmt.Errorf("OAuth callback must contain exactly one state")
	}
	if strings.TrimSpace(values.Get("state")) != strings.TrimSpace(expectedState) {
		return CallbackResult{}, fmt.Errorf("OAuth callback state mismatch")
	}
	if oauthErr := strings.TrimSpace(values.Get("error")); oauthErr != "" {
		desc := strings.TrimSpace(values.Get("error_description"))
		if desc != "" {
			return CallbackResult{}, fmt.Errorf("Google authorization failed: %s: %s", oauthErr, desc)
		}
		return CallbackResult{}, fmt.Errorf("Google authorization failed: %s", oauthErr)
	}
	if requireIssuer {
		if len(values["iss"]) != 1 || strings.TrimSpace(values.Get("iss")) != strings.TrimSpace(expectedIssuer) {
			return CallbackResult{}, fmt.Errorf("OAuth callback issuer mismatch")
		}
	}
	if len(values["code"]) > 1 {
		return CallbackResult{}, fmt.Errorf("OAuth callback must contain exactly one code")
	}
	code := strings.TrimSpace(values.Get("code"))
	if code == "" {
		return CallbackResult{}, fmt.Errorf("OAuth callback missing code")
	}
	return CallbackResult{Code: code}, nil
}

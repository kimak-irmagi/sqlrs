package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// CodeExchangeRequest is the secret-free installation API request.
type CodeExchangeRequest struct {
	InstallationID string `json:"expected_installation_id"`
	Issuer         string `json:"expected_issuer"`
	ClientID       string `json:"expected_client_id"`
	Nonce          string `json:"expected_nonce"`
	Code           string `json:"code"`
	RedirectURI    string `json:"redirect_uri"`
	CodeVerifier   string `json:"code_verifier"`
}

// RefreshTokensRequest retains the stored identity and login nonce.
type RefreshTokensRequest struct {
	InstallationID string `json:"expected_installation_id"`
	Issuer         string `json:"expected_issuer"`
	ClientID       string `json:"expected_client_id"`
	Nonce          string `json:"expected_nonce"`
	Subject        string `json:"expected_subject"`
	RefreshToken   string `json:"refresh_token"`
}

// AuthTokens are provider credentials stored only after local validation.
type AuthTokens struct {
	IDToken      string `json:"id_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	AccessToken  string `json:"access_token,omitempty"`
}

// ValidateOperationEndpoint restricts credentials to the selected control origin/base.
func ValidateOperationEndpoint(base, raw string) error {
	control, err := validateServiceBaseURL(base)
	if err != nil {
		return fmt.Errorf("invalid installation control base")
	}
	endpoint, err := validateServiceBaseURL(raw)
	if err != nil || !sameURLOrigin(control, endpoint) {
		return fmt.Errorf("authentication endpoint is outside the trusted installation")
	}
	// validateServiceBaseURL already rejects encoded separators and dot segments.
	prefix := strings.TrimSuffix(control.Path, "/") + "/"
	if !strings.HasPrefix(endpoint.Path, prefix) {
		return fmt.Errorf("authentication endpoint is outside the control base")
	}
	return nil
}

// ExchangeCode never attaches a cached bearer and never follows redirects.
func (c *Client) ExchangeCode(ctx context.Context, endpoint string, request CodeExchangeRequest) (AuthTokens, error) {
	// Both request DTOs contain only strings: JSON encoding has no failure case.
	data, _ := json.Marshal(request)
	result, err := c.postAuth(ctx, endpoint, data)
	if err == nil && result.RefreshToken == "" {
		return AuthTokens{}, fmt.Errorf("authentication response lacks refresh_token")
	}
	return result, err
}

// RefreshTokens permits omission, but not an explicit empty replacement token.
func (c *Client) RefreshTokens(ctx context.Context, endpoint string, request RefreshTokensRequest) (AuthTokens, error) {
	data, _ := json.Marshal(request)
	return c.postAuth(ctx, endpoint, data)
}

func (c *Client) postAuth(ctx context.Context, endpoint string, data []byte) (AuthTokens, error) {
	if err := ValidateOperationEndpoint(c.baseURL, endpoint); err != nil {
		return AuthTokens{}, err
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(data))
	if err != nil {
		return AuthTokens{}, fmt.Errorf("invalid authentication endpoint")
	}
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json")
	if c.userAgent != "" {
		request.Header.Set("User-Agent", c.userAgent)
	}
	transport := *c.http
	transport.CheckRedirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	response, err := transport.Do(request)
	if err != nil {
		return AuthTokens{}, fmt.Errorf("authentication request failed: %w", err)
	}
	defer response.Body.Close()
	data, err = io.ReadAll(io.LimitReader(response.Body, (1<<20)+1))
	if err != nil || len(data) > 1<<20 {
		return AuthTokens{}, fmt.Errorf("invalid or oversized authentication response")
	}
	if response.StatusCode != 200 {
		var failure struct {
			Code string `json:"code"`
		}
		_ = json.Unmarshal(data, &failure)
		switch failure.Code {
		case "auth_invalid_request", "auth_invalid_grant", "auth_provider_not_found", "auth_configuration_changed", "auth_configuration_unavailable", "auth_provider_unavailable", "auth_provider_response_invalid", "auth_provider_timeout":
		default:
			failure.Code = "auth_response_invalid"
		}
		return AuthTokens{}, &ErrorResponseError{StatusCode: response.StatusCode, Code: failure.Code, Message: "authentication request failed; rediscover or log in if required"}
	}
	if strings.ToLower(strings.TrimSpace(strings.Split(response.Header.Get("Content-Type"), ";")[0])) != "application/json" {
		return AuthTokens{}, fmt.Errorf("invalid authentication response content type")
	}
	var result AuthTokens
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&result) != nil {
		return AuthTokens{}, fmt.Errorf("invalid authentication response")
	}
	if _, err := decoder.Token(); err != io.EOF || strings.TrimSpace(result.IDToken) == "" {
		return AuthTokens{}, fmt.Errorf("invalid authentication token result")
	}
	var fields map[string]json.RawMessage
	_ = json.Unmarshal(data, &fields)
	for _, field := range []string{"refresh_token", "access_token"} {
		if raw, ok := fields[field]; ok {
			var value string
			if json.Unmarshal(raw, &value) != nil || strings.TrimSpace(value) == "" {
				return AuthTokens{}, fmt.Errorf("invalid optional authentication token")
			}
		}
	}
	return result, nil
}

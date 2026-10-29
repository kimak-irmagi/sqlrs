package authsession

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	googleAuthorizationEndpoint = "https://accounts.google.com/o/oauth2/v2/auth"
	googleRevocationEndpoint    = "https://oauth2.googleapis.com/revoke"
	defaultGoogleIssuer         = "https://accounts.google.com"
)

var googleOIDCScopes = []string{"openid", "email", "profile"}

// AuthURLOptions are the non-secret inputs used to construct the OIDC
// authorization URL for the flow documented in docs/architecture/cli-auth-flow.md.
type AuthURLOptions struct {
	ClientID                string
	RedirectURI             string
	State                   string
	Nonce                   string
	CodeChallenge           string
	LoginHint               string
	AuthorizationEndpoint   string
	Scopes                  []string
	AuthorizationParameters map[string]string
}

// BuildOIDCAuthURL builds a PKCE authorization URL from explicit provider metadata.
func BuildOIDCAuthURL(opts AuthURLOptions) (string, error) {
	endpoint := strings.TrimSpace(opts.AuthorizationEndpoint)
	if endpoint == "" {
		return "", fmt.Errorf("authorization endpoint is required")
	}
	if strings.TrimSpace(opts.ClientID) == "" {
		return "", fmt.Errorf("auth.clientID is required")
	}
	if strings.TrimSpace(opts.RedirectURI) == "" {
		return "", fmt.Errorf("redirect URI is required")
	}
	if strings.TrimSpace(opts.State) == "" {
		return "", fmt.Errorf("state is required")
	}
	if strings.TrimSpace(opts.Nonce) == "" {
		return "", fmt.Errorf("nonce is required")
	}
	if strings.TrimSpace(opts.CodeChallenge) == "" {
		return "", fmt.Errorf("code challenge is required")
	}
	scopes := opts.Scopes
	if len(scopes) == 0 {
		return "", fmt.Errorf("openid scope is required")
	}

	parsed, err := url.Parse(endpoint)
	if err != nil {
		return "", fmt.Errorf("parse authorization endpoint: %w", err)
	}
	query := parsed.Query()
	for key := range query {
		if _, reserved := reservedAuthURLParameters[key]; reserved {
			return "", fmt.Errorf("authorization endpoint query parameter %q is reserved", key)
		}
	}
	query.Set("response_type", "code")
	query.Set("client_id", strings.TrimSpace(opts.ClientID))
	query.Set("redirect_uri", strings.TrimSpace(opts.RedirectURI))
	query.Set("scope", strings.Join(scopes, " "))
	query.Set("state", strings.TrimSpace(opts.State))
	query.Set("nonce", strings.TrimSpace(opts.Nonce))
	query.Set("code_challenge", strings.TrimSpace(opts.CodeChallenge))
	query.Set("code_challenge_method", "S256")
	for key, value := range opts.AuthorizationParameters {
		if _, reserved := reservedAuthURLParameters[key]; reserved || query.Has(key) {
			return "", fmt.Errorf("authorization parameter %q is reserved", key)
		}
		query.Set(key, value)
	}
	if strings.TrimSpace(opts.LoginHint) != "" {
		query.Set("login_hint", strings.TrimSpace(opts.LoginHint))
	}
	parsed.RawQuery = query.Encode()
	return parsed.String(), nil
}

// InstallationBinding identifies the trusted installation and configured instance.
type InstallationBinding struct{ InstallationID, ControlEndpoint, ProviderID, Issuer, ClientID string }

// LoginProvider contains public metadata, not credentials of the provider application.
type LoginProvider struct {
	Binding                           InstallationBinding
	ExchangeEndpoint, RefreshEndpoint string
	Timeout                           time.Duration
}

// CodeExchangeRequest carries only attempt-local authorization artifacts.
type CodeExchangeRequest struct{ Code, CodeVerifier, RedirectURI, ExpectedNonce string }

// RefreshRequest binds refresh to the original subject and login nonce.
type RefreshRequest struct{ RefreshToken, ExpectedSubject, ExpectedNonce string }

// TokenResponse is the provider token subset returned by the installation.
type TokenResponse struct{ IDToken, RefreshToken, AccessToken string }

// TokenExchanger is implemented by the installation HTTP adapter, never a direct IdP client.
type TokenExchanger interface {
	ExchangeCode(context.Context, LoginProvider, CodeExchangeRequest) (TokenResponse, error)
	Refresh(context.Context, LoginProvider, RefreshRequest) (TokenResponse, error)
}

// TokenRevoker is an independent, explicit secret-free provider capability.
type TokenRevoker interface {
	Revoke(context.Context, string, string) error
}

// HTTPClient permits revocation transport substitution in tests.
type HTTPClient interface {
	Do(*http.Request) (*http.Response, error)
}

// ProviderRevoker performs the configured public revocation without redirects.
type ProviderRevoker struct{ HTTP HTTPClient }

// OAuthError is a safe typed refusal; no upstream description is populated by the HTTP adapter.
type OAuthError struct {
	Code, Description string
	Status            int
}

func (e *OAuthError) Error() string { return fmt.Sprintf("OAuth %s", e.Code) }
func (c ProviderRevoker) Revoke(ctx context.Context, endpoint, token string) error {
	form := url.Values{"token": {strings.TrimSpace(token)}}
	httpClient := c.HTTP
	if httpClient == nil {
		httpClient = noRedirectHTTPClient()
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		data, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		_ = data
		return fmt.Errorf("provider token revocation failed with status %d", resp.StatusCode)
	}
	return nil
}

func noRedirectHTTPClient() *http.Client {
	return &http.Client{Timeout: 30 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}
}

var reservedAuthURLParameters = map[string]struct{}{
	"client_id": {}, "response_type": {}, "redirect_uri": {}, "scope": {},
	"state": {}, "nonce": {}, "code_challenge": {}, "code_challenge_method": {},
	"login_hint": {}, "response_mode": {}, "request": {}, "request_uri": {},
}

func defaultIssuer(issuer string) string {
	if strings.TrimSpace(issuer) == "" {
		return defaultGoogleIssuer
	}
	return strings.TrimSpace(issuer)
}

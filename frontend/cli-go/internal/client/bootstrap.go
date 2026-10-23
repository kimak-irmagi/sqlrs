package client

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// ConnectionInfo is the unauthenticated installation bootstrap document from
// docs/api-guides/sqlrs-engine.openapi.yaml.
type ConnectionInfo struct {
	InstallationID string                `json:"installation_id"`
	Endpoints      ConnectionEndpoints   `json:"endpoints"`
	AuthProviders  []AuthProviderSummary `json:"auth_providers"`
}

type ConnectionEndpoints struct {
	Control string `json:"control"`
	Current string `json:"current"`
}

type AuthProviderSummary struct {
	ID          string `json:"id"`
	DisplayName string `json:"display_name"`
	Adapter     string `json:"adapter"`
}

// AuthProviderList is the public, current provider catalogue of an installation.
type AuthProviderList struct {
	Providers []AuthProviderSummary `json:"providers"`
}

// ErrInvalidPublicResponse distinguishes malformed discovery data from a
// transport or service failure without exposing the response body.
var ErrInvalidPublicResponse = errors.New("invalid public response")

const maxAuthProviderListBytes = 1 << 20

// GetAuthProviders fetches the current catalogue without client credentials.
// It implements docs/architecture/cli-auth-provider-discovery-structure.md:
// the wire response is bounded and validated before being returned.
func (c *Client) GetAuthProviders(ctx context.Context) (AuthProviderList, error) {
	var wire struct {
		Providers *[]AuthProviderSummary `json:"providers"`
	}
	if err := c.getPublicJSONWithLimit(ctx, "/v1/auth/providers", &wire, maxAuthProviderListBytes); err != nil {
		return AuthProviderList{}, err
	}
	if wire.Providers == nil {
		return AuthProviderList{}, fmt.Errorf("%w: providers array is required", ErrInvalidPublicResponse)
	}
	if err := validateAuthProviderSummaries(*wire.Providers); err != nil {
		return AuthProviderList{}, fmt.Errorf("%w: %v", ErrInvalidPublicResponse, err)
	}
	return AuthProviderList{Providers: *wire.Providers}, nil
}

// AuthProviderConfiguration is version 1 of the service-advertised generic
// OIDC public-client configuration. It deliberately contains no client secret.
type AuthProviderConfiguration struct {
	ID                                         string            `json:"id"`
	DisplayName                                string            `json:"display_name"`
	Adapter                                    string            `json:"adapter"`
	ConfigurationVersion                       int               `json:"configuration_version"`
	Flow                                       string            `json:"flow"`
	Issuer                                     string            `json:"issuer"`
	ClientID                                   string            `json:"client_id"`
	AuthorizationEndpoint                      string            `json:"authorization_endpoint"`
	TokenEndpoint                              string            `json:"token_endpoint"`
	TokenEndpointAuthMethod                    string            `json:"token_endpoint_auth_method"`
	AuthorizationResponseISSParameterSupported bool              `json:"authorization_response_iss_parameter_supported"`
	RevocationEndpoint                         string            `json:"revocation_endpoint,omitempty"`
	Scopes                                     []string          `json:"scopes"`
	AuthorizationParameters                    map[string]string `json:"authorization_parameters,omitempty"`
}

// GetConnectionInfo performs public discovery and never attaches the client's
// configured bearer token.
func (c *Client) GetConnectionInfo(ctx context.Context) (ConnectionInfo, error) {
	var out ConnectionInfo
	if err := c.getPublicJSON(ctx, "/v1/connection-info", &out); err != nil {
		return out, err
	}
	if err := ValidateConnectionInfo(out); err != nil {
		return ConnectionInfo{}, fmt.Errorf("invalid connection info: %w", err)
	}
	out.Endpoints.Control, _ = NormalizeServiceEndpoint(out.Endpoints.Control)
	out.Endpoints.Current, _ = NormalizeServiceEndpoint(out.Endpoints.Current)
	return out, nil
}

// GetAuthProvider fetches and validates one provider detail directly; listing
// the provider collection is not a prerequisite for login.
func (c *Client) GetAuthProvider(ctx context.Context, provider string) (AuthProviderConfiguration, error) {
	provider = strings.TrimSpace(provider)
	if !providerIDPattern.MatchString(provider) {
		return AuthProviderConfiguration{}, fmt.Errorf("invalid auth provider id %q", provider)
	}
	var out AuthProviderConfiguration
	if err := c.getPublicJSON(ctx, "/v1/auth/providers/"+url.PathEscape(provider), &out); err != nil {
		return out, err
	}
	if err := ValidateOIDCProvider(provider, out); err != nil {
		return AuthProviderConfiguration{}, fmt.Errorf("invalid auth provider %q: %w", provider, err)
	}
	return out, nil
}

func (c *Client) getPublicJSON(ctx context.Context, path string, out any) error {
	return c.getPublicJSONWithLimit(ctx, path, out, 0)
}

// getPublicJSONWithLimit preserves the public discovery redirect policy and
// optionally rejects a body larger than limit bytes before JSON decoding.
func (c *Client) getPublicJSONWithLimit(ctx context.Context, path string, out any, limit int64) error {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return err
	}
	if c.userAgent != "" {
		req.Header.Set("User-Agent", c.userAgent)
	}
	httpClient := *c.http
	httpClient.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 10 {
			return fmt.Errorf("bootstrap redirect limit exceeded")
		}
		if len(via) == 0 || req.URL.User != nil || !sameURLOrigin(via[0].URL, req.URL) {
			return fmt.Errorf("bootstrap redirect changed origin")
		}
		return nil
	}
	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var body io.Reader = resp.Body
	if limit > 0 {
		data, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
		if err != nil {
			return err
		}
		if int64(len(data)) > limit {
			return fmt.Errorf("%w: response exceeds %d bytes", ErrInvalidPublicResponse, limit)
		}
		body = bytes.NewReader(data)
		resp.Body = io.NopCloser(bytes.NewReader(data))
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return parseErrorResponse(resp)
	}
	if mediaType := strings.ToLower(strings.TrimSpace(strings.Split(resp.Header.Get("Content-Type"), ";")[0])); mediaType != "application/json" {
		return fmt.Errorf("%w: unexpected content type %q", ErrInvalidPublicResponse, resp.Header.Get("Content-Type"))
	}
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(out); err != nil {
		return fmt.Errorf("%w: decode bootstrap response: %v", ErrInvalidPublicResponse, err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return fmt.Errorf("%w: decode bootstrap response: trailing JSON content", ErrInvalidPublicResponse)
	}
	return nil
}

var providerIDPattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,62}$`)
var organizationSlugPattern = regexp.MustCompile(`^[a-z0-9](?:[a-z0-9]|-(?:[a-z0-9])){1,61}[a-z0-9]$`)

var reservedAuthorizationParameters = map[string]struct{}{
	"client_id": {}, "response_type": {}, "redirect_uri": {}, "scope": {},
	"state": {}, "nonce": {}, "code_challenge": {}, "code_challenge_method": {},
	"login_hint": {}, "response_mode": {}, "request": {}, "request_uri": {},
}

func ValidateConnectionInfo(info ConnectionInfo) error {
	if strings.TrimSpace(info.InstallationID) == "" {
		return fmt.Errorf("installation_id is required")
	}
	controlCanonical, err := NormalizeServiceEndpoint(info.Endpoints.Control)
	if err != nil {
		return fmt.Errorf("control endpoint: %w", err)
	}
	currentCanonical, err := NormalizeServiceEndpoint(info.Endpoints.Current)
	if err != nil {
		return fmt.Errorf("current endpoint: %w", err)
	}
	control, _ := url.Parse(controlCanonical)
	current, _ := url.Parse(currentCanonical)
	if control.EscapedPath() != "" {
		return fmt.Errorf("control endpoint must be the installation root")
	}
	if !strings.EqualFold(control.Scheme, current.Scheme) || !strings.EqualFold(control.Host, current.Host) {
		return fmt.Errorf("control and current endpoints must share an origin")
	}
	return validateAuthProviderSummaries(info.AuthProviders)
}

func validateAuthProviderSummaries(providers []AuthProviderSummary) error {
	seenProviders := map[string]struct{}{}
	for _, provider := range providers {
		if !providerIDPattern.MatchString(provider.ID) || strings.TrimSpace(provider.DisplayName) == "" || strings.TrimSpace(provider.Adapter) == "" {
			return fmt.Errorf("invalid auth provider summary %q", provider.ID)
		}
		if _, exists := seenProviders[provider.ID]; exists {
			return fmt.Errorf("duplicate auth provider %q", provider.ID)
		}
		seenProviders[provider.ID] = struct{}{}
	}
	return nil
}

// ValidateServiceEndpoint validates a caller-supplied bootstrap base before
// the CLI performs its first network request.
func ValidateServiceEndpoint(raw string) error {
	_, err := NormalizeServiceEndpoint(raw)
	return err
}

// NormalizeServiceEndpoint validates and canonicalizes the root-or-one-slug
// service-base grammar defined by the remote bootstrap architecture.
func NormalizeServiceEndpoint(raw string) (string, error) {
	parsed, err := validateServiceBaseURL(raw)
	if err != nil {
		return "", err
	}
	path := strings.TrimSuffix(parsed.EscapedPath(), "/")
	if path != "" {
		slug := strings.TrimPrefix(path, "/")
		if strings.Contains(slug, "/") || !organizationSlugPattern.MatchString(slug) {
			return "", fmt.Errorf("endpoint path must contain exactly one valid organization slug")
		}
		parsed.Path = "/" + slug
	} else {
		parsed.Path = ""
	}
	parsed.RawPath = ""
	parsed.Scheme = strings.ToLower(parsed.Scheme)
	hostname := strings.ToLower(parsed.Hostname())
	port := parsed.Port()
	if (parsed.Scheme == "https" && port == "443") || (parsed.Scheme == "http" && port == "80") {
		port = ""
	}
	if strings.Contains(hostname, ":") {
		hostname = "[" + hostname + "]"
	}
	parsed.Host = hostname
	if port != "" {
		parsed.Host += ":" + port
	}
	return parsed.String(), nil
}

func ValidateOIDCProvider(requested string, provider AuthProviderConfiguration) error {
	if provider.ID != requested {
		return fmt.Errorf("response id %q does not match requested provider", provider.ID)
	}
	if provider.Adapter != "oidc" || provider.ConfigurationVersion != 1 || provider.Flow != "authorizationCodePKCE" {
		return fmt.Errorf("unsupported adapter configuration")
	}
	if provider.TokenEndpointAuthMethod != "none" || !provider.AuthorizationResponseISSParameterSupported {
		return fmt.Errorf("provider is not a supported public OIDC client")
	}
	if strings.TrimSpace(provider.ClientID) == "" || strings.TrimSpace(provider.DisplayName) == "" {
		return fmt.Errorf("display_name and client_id are required")
	}
	if err := validateHTTPSURL(provider.Issuer, false); err != nil {
		return fmt.Errorf("issuer: %w", err)
	}
	if err := validateHTTPSURL(provider.AuthorizationEndpoint, true); err != nil {
		return fmt.Errorf("authorization endpoint: %w", err)
	}
	authorizationURL, _ := url.Parse(provider.AuthorizationEndpoint)
	for key := range authorizationURL.Query() {
		if _, reserved := reservedAuthorizationParameters[key]; reserved {
			return fmt.Errorf("authorization endpoint query parameter %q is reserved", key)
		}
	}
	if err := validateHTTPSURL(provider.TokenEndpoint, true); err != nil {
		return fmt.Errorf("token endpoint: %w", err)
	}
	if provider.RevocationEndpoint != "" {
		if err := validateHTTPSURL(provider.RevocationEndpoint, true); err != nil {
			return fmt.Errorf("revocation endpoint: %w", err)
		}
	}
	seen := map[string]struct{}{}
	hasOpenID := false
	for _, scope := range provider.Scopes {
		scope = strings.TrimSpace(scope)
		if scope == "" {
			return fmt.Errorf("scopes must not contain empty values")
		}
		if _, ok := seen[scope]; ok {
			return fmt.Errorf("duplicate scope %q", scope)
		}
		seen[scope] = struct{}{}
		hasOpenID = hasOpenID || scope == "openid"
	}
	if !hasOpenID {
		return fmt.Errorf("openid scope is required")
	}
	for key := range provider.AuthorizationParameters {
		if _, reserved := reservedAuthorizationParameters[key]; reserved {
			return fmt.Errorf("authorization parameter %q is reserved", key)
		}
	}
	return nil
}

func validateServiceBaseURL(raw string) (*url.URL, error) {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || !parsed.IsAbs() || parsed.Host == "" {
		return nil, fmt.Errorf("must be an absolute URL")
	}
	if parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return nil, fmt.Errorf("userinfo, query, and fragment are not allowed")
	}
	escapedPath := strings.ToLower(parsed.EscapedPath())
	if strings.Contains(escapedPath, "%2f") || strings.Contains(escapedPath, "%5c") || strings.Contains(escapedPath, "%2e") || strings.Contains(parsed.Path, "\\") {
		return nil, fmt.Errorf("encoded separators and dot segments are not allowed")
	}
	for _, segment := range strings.Split(parsed.Path, "/") {
		if segment == "." || segment == ".." {
			return nil, fmt.Errorf("encoded separators and dot segments are not allowed")
		}
	}
	if parsed.Scheme != "https" && !(parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")) {
		return nil, fmt.Errorf("HTTPS is required outside loopback")
	}
	return parsed, nil
}

func sameURLOrigin(left, right *url.URL) bool {
	return strings.EqualFold(left.Scheme, right.Scheme) &&
		strings.EqualFold(left.Hostname(), right.Hostname()) &&
		serviceURLPort(left) == serviceURLPort(right)
}

func serviceURLPort(value *url.URL) string {
	if port := value.Port(); port != "" {
		return port
	}
	if strings.EqualFold(value.Scheme, "https") {
		return "443"
	}
	if strings.EqualFold(value.Scheme, "http") {
		return "80"
	}
	return ""
}

func validateHTTPSURL(raw string, allowQuery bool) error {
	parsed, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" {
		return fmt.Errorf("must be an absolute HTTPS URL")
	}
	if parsed.User != nil || parsed.Fragment != "" || (!allowQuery && parsed.RawQuery != "") {
		return fmt.Errorf("contains forbidden URL components")
	}
	return nil
}

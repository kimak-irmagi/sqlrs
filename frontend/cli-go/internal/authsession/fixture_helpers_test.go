package authsession

import (
	"context"
	"net/url"
)

// Fixtures supply explicit discovery data that the former tests obtained from defaults.
func newFixtureManager(opts ManagerOptions) *Manager {
	if opts.Revoker == nil {
		if revoker, ok := opts.Exchanger.(TokenRevoker); ok {
			opts.Revoker = revoker
		}
	}
	return NewManager(opts)
}
func (m *Manager) loginFixture(ctx context.Context, opts LoginOptions) (LoginResult, error) {
	if opts.Issuer == "" {
		opts.Issuer = defaultGoogleIssuer
	}
	if opts.AuthorizationEndpoint == "" {
		opts.AuthorizationEndpoint = googleAuthorizationEndpoint
	}
	if opts.ExchangeEndpoint == "" {
		opts.ExchangeEndpoint = "https://sqlrs.example.org/v1/auth/providers/google/exchange"
	}
	if opts.RefreshEndpoint == "" {
		opts.RefreshEndpoint = "https://sqlrs.example.org/v1/auth/providers/google/refresh"
	}
	if len(opts.Scopes) == 0 {
		opts.Scopes = googleOIDCScopes
	}
	return m.LoginOIDC(ctx, opts)
}
func buildFixtureAuthURL(opts AuthURLOptions) (string, error) {
	if opts.AuthorizationEndpoint == "" {
		opts.AuthorizationEndpoint = googleAuthorizationEndpoint
	}
	if len(opts.Scopes) == 0 {
		opts.Scopes = googleOIDCScopes
	}
	if opts.AuthorizationParameters == nil {
		opts.AuthorizationParameters = map[string]string{"access_type": "offline", "prompt": "consent"}
	}
	return BuildOIDCAuthURL(opts)
}

var _ url.Values

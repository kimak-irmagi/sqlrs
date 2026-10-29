package app

import (
	"context"
	"errors"
	"github.com/sqlrs/cli/internal/authsession"
	"github.com/sqlrs/cli/internal/client"
)

// installationTokenExchanger translates session-owned types to the public API.
// Provider credentials never enter this adapter or the CLI credential store.
type installationTokenExchanger struct{}

func (installationTokenExchanger) ExchangeCode(ctx context.Context, p authsession.LoginProvider, r authsession.CodeExchangeRequest) (authsession.TokenResponse, error) {
	c := client.New(p.Binding.ControlEndpoint, client.Options{Timeout: p.Timeout})
	tokens, err := c.ExchangeCode(ctx, p.ExchangeEndpoint, client.CodeExchangeRequest{InstallationID: p.Binding.InstallationID, Issuer: p.Binding.Issuer, ClientID: p.Binding.ClientID, Nonce: r.ExpectedNonce, Code: r.Code, RedirectURI: r.RedirectURI, CodeVerifier: r.CodeVerifier})
	return sessionTokenResult(tokens, err)
}
func (installationTokenExchanger) Refresh(ctx context.Context, p authsession.LoginProvider, r authsession.RefreshRequest) (authsession.TokenResponse, error) {
	c := client.New(p.Binding.ControlEndpoint, client.Options{Timeout: p.Timeout})
	tokens, err := c.RefreshTokens(ctx, p.RefreshEndpoint, client.RefreshTokensRequest{InstallationID: p.Binding.InstallationID, Issuer: p.Binding.Issuer, ClientID: p.Binding.ClientID, Nonce: r.ExpectedNonce, Subject: r.ExpectedSubject, RefreshToken: r.RefreshToken})
	return sessionTokenResult(tokens, err)
}
func sessionTokenResult(tokens client.AuthTokens, err error) (authsession.TokenResponse, error) {
	if err != nil {
		var failure *client.ErrorResponseError
		if errors.As(err, &failure) && failure.Code == "auth_invalid_grant" {
			return authsession.TokenResponse{}, &authsession.OAuthError{Code: "invalid_grant", Status: failure.StatusCode}
		}
		return authsession.TokenResponse{}, err
	}
	return authsession.TokenResponse{IDToken: tokens.IDToken, RefreshToken: tokens.RefreshToken, AccessToken: tokens.AccessToken}, nil
}

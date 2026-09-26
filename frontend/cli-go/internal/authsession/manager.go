package authsession

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"reflect"
	"strings"
	"time"
)

const defaultRefreshWindow = 5 * time.Minute

var ErrLoginRequired = errors.New("Google auth session is expired or unavailable; run `sqlrs auth login google`")

// Clock is injected so refresh decisions can be tested deterministically.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time {
	return time.Now().UTC()
}

// BrowserOpener opens the authorization URL in the user's browser.
type BrowserOpener func(context.Context, string) error

// LoopbackProvider creates the temporary 127.0.0.1 listener used by login.
type LoopbackProvider interface {
	Start(context.Context) (LoopbackSession, error)
}

// LoopbackSession represents one authorization callback listener.
type LoopbackSession interface {
	RedirectURI() string
	Wait(context.Context) (url.Values, error)
	Close() error
}

// ManagerOptions wires the auth session manager dependencies.
type ManagerOptions struct {
	Store       CredentialStore
	OAuth       OAuthClient
	Clock       Clock
	Rand        io.Reader
	Loopback    LoopbackProvider
	OpenBrowser BrowserOpener
}

// Manager owns Google OIDC login, status, logout, and bearer-token resolution
// for CLI profiles, per docs/architecture/cli-auth-component-structure.md.
type Manager struct {
	store       CredentialStore
	oauth       OAuthClient
	clock       Clock
	rand        io.Reader
	loopback    LoopbackProvider
	openBrowser BrowserOpener
}

// LegacyMigrationOptions identifies the deprecated profile credential and its
// installation-scoped replacement. See the RC migration sequence in
// docs/architecture/cli-auth-component-structure.md.
type LegacyMigrationOptions struct {
	ProfileName     string
	LegacyEndpoint  string
	LegacyProvider  string
	LegacyIssuer    string
	LegacyClientID  string
	InstallationID  string
	ControlEndpoint string
}

// LegacyMigrationResult reports whether a secret was copied and whether the
// caller should direct the user to authenticate again after committing config.
type LegacyMigrationResult struct {
	Migrated        bool
	LoginRequired   bool
	DeletionWarning string
}

func NewManager(opts ManagerOptions) *Manager {
	store := opts.Store
	if store == nil {
		store = NewSystemCredentialStore()
	}
	oauth := opts.OAuth
	if oauth == nil {
		oauth = GoogleOAuthClient{}
	}
	clock := opts.Clock
	if clock == nil {
		clock = realClock{}
	}
	random := opts.Rand
	if random == nil {
		random = rand.Reader
	}
	loopback := opts.Loopback
	if loopback == nil {
		loopback = LoopbackServerProvider{}
	}
	openBrowser := opts.OpenBrowser
	if openBrowser == nil {
		openBrowser = OpenBrowser
	}
	return &Manager{store: store, oauth: oauth, clock: clock, rand: random, loopback: loopback, openBrowser: openBrowser}
}

// MigrateLegacyCredential performs the write/verify/commit/delete sequence for
// an explicit oidcSession-to-remoteSession update. The destination is never
// overwritten, and commit is not called after an unverified credential write.
func (m *Manager) MigrateLegacyCredential(ctx context.Context, opts LegacyMigrationOptions, commit func() error) (LegacyMigrationResult, error) {
	if commit == nil {
		return LegacyMigrationResult{}, fmt.Errorf("migration config commit is required")
	}
	destination := credentialKeyForTrust(opts.ProfileName, opts.LegacyEndpoint, opts.InstallationID, opts.ControlEndpoint, "", "", "")
	if _, exists, err := m.store.Get(ctx, destination); err != nil {
		return LegacyMigrationResult{}, fmt.Errorf("read destination credential: %w", err)
	} else if exists {
		return LegacyMigrationResult{}, commit()
	}

	if strings.TrimSpace(opts.LegacyEndpoint) == "" || strings.TrimSpace(opts.LegacyIssuer) == "" || strings.TrimSpace(opts.LegacyClientID) == "" || !sameOrigin(opts.LegacyEndpoint, opts.ControlEndpoint) {
		if err := commit(); err != nil {
			return LegacyMigrationResult{}, err
		}
		return LegacyMigrationResult{LoginRequired: true}, nil
	}
	legacy := credentialKeyForTrust(opts.ProfileName, opts.LegacyEndpoint, "", "", opts.LegacyProvider, opts.LegacyIssuer, opts.LegacyClientID)
	session, exists, err := m.store.Get(ctx, legacy)
	if err != nil {
		return LegacyMigrationResult{}, fmt.Errorf("read legacy credential: %w", err)
	}
	if !exists {
		if err := commit(); err != nil {
			return LegacyMigrationResult{}, err
		}
		return LegacyMigrationResult{LoginRequired: true}, nil
	}
	if err := m.store.Put(ctx, destination, session); err != nil {
		return LegacyMigrationResult{}, fmt.Errorf("write destination credential: %w", err)
	}
	verified, exists, err := m.store.Get(ctx, destination)
	if err != nil {
		return LegacyMigrationResult{}, fmt.Errorf("verify destination credential: %w", err)
	}
	if !exists || !reflect.DeepEqual(verified, session) {
		return LegacyMigrationResult{}, fmt.Errorf("verify destination credential: stored session differs from source")
	}
	if err := commit(); err != nil {
		return LegacyMigrationResult{}, err
	}
	result := LegacyMigrationResult{Migrated: true}
	if err := m.store.Delete(ctx, legacy); err != nil {
		result.DeletionWarning = fmt.Sprintf("legacy credential remains after successful migration: %v", err)
	}
	return result, nil
}

func sameOrigin(left, right string) bool {
	leftURL, leftErr := url.Parse(strings.TrimSpace(left))
	rightURL, rightErr := url.Parse(strings.TrimSpace(right))
	if leftErr != nil || rightErr != nil || leftURL.Hostname() == "" || rightURL.Hostname() == "" {
		return false
	}
	return strings.EqualFold(leftURL.Scheme, rightURL.Scheme) &&
		strings.EqualFold(leftURL.Hostname(), rightURL.Hostname()) &&
		effectivePort(leftURL) == effectivePort(rightURL)
}

func effectivePort(value *url.URL) string {
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

type LoginOptions struct {
	ProfileName             string
	Endpoint                string
	InstallationID          string
	ControlEndpoint         string
	Provider                string
	ClientID                string
	ClientSecret            string
	Issuer                  string
	LoginHint               string
	NoBrowser               bool
	AuthorizationURLReady   func(string) error
	AuthorizationEndpoint   string
	TokenEndpoint           string
	Scopes                  []string
	AuthorizationParameters map[string]string
	RequireCallbackIssuer   bool
}

type LoginResult struct {
	LoggedIn         bool      `json:"logged_in"`
	Provider         string    `json:"provider"`
	Email            string    `json:"email,omitempty"`
	Issuer           string    `json:"issuer"`
	Audience         string    `json:"audience"`
	Subject          string    `json:"subject,omitempty"`
	TokenExpiry      time.Time `json:"token_expiry,omitempty"`
	Profile          string    `json:"profile"`
	Endpoint         string    `json:"endpoint"`
	AuthorizationURL string    `json:"authorization_url,omitempty"`
	BearerToken      string    `json:"-"`
}

func (m *Manager) LoginGoogle(ctx context.Context, opts LoginOptions) (LoginResult, error) {
	clientID := strings.TrimSpace(opts.ClientID)
	if clientID == "" {
		return LoginResult{}, fmt.Errorf("OIDC public client ID is required for auth login")
	}
	issuer := defaultIssuer(opts.Issuer)
	pair, err := GeneratePKCEPair(m.rand)
	if err != nil {
		return LoginResult{}, err
	}
	state, err := generateOpaqueToken(m.rand)
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate OAuth state: %w", err)
	}
	nonce, err := generateOpaqueToken(m.rand)
	if err != nil {
		return LoginResult{}, fmt.Errorf("generate OIDC nonce: %w", err)
	}
	listener, err := m.loopback.Start(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	defer listener.Close()

	authURL, err := BuildGoogleAuthURL(AuthURLOptions{
		ClientID:                clientID,
		RedirectURI:             listener.RedirectURI(),
		State:                   state,
		Nonce:                   nonce,
		CodeChallenge:           pair.Challenge,
		LoginHint:               opts.LoginHint,
		AuthorizationEndpoint:   opts.AuthorizationEndpoint,
		Scopes:                  opts.Scopes,
		AuthorizationParameters: opts.AuthorizationParameters,
	})
	if err != nil {
		return LoginResult{}, err
	}
	if opts.NoBrowser {
		if opts.AuthorizationURLReady != nil {
			if err := opts.AuthorizationURLReady(authURL); err != nil {
				return LoginResult{}, err
			}
		}
	} else {
		if err := m.openBrowser(ctx, authURL); err != nil {
			return LoginResult{}, err
		}
	}
	callbackValues, err := listener.Wait(ctx)
	if err != nil {
		return LoginResult{}, err
	}
	callback, err := ValidateCallbackIssuer(callbackValues, state, issuer, opts.RequireCallbackIssuer)
	if err != nil {
		return LoginResult{}, err
	}
	resp, err := m.oauth.ExchangeCode(ctx, CodeExchangeRequest{
		ClientID:      clientID,
		ClientSecret:  opts.ClientSecret,
		Code:          callback.Code,
		CodeVerifier:  pair.Verifier,
		RedirectURI:   listener.RedirectURI(),
		TokenEndpoint: opts.TokenEndpoint,
	})
	if err != nil {
		return LoginResult{}, err
	}
	if strings.TrimSpace(resp.IDToken) == "" {
		return LoginResult{}, fmt.Errorf("OIDC token response missing id_token")
	}
	if strings.TrimSpace(resp.RefreshToken) == "" {
		return LoginResult{}, fmt.Errorf("OIDC token response missing refresh_token; retry auth login and check offline consent")
	}
	claims, err := DecodeIDTokenClaims(resp.IDToken)
	if err != nil {
		return LoginResult{}, err
	}
	if err := ValidateIDTokenClaims(claims, issuer, clientID, nonce, m.clock.Now()); err != nil {
		return LoginResult{}, err
	}
	now := m.clock.Now().UTC()
	session := Session{
		Provider:      defaultProvider(opts.Provider),
		Issuer:        issuer,
		ClientID:      clientID,
		Subject:       claims.Subject,
		Email:         claims.Email,
		Scopes:        loginScopes(opts.Scopes),
		RefreshToken:  strings.TrimSpace(resp.RefreshToken),
		CachedIDToken: strings.TrimSpace(resp.IDToken),
		IDTokenExpiry: claims.Expiry,
		CreatedAt:     now,
		UpdatedAt:     now,
		LoginNonce:    nonce,
	}
	key := credentialKeyForTrust(opts.ProfileName, opts.Endpoint, opts.InstallationID, opts.ControlEndpoint, session.Provider, issuer, clientID)
	if err := m.store.Put(ctx, key, session); err != nil {
		return LoginResult{}, err
	}
	result := loginResultFromSession(opts.ProfileName, opts.Endpoint, session)
	result.LoggedIn = true
	result.BearerToken = session.CachedIDToken
	return result, nil
}

func loginScopes(scopes []string) []string {
	if len(scopes) == 0 {
		scopes = googleOIDCScopes
	}
	return append([]string(nil), scopes...)
}

type StatusOptions struct {
	ProfileName     string
	Endpoint        string
	InstallationID  string
	ControlEndpoint string
	AuthMode        string
	ClientID        string
	Issuer          string
	TokenEnv        string
}

type StatusResult struct {
	LoggedIn    bool      `json:"logged_in"`
	Provider    string    `json:"provider"`
	Email       string    `json:"email,omitempty"`
	Issuer      string    `json:"issuer"`
	Audience    string    `json:"audience"`
	Subject     string    `json:"subject,omitempty"`
	TokenExpiry time.Time `json:"token_expiry,omitempty"`
	Profile     string    `json:"profile"`
	Endpoint    string    `json:"endpoint"`
	Override    string    `json:"override,omitempty"`
}

func (m *Manager) Status(ctx context.Context, opts StatusOptions) (StatusResult, error) {
	overrideEnv := configuredTokenEnv(opts.TokenEnv, opts.AuthMode)
	result := StatusResult{
		Profile:  strings.TrimSpace(opts.ProfileName),
		Endpoint: strings.TrimSpace(opts.Endpoint),
		Issuer:   defaultIssuer(opts.Issuer),
		Audience: strings.TrimSpace(opts.ClientID),
		Provider: "google",
	}
	if overrideEnv != "" && strings.TrimSpace(os.Getenv(overrideEnv)) != "" {
		result.LoggedIn = true
		result.Override = overrideEnv
		return result, nil
	}
	if !isSessionMode(opts.AuthMode) {
		return result, nil
	}
	session, ok, err := m.store.Get(ctx, credentialKeyForTrust(opts.ProfileName, opts.Endpoint, opts.InstallationID, opts.ControlEndpoint, "", opts.Issuer, opts.ClientID))
	if err != nil {
		return StatusResult{}, err
	}
	if !ok {
		return result, nil
	}
	return statusResultFromSession(opts.ProfileName, opts.Endpoint, session), nil
}

type LogoutOptions struct {
	ProfileName      string
	Endpoint         string
	ClientID         string
	Issuer           string
	InstallationID   string
	ControlEndpoint  string
	NoRevoke         bool
	ProviderResolver func(context.Context, string) (ProviderConfiguration, error)
}

type LogoutResult struct {
	Provider         string `json:"provider"`
	Profile          string `json:"profile"`
	Endpoint         string `json:"endpoint"`
	Deleted          bool   `json:"deleted"`
	Revoked          bool   `json:"revoked"`
	RevocationFailed string `json:"revocation_failed,omitempty"`
}

func (m *Manager) Logout(ctx context.Context, opts LogoutOptions) (LogoutResult, error) {
	key := credentialKeyForTrust(opts.ProfileName, opts.Endpoint, opts.InstallationID, opts.ControlEndpoint, "", opts.Issuer, opts.ClientID)
	session, ok, err := m.store.Get(ctx, key)
	if err != nil {
		return LogoutResult{}, err
	}
	result := LogoutResult{
		Provider: "google",
		Profile:  strings.TrimSpace(opts.ProfileName),
		Endpoint: strings.TrimSpace(opts.Endpoint),
	}
	if !ok {
		result.Deleted = true
		return result, nil
	}
	result.Provider = defaultProvider(session.Provider)
	if !opts.NoRevoke && strings.TrimSpace(session.RefreshToken) != "" {
		var revokeErr error
		revokeAttempted := false
		if opts.ProviderResolver != nil {
			provider, discoverErr := opts.ProviderResolver(ctx, defaultProvider(session.Provider))
			if discoverErr != nil {
				revokeErr = discoverErr
			} else if provider.ID != defaultProvider(session.Provider) || provider.Issuer != session.Issuer || provider.ClientID != session.ClientID {
				revokeErr = fmt.Errorf("stored auth session no longer matches provider configuration")
			} else if provider.RevocationEndpoint != "" {
				revokeAttempted = true
				if revoker, ok := m.oauth.(interface {
					RevokeAt(context.Context, string, string) error
				}); ok {
					revokeErr = revoker.RevokeAt(ctx, provider.RevocationEndpoint, session.RefreshToken)
				} else {
					revokeErr = m.oauth.Revoke(ctx, session.RefreshToken)
				}
			}
		} else {
			revokeAttempted = true
			revokeErr = m.oauth.Revoke(ctx, session.RefreshToken)
		}
		if revokeErr != nil {
			result.RevocationFailed = revokeErr.Error()
		} else if revokeAttempted {
			result.Revoked = true
		}
	}
	if err := m.store.Delete(ctx, key); err != nil {
		return result, err
	}
	result.Deleted = true
	return result, nil
}

type ResolveOptions struct {
	ProfileName      string
	Endpoint         string
	AuthMode         string
	ClientID         string
	ClientSecret     string
	Issuer           string
	TokenEnv         string
	StaticToken      string
	RefreshWindow    time.Duration
	InstallationID   string
	ControlEndpoint  string
	ProviderResolver func(context.Context, string) (ProviderConfiguration, error)
}

// ProviderConfiguration supplies the current public OIDC fields needed to
// refresh an existing session without persisting provider configuration.
type ProviderConfiguration struct {
	ID                 string
	Issuer             string
	ClientID           string
	TokenEndpoint      string
	RevocationEndpoint string
}

type TokenSource string

const (
	TokenSourceStoredRemoteSession TokenSource = "StoredRemoteSession"
	TokenSourceEnvironmentOverride TokenSource = "EnvironmentOverride"
	TokenSourceLegacyBearer        TokenSource = "LegacyBearer"
)

type ResolvedBearerToken struct {
	Token  string
	Source TokenSource
}

func (m *Manager) ResolveBearerToken(ctx context.Context, opts ResolveOptions) (ResolvedBearerToken, error) {
	tokenEnv := configuredTokenEnv(opts.TokenEnv, opts.AuthMode)
	if tokenEnv != "" {
		if value := strings.TrimSpace(os.Getenv(tokenEnv)); value != "" {
			return ResolvedBearerToken{Token: value, Source: TokenSourceEnvironmentOverride}, nil
		}
	}
	if !isSessionMode(opts.AuthMode) {
		if token := strings.TrimSpace(opts.StaticToken); token != "" {
			return ResolvedBearerToken{Token: token, Source: TokenSourceLegacyBearer}, nil
		}
		return ResolvedBearerToken{}, nil
	}
	clientID := strings.TrimSpace(opts.ClientID)
	if clientID == "" && !strings.EqualFold(strings.TrimSpace(opts.AuthMode), "remoteSession") {
		return ResolvedBearerToken{}, fmt.Errorf("auth.clientID is required for oidcSession profiles")
	}
	key := credentialKeyForTrust(opts.ProfileName, opts.Endpoint, opts.InstallationID, opts.ControlEndpoint, "", opts.Issuer, clientID)
	session, ok, err := m.store.Get(ctx, key)
	if err != nil {
		return ResolvedBearerToken{}, err
	}
	if !ok || strings.TrimSpace(session.RefreshToken) == "" {
		return ResolvedBearerToken{}, ErrLoginRequired
	}
	window := opts.RefreshWindow
	if window == 0 {
		window = defaultRefreshWindow
	}
	now := m.clock.Now().UTC()
	if strings.TrimSpace(session.CachedIDToken) != "" && !ShouldRefresh(session.IDTokenExpiry, now, window) {
		return ResolvedBearerToken{Token: session.CachedIDToken, Source: TokenSourceStoredRemoteSession}, nil
	}
	issuer := defaultIssuer(opts.Issuer)
	tokenEndpoint := ""
	provider := defaultProvider(session.Provider)
	if strings.EqualFold(strings.TrimSpace(opts.AuthMode), "remoteSession") {
		if opts.ProviderResolver == nil {
			return ResolvedBearerToken{}, fmt.Errorf("provider discovery is required to refresh remote session")
		}
		current, resolveErr := opts.ProviderResolver(ctx, provider)
		if resolveErr != nil {
			return ResolvedBearerToken{}, resolveErr
		}
		if current.ID != provider || current.Issuer != session.Issuer || current.ClientID != session.ClientID {
			return ResolvedBearerToken{}, fmt.Errorf("stored auth session no longer matches provider configuration; run `sqlrs auth login %s`", provider)
		}
		clientID, issuer, tokenEndpoint = current.ClientID, current.Issuer, current.TokenEndpoint
	}
	resp, err := m.oauth.Refresh(ctx, RefreshRequest{ClientID: clientID, ClientSecret: opts.ClientSecret, RefreshToken: session.RefreshToken, TokenEndpoint: tokenEndpoint})
	if err != nil {
		if isInvalidGrant(err) {
			return ResolvedBearerToken{}, m.deleteInvalidSession(ctx, key, err)
		}
		return ResolvedBearerToken{}, fmt.Errorf("refresh remote auth session: %w", err)
	}
	if strings.TrimSpace(resp.IDToken) == "" {
		return ResolvedBearerToken{}, errors.New("OIDC token response missing id_token; local session retained")
	}
	claims, err := DecodeIDTokenClaims(resp.IDToken)
	if err != nil {
		return ResolvedBearerToken{}, fmt.Errorf("validate refreshed ID token (local session retained): %w", err)
	}
	if err := ValidateIDTokenClaims(claims, issuer, clientID, "", now); err != nil {
		return ResolvedBearerToken{}, fmt.Errorf("validate refreshed ID token (local session retained): %w", err)
	}
	if claims.Subject != session.Subject {
		return ResolvedBearerToken{}, fmt.Errorf("validate refreshed ID token (local session retained): subject changed")
	}
	if claims.Nonce != "" {
		if session.LoginNonce == "" {
			return ResolvedBearerToken{}, fmt.Errorf("validate refreshed ID token (local session retained): nonce cannot be verified; run auth login again")
		}
		if claims.Nonce != session.LoginNonce {
			return ResolvedBearerToken{}, fmt.Errorf("validate refreshed ID token (local session retained): nonce mismatch")
		}
	}
	if strings.TrimSpace(resp.RefreshToken) != "" {
		session.RefreshToken = strings.TrimSpace(resp.RefreshToken)
	}
	session.Provider = provider
	session.Issuer = issuer
	session.ClientID = clientID
	session.Subject = claims.Subject
	session.Email = claims.Email
	session.CachedIDToken = strings.TrimSpace(resp.IDToken)
	session.IDTokenExpiry = claims.Expiry
	session.UpdatedAt = now
	if session.CreatedAt.IsZero() {
		session.CreatedAt = now
	}
	if err := m.store.Put(ctx, key, session); err != nil {
		return ResolvedBearerToken{}, err
	}
	return ResolvedBearerToken{Token: session.CachedIDToken, Source: TokenSourceStoredRemoteSession}, nil
}

func isInvalidGrant(err error) bool {
	var oauthErr *OAuthError
	if errors.As(err, &oauthErr) {
		return oauthErr.Code == "invalid_grant"
	}
	return false
}

func (m *Manager) deleteInvalidSession(ctx context.Context, key CredentialKey, cause error) error {
	if err := m.store.Delete(ctx, key); err != nil {
		return fmt.Errorf("%w: %v; failed to delete local auth session: %v", ErrLoginRequired, cause, err)
	}
	return fmt.Errorf("%w: %v", ErrLoginRequired, cause)
}

func credentialKey(profileName, endpoint, issuer, clientID string) CredentialKey {
	return credentialKeyForTrust(profileName, endpoint, "", "", "google", issuer, clientID)
}

func credentialKeyForTrust(profileName, endpoint, installationID, controlEndpoint, provider, issuer, clientID string) CredentialKey {
	return CredentialKey{
		ProfileName:     profileName,
		Endpoint:        endpoint,
		InstallationID:  installationID,
		ControlEndpoint: controlEndpoint,
		Provider:        provider,
		Issuer:          defaultIssuer(issuer),
		ClientID:        strings.TrimSpace(clientID),
	}
}

func configuredTokenEnv(tokenEnv string, authMode string) string {
	tokenEnv = strings.TrimSpace(tokenEnv)
	if tokenEnv == "" && isSessionMode(authMode) {
		return "SQLRS_TOKEN"
	}
	return tokenEnv
}

func isSessionMode(authMode string) bool {
	mode := strings.TrimSpace(authMode)
	return strings.EqualFold(mode, "oidcSession") || strings.EqualFold(mode, "remoteSession")
}

func loginResultFromSession(profileName, endpoint string, session Session) LoginResult {
	return LoginResult{
		LoggedIn:    true,
		Provider:    session.Provider,
		Email:       session.Email,
		Issuer:      session.Issuer,
		Audience:    session.ClientID,
		Subject:     maskSubject(session.Subject),
		TokenExpiry: session.IDTokenExpiry,
		Profile:     strings.TrimSpace(profileName),
		Endpoint:    strings.TrimSpace(endpoint),
	}
}

func statusResultFromSession(profileName, endpoint string, session Session) StatusResult {
	return StatusResult{
		LoggedIn:    true,
		Provider:    session.Provider,
		Email:       session.Email,
		Issuer:      session.Issuer,
		Audience:    session.ClientID,
		Subject:     maskSubject(session.Subject),
		TokenExpiry: session.IDTokenExpiry,
		Profile:     strings.TrimSpace(profileName),
		Endpoint:    strings.TrimSpace(endpoint),
	}
}

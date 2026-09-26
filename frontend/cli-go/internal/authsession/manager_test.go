package authsession

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestResolveBearerTokenUsesEnvOverrideBeforeStoredSession(t *testing.T) {
	store := newMemoryCredentialStore()
	key := testCredentialKey()
	if err := store.Put(context.Background(), key, Session{CachedIDToken: "stored", IDTokenExpiry: time.Now().Add(time.Hour), RefreshToken: "refresh"}); err != nil {
		t.Fatalf("put session: %v", err)
	}
	t.Setenv("SQLRS_TOKEN", "env-token")

	manager := NewManager(ManagerOptions{Store: store, OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: time.Now()}})
	got, err := manager.ResolveBearerToken(context.Background(), ResolveOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		AuthMode:    "oidcSession",
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
		TokenEnv:    "SQLRS_TOKEN",
	})
	if err != nil {
		t.Fatalf("ResolveBearerToken: %v", err)
	}
	if got.Token != "env-token" || got.Source != TokenSourceEnvironmentOverride {
		t.Fatalf("resolved = %+v, want env token", got)
	}
}

func TestResolveBearerTokenUsesFreshCachedIDToken(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	store := newMemoryCredentialStore()
	key := testCredentialKey()
	session := Session{
		Provider:      "google",
		Issuer:        key.Issuer,
		ClientID:      key.ClientID,
		Email:         "alice@example.com",
		RefreshToken:  "refresh",
		CachedIDToken: "cached-id-token",
		IDTokenExpiry: now.Add(time.Hour),
	}
	if err := store.Put(context.Background(), key, session); err != nil {
		t.Fatalf("put session: %v", err)
	}

	manager := NewManager(ManagerOptions{Store: store, OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: now}})
	got, err := manager.ResolveBearerToken(context.Background(), testResolveOptions())
	if err != nil {
		t.Fatalf("ResolveBearerToken: %v", err)
	}
	if got.Token != "cached-id-token" || got.Source != TokenSourceStoredRemoteSession {
		t.Fatalf("resolved = %+v, want cached token", got)
	}
}

func TestResolveBearerTokenRefreshesExpiringSessionAndStoresIDToken(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	store := newMemoryCredentialStore()
	key := testCredentialKey()
	if err := store.Put(context.Background(), key, Session{
		Provider:      "google",
		Issuer:        key.Issuer,
		ClientID:      key.ClientID,
		Subject:       "subject-1",
		RefreshToken:  "refresh-old",
		CachedIDToken: "old-id-token",
		IDTokenExpiry: now.Add(time.Minute),
	}); err != nil {
		t.Fatalf("put session: %v", err)
	}
	newIDToken := testIDToken(t, map[string]any{
		"iss":   key.Issuer,
		"aud":   key.ClientID,
		"sub":   "subject-1",
		"email": "alice@example.com",
		"exp":   now.Add(time.Hour).Unix(),
	})
	oauth := &fakeOAuthClient{refreshResponse: TokenResponse{IDToken: newIDToken}}

	manager := NewManager(ManagerOptions{Store: store, OAuth: oauth, Clock: fixedClock{now: now}})
	got, err := manager.ResolveBearerToken(context.Background(), testResolveOptions())
	if err != nil {
		t.Fatalf("ResolveBearerToken: %v", err)
	}
	if got.Token != newIDToken || got.Source != TokenSourceStoredRemoteSession {
		t.Fatalf("resolved = %+v, want refreshed token", got)
	}
	if oauth.refreshToken != "refresh-old" {
		t.Fatalf("refresh token = %q, want refresh-old", oauth.refreshToken)
	}
	if oauth.refreshClientSecret != "client-secret" {
		t.Fatalf("refresh client secret = %q, want client-secret", oauth.refreshClientSecret)
	}
	stored, ok, err := store.Get(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("get stored session: ok=%v err=%v", ok, err)
	}
	if stored.CachedIDToken != newIDToken || !stored.IDTokenExpiry.Equal(now.Add(time.Hour)) {
		t.Fatalf("stored session = %+v, want refreshed token metadata", stored)
	}
}

func TestResolveBearerTokenDeletesSessionOnRefreshFailure(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	store := newMemoryCredentialStore()
	key := testCredentialKey()
	if err := store.Put(context.Background(), key, Session{
		Provider:      "google",
		Issuer:        key.Issuer,
		ClientID:      key.ClientID,
		RefreshToken:  "refresh-old",
		IDTokenExpiry: now.Add(-time.Minute),
	}); err != nil {
		t.Fatalf("put session: %v", err)
	}
	manager := NewManager(ManagerOptions{
		Store: store,
		OAuth: &fakeOAuthClient{refreshErr: &OAuthError{Code: "invalid_grant", Status: 400}},
		Clock: fixedClock{now: now},
	})

	_, err := manager.ResolveBearerToken(context.Background(), testResolveOptions())
	if err == nil || !strings.Contains(err.Error(), "auth login google") {
		t.Fatalf("expected login guidance, got %v", err)
	}
	if _, ok, err := store.Get(context.Background(), key); err != nil || ok {
		t.Fatalf("session should be deleted after refresh failure: ok=%v err=%v", ok, err)
	}
}

func TestLoginGoogleStoresRefreshTokenAndSafeMetadata(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	entropy := make([]byte, pkceEntropyBytes*3)
	for i := range entropy {
		entropy[i] = byte(i)
	}
	state := base64URL(entropy[pkceEntropyBytes : pkceEntropyBytes*2])
	nonce := base64URL(entropy[pkceEntropyBytes*2 : pkceEntropyBytes*3])
	key := testCredentialKey()
	idToken := testIDToken(t, map[string]any{
		"iss":   key.Issuer,
		"aud":   key.ClientID,
		"sub":   "subject-1",
		"email": "alice@example.com",
		"exp":   now.Add(time.Hour).Unix(),
		"nonce": nonce,
	})
	store := newMemoryCredentialStore()
	oauth := &fakeOAuthClient{exchangeResponse: TokenResponse{IDToken: idToken, RefreshToken: "refresh-token"}}
	loopback := fakeLoopbackProvider{session: &fakeLoopbackSession{
		redirectURI: "http://127.0.0.1:49152",
		callback:    url.Values{"state": {state}, "code": {"code-1"}},
	}}
	var readyURL string
	loopback.session.onWait = func() {
		if readyURL == "" {
			t.Fatalf("authorization URL was not reported before waiting for callback")
		}
	}
	manager := NewManager(ManagerOptions{
		Store:    store,
		OAuth:    oauth,
		Clock:    fixedClock{now: now},
		Rand:     bytes.NewReader(entropy),
		Loopback: loopback,
	})

	result, err := manager.LoginGoogle(context.Background(), LoginOptions{
		ProfileName:  key.ProfileName,
		Endpoint:     key.Endpoint,
		ClientID:     key.ClientID,
		ClientSecret: "client-secret",
		Issuer:       key.Issuer,
		Scopes:       []string{"openid", "email"},
		NoBrowser:    true,
		AuthorizationURLReady: func(authURL string) error {
			readyURL = authURL
			return nil
		},
	})
	if err != nil {
		t.Fatalf("LoginGoogle: %v", err)
	}
	if result.Email != "alice@example.com" || result.Provider != "google" {
		t.Fatalf("result = %+v, want safe metadata", result)
	}
	if result.AuthorizationURL != "" || readyURL == "" {
		t.Fatalf("authorization URL must be transient only: result=%q reported=%q", result.AuthorizationURL, readyURL)
	}
	if oauth.exchangeCode != "code-1" {
		t.Fatalf("exchange code = %q, want code-1", oauth.exchangeCode)
	}
	if oauth.exchangeClientSecret != "client-secret" {
		t.Fatalf("exchange client secret = %q, want client-secret", oauth.exchangeClientSecret)
	}
	stored, ok, err := store.Get(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("get session: ok=%v err=%v", ok, err)
	}
	if stored.RefreshToken != "refresh-token" || stored.CachedIDToken != idToken {
		t.Fatalf("stored token metadata = %+v", stored)
	}
	if strings.Join(stored.Scopes, " ") != "openid email" {
		t.Fatalf("stored scopes = %+v", stored.Scopes)
	}
}

func TestLoginGoogleRequiresRefreshToken(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	entropy := make([]byte, pkceEntropyBytes*3)
	state := base64URL(entropy[pkceEntropyBytes : pkceEntropyBytes*2])
	nonce := base64URL(entropy[pkceEntropyBytes*2 : pkceEntropyBytes*3])
	key := testCredentialKey()
	idToken := testIDToken(t, map[string]any{
		"iss":   key.Issuer,
		"aud":   key.ClientID,
		"sub":   "subject-1",
		"exp":   now.Add(time.Hour).Unix(),
		"nonce": nonce,
	})
	manager := NewManager(ManagerOptions{
		Store: newMemoryCredentialStore(),
		OAuth: &fakeOAuthClient{exchangeResponse: TokenResponse{IDToken: idToken}},
		Clock: fixedClock{now: now},
		Rand:  bytes.NewReader(entropy),
		Loopback: fakeLoopbackProvider{session: &fakeLoopbackSession{
			redirectURI: "http://127.0.0.1:49152",
			callback:    url.Values{"state": {state}, "code": {"code-1"}},
		}},
	})

	_, err := manager.LoginGoogle(context.Background(), LoginOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
		NoBrowser:   true,
	})
	if err == nil || !strings.Contains(err.Error(), "refresh_token") {
		t.Fatalf("expected refresh_token error, got %v", err)
	}
}

func TestResolveRemoteSessionDiscoversProviderAndUsesStableTrustKey(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	key := CredentialKey{ProfileName: "remote", Endpoint: "https://api.example.test/nsu", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test"}
	store := newMemoryCredentialStore()
	if err := store.Put(context.Background(), key, Session{Provider: "google", Issuer: "https://issuer.example.test", ClientID: "public-client", Subject: "subject-1", RefreshToken: "refresh", IDTokenExpiry: now.Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	idToken := testIDToken(t, map[string]any{"iss": "https://issuer.example.test", "aud": "public-client", "sub": "subject-1", "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()})
	oauth := &fakeOAuthClient{refreshResponse: TokenResponse{IDToken: idToken}}
	manager := NewManager(ManagerOptions{Store: store, OAuth: oauth, Clock: fixedClock{now: now}})
	resolved, err := manager.ResolveBearerToken(context.Background(), ResolveOptions{
		ProfileName: "remote", Endpoint: "https://api.example.test/nsu", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test", AuthMode: "remoteSession",
		ProviderResolver: func(_ context.Context, provider string) (ProviderConfiguration, error) {
			if provider != "google" {
				t.Fatalf("provider = %q", provider)
			}
			return ProviderConfiguration{ID: "google", Issuer: "https://issuer.example.test", ClientID: "public-client", TokenEndpoint: "https://issuer.example.test/token"}, nil
		},
	})
	if err != nil || resolved.Token != idToken || resolved.Source != TokenSourceStoredRemoteSession {
		t.Fatalf("resolved=%+v err=%v", resolved, err)
	}
	if oauth.refreshEndpoint != "https://issuer.example.test/token" || oauth.refreshClientSecret != "" {
		t.Fatalf("refresh endpoint=%q secret=%q", oauth.refreshEndpoint, oauth.refreshClientSecret)
	}
	key.Endpoint = "https://api.example.test/other"
	if _, ok, _ := store.Get(context.Background(), key); !ok {
		t.Fatalf("mutable request endpoint should not change remote-session key")
	}
}

func TestMigrateLegacyCredentialWritesVerifiesCommitsAndDeletes(t *testing.T) {
	ctx := context.Background()
	store := newMemoryCredentialStore()
	legacyKey := CredentialKey{ProfileName: "remote", Endpoint: "https://api.example.test/nsu", Provider: "google", Issuer: "https://issuer.example.test", ClientID: "public-client"}
	session := Session{Provider: "google", Issuer: legacyKey.Issuer, ClientID: legacyKey.ClientID, Subject: "subject-1", RefreshToken: "refresh"}
	if err := store.Put(ctx, legacyKey, session); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ManagerOptions{Store: store})
	committed := false
	result, err := manager.MigrateLegacyCredential(ctx, LegacyMigrationOptions{
		ProfileName: "remote", LegacyEndpoint: legacyKey.Endpoint, LegacyProvider: "google", LegacyIssuer: legacyKey.Issuer, LegacyClientID: legacyKey.ClientID,
		InstallationID: "installation-1", ControlEndpoint: "https://api.example.test",
	}, func() error {
		destination := CredentialKey{ProfileName: "remote", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test"}
		if _, ok, getErr := store.Get(ctx, destination); getErr != nil || !ok {
			t.Fatalf("destination must be verified before commit: ok=%v err=%v", ok, getErr)
		}
		if _, ok, getErr := store.Get(ctx, legacyKey); getErr != nil || !ok {
			t.Fatalf("legacy must remain until after commit: ok=%v err=%v", ok, getErr)
		}
		committed = true
		return nil
	})
	if err != nil || !result.Migrated || !committed {
		t.Fatalf("result=%+v committed=%v err=%v", result, committed, err)
	}
	if _, ok, _ := store.Get(ctx, legacyKey); ok {
		t.Fatal("legacy credential was not deleted after commit")
	}
}

func TestMigrateLegacyCredentialDoesNotOverwriteDestination(t *testing.T) {
	ctx := context.Background()
	store := newMemoryCredentialStore()
	legacyKey := CredentialKey{ProfileName: "remote", Endpoint: "https://api.example.test", Provider: "google", Issuer: "https://issuer.example.test", ClientID: "public-client"}
	destination := CredentialKey{ProfileName: "remote", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test"}
	_ = store.Put(ctx, legacyKey, Session{Subject: "legacy", RefreshToken: "legacy-refresh"})
	_ = store.Put(ctx, destination, Session{Subject: "current", RefreshToken: "current-refresh"})
	manager := NewManager(ManagerOptions{Store: store})
	result, err := manager.MigrateLegacyCredential(ctx, LegacyMigrationOptions{
		ProfileName: "remote", LegacyEndpoint: legacyKey.Endpoint, LegacyProvider: "google", LegacyIssuer: legacyKey.Issuer, LegacyClientID: legacyKey.ClientID,
		InstallationID: destination.InstallationID, ControlEndpoint: destination.ControlEndpoint,
	}, func() error { return nil })
	if err != nil || result.Migrated {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	got, _, _ := store.Get(ctx, destination)
	if got.Subject != "current" || got.RefreshToken != "current-refresh" {
		t.Fatalf("destination overwritten: %+v", got)
	}
	if _, ok, _ := store.Get(ctx, legacyKey); !ok {
		t.Fatal("legacy credential should remain when destination was authoritative")
	}
}

func TestMigrateLegacyCredentialRequiresLoginWhenNotDerivable(t *testing.T) {
	manager := NewManager(ManagerOptions{Store: newMemoryCredentialStore()})
	committed := false
	result, err := manager.MigrateLegacyCredential(context.Background(), LegacyMigrationOptions{
		ProfileName: "remote", LegacyEndpoint: "https://other.example.test", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test",
	}, func() error { committed = true; return nil })
	if err != nil || !result.LoginRequired || !committed {
		t.Fatalf("result=%+v committed=%v err=%v", result, committed, err)
	}
}

func TestMigrateLegacyCredentialFailureOrdering(t *testing.T) {
	ctx := context.Background()
	legacyKey := CredentialKey{ProfileName: "remote", Endpoint: "https://api.example.test", Provider: "google", Issuer: "https://issuer.example.test", ClientID: "public-client"}
	destination := CredentialKey{ProfileName: "remote", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test"}
	session := Session{Provider: "google", Issuer: legacyKey.Issuer, ClientID: legacyKey.ClientID, Subject: "subject-1", RefreshToken: "refresh"}
	options := LegacyMigrationOptions{
		ProfileName: "remote", LegacyEndpoint: legacyKey.Endpoint, LegacyProvider: "google", LegacyIssuer: legacyKey.Issuer, LegacyClientID: legacyKey.ClientID,
		InstallationID: destination.InstallationID, ControlEndpoint: destination.ControlEndpoint,
	}
	cases := []struct {
		name          string
		configure     func(*migrationCredentialStore)
		commitErr     error
		wantCommitted bool
		wantWarning   bool
	}{
		{name: "destination put", configure: func(s *migrationCredentialStore) { s.putErr = errors.New("put failed") }},
		{name: "destination verify", configure: func(s *migrationCredentialStore) { s.corruptDestination = true }},
		{name: "config commit", commitErr: errors.New("commit failed"), wantCommitted: true},
		{name: "legacy delete", configure: func(s *migrationCredentialStore) { s.deleteErr = errors.New("delete failed") }, wantCommitted: true, wantWarning: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := &migrationCredentialStore{memoryCredentialStore: newMemoryCredentialStore()}
			if err := store.memoryCredentialStore.Put(ctx, legacyKey, session); err != nil {
				t.Fatal(err)
			}
			if tc.configure != nil {
				tc.configure(store)
			}
			committed := false
			result, err := NewManager(ManagerOptions{Store: store}).MigrateLegacyCredential(ctx, options, func() error {
				committed = true
				return tc.commitErr
			})
			if tc.wantWarning {
				if err != nil || result.DeletionWarning == "" {
					t.Fatalf("result=%+v err=%v", result, err)
				}
			} else if err == nil {
				t.Fatal("expected migration error")
			}
			if committed != tc.wantCommitted {
				t.Fatalf("committed=%v want %v", committed, tc.wantCommitted)
			}
			if _, ok, _ := store.memoryCredentialStore.Get(ctx, legacyKey); !ok {
				t.Fatal("failure path removed legacy credential")
			}
		})
	}
}

func TestMigrateLegacyCredentialReadAndCommitFailures(t *testing.T) {
	ctx := context.Background()
	legacyKey := CredentialKey{ProfileName: "remote", Endpoint: "https://api.example.test", Provider: "google", Issuer: "https://issuer.example.test", ClientID: "public-client"}
	destination := CredentialKey{ProfileName: "remote", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test"}
	options := LegacyMigrationOptions{
		ProfileName: "remote", LegacyEndpoint: legacyKey.Endpoint, LegacyProvider: "google", LegacyIssuer: legacyKey.Issuer, LegacyClientID: legacyKey.ClientID,
		InstallationID: destination.InstallationID, ControlEndpoint: destination.ControlEndpoint,
	}
	if _, err := NewManager(ManagerOptions{Store: newMemoryCredentialStore()}).MigrateLegacyCredential(ctx, options, nil); err == nil || !strings.Contains(err.Error(), "commit") {
		t.Fatalf("nil commit error = %v", err)
	}

	t.Run("destination read", func(t *testing.T) {
		store := &migrationCredentialStore{memoryCredentialStore: newMemoryCredentialStore(), getErrors: map[int]error{1: errors.New("destination read failed")}}
		if _, err := NewManager(ManagerOptions{Store: store}).MigrateLegacyCredential(ctx, options, func() error { t.Fatal("commit called"); return nil }); err == nil || !strings.Contains(err.Error(), "read destination") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("authoritative destination commit", func(t *testing.T) {
		store := newMemoryCredentialStore()
		_ = store.Put(ctx, destination, Session{RefreshToken: "current"})
		wantErr := errors.New("commit failed")
		if _, err := NewManager(ManagerOptions{Store: store}).MigrateLegacyCredential(ctx, options, func() error { return wantErr }); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("insufficient metadata commit", func(t *testing.T) {
		wantErr := errors.New("commit failed")
		incomplete := options
		incomplete.LegacyIssuer = ""
		if _, err := NewManager(ManagerOptions{Store: newMemoryCredentialStore()}).MigrateLegacyCredential(ctx, incomplete, func() error { return wantErr }); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("legacy read", func(t *testing.T) {
		store := &migrationCredentialStore{memoryCredentialStore: newMemoryCredentialStore(), getErrors: map[int]error{2: errors.New("legacy read failed")}}
		if _, err := NewManager(ManagerOptions{Store: store}).MigrateLegacyCredential(ctx, options, func() error { t.Fatal("commit called"); return nil }); err == nil || !strings.Contains(err.Error(), "read legacy") {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("missing legacy commit", func(t *testing.T) {
		wantErr := errors.New("commit failed")
		if _, err := NewManager(ManagerOptions{Store: newMemoryCredentialStore()}).MigrateLegacyCredential(ctx, options, func() error { return wantErr }); !errors.Is(err, wantErr) {
			t.Fatalf("error = %v", err)
		}
	})
	t.Run("destination verification read", func(t *testing.T) {
		store := &migrationCredentialStore{memoryCredentialStore: newMemoryCredentialStore(), getErrors: map[int]error{3: errors.New("verify read failed")}}
		_ = store.memoryCredentialStore.Put(ctx, legacyKey, Session{Subject: "subject-1", RefreshToken: "refresh"})
		if _, err := NewManager(ManagerOptions{Store: store}).MigrateLegacyCredential(ctx, options, func() error { t.Fatal("commit called"); return nil }); err == nil || !strings.Contains(err.Error(), "verify destination") {
			t.Fatalf("error = %v", err)
		}
	})
}

func TestLegacyMigrationOriginAndPortRules(t *testing.T) {
	for _, tc := range []struct {
		left, right string
		want        bool
	}{
		{left: "https://API.example.test/nsu", right: "https://api.example.test:443", want: true},
		{left: "http://127.0.0.1/nsu", right: "http://127.0.0.1:80", want: true},
		{left: "https://api.example.test", right: "https://other.example.test", want: false},
		{left: "://bad", right: "https://api.example.test", want: false},
	} {
		if got := sameOrigin(tc.left, tc.right); got != tc.want {
			t.Fatalf("sameOrigin(%q, %q) = %v", tc.left, tc.right, got)
		}
	}
	custom, err := url.Parse("custom://host")
	if err != nil || effectivePort(custom) != "" {
		t.Fatalf("custom effective port = %q, %v", effectivePort(custom), err)
	}
}

type migrationCredentialStore struct {
	*memoryCredentialStore
	putErr             error
	deleteErr          error
	corruptDestination bool
	getCalls           int
	getErrors          map[int]error
}

func (s *migrationCredentialStore) Get(ctx context.Context, key CredentialKey) (Session, bool, error) {
	s.getCalls++
	if err := s.getErrors[s.getCalls]; err != nil {
		return Session{}, false, err
	}
	session, ok, err := s.memoryCredentialStore.Get(ctx, key)
	if ok && key.InstallationID != "" && s.corruptDestination {
		session.Subject = "corrupted"
	}
	return session, ok, err
}

func (s *migrationCredentialStore) Put(ctx context.Context, key CredentialKey, session Session) error {
	if s.putErr != nil {
		return s.putErr
	}
	return s.memoryCredentialStore.Put(ctx, key, session)
}

func (s *migrationCredentialStore) Delete(ctx context.Context, key CredentialKey) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.memoryCredentialStore.Delete(ctx, key)
}

func TestResolveRemoteSessionRefreshKeepsIdentityAndNonceBound(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	key := CredentialKey{ProfileName: "remote", Endpoint: "https://api.example.test/nsu", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test"}
	base := Session{
		Provider: "google", Issuer: "https://issuer.example.test", ClientID: "public-client",
		Subject: "subject-1", RefreshToken: "refresh", IDTokenExpiry: now.Add(-time.Minute), LoginNonce: "login-nonce",
	}
	provider := func(context.Context, string) (ProviderConfiguration, error) {
		return ProviderConfiguration{ID: "google", Issuer: base.Issuer, ClientID: base.ClientID, TokenEndpoint: "https://issuer.example.test/token"}, nil
	}
	cases := []struct {
		name    string
		subject string
		nonce   string
		wantErr string
	}{
		{name: "stable subject without nonce", subject: base.Subject},
		{name: "stable subject with matching nonce", subject: base.Subject, nonce: base.LoginNonce},
		{name: "changed subject", subject: "subject-2", wantErr: "subject changed"},
		{name: "mismatching nonce", subject: base.Subject, nonce: "other-nonce", wantErr: "nonce mismatch"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMemoryCredentialStore()
			if err := store.Put(context.Background(), key, base); err != nil {
				t.Fatal(err)
			}
			payload := map[string]any{"iss": base.Issuer, "aud": base.ClientID, "sub": tc.subject, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix()}
			if tc.nonce != "" {
				payload["nonce"] = tc.nonce
			}
			idToken := testIDToken(t, payload)
			manager := NewManager(ManagerOptions{Store: store, OAuth: &fakeOAuthClient{refreshResponse: TokenResponse{IDToken: idToken}}, Clock: fixedClock{now: now}})
			_, err := manager.ResolveBearerToken(context.Background(), ResolveOptions{
				ProfileName: key.ProfileName, Endpoint: key.Endpoint, InstallationID: key.InstallationID, ControlEndpoint: key.ControlEndpoint,
				AuthMode: "remoteSession", ProviderResolver: provider,
			})
			if tc.wantErr == "" && err != nil {
				t.Fatalf("ResolveBearerToken: %v", err)
			}
			if tc.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tc.wantErr)) {
				t.Fatalf("error = %v, want %q", err, tc.wantErr)
			}
			stored, ok, getErr := store.Get(context.Background(), key)
			if getErr != nil || !ok {
				t.Fatalf("Get: ok=%v err=%v", ok, getErr)
			}
			if tc.wantErr != "" && stored.Subject != base.Subject {
				t.Fatalf("rejected refresh changed stored subject to %q", stored.Subject)
			}
		})
	}
}

func TestResolveRemoteSessionRefreshRejectsNonceWithoutStoredBinding(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	key := CredentialKey{ProfileName: "remote", Endpoint: "https://api.example.test", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test"}
	session := Session{Provider: "google", Issuer: "https://issuer.example.test", ClientID: "public-client", Subject: "subject-1", RefreshToken: "refresh", IDTokenExpiry: now.Add(-time.Minute)}
	store := newMemoryCredentialStore()
	_ = store.Put(context.Background(), key, session)
	idToken := testIDToken(t, map[string]any{"iss": session.Issuer, "aud": session.ClientID, "sub": session.Subject, "iat": now.Unix(), "exp": now.Add(time.Hour).Unix(), "nonce": "unbound"})
	manager := NewManager(ManagerOptions{Store: store, OAuth: &fakeOAuthClient{refreshResponse: TokenResponse{IDToken: idToken}}, Clock: fixedClock{now: now}})
	_, err := manager.ResolveBearerToken(context.Background(), ResolveOptions{
		ProfileName: key.ProfileName, Endpoint: key.Endpoint, InstallationID: key.InstallationID, ControlEndpoint: key.ControlEndpoint, AuthMode: "remoteSession",
		ProviderResolver: func(context.Context, string) (ProviderConfiguration, error) {
			return ProviderConfiguration{ID: "google", Issuer: session.Issuer, ClientID: session.ClientID, TokenEndpoint: "https://issuer.example.test/token"}, nil
		},
	})
	if err == nil || !strings.Contains(err.Error(), "nonce cannot be verified") {
		t.Fatalf("error = %v", err)
	}
}

func TestRemoteSessionProviderMismatchRetainsCredential(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	now := time.Date(2026, 9, 25, 12, 0, 0, 0, time.UTC)
	key := CredentialKey{ProfileName: "remote", Endpoint: "https://api.example.test", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test"}
	store := newMemoryCredentialStore()
	_ = store.Put(context.Background(), key, Session{Provider: "google", Issuer: "https://issuer.example.test", ClientID: "public-client", RefreshToken: "refresh", IDTokenExpiry: now.Add(-time.Minute)})
	manager := NewManager(ManagerOptions{Store: store, OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: now}})
	_, err := manager.ResolveBearerToken(context.Background(), ResolveOptions{ProfileName: "remote", Endpoint: key.Endpoint, InstallationID: key.InstallationID, ControlEndpoint: key.ControlEndpoint, AuthMode: "remoteSession", ProviderResolver: func(context.Context, string) (ProviderConfiguration, error) {
		return ProviderConfiguration{ID: "google", Issuer: "https://changed.example.test", ClientID: "public-client"}, nil
	}})
	if err == nil || !strings.Contains(err.Error(), "no longer matches") {
		t.Fatalf("error = %v", err)
	}
	if _, ok, _ := store.Get(context.Background(), key); !ok {
		t.Fatalf("provider mismatch must retain local credential")
	}
}

func TestStatusReportsStoredSessionAndEnvOverride(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	key := testCredentialKey()
	store := newMemoryCredentialStore()
	if err := store.Put(context.Background(), key, Session{
		Provider:      "google",
		Issuer:        key.Issuer,
		ClientID:      key.ClientID,
		Email:         "alice@example.com",
		Subject:       "subject-1",
		IDTokenExpiry: now.Add(time.Hour),
		RefreshToken:  "refresh-token",
	}); err != nil {
		t.Fatalf("put session: %v", err)
	}
	manager := NewManager(ManagerOptions{Store: store, OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: now}})

	status, err := manager.Status(context.Background(), StatusOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		AuthMode:    "oidcSession",
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
		TokenEnv:    "SQLRS_TOKEN",
	})
	if err != nil {
		t.Fatalf("Status: %v", err)
	}
	if !status.LoggedIn || status.Email != "alice@example.com" || status.Override != "" {
		t.Fatalf("status = %+v, want stored logged-in session", status)
	}

	t.Setenv("SQLRS_TOKEN", "debug-token")
	status, err = manager.Status(context.Background(), StatusOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		AuthMode:    "oidcSession",
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
		TokenEnv:    "SQLRS_TOKEN",
	})
	if err != nil {
		t.Fatalf("Status with env: %v", err)
	}
	if !status.LoggedIn || status.Override != "SQLRS_TOKEN" {
		t.Fatalf("status = %+v, want SQLRS_TOKEN override", status)
	}
}

func TestLogoutRevokesThenDeletesStoredSession(t *testing.T) {
	key := testCredentialKey()
	store := newMemoryCredentialStore()
	if err := store.Put(context.Background(), key, Session{Provider: "google", RefreshToken: "refresh-token"}); err != nil {
		t.Fatalf("put session: %v", err)
	}
	oauth := &fakeOAuthClient{}
	manager := NewManager(ManagerOptions{Store: store, OAuth: oauth, Clock: fixedClock{now: time.Now()}})

	result, err := manager.Logout(context.Background(), LogoutOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
	})
	if err != nil {
		t.Fatalf("Logout: %v", err)
	}
	if !result.Revoked {
		t.Fatalf("result = %+v, want revoked", result)
	}
	if oauth.revokedToken != "refresh-token" {
		t.Fatalf("revoked token = %q, want refresh-token", oauth.revokedToken)
	}
	if _, ok, err := store.Get(context.Background(), key); err != nil || ok {
		t.Fatalf("session should be deleted: ok=%v err=%v", ok, err)
	}
}

func testResolveOptions() ResolveOptions {
	key := testCredentialKey()
	return ResolveOptions{
		ProfileName:  key.ProfileName,
		Endpoint:     key.Endpoint,
		AuthMode:     "oidcSession",
		ClientID:     key.ClientID,
		ClientSecret: "client-secret",
		Issuer:       key.Issuer,
		TokenEnv:     "SQLRS_TOKEN",
	}
}

func testCredentialKey() CredentialKey {
	return CredentialKey{
		ProfileName: "remote-dev",
		Endpoint:    "https://sqlrs.example.org",
		Provider:    "google",
		Issuer:      "https://accounts.google.com",
		ClientID:    "client-id",
	}
}

type fakeOAuthClient struct {
	exchangeResponse     TokenResponse
	exchangeErr          error
	exchangeCode         string
	exchangeClientSecret string

	refreshResponse     TokenResponse
	refreshErr          error
	refreshToken        string
	refreshClientSecret string
	refreshEndpoint     string

	revokeErr    error
	revokedToken string
}

func (f *fakeOAuthClient) ExchangeCode(_ context.Context, req CodeExchangeRequest) (TokenResponse, error) {
	f.exchangeCode = req.Code
	f.exchangeClientSecret = req.ClientSecret
	if f.exchangeErr != nil {
		return TokenResponse{}, f.exchangeErr
	}
	return f.exchangeResponse, nil
}

func (f *fakeOAuthClient) Refresh(_ context.Context, req RefreshRequest) (TokenResponse, error) {
	f.refreshToken = req.RefreshToken
	f.refreshClientSecret = req.ClientSecret
	f.refreshEndpoint = req.TokenEndpoint
	if f.refreshErr != nil {
		return TokenResponse{}, f.refreshErr
	}
	return f.refreshResponse, nil
}

func (f *fakeOAuthClient) Revoke(_ context.Context, token string) error {
	f.revokedToken = token
	return f.revokeErr
}

type fixedClock struct {
	now time.Time
}

func (c fixedClock) Now() time.Time {
	return c.now
}

type fakeLoopbackProvider struct {
	session *fakeLoopbackSession
	err     error
}

func (p fakeLoopbackProvider) Start(context.Context) (LoopbackSession, error) {
	if p.err != nil {
		return nil, p.err
	}
	return p.session, nil
}

type fakeLoopbackSession struct {
	redirectURI string
	callback    url.Values
	closed      bool
	onWait      func()
}

func (s *fakeLoopbackSession) RedirectURI() string {
	return s.redirectURI
}

func (s *fakeLoopbackSession) Wait(context.Context) (url.Values, error) {
	if s.onWait != nil {
		s.onWait()
	}
	return s.callback, nil
}

func (s *fakeLoopbackSession) Close() error {
	s.closed = true
	return nil
}

func base64URL(data []byte) string {
	return strings.TrimRight(base64Raw(data), "=")
}

func base64Raw(data []byte) string {
	return strings.TrimRight(base64.URLEncoding.EncodeToString(data), "=")
}

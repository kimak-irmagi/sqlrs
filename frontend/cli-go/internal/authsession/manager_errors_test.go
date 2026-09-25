package authsession

import (
	"bytes"
	"context"
	"errors"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestLoginGoogleValidatesRequiredClientID(t *testing.T) {
	manager := NewManager(ManagerOptions{Store: newMemoryCredentialStore(), OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: time.Now()}})
	if _, err := manager.LoginGoogle(context.Background(), LoginOptions{}); err == nil || !strings.Contains(err.Error(), "client ID") {
		t.Fatalf("expected clientID error, got %v", err)
	}
}

func TestLoginGoogleReportsEntropyErrors(t *testing.T) {
	key := testCredentialKey()
	manager := NewManager(ManagerOptions{Store: newMemoryCredentialStore(), OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: time.Now()}, Rand: errorReader{}})
	if _, err := manager.LoginGoogle(context.Background(), LoginOptions{ClientID: key.ClientID}); err == nil || !strings.Contains(err.Error(), "PKCE") {
		t.Fatalf("expected PKCE entropy error, got %v", err)
	}

	manager = NewManager(ManagerOptions{Store: newMemoryCredentialStore(), OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: time.Now()}, Rand: bytes.NewReader(make([]byte, pkceEntropyBytes))})
	if _, err := manager.LoginGoogle(context.Background(), LoginOptions{ClientID: key.ClientID}); err == nil || !strings.Contains(err.Error(), "state") {
		t.Fatalf("expected state entropy error, got %v", err)
	}

	manager = NewManager(ManagerOptions{Store: newMemoryCredentialStore(), OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: time.Now()}, Rand: bytes.NewReader(make([]byte, pkceEntropyBytes*2))})
	if _, err := manager.LoginGoogle(context.Background(), LoginOptions{ClientID: key.ClientID}); err == nil || !strings.Contains(err.Error(), "nonce") {
		t.Fatalf("expected nonce entropy error, got %v", err)
	}
}

func TestLoginGooglePropagatesBrowserAndCallbackErrors(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	entropy := bytes.Repeat([]byte{0x11}, pkceEntropyBytes*3)
	key := testCredentialKey()

	t.Run("browser", func(t *testing.T) {
		manager := NewManager(ManagerOptions{
			Store: newMemoryCredentialStore(),
			OAuth: &fakeOAuthClient{},
			Clock: fixedClock{now: now},
			Rand:  bytes.NewReader(entropy),
			Loopback: fakeLoopbackProvider{session: &fakeLoopbackSession{
				redirectURI: "http://127.0.0.1:49152",
			}},
			OpenBrowser: func(context.Context, string) error { return errors.New("browser failed") },
		})
		_, err := manager.LoginGoogle(context.Background(), LoginOptions{
			ProfileName: key.ProfileName,
			Endpoint:    key.Endpoint,
			ClientID:    key.ClientID,
			Issuer:      key.Issuer,
		})
		if err == nil || !strings.Contains(err.Error(), "browser failed") {
			t.Fatalf("expected browser error, got %v", err)
		}
	})

	t.Run("callback", func(t *testing.T) {
		manager := NewManager(ManagerOptions{
			Store: newMemoryCredentialStore(),
			OAuth: &fakeOAuthClient{},
			Clock: fixedClock{now: now},
			Rand:  bytes.NewReader(entropy),
			Loopback: fakeLoopbackProvider{session: &fakeLoopbackSession{
				redirectURI: "http://127.0.0.1:49152",
				callback:    url.Values{"state": {"wrong"}, "code": {"code"}},
			}},
		})
		_, err := manager.LoginGoogle(context.Background(), LoginOptions{
			ProfileName: key.ProfileName,
			Endpoint:    key.Endpoint,
			ClientID:    key.ClientID,
			Issuer:      key.Issuer,
			NoBrowser:   true,
		})
		if err == nil || !strings.Contains(err.Error(), "state") {
			t.Fatalf("expected callback state error, got %v", err)
		}
	})
}

func TestNewManagerDefaultsAreUsable(t *testing.T) {
	manager := NewManager(ManagerOptions{})
	if manager.store == nil || manager.oauth == nil || manager.clock == nil || manager.rand == nil || manager.loopback == nil || manager.openBrowser == nil {
		t.Fatalf("default manager has nil dependency: %+v", manager)
	}
	if (realClock{}).Now().IsZero() {
		t.Fatalf("real clock returned zero time")
	}
}

func TestLoginGooglePropagatesOAuthClaimAndStoreErrors(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	key := testCredentialKey()

	t.Run("loopback", func(t *testing.T) {
		manager := NewManager(ManagerOptions{
			Store:    newMemoryCredentialStore(),
			OAuth:    &fakeOAuthClient{},
			Clock:    fixedClock{now: now},
			Rand:     bytes.NewReader(bytes.Repeat([]byte{0x22}, pkceEntropyBytes*3)),
			Loopback: fakeLoopbackProvider{err: errors.New("listen failed")},
		})
		_, err := manager.LoginGoogle(context.Background(), LoginOptions{ClientID: key.ClientID, Issuer: key.Issuer})
		if err == nil || !strings.Contains(err.Error(), "listen failed") {
			t.Fatalf("expected loopback error, got %v", err)
		}
	})

	t.Run("oauth exchange", func(t *testing.T) {
		state, _ := generateOpaqueToken(bytes.NewReader(bytes.Repeat([]byte{0x33}, pkceEntropyBytes)))
		manager := NewManager(ManagerOptions{
			Store: newMemoryCredentialStore(),
			OAuth: &fakeOAuthClient{exchangeErr: errors.New("exchange failed")},
			Clock: fixedClock{now: now},
			Rand:  bytes.NewReader(bytes.Repeat([]byte{0x33}, pkceEntropyBytes*3)),
			Loopback: fakeLoopbackProvider{session: &fakeLoopbackSession{
				redirectURI: "http://127.0.0.1:49152",
				callback:    url.Values{"state": {state}, "code": {"code"}},
			}},
		})
		_, err := manager.LoginGoogle(context.Background(), LoginOptions{ClientID: key.ClientID, Issuer: key.Issuer, NoBrowser: true})
		if err == nil || !strings.Contains(err.Error(), "exchange failed") {
			t.Fatalf("expected exchange error, got %v", err)
		}
	})

	t.Run("missing id token", func(t *testing.T) {
		state, _ := generateOpaqueToken(bytes.NewReader(bytes.Repeat([]byte{0x44}, pkceEntropyBytes)))
		manager := NewManager(ManagerOptions{
			Store: newMemoryCredentialStore(),
			OAuth: &fakeOAuthClient{exchangeResponse: TokenResponse{RefreshToken: "refresh"}},
			Clock: fixedClock{now: now},
			Rand:  bytes.NewReader(bytes.Repeat([]byte{0x44}, pkceEntropyBytes*3)),
			Loopback: fakeLoopbackProvider{session: &fakeLoopbackSession{
				redirectURI: "http://127.0.0.1:49152",
				callback:    url.Values{"state": {state}, "code": {"code"}},
			}},
		})
		_, err := manager.LoginGoogle(context.Background(), LoginOptions{ClientID: key.ClientID, Issuer: key.Issuer, NoBrowser: true})
		if err == nil || !strings.Contains(err.Error(), "id_token") {
			t.Fatalf("expected id_token error, got %v", err)
		}
	})

	t.Run("invalid claims", func(t *testing.T) {
		entropy := bytes.Repeat([]byte{0x55}, pkceEntropyBytes*3)
		state := base64URL(entropy[pkceEntropyBytes : pkceEntropyBytes*2])
		nonce := base64URL(entropy[pkceEntropyBytes*2 : pkceEntropyBytes*3])
		idToken := testIDToken(t, map[string]any{
			"iss":   key.Issuer,
			"aud":   "other-client",
			"sub":   "subject-1",
			"exp":   now.Add(time.Hour).Unix(),
			"nonce": nonce,
		})
		manager := NewManager(ManagerOptions{
			Store: newMemoryCredentialStore(),
			OAuth: &fakeOAuthClient{exchangeResponse: TokenResponse{IDToken: idToken, RefreshToken: "refresh"}},
			Clock: fixedClock{now: now},
			Rand:  bytes.NewReader(entropy),
			Loopback: fakeLoopbackProvider{session: &fakeLoopbackSession{
				redirectURI: "http://127.0.0.1:49152",
				callback:    url.Values{"state": {state}, "code": {"code"}},
			}},
		})
		_, err := manager.LoginGoogle(context.Background(), LoginOptions{ClientID: key.ClientID, Issuer: key.Issuer, NoBrowser: true})
		if err == nil || !strings.Contains(err.Error(), "audience") {
			t.Fatalf("expected audience error, got %v", err)
		}
	})

	t.Run("store put", func(t *testing.T) {
		entropy := bytes.Repeat([]byte{0x66}, pkceEntropyBytes*3)
		state := base64URL(entropy[pkceEntropyBytes : pkceEntropyBytes*2])
		nonce := base64URL(entropy[pkceEntropyBytes*2 : pkceEntropyBytes*3])
		idToken := testIDToken(t, map[string]any{
			"iss":   key.Issuer,
			"aud":   key.ClientID,
			"sub":   "subject-1",
			"exp":   now.Add(time.Hour).Unix(),
			"nonce": nonce,
		})
		manager := NewManager(ManagerOptions{
			Store: failingCredentialStore{putErr: errors.New("store failed")},
			OAuth: &fakeOAuthClient{exchangeResponse: TokenResponse{IDToken: idToken, RefreshToken: "refresh"}},
			Clock: fixedClock{now: now},
			Rand:  bytes.NewReader(entropy),
			Loopback: fakeLoopbackProvider{session: &fakeLoopbackSession{
				redirectURI: "http://127.0.0.1:49152",
				callback:    url.Values{"state": {state}, "code": {"code"}},
			}},
		})
		_, err := manager.LoginGoogle(context.Background(), LoginOptions{ClientID: key.ClientID, Issuer: key.Issuer, NoBrowser: true})
		if err == nil || !strings.Contains(err.Error(), "store failed") {
			t.Fatalf("expected store error, got %v", err)
		}
	})

	t.Run("browser opener success", func(t *testing.T) {
		entropy := bytes.Repeat([]byte{0x77}, pkceEntropyBytes*3)
		state := base64URL(entropy[pkceEntropyBytes : pkceEntropyBytes*2])
		nonce := base64URL(entropy[pkceEntropyBytes*2 : pkceEntropyBytes*3])
		idToken := testIDToken(t, map[string]any{
			"iss":   key.Issuer,
			"aud":   key.ClientID,
			"sub":   "subject-1",
			"exp":   now.Add(time.Hour).Unix(),
			"nonce": nonce,
		})
		var opened string
		manager := NewManager(ManagerOptions{
			Store: newMemoryCredentialStore(),
			OAuth: &fakeOAuthClient{exchangeResponse: TokenResponse{IDToken: idToken, RefreshToken: "refresh"}},
			Clock: fixedClock{now: now},
			Rand:  bytes.NewReader(entropy),
			Loopback: fakeLoopbackProvider{session: &fakeLoopbackSession{
				redirectURI: "http://127.0.0.1:49152",
				callback:    url.Values{"state": {state}, "code": {"code"}},
			}},
			OpenBrowser: func(_ context.Context, authURL string) error {
				opened = authURL
				return nil
			},
		})
		result, err := manager.LoginGoogle(context.Background(), LoginOptions{ClientID: key.ClientID, Issuer: key.Issuer})
		if err != nil {
			t.Fatalf("LoginGoogle: %v", err)
		}
		if opened == "" {
			t.Fatalf("browser opener was not called")
		}
		if result.AuthorizationURL != "" {
			t.Fatalf("interactive login should not print authorization URL: %+v", result)
		}
	})
}

func TestResolveBearerTokenStaticAndConfigurationBranches(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	manager := NewManager(ManagerOptions{Store: newMemoryCredentialStore(), OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: time.Now()}})

	got, err := manager.ResolveBearerToken(context.Background(), ResolveOptions{AuthMode: "bearer", StaticToken: " static-token "})
	if err != nil {
		t.Fatalf("ResolveBearerToken static: %v", err)
	}
	if got.Token != "static-token" || got.Source != TokenSourceLegacyBearer {
		t.Fatalf("resolved static = %+v", got)
	}

	empty, err := manager.ResolveBearerToken(context.Background(), ResolveOptions{AuthMode: "bearer"})
	if err != nil {
		t.Fatalf("ResolveBearerToken empty bearer: %v", err)
	}
	if empty.Token != "" {
		t.Fatalf("empty bearer resolved token = %+v", empty)
	}

	_, err = manager.ResolveBearerToken(context.Background(), ResolveOptions{AuthMode: "oidcSession"})
	if err == nil || !strings.Contains(err.Error(), "clientID") {
		t.Fatalf("expected clientID error, got %v", err)
	}

	_, err = manager.ResolveBearerToken(context.Background(), testResolveOptions())
	if !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("expected login required for missing session, got %v", err)
	}

	key := testCredentialKey()
	store := newMemoryCredentialStore()
	if err := store.Put(context.Background(), key, Session{CachedIDToken: "id-token", IDTokenExpiry: time.Now().Add(time.Hour)}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	manager = NewManager(ManagerOptions{Store: store, OAuth: &fakeOAuthClient{}, Clock: fixedClock{now: time.Now()}})
	_, err = manager.ResolveBearerToken(context.Background(), testResolveOptions())
	if !errors.Is(err, ErrLoginRequired) {
		t.Fatalf("expected login required for missing refresh token, got %v", err)
	}
}

func TestResolveBearerTokenRefreshRejectsMissingOrInvalidIDToken(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)
	key := testCredentialKey()

	cases := []struct {
		name string
		resp TokenResponse
		want string
	}{
		{name: "missing", resp: TokenResponse{}, want: "missing id_token"},
		{name: "invalid", resp: TokenResponse{IDToken: "bad"}, want: "invalid ID token"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			store := newMemoryCredentialStore()
			if err := store.Put(context.Background(), key, Session{RefreshToken: "refresh", IDTokenExpiry: now.Add(-time.Minute)}); err != nil {
				t.Fatalf("Put: %v", err)
			}
			manager := NewManager(ManagerOptions{
				Store: store,
				OAuth: &fakeOAuthClient{refreshResponse: tc.resp},
				Clock: fixedClock{now: now},
			})
			_, err := manager.ResolveBearerToken(context.Background(), testResolveOptions())
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q error, got %v", tc.want, err)
			}
			if _, ok, err := store.Get(context.Background(), key); err != nil || !ok {
				t.Fatalf("session should be retained after non-invalid_grant refresh failure: ok=%v err=%v", ok, err)
			}
		})
	}
}

func TestResolveBearerTokenStoreErrors(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	key := testCredentialKey()
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	manager := NewManager(ManagerOptions{
		Store: failingCredentialStore{getErr: errors.New("get failed")},
		OAuth: &fakeOAuthClient{},
		Clock: fixedClock{now: now},
	})
	_, err := manager.ResolveBearerToken(context.Background(), testResolveOptions())
	if err == nil || !strings.Contains(err.Error(), "get failed") {
		t.Fatalf("expected get error, got %v", err)
	}

	idToken := testIDToken(t, map[string]any{
		"iss": key.Issuer,
		"aud": key.ClientID,
		"sub": "subject-1",
		"exp": now.Add(time.Hour).Unix(),
	})
	manager = NewManager(ManagerOptions{
		Store: failingCredentialStore{
			session: Session{Subject: "subject-1", RefreshToken: "refresh", IDTokenExpiry: now.Add(-time.Minute)},
			found:   true,
			putErr:  errors.New("put failed"),
		},
		OAuth: &fakeOAuthClient{refreshResponse: TokenResponse{IDToken: idToken}},
		Clock: fixedClock{now: now},
	})
	_, err = manager.ResolveBearerToken(context.Background(), testResolveOptions())
	if err == nil || !strings.Contains(err.Error(), "put failed") {
		t.Fatalf("expected put error, got %v", err)
	}

	manager = NewManager(ManagerOptions{
		Store: failingCredentialStore{
			session:   Session{RefreshToken: "refresh", IDTokenExpiry: now.Add(-time.Minute)},
			found:     true,
			deleteErr: errors.New("delete failed"),
		},
		OAuth: &fakeOAuthClient{refreshErr: &OAuthError{Code: "invalid_grant", Status: 400}},
		Clock: fixedClock{now: now},
	})
	_, err = manager.ResolveBearerToken(context.Background(), testResolveOptions())
	if !errors.Is(err, ErrLoginRequired) || !strings.Contains(err.Error(), "failed to delete local auth session") {
		t.Fatalf("expected login required delete error, got %v", err)
	}
}

func TestResolveBearerTokenDoesNotDeleteForUntypedInvalidGrantText(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	key := testCredentialKey()
	store := newMemoryCredentialStore()
	if err := store.Put(context.Background(), key, Session{Subject: "subject-1", RefreshToken: "refresh", IDTokenExpiry: time.Now().Add(-time.Minute)}); err != nil {
		t.Fatal(err)
	}
	manager := NewManager(ManagerOptions{Store: store, OAuth: &fakeOAuthClient{refreshErr: errors.New("dial invalid_grant.example: temporary failure")}, Clock: fixedClock{now: time.Now()}})
	if _, err := manager.ResolveBearerToken(context.Background(), testResolveOptions()); err == nil || errors.Is(err, ErrLoginRequired) {
		t.Fatalf("untyped transport error was classified as invalid_grant: %v", err)
	}
	if _, ok, err := store.Get(context.Background(), key); err != nil || !ok {
		t.Fatalf("session should be retained: ok=%v err=%v", ok, err)
	}
}

func TestStatusStoreErrorAndLegacyMode(t *testing.T) {
	t.Setenv("SQLRS_TOKEN", "")
	key := testCredentialKey()
	manager := NewManager(ManagerOptions{
		Store: failingCredentialStore{getErr: errors.New("get failed")},
		OAuth: &fakeOAuthClient{},
		Clock: fixedClock{now: time.Now()},
	})
	_, err := manager.Status(context.Background(), StatusOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		AuthMode:    "oidcSession",
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
	})
	if err == nil || !strings.Contains(err.Error(), "get failed") {
		t.Fatalf("expected status get error, got %v", err)
	}

	status, err := manager.Status(context.Background(), StatusOptions{AuthMode: "bearer"})
	if err != nil {
		t.Fatalf("legacy status: %v", err)
	}
	if status.LoggedIn {
		t.Fatalf("legacy status should not be logged in: %+v", status)
	}

	status, err = NewManager(ManagerOptions{
		Store: newMemoryCredentialStore(),
		OAuth: &fakeOAuthClient{},
		Clock: fixedClock{now: time.Now()},
	}).Status(context.Background(), StatusOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		AuthMode:    "oidcSession",
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
	})
	if err != nil {
		t.Fatalf("oidcSession status without session: %v", err)
	}
	if status.LoggedIn {
		t.Fatalf("missing session status should not be logged in: %+v", status)
	}
}

func TestLogoutNoSessionAndRevocationFailure(t *testing.T) {
	key := testCredentialKey()
	store := newMemoryCredentialStore()
	oauth := &fakeOAuthClient{revokeErr: errors.New("revocation failed")}
	manager := NewManager(ManagerOptions{Store: store, OAuth: oauth, Clock: fixedClock{now: time.Now()}})

	result, err := manager.Logout(context.Background(), LogoutOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
	})
	if err != nil {
		t.Fatalf("Logout no session: %v", err)
	}
	if !result.Deleted || result.Revoked || result.RevocationFailed != "" {
		t.Fatalf("no-session logout result = %+v", result)
	}

	if err := store.Put(context.Background(), key, Session{RefreshToken: "refresh"}); err != nil {
		t.Fatalf("Put: %v", err)
	}
	result, err = manager.Logout(context.Background(), LogoutOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
	})
	if err != nil {
		t.Fatalf("Logout revoke failure: %v", err)
	}
	if result.Revoked || !strings.Contains(result.RevocationFailed, "revocation failed") {
		t.Fatalf("revoke-failure logout result = %+v", result)
	}
}

func TestLogoutStoreErrors(t *testing.T) {
	key := testCredentialKey()
	manager := NewManager(ManagerOptions{
		Store: failingCredentialStore{getErr: errors.New("get failed")},
		OAuth: &fakeOAuthClient{},
		Clock: fixedClock{now: time.Now()},
	})
	_, err := manager.Logout(context.Background(), LogoutOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
	})
	if err == nil || !strings.Contains(err.Error(), "get failed") {
		t.Fatalf("expected logout get error, got %v", err)
	}

	manager = NewManager(ManagerOptions{
		Store: failingCredentialStore{found: true, session: Session{RefreshToken: "refresh"}, deleteErr: errors.New("delete failed")},
		OAuth: &fakeOAuthClient{},
		Clock: fixedClock{now: time.Now()},
	})
	_, err = manager.Logout(context.Background(), LogoutOptions{
		ProfileName: key.ProfileName,
		Endpoint:    key.Endpoint,
		ClientID:    key.ClientID,
		Issuer:      key.Issuer,
		NoRevoke:    true,
	})
	if err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("expected logout delete error, got %v", err)
	}
}

type failingCredentialStore struct {
	session   Session
	found     bool
	getErr    error
	putErr    error
	deleteErr error
}

func (s failingCredentialStore) Get(context.Context, CredentialKey) (Session, bool, error) {
	return s.session, s.found, s.getErr
}

func (s failingCredentialStore) Put(context.Context, CredentialKey, Session) error {
	return s.putErr
}

func (s failingCredentialStore) Delete(context.Context, CredentialKey) error {
	return s.deleteErr
}

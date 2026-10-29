package authsession

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"
)

func TestMissingInstallationExchangerFailsBeforeBrowser(t *testing.T) {
	opened := false
	m := NewManager(ManagerOptions{Store: newMemoryCredentialStore(), OpenBrowser: func(context.Context, string) error { opened = true; return nil }})
	_, err := m.LoginOIDC(context.Background(), LoginOptions{ClientID: "client", Issuer: "https://issuer.example", AuthorizationEndpoint: "https://issuer.example/authorize", ExchangeEndpoint: "https://api.example/exchange", RefreshEndpoint: "https://api.example/refresh"})
	if err == nil || opened {
		t.Fatal("missing exchanger did not stop login")
	}
}

// R02/R04: freshness is checked after network latency, before replacing credentials.
// See docs/architecture/cli-auth-flow.md for strict token-result expiration.
func TestRefreshRejectsTokenExpiredDuringRequest(t *testing.T) {
	now := time.Now().UTC()
	original := Session{Provider: "university", Issuer: "https://issuer.example", ClientID: "client", Subject: "subject", LoginNonce: "nonce", RefreshToken: "old", IDTokenExpiry: now.Add(-time.Hour)}
	store := &synchronizedStore{session: original}
	clock := &refreshResponseClock{now: now}
	manager := NewManager(ManagerOptions{Store: store, Clock: clock, Exchanger: &fakeOAuthClient{refreshResponse: TokenResponse{IDToken: testIDToken(t, map[string]any{"iss": original.Issuer, "aud": "client", "sub": "subject", "nonce": "nonce", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix()}), RefreshToken: "replacement"}}})
	_, err := manager.ResolveBearerToken(context.Background(), ResolveOptions{AuthMode: "remoteSession", ProviderResolver: func(context.Context, string) (ProviderConfiguration, error) {
		return ProviderConfiguration{ID: original.Provider, Issuer: original.Issuer, ClientID: original.ClientID, RefreshEndpoint: "https://api.example/refresh"}, nil
	}})
	if err == nil || store.puts != 0 || store.deletes != 0 || !reflect.DeepEqual(store.session, original) {
		t.Fatal("token expired during request was accepted or altered credentials")
	}
}

// The first clock read models refresh selection; subsequent reads follow its response.
type refreshResponseClock struct {
	now   time.Time
	reads int
}

func (c *refreshResponseClock) Now() time.Time {
	c.reads++
	if c.reads == 1 {
		return c.now
	}
	return c.now.Add(2 * time.Minute)
}

// R04/R05: failed refresh never writes a record read before a competing success.
func TestRefreshFailureCannotUndoConcurrentSuccess(t *testing.T) {
	for _, failureFirst := range []bool{false, true} {
		t.Run(map[bool]string{true: "failure-first", false: "success-first"}[failureFirst], func(t *testing.T) {
			store := &synchronizedStore{session: Session{Provider: "university", Issuer: "https://issuer.example", ClientID: "client", Subject: "subject", LoginNonce: "nonce", RefreshToken: "old", IDTokenExpiry: time.Now().Add(-time.Hour)}}
			started := make(chan struct{})
			release := make(chan struct{})
			failed := NewManager(ManagerOptions{Store: store, Exchanger: blockedFailureExchanger{started: started, release: release}})
			options := ResolveOptions{AuthMode: "remoteSession", ProfileName: "profile", Endpoint: "https://api.example", InstallationID: "installation", ControlEndpoint: "https://api.example", ProviderResolver: func(context.Context, string) (ProviderConfiguration, error) {
				return ProviderConfiguration{ID: "university", Issuer: "https://issuer.example", ClientID: "client", RefreshEndpoint: "https://api.example/refresh"}, nil
			}}
			done := make(chan error, 1)
			go func() { _, err := failed.ResolveBearerToken(context.Background(), options); done <- err }()
			<-started
			winner := store.session
			winner.RefreshToken = "winner"
			winner.CachedIDToken = "winner-id"
			if failureFirst {
				close(release)
				if err := <-done; err == nil {
					t.Fatal("expected refusal")
				}
				store.set(winner)
			} else {
				store.set(winner)
				close(release)
				if err := <-done; err == nil {
					t.Fatal("expected refusal")
				}
			}
			got, _, _ := store.Get(context.Background(), CredentialKey{})
			if !reflect.DeepEqual(got, winner) || store.puts != 0 || store.deletes != 0 {
				t.Fatal("failed refresh mutated concurrent credentials")
			}
		})
	}
}

type synchronizedStore struct {
	mu            sync.Mutex
	session       Session
	puts, deletes int
}

func (s *synchronizedStore) Get(context.Context, CredentialKey) (Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.session, true, nil
}
func (s *synchronizedStore) Put(_ context.Context, _ CredentialKey, value Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.puts++
	s.session = value
	return nil
}
func (s *synchronizedStore) Delete(context.Context, CredentialKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deletes++
	return nil
}
func (s *synchronizedStore) set(value Session) { s.mu.Lock(); defer s.mu.Unlock(); s.session = value }

type blockedFailureExchanger struct{ started, release chan struct{} }

func (e blockedFailureExchanger) ExchangeCode(context.Context, LoginProvider, CodeExchangeRequest) (TokenResponse, error) {
	return TokenResponse{}, errors.New("unused")
}
func (e blockedFailureExchanger) Refresh(context.Context, LoginProvider, RefreshRequest) (TokenResponse, error) {
	close(e.started)
	<-e.release
	return TokenResponse{}, errors.New("provider rejected grant")
}

// L09: incomplete installation identity requires login without remote I/O or writes.
func TestIncompleteRemoteSessionFailsBeforeDiscovery(t *testing.T) {
	for _, field := range []string{"provider", "issuer", "client", "subject", "nonce"} {
		t.Run(field, func(t *testing.T) {
			session := Session{Provider: "university", Issuer: "https://issuer.example", ClientID: "client", Subject: "subject", LoginNonce: "nonce", RefreshToken: "refresh", CachedIDToken: "cached", IDTokenExpiry: time.Now().Add(time.Hour)}
			switch field {
			case "provider":
				session.Provider = ""
			case "issuer":
				session.Issuer = ""
			case "client":
				session.ClientID = ""
			case "subject":
				session.Subject = ""
			case "nonce":
				session.LoginNonce = ""
			}
			store := &synchronizedStore{session: session}
			m := NewManager(ManagerOptions{Store: store})
			_, err := m.ResolveBearerToken(context.Background(), ResolveOptions{AuthMode: "remoteSession", ProviderResolver: func(context.Context, string) (ProviderConfiguration, error) {
				t.Fatal("incomplete session caused discovery")
				return ProviderConfiguration{}, nil
			}})
			if err == nil || store.puts != 0 || store.deletes != 0 {
				t.Fatal("incomplete session accepted or mutated")
			}
		})
	}
}

// Q01/R02-R04: each rejected refresh preserves the exact current record.
func TestRemoteRefreshRefusalsDoNotWrite(t *testing.T) {
	for _, mode := range []string{"no-discovery", "discovery-error", "no-exchanger", "nonce", "subject", "expired", "save-error"} {
		t.Run(mode, func(t *testing.T) {
			now := time.Now().UTC()
			original := Session{Provider: "university", Issuer: "https://issuer.example", ClientID: "client", Subject: "subject", LoginNonce: "nonce", RefreshToken: "old", IDTokenExpiry: now.Add(-time.Hour)}
			store := &synchronizedStore{session: original}
			claims := map[string]any{"iss": original.Issuer, "aud": "client", "sub": "subject", "nonce": "nonce", "iat": now.Unix(), "exp": now.Add(time.Minute).Unix()}
			switch mode {
			case "nonce":
				claims["nonce"] = "wrong"
			case "subject":
				claims["sub"] = "wrong"
			case "expired":
				claims["exp"] = now.Unix()
			}
			options := ResolveOptions{AuthMode: "remoteSession", ProviderResolver: func(context.Context, string) (ProviderConfiguration, error) {
				if mode == "discovery-error" {
					return ProviderConfiguration{}, errors.New("discovery failed")
				}
				return ProviderConfiguration{ID: original.Provider, Issuer: original.Issuer, ClientID: "client", RefreshEndpoint: "https://api.example/refresh"}, nil
			}}
			if mode == "no-discovery" {
				options.ProviderResolver = nil
			}
			dep := ManagerOptions{Store: store, Clock: fixedClock{now: now}, Exchanger: &fakeOAuthClient{refreshResponse: TokenResponse{IDToken: testIDToken(t, claims)}}}
			if mode == "no-exchanger" {
				dep.Exchanger = nil
			}
			if mode == "save-error" {
				dep.Store = failingCredentialStore{session: original, found: true, putErr: errors.New("store unavailable")}
			}
			if _, err := NewManager(dep).ResolveBearerToken(context.Background(), options); err == nil {
				t.Fatal("refusal accepted")
			}
			got, _, _ := store.Get(context.Background(), CredentialKey{})
			if !reflect.DeepEqual(got, original) || store.puts != 0 || store.deletes != 0 {
				t.Fatal("rejected refresh mutated store")
			}
		})
	}
}

// Q01: cancellation closes the actual local listener and interrupted listeners fail waiting.
func TestLocalLoginListenerCancellationAndInterruption(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	session, err := (LoopbackServerProvider{}).Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	cancel()
	defer session.Close()
	if _, err = session.Wait(ctx); err == nil {
		t.Fatal("cancelled login continued")
	}
	interrupted, err := (LoopbackServerProvider{}).Start(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	defer interrupted.Close()
	interrupted.(*loopbackServerSession).listener.Close()
	deadline, stop := context.WithTimeout(context.Background(), time.Second)
	defer stop()
	if _, err = interrupted.Wait(deadline); err == nil || errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("listener interruption lost: %v", err)
	}
}

// Q05: no fallback endpoints/scopes and no reserved endpoint parameters are accepted.
func TestExplicitOIDCAuthorizationMetadataRequired(t *testing.T) {
	base := AuthURLOptions{ClientID: "client", RedirectURI: "http://127.0.0.1:1234/", State: "state", Nonce: "nonce", CodeChallenge: "challenge", AuthorizationEndpoint: "https://issuer.example/auth", Scopes: []string{"openid"}}
	for _, mode := range []string{"endpoint", "scope", "malformed", "reserved"} {
		o := base
		switch mode {
		case "endpoint":
			o.AuthorizationEndpoint = ""
		case "scope":
			o.Scopes = nil
		case "malformed":
			o.AuthorizationEndpoint = "https://%zz"
		case "reserved":
			o.AuthorizationEndpoint += "?state=other"
		}
		if _, err := BuildOIDCAuthURL(o); err == nil {
			t.Fatalf("invalid %s accepted", mode)
		}
	}
}

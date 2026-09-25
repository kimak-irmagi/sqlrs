package authsession

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"
)

// CredentialKey scopes one active OIDC session to one profile, endpoint, issuer,
// and OAuth client as defined in docs/architecture/cli-auth-component-structure.md.
type CredentialKey struct {
	ProfileName     string
	Endpoint        string
	InstallationID  string
	ControlEndpoint string
	Provider        string
	Issuer          string
	ClientID        string
}

// Session is stored as a secret credential value. RefreshToken and CachedIDToken
// must never be rendered to CLI output.
type Session struct {
	Provider      string    `json:"provider"`
	Issuer        string    `json:"issuer"`
	ClientID      string    `json:"client_id"`
	Subject       string    `json:"subject"`
	Email         string    `json:"email"`
	Scopes        []string  `json:"scopes"`
	RefreshToken  string    `json:"refresh_token"`
	CachedIDToken string    `json:"cached_id_token"`
	IDTokenExpiry time.Time `json:"id_token_expiry"`
	CreatedAt     time.Time `json:"created_at"`
	UpdatedAt     time.Time `json:"updated_at"`
	LoginNonce    string    `json:"login_nonce,omitempty"`
}

// CredentialStore hides the OS credential store implementation behind a small
// interface so tests can mock refresh-token persistence.
type CredentialStore interface {
	Get(context.Context, CredentialKey) (Session, bool, error)
	Put(context.Context, CredentialKey, Session) error
	Delete(context.Context, CredentialKey) error
}

type memoryCredentialStore struct {
	mu       sync.Mutex
	sessions map[string]Session
}

func newMemoryCredentialStore() *memoryCredentialStore {
	return &memoryCredentialStore{sessions: map[string]Session{}}
}

func (s *memoryCredentialStore) Get(_ context.Context, key CredentialKey) (Session, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	session, ok := s.sessions[key.stableName()]
	if !ok {
		return Session{}, false, nil
	}
	session.Scopes = append([]string(nil), session.Scopes...)
	return session, true, nil
}

func (s *memoryCredentialStore) Put(_ context.Context, key CredentialKey, session Session) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	session.Scopes = append([]string(nil), session.Scopes...)
	s.sessions[key.stableName()] = session
	return nil
}

func (s *memoryCredentialStore) Delete(_ context.Context, key CredentialKey) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.sessions, key.stableName())
	return nil
}

func (k CredentialKey) normalized() CredentialKey {
	return CredentialKey{
		ProfileName:     strings.TrimSpace(k.ProfileName),
		Endpoint:        strings.TrimSpace(k.Endpoint),
		InstallationID:  strings.TrimSpace(k.InstallationID),
		ControlEndpoint: strings.TrimSpace(k.ControlEndpoint),
		Provider:        defaultProvider(k.Provider),
		Issuer:          defaultIssuer(k.Issuer),
		ClientID:        strings.TrimSpace(k.ClientID),
	}
}

func (k CredentialKey) stableName() string {
	n := k.normalized()
	trustEndpoint := n.ControlEndpoint
	if trustEndpoint == "" {
		trustEndpoint = n.Endpoint
	}
	var raw string
	if n.InstallationID == "" && n.ControlEndpoint == "" {
		// Preserve the public-RC oidcSession key for explicit legacy reads and
		// migration; remoteSession profiles use the installation trust anchor.
		raw = strings.Join([]string{n.ProfileName, n.Endpoint, n.Provider, n.Issuer, n.ClientID}, "\n")
	} else {
		raw = strings.Join([]string{n.ProfileName, n.InstallationID, trustEndpoint}, "\n")
	}
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (k CredentialKey) displayAccount() string {
	n := k.normalized()
	sum := n.stableName()
	return fmt.Sprintf("%s:%s:%s", n.ProfileName, n.Provider, sum[:16])
}

func encodeSession(session Session) string {
	data, _ := json.Marshal(session)
	return string(data)
}

func decodeSession(data string) (Session, error) {
	var session Session
	if err := json.Unmarshal([]byte(data), &session); err != nil {
		return Session{}, err
	}
	return session, nil
}

func defaultProvider(provider string) string {
	if strings.TrimSpace(provider) == "" {
		return "google"
	}
	return strings.ToLower(strings.TrimSpace(provider))
}

const credentialService = "sqlrs-cli-auth"

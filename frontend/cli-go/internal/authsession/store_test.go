package authsession

import (
	"context"
	"testing"
	"time"
)

func TestMemoryCredentialStoreScopesSessionsByCredentialKey(t *testing.T) {
	store := newMemoryCredentialStore()
	key := testCredentialKey()
	other := key
	other.ProfileName = "other"
	session := Session{
		Provider:      "google",
		Issuer:        key.Issuer,
		ClientID:      key.ClientID,
		RefreshToken:  "refresh-token",
		CachedIDToken: "id-token",
		IDTokenExpiry: time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC),
	}

	if err := store.Put(context.Background(), key, session); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if _, ok, err := store.Get(context.Background(), other); err != nil || ok {
		t.Fatalf("other key should not be found: ok=%v err=%v", ok, err)
	}
	got, ok, err := store.Get(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("Get: ok=%v err=%v", ok, err)
	}
	if got.RefreshToken != "refresh-token" || got.CachedIDToken != "id-token" {
		t.Fatalf("session = %+v, want stored copy", got)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, ok, err := store.Get(context.Background(), key); err != nil || ok {
		t.Fatalf("deleted key should not be found: ok=%v err=%v", ok, err)
	}
}

func TestSessionEncodingAndCredentialKeyHelpers(t *testing.T) {
	key := testCredentialKey()
	if key.stableName() == "" || key.displayAccount() == "" {
		t.Fatalf("expected stable credential names")
	}
	if got := defaultProvider(""); got != "google" {
		t.Fatalf("defaultProvider empty = %q", got)
	}
	if got := defaultProvider(" GOOGLE "); got != "google" {
		t.Fatalf("defaultProvider normalized = %q", got)
	}

	session := Session{Provider: "google", RefreshToken: "refresh-token", CachedIDToken: "id-token"}
	encoded := encodeSession(session)
	decoded, err := decodeSession(encoded)
	if err != nil {
		t.Fatalf("decodeSession: %v", err)
	}
	if decoded.RefreshToken != "refresh-token" || decoded.CachedIDToken != "id-token" {
		t.Fatalf("decoded session = %+v", decoded)
	}
	if _, err := decodeSession("{"); err == nil {
		t.Fatalf("expected decode error")
	}
}

func TestCredentialKeyUsesInstallationTrustAnchorNotMutableRequestEndpoint(t *testing.T) {
	root := CredentialKey{ProfileName: "remote", Endpoint: "https://api.example.test", InstallationID: "installation-1", ControlEndpoint: "https://api.example.test", Provider: "google", Issuer: "https://issuer.example.test", ClientID: "public-client"}
	org := root
	org.Endpoint = "https://api.example.test/nsu"
	if root.stableName() != org.stableName() {
		t.Fatalf("organization endpoint switch changed credential key")
	}
	other := org
	other.InstallationID = "installation-2"
	if other.stableName() == root.stableName() {
		t.Fatalf("different installation reused credential key")
	}
}

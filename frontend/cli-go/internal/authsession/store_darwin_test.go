//go:build darwin && cgo

package authsession

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func withDarwinCredentialStubs(
	t *testing.T,
	find func(string, string) ([]byte, bool, error),
	put func(string, string, []byte) error,
	deleteFn func(string, string) error,
) {
	t.Helper()
	oldFind := findDarwinGenericPasswordFn
	oldPut := putDarwinGenericPasswordFn
	oldDelete := deleteDarwinGenericPasswordFn
	findDarwinGenericPasswordFn = find
	putDarwinGenericPasswordFn = put
	deleteDarwinGenericPasswordFn = deleteFn
	t.Cleanup(func() {
		findDarwinGenericPasswordFn = oldFind
		putDarwinGenericPasswordFn = oldPut
		deleteDarwinGenericPasswordFn = oldDelete
	})
}

func TestSystemCredentialStoreDarwinPutGetDelete(t *testing.T) {
	key := CredentialKey{ProfileName: "remote", Endpoint: "https://sqlrs.example.org", Issuer: "issuer", ClientID: "client"}
	want := Session{Provider: "google", RefreshToken: "refresh", IDTokenExpiry: time.Date(2026, 8, 4, 12, 0, 0, 0, time.UTC)}
	var stored []byte
	var account string
	deleted := false
	withDarwinCredentialStubs(t,
		func(service, gotAccount string) ([]byte, bool, error) {
			if service != credentialService {
				t.Fatalf("service = %q", service)
			}
			account = gotAccount
			return append([]byte(nil), stored...), stored != nil, nil
		},
		func(service, gotAccount string, data []byte) error {
			if service != credentialService {
				t.Fatalf("service = %q", service)
			}
			account = gotAccount
			stored = append([]byte(nil), data...)
			return nil
		},
		func(service, gotAccount string) error {
			if service != credentialService || gotAccount != account {
				t.Fatalf("delete service/account = %q/%q", service, gotAccount)
			}
			deleted = true
			return nil
		},
	)

	store := NewSystemCredentialStore()
	if err := store.Put(context.Background(), key, want); err != nil {
		t.Fatalf("Put: %v", err)
	}
	got, ok, err := store.Get(context.Background(), key)
	if err != nil || !ok {
		t.Fatalf("Get = %#v, %t, %v", got, ok, err)
	}
	if got.RefreshToken != want.RefreshToken || !got.IDTokenExpiry.Equal(want.IDTokenExpiry) {
		t.Fatalf("session = %#v, want %#v", got, want)
	}
	if !strings.Contains(account, "remote") {
		t.Fatalf("account = %q, want profile context", account)
	}
	if err := store.Delete(context.Background(), key); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if !deleted {
		t.Fatal("Delete did not call Keychain adapter")
	}
}

func TestSystemCredentialStoreDarwinErrorsAndMissingSession(t *testing.T) {
	key := CredentialKey{ProfileName: "remote"}
	store := SystemCredentialStore{}

	withDarwinCredentialStubs(t,
		func(string, string) ([]byte, bool, error) { return nil, false, nil },
		func(string, string, []byte) error { return errors.New("put failed") },
		func(string, string) error { return errors.New("delete failed") },
	)
	if _, ok, err := store.Get(context.Background(), key); err != nil || ok {
		t.Fatalf("missing Get = ok %t, error %v", ok, err)
	}
	if err := store.Put(context.Background(), key, Session{}); err == nil || !strings.Contains(err.Error(), "put failed") {
		t.Fatalf("Put error = %v", err)
	}
	if err := store.Delete(context.Background(), key); err == nil || !strings.Contains(err.Error(), "delete failed") {
		t.Fatalf("Delete error = %v", err)
	}

	findDarwinGenericPasswordFn = func(string, string) ([]byte, bool, error) {
		return []byte("not-json"), true, nil
	}
	if _, ok, err := store.Get(context.Background(), key); err == nil || ok {
		t.Fatalf("invalid Get = ok %t, error %v", ok, err)
	}

	findDarwinGenericPasswordFn = func(string, string) ([]byte, bool, error) {
		return nil, false, errors.New("find failed")
	}
	if _, ok, err := store.Get(context.Background(), key); err == nil || ok {
		t.Fatalf("failed Get = ok %t, error %v", ok, err)
	}
}

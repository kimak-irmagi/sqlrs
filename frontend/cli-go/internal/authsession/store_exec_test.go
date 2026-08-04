//go:build linux

package authsession

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSystemCredentialStoreLinuxGetBranches(t *testing.T) {
	toolDir := writeFakeSecretTool(t)
	key := CredentialKey{ProfileName: "remote", Endpoint: "https://example.org"}
	store := SystemCredentialStore{}

	t.Run("success", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "lookup-success")
		t.Setenv("SECRET_TOOL_SESSION", encodeSession(Session{Provider: "google", RefreshToken: "refresh"}))
		got, ok, err := store.Get(context.Background(), key)
		if err != nil || !ok || got.RefreshToken != "refresh" {
			t.Fatalf("Get = %#v, %t, %v", got, ok, err)
		}
	})

	t.Run("missing credential", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "lookup-missing")
		_, ok, err := store.Get(context.Background(), key)
		if err != nil || ok {
			t.Fatalf("Get missing = %t, %v", ok, err)
		}
	})

	t.Run("provider stderr", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "lookup-stderr")
		_, ok, err := store.Get(context.Background(), key)
		if err == nil || ok || !strings.Contains(err.Error(), "lookup denied") {
			t.Fatalf("Get stderr = %t, %v", ok, err)
		}
	})

	t.Run("invalid session", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "lookup-success")
		t.Setenv("SECRET_TOOL_SESSION", "not-json")
		_, ok, err := store.Get(context.Background(), key)
		if err == nil || ok {
			t.Fatalf("Get invalid = %t, %v", ok, err)
		}
	})

	t.Run("cancelled context", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "lookup-success")
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		_, ok, err := store.Get(ctx, key)
		if !errors.Is(err, context.Canceled) || ok {
			t.Fatalf("Get cancelled = %t, %v", ok, err)
		}
	})

	t.Run("missing tool", func(t *testing.T) {
		t.Setenv("PATH", t.TempDir())
		_, ok, err := store.Get(context.Background(), key)
		if err == nil || ok || !strings.Contains(err.Error(), "install libsecret") {
			t.Fatalf("Get missing tool = %t, %v", ok, err)
		}
	})
}

func TestSystemCredentialStoreLinuxPutAndDeleteBranches(t *testing.T) {
	toolDir := writeFakeSecretTool(t)
	key := CredentialKey{ProfileName: "remote"}
	store := NewSystemCredentialStore()

	t.Run("put success", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "store-success")
		if err := store.Put(context.Background(), key, Session{RefreshToken: "refresh"}); err != nil {
			t.Fatalf("Put: %v", err)
		}
	})

	t.Run("put failure", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "store-failure")
		if err := store.Put(context.Background(), key, Session{}); err == nil || !strings.Contains(err.Error(), "store failed") {
			t.Fatalf("Put error = %v", err)
		}
	})

	t.Run("delete success", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "clear-success")
		if err := store.Delete(context.Background(), key); err != nil {
			t.Fatalf("Delete: %v", err)
		}
	})

	t.Run("delete stderr", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "clear-stderr")
		if err := store.Delete(context.Background(), key); err == nil || !strings.Contains(err.Error(), "clear denied") {
			t.Fatalf("Delete stderr = %v", err)
		}
	})

	t.Run("delete failure", func(t *testing.T) {
		configureFakeSecretTool(t, toolDir, "clear-failure")
		if err := store.Delete(context.Background(), key); err == nil || !strings.Contains(err.Error(), "clear failed") {
			t.Fatalf("Delete error = %v", err)
		}
	})
}

func writeFakeSecretTool(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, "secret-tool")
	script := `#!/bin/sh
case "$SECRET_TOOL_MODE" in
  lookup-success) printf '%s' "$SECRET_TOOL_SESSION" ;;
  lookup-missing) exit 1 ;;
  lookup-stderr) printf 'lookup denied' >&2; exit 2 ;;
  store-success) cat >/dev/null ;;
  store-failure) exit 3 ;;
  clear-success) exit 0 ;;
  clear-stderr) printf 'clear denied' >&2; exit 4 ;;
  clear-failure) exit 5 ;;
  *) exit 6 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write fake secret-tool: %v", err)
	}
	return dir
}

func configureFakeSecretTool(t *testing.T, toolDir, mode string) {
	t.Helper()
	t.Setenv("PATH", toolDir+string(os.PathListSeparator)+os.Getenv("PATH"))
	t.Setenv("SECRET_TOOL_MODE", mode)
}

package effectiveconfig

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// S02b/S02c: connection credentials are carried only in a short-lived file,
// with no secret in its path or in the container-side mount name.
func TestCreateProtectedConfigAndCleanup(t *testing.T) {
	root := t.TempDir()
	password := "top=secret\\line\nnext"
	config, err := CreateProtected(ProtectedOptions{
		Root: root, URL: "jdbc:postgresql://database:5432/postgres",
		Username: "sqlrs", Password: password,
		Properties: map[string]string{"liquibase.command.contextFilter": "dev"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(config.HostPath, password) || strings.Contains(config.ContainerPath, password) {
		t.Fatal("secret appeared in a mount path")
	}
	if !strings.HasPrefix(filepath.Clean(config.HostPath), filepath.Clean(root)+string(os.PathSeparator)) {
		t.Fatalf("protected file escaped supplied root: %q", config.HostPath)
	}
	data, err := os.ReadFile(config.HostPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" {
		file, err := os.Stat(config.HostPath)
		if err != nil || file.Mode().Perm()&0o004 == 0 || file.Mode().Perm()&0o222 != 0 {
			t.Fatalf("container cannot read read-only settings: info=%v err=%v", file, err)
		}
		parent, err := os.Stat(filepath.Dir(config.HostPath))
		if err != nil || parent.Mode().Perm()&0o077 != 0 {
			t.Fatalf("host-side settings directory is not private: info=%v err=%v", parent, err)
		}
	}
	for _, want := range []string{
		"liquibase.command.url=jdbc:postgresql://database:5432/postgres",
		"liquibase.command.username=sqlrs",
		"liquibase.command.password=top=secret\\\\line\\nnext",
		"liquibase.command.contextFilter=dev",
	} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("protected file lacks required property %q", want)
		}
	}
	if err := config.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(config.HostPath); !os.IsNotExist(err) {
		t.Fatalf("protected file survived cleanup: %v", err)
	}
	if _, err := os.Stat(filepath.Dir(config.HostPath)); !os.IsNotExist(err) {
		t.Fatalf("protected directory survived cleanup: %v", err)
	}
}

// S01b: a project property must not override the engine-owned connection.
func TestCreateProtectedConfigRejectsConnectionOverrides(t *testing.T) {
	for _, key := range []string{"url", "liquibase.command.username", "liquibase.command.password", "driver", "classpath"} {
		t.Run(key, func(t *testing.T) {
			root := t.TempDir()
			_, err := CreateProtected(ProtectedOptions{Root: root, URL: "jdbc:postgresql://database/postgres", Username: "sqlrs", Password: "secret", Properties: map[string]string{key: "attacker"}})
			if err == nil {
				t.Fatalf("protected key %q was accepted", key)
			}
			entries, readErr := os.ReadDir(root)
			if readErr != nil || len(entries) != 0 {
				t.Fatalf("rejected properties left protected files: entries=%v error=%v", entries, readErr)
			}
		})
	}
}

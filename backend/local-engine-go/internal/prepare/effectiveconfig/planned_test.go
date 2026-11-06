package effectiveconfig

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPlannedProtectedSettingsUseExclusiveSavedReference(t *testing.T) {
	root := t.TempDir()
	ref := "plan-" + strings.Repeat("a", 24) + "-input"
	options := ProtectedOptions{Root: root, URL: "jdbc:postgresql://db/postgres",
		Username: "sqlrs", Password: "private-password"}
	config, err := CreatePlannedProtected(options, ref)
	if err != nil {
		t.Fatal(err)
	}
	if config.HostPath != filepath.Join(root, ref, "liquibase.properties") {
		t.Fatalf("unexpected protected path: %q", config.HostPath)
	}
	loaded, err := OpenPlannedProtected(root, ref)
	if err != nil || loaded.HostPath != config.HostPath {
		t.Fatalf("saved protected file=%+v err=%v", loaded, err)
	}
	if _, err := CreatePlannedProtected(options, ref); err == nil {
		t.Fatal("existing saved settings were overwritten")
	}
	contents, err := os.ReadFile(config.HostPath)
	if err != nil || !strings.Contains(string(contents), "private-password") {
		t.Fatalf("original settings changed: %v", err)
	}
	if err := loaded.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenPlannedProtected(root, ref); err == nil {
		t.Fatal("deleted settings were treated as recoverable")
	}
	if _, err := CreatePlannedProtected(options, "../other"); err == nil {
		t.Fatal("traversal reference accepted")
	}
}

// C01a: retirement may resume after the file or directory was already
// removed. It cannot create replacement credentials or erase unrelated data.
func TestRetirePlannedProtectedResumesExactPartialCleanup(t *testing.T) {
	for _, phase := range []string{"complete", "file absent", "directory absent", "unexpected file", "substituted settings"} {
		t.Run(phase, func(t *testing.T) {
			root := t.TempDir()
			ref := "plan-" + strings.Repeat("a", 24) + "-input"
			config, err := CreatePlannedProtected(ProtectedOptions{Root: root, URL: "jdbc:postgresql://db/postgres", Username: "sqlrs", Password: "secret"}, ref)
			if err != nil {
				t.Fatal(err)
			}
			switch phase {
			case "file absent":
				if err := os.Remove(config.HostPath); err != nil {
					t.Fatal(err)
				}
			case "directory absent":
				if err := config.Close(); err != nil {
					t.Fatal(err)
				}
			case "unexpected file":
				if err := os.WriteFile(filepath.Join(root, ref, "unrelated"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			case "substituted settings":
				if err := os.Remove(config.HostPath); err != nil {
					t.Fatal(err)
				}
				if err := os.Mkdir(config.HostPath, 0700); err != nil {
					t.Fatal(err)
				}
			}
			err = RetirePlannedProtected(root, ref)
			wantErr := phase == "unexpected file" || phase == "substituted settings"
			if (err != nil) != wantErr {
				t.Fatalf("retirement err=%v want error=%t", err, wantErr)
			}
			if wantErr {
				if _, err := os.Lstat(filepath.Join(root, ref)); err != nil {
					t.Fatal("substituted or unrelated data removed")
				}
			} else {
				if err := RetirePlannedProtected(root, ref); err != nil {
					t.Fatalf("repeated cleanup: %v", err)
				}
				if _, err := os.Lstat(filepath.Join(root, ref)); !os.IsNotExist(err) {
					t.Fatal("protected directory remains")
				}
			}
		})
	}
	if err := RetirePlannedProtected(t.TempDir(), "../other"); err == nil {
		t.Fatal("traversal admitted")
	}
}

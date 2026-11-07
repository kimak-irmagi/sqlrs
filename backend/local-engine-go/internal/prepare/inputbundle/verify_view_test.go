package inputbundle

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// LC05/08: reconnect verifies the same admitted view without recreating it.
// Missing/changed/extra entries are errors; neither CAS nor the view is repaired.
func TestVerifyMaterializedIsReadOnly(t *testing.T) {
	for _, variant := range []string{"control", "changed", "missing_file", "extra_file", "extra_directory", "missing_root", "relative_root", "cancelled_context"} {
		t.Run(variant, func(t *testing.T) {
			ctx := context.Background()
			root := physicalInputTestRoot(t)
			s := Store{Root: filepath.Join(root, "artifacts")}
			ref, err := s.PublishMixed(ctx, nil, []MemoryFile{{Path: "db/input.sql", Data: []byte("original")}})
			if err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(root, "copy")
			if err := s.Materialize(ctx, ref, destination); err != nil {
				t.Fatal(err)
			}
			file := filepath.Join(destination, "db", "input.sql")
			switch variant {
			case "changed":
				if err := os.Chmod(file, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(file, []byte("different"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing_file":
				if err := os.Chmod(file, 0600); err != nil {
					t.Fatal(err)
				}
				if err := os.Remove(file); err != nil {
					t.Fatal(err)
				}
			case "extra_file":
				if err := os.WriteFile(filepath.Join(destination, "extra"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			case "extra_directory":
				if err := os.Mkdir(filepath.Join(destination, "extra"), 0700); err != nil {
					t.Fatal(err)
				}
			case "missing_root":
				destination = filepath.Join(root, "absent")
			case "relative_root":
				destination = "relative-copy"
			case "cancelled_context":
				cancelled, cancel := context.WithCancel(ctx)
				cancel()
				ctx = cancelled
			}
			if err := s.VerifyMaterialized(ctx, ref, destination); (err == nil) != (variant == "control") {
				t.Fatalf("verification=%v", err)
			}
			if variant == "changed" {
				if data, err := os.ReadFile(file); err != nil || string(data) != "different" {
					t.Fatal("verification repaired a live mount", err)
				}
			}
			if variant == "missing_file" {
				if _, err := os.Lstat(file); !os.IsNotExist(err) {
					t.Fatal("verification recreated missing input", err)
				}
			}
			if _, err := s.ReadManifest(ref); err != nil {
				t.Fatal("verification changed immutable artifact", err)
			}
		})
	}
}

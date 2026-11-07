package driverbundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
)

func fixtureDigest(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

// physicalDriverTestRoot resolves only the platform temporary-directory alias;
// deliberately created input symlinks remain subject to production checks.
func physicalDriverTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPublishAndRestoreVerifiedPackagedDriver(t *testing.T) {
	root := physicalDriverTestRoot(t)
	exe := filepath.Join(root, "sqlrs-engine")
	if err := os.WriteFile(exe, []byte("engine"), 0o700); err != nil {
		t.Fatal(err)
	}
	packageDir := filepath.Join(root, "lib")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	jar := filepath.Join(packageDir, DriverFileName)
	data := []byte("fixed driver fixture")
	if err := os.WriteFile(jar, data, 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(root, "state"), ExecutablePath: exe, expectedDigest: fixtureDigest(data)}
	binding, err := store.Publish(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if binding.Mode != "mounted" || binding.Digest != fixtureDigest(data) || binding.ArtifactRef == "" {
		t.Fatalf("invalid driver binding: %+v", binding)
	}
	if err := os.WriteFile(jar, []byte("mutated package"), 0o600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "job-copy", "driver", "files")
	if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
		t.Fatal(err)
	}
	materialized, err := store.Materialize(context.Background(), binding, destination)
	if err != nil {
		t.Fatal(err)
	}
	defer materialized.Close()
	if materialized.HostPath != filepath.Join(destination, DriverFileName) {
		t.Fatal("provider ignored the owned destination")
	}
	got, err := os.ReadFile(materialized.HostPath)
	if err != nil || string(got) != string(data) {
		t.Fatalf("restored driver = %q, err=%v", got, err)
	}
	if materialized.ContainerPath != "/liquibase/lib/"+DriverFileName {
		t.Fatalf("container path = %q", materialized.ContainerPath)
	}
	reopened, err := store.OpenMaterialized(context.Background(), binding, destination)
	if err != nil || reopened.HostPath != materialized.HostPath || reopened.ContainerPath != materialized.ContainerPath {
		t.Fatal("reconnect replaced verified driver mount", err)
	}
	if _, err := store.OpenMaterialized(context.Background(), binding, filepath.Join(root, "missing-view")); err == nil {
		t.Fatal("missing driver view was recreated/adopted")
	}
	if _, err := os.Lstat(filepath.Join(root, "missing-view")); !os.IsNotExist(err) {
		t.Fatal("driver reopen created files", err)
	}
	manifest, err := (inputbundle.Store{Root: store.Root}).ReadManifest(inputbundle.BundleRef{ManifestDigest: binding.ArtifactRef})
	if err != nil {
		t.Fatal(err)
	}
	blob := filepath.Join(store.Root, "blobs", manifest.Files[0].Digest[7:])
	if err := os.WriteFile(blob, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	retryDestination := filepath.Join(root, "retry-copy", "driver", "files")
	if err := os.MkdirAll(filepath.Dir(retryDestination), 0700); err != nil {
		t.Fatal(err)
	}
	if partial, err := store.Materialize(context.Background(), binding, retryDestination); err == nil || partial == nil {
		t.Fatal("corrupt stored driver was accepted")
	}
}

// LC08: a caller, not the immutable artifact store, owns the disposable view.
// Rejected destinations cannot be adopted, overwritten or later erased by Close.
func TestMaterializeRequiresDisjointOwnedDestination(t *testing.T) {
	for _, name := range []string{"relative", "store", "store_child", "store_ancestor", "existing_directory", "existing_file", "missing_binding"} {
		t.Run(name, func(t *testing.T) {
			root := physicalDriverTestRoot(t)
			store := Store{Root: filepath.Join(root, "artifacts"), expectedDigest: fixtureDigest([]byte("driver"))}
			if err := os.MkdirAll(store.Root, 0700); err != nil {
				t.Fatal(err)
			}
			sentinel := filepath.Join(store.Root, "keep")
			if err := os.WriteFile(sentinel, []byte("immutable"), 0600); err != nil {
				t.Fatal(err)
			}
			source := filepath.Join(root, "source.jar")
			if err := os.WriteFile(source, []byte("driver"), 0600); err != nil {
				t.Fatal(err)
			}
			ref, err := (inputbundle.Store{Root: store.Root}).Publish(context.Background(), []inputbundle.File{{Path: DriverFileName, Source: source}})
			if err != nil {
				t.Fatal(err)
			}
			destination := filepath.Join(root, "copy", "driver", "files")
			binding := Binding{Mode: "mounted", Digest: store.expectedDigest, ArtifactRef: ref.ManifestDigest}
			switch name {
			case "relative":
				destination = "relative-driver"
			case "store":
				destination = store.Root
			case "store_child":
				destination = filepath.Join(store.Root, "view")
			case "store_ancestor":
				destination = root
			case "existing_directory":
				if err := os.MkdirAll(destination, 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(destination, "foreign"), []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			case "existing_file":
				if err := os.MkdirAll(filepath.Dir(destination), 0700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(destination, []byte("keep"), 0600); err != nil {
					t.Fatal(err)
				}
			case "missing_binding":
				binding.ArtifactRef = "missing"
			}
			handle, err := store.Materialize(context.Background(), binding, destination)
			if err == nil || handle != nil {
				t.Fatal("rejected destination received cleanup authority", err)
			}
			if handle, err := store.OpenMaterialized(context.Background(), binding, destination); err == nil || handle != nil {
				t.Fatal("rejected retained destination received cleanup authority", err)
			}
			if got, err := os.ReadFile(sentinel); err != nil || string(got) != "immutable" {
				t.Fatal("artifact store changed", err)
			}
			if name == "existing_directory" {
				if got, err := os.ReadFile(filepath.Join(destination, "foreign")); err != nil || string(got) != "keep" {
					t.Fatal("existing payload changed", err)
				}
			}
		})
	}
}

// LC07/08: a failed Close retains the exact locator and checks the complete
// directory before deleting anything. Repairing ambiguity makes the same
// handle retryable; an independent artifact/sibling is never removed.
func TestMaterializedCloseRetainsLocatorAndRejectsAmbiguity(t *testing.T) {
	root := physicalDriverTestRoot(t)
	destination := filepath.Join(root, "driver")
	if err := os.Mkdir(destination, 0700); err != nil {
		t.Fatal(err)
	}
	jar := filepath.Join(destination, DriverFileName)
	if err := os.WriteFile(jar, []byte("copy"), 0400); err != nil {
		t.Fatal(err)
	}
	foreign := filepath.Join(destination, "foreign")
	if err := os.WriteFile(foreign, []byte("unknown"), 0600); err != nil {
		t.Fatal(err)
	}
	sibling := filepath.Join(root, "artifact")
	if err := os.WriteFile(sibling, []byte("keep"), 0600); err != nil {
		t.Fatal(err)
	}
	handle := &Materialized{root: destination, HostPath: jar, ContainerPath: driverMountPath}
	if err := handle.Close(); err == nil {
		t.Fatal("ambiguous directory was removed")
	}
	if handle.root != destination {
		t.Fatal("failed Close lost locator")
	}
	if got, err := os.ReadFile(jar); err != nil || string(got) != "copy" {
		t.Fatal("partial cleanup occurred before namespace validation", err)
	}
	if err := os.Remove(foreign); err != nil {
		t.Fatal(err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(destination); !os.IsNotExist(err) {
		t.Fatal("owned driver remains", err)
	}
	if err := handle.Close(); err != nil {
		t.Fatal("repeated Close", err)
	}
	if got, err := os.ReadFile(sibling); err != nil || string(got) != "keep" {
		t.Fatal("sibling was removed", err)
	}
}

func TestDriverPublicationRejectsChecksumAndSymlink(t *testing.T) {
	root := physicalDriverTestRoot(t)
	exe := filepath.Join(root, "sqlrs-engine")
	if err := os.WriteFile(exe, []byte("engine"), 0o700); err != nil {
		t.Fatal(err)
	}
	packageDir := filepath.Join(root, "lib")
	if err := os.MkdirAll(packageDir, 0o700); err != nil {
		t.Fatal(err)
	}
	jar := filepath.Join(packageDir, DriverFileName)
	if err := os.WriteFile(jar, []byte("wrong"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(root, "state"), ExecutablePath: exe, expectedDigest: fixtureDigest([]byte("expected"))}
	if _, err := store.Publish(context.Background()); err == nil {
		t.Fatal("wrong packaged checksum was accepted")
	}
	if err := os.Remove(jar); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "external.jar")
	if err := os.WriteFile(target, []byte("expected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, jar); err == nil {
		if _, err := store.Publish(context.Background()); err == nil {
			t.Fatal("symlinked package driver was accepted")
		}
	}
}

func TestLocatePackagedDriverBesideWSLEngine(t *testing.T) {
	root := physicalDriverTestRoot(t)
	exe := filepath.Join(root, "libexec", "linux-amd64", "sqlrs-engine")
	if err := os.MkdirAll(filepath.Dir(exe), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(exe, []byte("engine"), 0o700); err != nil {
		t.Fatal(err)
	}
	jar := filepath.Join(root, "lib", DriverFileName)
	if err := os.MkdirAll(filepath.Dir(jar), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(jar, []byte("driver"), 0o600); err != nil {
		t.Fatal(err)
	}
	got, err := locatePackaged(exe)
	if err != nil || got != jar {
		t.Fatalf("packaged driver path = %q, err=%v", got, err)
	}
}

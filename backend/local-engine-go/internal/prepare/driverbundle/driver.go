// Package driverbundle binds the release-supplied PostgreSQL JDBC driver to
// a verified, recoverable artifact. See
// docs/architecture/liquibase-container-component-structure.md.
package driverbundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
)

const (
	DriverFileName  = "postgresql-42.7.13.jar"
	DriverDigest    = "sha256:6e0e4cc2d8cae902084f8a2b18728b073a6fd9d1f87c9d8bff8f298c18185b93"
	driverMountPath = "/liquibase/lib/" + DriverFileName
)

// Binding contains only durable, non-secret driver identity. ArtifactRef is
// the content-addressed store manifest, never a path in a release bundle.
type Binding struct {
	Mode        string
	Digest      string
	ArtifactRef string
}

// Store copies the verified release dependency into durable engine storage.
// Package bytes are not read again when a bound job resumes after a restart.
type Store struct {
	Root           string
	ExecutablePath string
	expectedDigest string
}

// New creates the production store for the locked pgJDBC Maven artifact.
func New(root string, executablePath string) Store {
	return Store{Root: root, ExecutablePath: executablePath, expectedDigest: DriverDigest}
}

// Publish validates the release JAR, publishes its bytes, then validates the
// resulting stored identity before a job may bind it.
func (s Store) Publish(ctx context.Context) (Binding, error) {
	if s.Root == "" {
		return Binding{}, fmt.Errorf("driver artifact store is required")
	}
	packaged, err := locatePackaged(s.ExecutablePath)
	if err != nil {
		return Binding{}, err
	}
	if err := verifyFile(packaged, s.expectedDigest); err != nil {
		return Binding{}, fmt.Errorf("packaged PostgreSQL JDBC driver: %w", err)
	}
	store := inputbundle.Store{Root: s.Root}
	ref, err := store.Publish(ctx, []inputbundle.File{{Path: DriverFileName, Source: packaged}})
	if err != nil {
		return Binding{}, err
	}
	manifest, err := store.ReadManifest(ref)
	if err != nil || len(manifest.Files) != 1 || manifest.Files[0].Digest != s.expectedDigest {
		return Binding{}, fmt.Errorf("published PostgreSQL JDBC driver digest mismatch")
	}
	return Binding{Mode: "mounted", Digest: s.expectedDigest, ArtifactRef: ref.ManifestDigest}, nil
}

// Materialized is a temporary, verified filesystem view of a bound driver.
// Its HostPath must be mounted read-only and Close must be called afterward.
type Materialized struct {
	HostPath      string
	ContainerPath string
	root          string
}

// Close removes the exact temporary tree created by Materialize.
func (m *Materialized) Close() error {
	if m == nil || m.root == "" {
		return nil
	}
	if err := driverProjectionParents(filepath.Dir(m.root)); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return fmt.Errorf("driver projection parents are unavailable")
	}
	// The locator is deliberately immutable. Failed or interrupted removal must
	// remain retryable, including file removed + directory still present (LC07).
	info, err := os.Lstat(m.root)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("driver projection directory is unavailable")
	}
	entries, err := os.ReadDir(m.root)
	if err != nil {
		return fmt.Errorf("cannot inspect driver projection")
	}
	for _, entry := range entries {
		info, err := os.Lstat(filepath.Join(m.root, entry.Name()))
		if entry.Name() != DriverFileName || err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("driver projection contains unrecognized material")
		}
	}
	if len(entries) != 0 {
		file := filepath.Join(m.root, DriverFileName)
		if err := os.Chmod(file, 0600); err != nil {
			return fmt.Errorf("cannot prepare driver projection removal")
		}
		if err := os.Remove(file); err != nil {
			return fmt.Errorf("cannot remove driver projection file")
		}
	}
	if err := os.Remove(m.root); err != nil {
		return fmt.Errorf("cannot remove driver projection directory")
	}
	return nil
}

// Materialize restores a saved binding without consulting the release JAR or
// a mutable tag. Corrupt or missing bytes abort before container creation.
func (s Store) Materialize(ctx context.Context, binding Binding, destination string) (*Materialized, error) {
	if s.Root == "" || binding.Mode != "mounted" || binding.Digest != s.expectedDigest || binding.ArtifactRef == "" {
		return nil, fmt.Errorf("unsupported PostgreSQL JDBC driver binding")
	}
	// LC08: the job owner chooses an exact disposable destination outside CAS.
	// Never adopt an existing directory, including a live executor's old mount.
	if err := validateDriverProjectionDestination(s.Root, destination); err != nil {
		return nil, err
	}
	destination = filepath.Clean(destination)
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("driver projection destination is not new")
	}
	store := inputbundle.Store{Root: s.Root}
	ref := inputbundle.BundleRef{ManifestDigest: binding.ArtifactRef}
	manifest, err := store.ReadManifest(ref)
	if err != nil || len(manifest.Files) != 1 || manifest.Files[0].Path != DriverFileName || manifest.Files[0].Digest != binding.Digest {
		return nil, fmt.Errorf("invalid PostgreSQL JDBC artifact reference")
	}
	// The job owner allocates the protected parent before invoking providers.
	// Do not create parents through an unchecked ancestor on its behalf.
	if err := driverProjectionParents(filepath.Dir(destination)); err != nil {
		return nil, fmt.Errorf("driver projection parents are unavailable")
	}
	hostPath := filepath.Join(destination, DriverFileName)
	materialized := &Materialized{HostPath: hostPath, ContainerPath: driverMountPath, root: destination}
	if err := store.Materialize(ctx, ref, destination); err != nil {
		return materialized, err
	}
	if err := verifyFile(hostPath, binding.Digest); err != nil {
		return materialized, err
	}
	return materialized, nil
}

// OpenMaterialized verifies the exact existing mount without recreating it or
// consulting the release JAR. Saved live-container mounts cannot be replaced
// during reconnect (projection-cleanup design, LC05/08).
func (s Store) OpenMaterialized(ctx context.Context, binding Binding, destination string) (*Materialized, error) {
	if s.Root == "" || binding.Mode != "mounted" || binding.Digest != s.expectedDigest || binding.ArtifactRef == "" || !filepath.IsAbs(destination) {
		return nil, fmt.Errorf("unsupported retained PostgreSQL JDBC binding")
	}
	if err := validateDriverProjectionDestination(s.Root, destination); err != nil {
		return nil, err
	}
	store := inputbundle.Store{Root: s.Root}
	ref := inputbundle.BundleRef{ManifestDigest: binding.ArtifactRef}
	manifest, err := store.ReadManifest(ref)
	if err != nil || len(manifest.Files) != 1 || manifest.Files[0].Path != DriverFileName || manifest.Files[0].Digest != binding.Digest {
		return nil, fmt.Errorf("invalid retained PostgreSQL JDBC artifact")
	}
	if err := store.VerifyMaterialized(ctx, ref, destination); err != nil {
		return nil, err
	}
	return &Materialized{HostPath: filepath.Join(destination, DriverFileName), ContainerPath: driverMountPath, root: destination}, nil
}

func validateDriverProjectionDestination(artifactRoot, destination string) error {
	if !filepath.IsAbs(artifactRoot) || !filepath.IsAbs(destination) {
		return fmt.Errorf("absolute driver projection destination is required")
	}
	for _, pair := range [][2]string{{artifactRoot, destination}, {destination, artifactRoot}} {
		rel, err := filepath.Rel(pair[0], pair[1])
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(os.PathSeparator)) {
			return fmt.Errorf("driver projection must be disjoint from artifact storage")
		}
	}
	return nil
}

// Recheck parent substitution before each cleanup; a retained handle does not
// authorize following a newly introduced link into somebody else's directory.
func driverProjectionParents(directory string) error {
	if !filepath.IsAbs(directory) {
		return fmt.Errorf("absolute driver projection parent is required")
	}
	parent := filepath.Dir(directory)
	if parent != directory {
		if err := driverProjectionParents(parent); err != nil {
			return err
		}
	}
	info, err := os.Lstat(directory)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("driver projection parent is not a plain directory")
	}
	return nil
}

func locatePackaged(executablePath string) (string, error) {
	if strings.TrimSpace(executablePath) == "" {
		return "", fmt.Errorf("engine executable path is required for JDBC dependency")
	}
	abs, err := filepath.Abs(executablePath)
	if err != nil {
		return "", err
	}
	dir := filepath.Dir(abs)
	candidates := []string{filepath.Join(dir, "lib", DriverFileName)}
	if filepath.Base(filepath.Dir(dir)) == "libexec" {
		candidates = append(candidates, filepath.Join(filepath.Dir(filepath.Dir(dir)), "lib", DriverFileName))
	}
	for _, candidate := range candidates {
		info, err := os.Lstat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return candidate, nil
		}
	}
	return "", fmt.Errorf("packaged PostgreSQL JDBC dependency is missing")
}

func verifyFile(path string, expected string) error {
	if !strings.HasPrefix(expected, "sha256:") || len(expected) != 71 {
		return fmt.Errorf("invalid driver checksum")
	}
	file, err := os.Open(path)
	if err != nil {
		return err
	}
	defer file.Close()
	hasher := sha256.New()
	if _, err := io.Copy(hasher, file); err != nil {
		return err
	}
	actual := "sha256:" + hex.EncodeToString(hasher.Sum(nil))
	if actual != expected {
		return fmt.Errorf("driver checksum mismatch")
	}
	return nil
}

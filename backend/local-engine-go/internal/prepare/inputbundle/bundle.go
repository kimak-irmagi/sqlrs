// Package inputbundle stores immutable container input bytes independently of
// the client's filesystem. It implements the local source boundaries in
// docs/architecture/liquibase-container-component-structure.md and
// docs/architecture/transformation-attempt-recovery-structure.md.
package inputbundle

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"regexp"
	"strings"
)

var digestPattern = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// File identifies one local source and its stable, slash-separated name in
// the bundle. The order of Files supplied to Publish is part of the manifest.
type File struct {
	Path   string
	Source string
}

// MemoryFile captures bytes already admitted in the request, such as psql
// stdin. Its Data is copied to the content-addressed store before return.
type MemoryFile struct {
	Path string
	Data []byte
}

// Entry is the immutable content identity of one admitted input file.
type Entry struct {
	Path   string `json:"path"`
	Digest string `json:"digest"`
	Size   int64  `json:"size"`
}

// Manifest preserves the declared order and presence of admitted files.
type Manifest struct {
	Files       []Entry  `json:"files"`
	Directories []string `json:"directories,omitempty"`
}

// MountedRoot maps one admitted host directory to its stable, slash-separated
// mount name. Its complete directory listing is captured, including empties.
type MountedRoot struct {
	Path   string
	Source string
}

// BundleRef is a content-addressed reference, not a filesystem path.
type BundleRef struct {
	ManifestDigest string
}

// Store owns verified source bytes beneath Root. A caller must hold a durable
// lease on a returned BundleRef before planning or executing a job.
type Store struct {
	Root string
}

// Publish captures files, checks that sources did not change during capture,
// and publishes the manifest only after every referenced blob is available.
func (s Store) Publish(ctx context.Context, files []File) (BundleRef, error) {
	return s.publish(ctx, files, nil, nil)
}

// PublishMixed puts file sources and in-memory input in one immutable
// manifest, so a tracked executor can mount exactly one bound input view.
func (s Store) PublishMixed(ctx context.Context, files []File, memory []MemoryFile) (BundleRef, error) {
	return s.publish(ctx, files, memory, nil)
}

// PublishRoots captures complete directory views for the local container
// boundary. A second listing prevents a directory changing during admission
// from being silently represented by only part of its entries.
func (s Store) PublishRoots(ctx context.Context, roots []MountedRoot) (BundleRef, error) {
	files, directories, err := listMountedRoots(ctx, roots)
	if err != nil {
		return BundleRef{}, err
	}
	ref, err := s.publish(ctx, files, nil, directories)
	if err != nil {
		return BundleRef{}, err
	}
	afterFiles, afterDirectories, err := listMountedRoots(ctx, roots)
	if err != nil || !reflect.DeepEqual(files, afterFiles) || !reflect.DeepEqual(directories, afterDirectories) {
		return BundleRef{}, fmt.Errorf("Liquibase input directory changed during capture")
	}
	return ref, nil
}

func listMountedRoots(ctx context.Context, roots []MountedRoot) ([]File, []string, error) {
	if len(roots) == 0 {
		return nil, nil, fmt.Errorf("at least one mounted input root is required")
	}
	var files []File
	var directories []string
	for _, root := range roots {
		if !validPath(root.Path) {
			return nil, nil, fmt.Errorf("invalid mounted input path %q", root.Path)
		}
		absolute, err := filepath.Abs(root.Source)
		if err != nil {
			return nil, nil, err
		}
		if err := rejectSymlinks(absolute); err != nil {
			return nil, nil, err
		}
		info, err := os.Lstat(absolute)
		if err != nil || !info.IsDir() {
			return nil, nil, fmt.Errorf("mounted Liquibase input is not a directory: %q", root.Source)
		}
		err = filepath.WalkDir(absolute, func(source string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if err := ctx.Err(); err != nil {
				return err
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("mounted Liquibase input contains symlink: %q", source)
			}
			relative, err := filepath.Rel(absolute, source)
			if err != nil {
				return err
			}
			logical := root.Path
			if relative != "." {
				logical += "/" + filepath.ToSlash(relative)
			}
			if !validPath(logical) {
				return fmt.Errorf("invalid mounted input path %q", logical)
			}
			if entry.IsDir() {
				directories = append(directories, logical)
				return nil
			}
			if !entry.Type().IsRegular() {
				return fmt.Errorf("mounted Liquibase input is not a regular file: %q", source)
			}
			files = append(files, File{Path: logical, Source: source})
			return nil
		})
		if err != nil {
			return nil, nil, err
		}
	}
	return files, directories, nil
}

func (s Store) publish(ctx context.Context, files []File, memory []MemoryFile, directories []string) (BundleRef, error) {
	if strings.TrimSpace(s.Root) == "" || len(files)+len(memory)+len(directories) == 0 {
		return BundleRef{}, fmt.Errorf("bundle store and at least one file are required")
	}
	if err := os.MkdirAll(filepath.Join(s.Root, "blobs"), 0o700); err != nil {
		return BundleRef{}, err
	}
	if err := os.MkdirAll(filepath.Join(s.Root, "manifests"), 0o700); err != nil {
		return BundleRef{}, err
	}
	manifest := Manifest{Files: make([]Entry, 0, len(files)+len(memory)), Directories: directories}
	seen := map[string]bool{}
	for _, directory := range directories {
		if !validPath(directory) || seen[strings.ToLower(directory)] {
			return BundleRef{}, fmt.Errorf("invalid or duplicate bundle directory %q", directory)
		}
		seen[strings.ToLower(directory)] = true
	}
	for _, file := range files {
		if err := ctx.Err(); err != nil {
			return BundleRef{}, err
		}
		if !validPath(file.Path) || seen[strings.ToLower(file.Path)] {
			return BundleRef{}, fmt.Errorf("invalid or duplicate bundle path %q", file.Path)
		}
		seen[strings.ToLower(file.Path)] = true
		entry, err := s.capture(ctx, file)
		if err != nil {
			return BundleRef{}, err
		}
		manifest.Files = append(manifest.Files, entry)
	}
	for _, file := range memory {
		if err := ctx.Err(); err != nil {
			return BundleRef{}, err
		}
		if !validPath(file.Path) || seen[strings.ToLower(file.Path)] {
			return BundleRef{}, fmt.Errorf("invalid or duplicate bundle path %q", file.Path)
		}
		seen[strings.ToLower(file.Path)] = true
		digest := hash(file.Data)
		if err := publishBytes(s.blobPath(digest), file.Data); err != nil {
			return BundleRef{}, err
		}
		manifest.Files = append(manifest.Files, Entry{Path: file.Path, Digest: digest, Size: int64(len(file.Data))})
	}
	bytes, err := json.Marshal(manifest)
	if err != nil {
		return BundleRef{}, err
	}
	digest := hash(bytes)
	if err := publishBytes(filepath.Join(s.Root, "manifests", digest[7:]+".json"), bytes); err != nil {
		return BundleRef{}, err
	}
	return BundleRef{ManifestDigest: digest}, nil
}

// ReadManifest verifies the immutable manifest and its path/digest grammar.
func (s Store) ReadManifest(ref BundleRef) (Manifest, error) {
	if !digestPattern.MatchString(ref.ManifestDigest) {
		return Manifest{}, fmt.Errorf("invalid bundle digest")
	}
	data, err := os.ReadFile(filepath.Join(s.Root, "manifests", ref.ManifestDigest[7:]+".json"))
	if err != nil {
		return Manifest{}, err
	}
	if hash(data) != ref.ManifestDigest {
		return Manifest{}, fmt.Errorf("bundle manifest digest mismatch")
	}
	var manifest Manifest
	if err := json.Unmarshal(data, &manifest); err != nil || len(manifest.Files)+len(manifest.Directories) == 0 {
		return Manifest{}, fmt.Errorf("invalid bundle manifest")
	}
	seen := map[string]bool{}
	for _, directory := range manifest.Directories {
		if !validPath(directory) || seen[strings.ToLower(directory)] {
			return Manifest{}, fmt.Errorf("invalid bundle manifest directory")
		}
		seen[strings.ToLower(directory)] = true
	}
	for _, entry := range manifest.Files {
		if !validPath(entry.Path) || seen[strings.ToLower(entry.Path)] || !digestPattern.MatchString(entry.Digest) || entry.Size < 0 {
			return Manifest{}, fmt.Errorf("invalid bundle manifest entry")
		}
		seen[strings.ToLower(entry.Path)] = true
	}
	return manifest, nil
}

// Materialize verifies every blob before publishing a fresh read-only tree.
// Destination must not exist; no client source path is accessed here.
func (s Store) Materialize(ctx context.Context, ref BundleRef, destination string) error {
	manifest, err := s.ReadManifest(ref)
	if err != nil {
		return err
	}
	if destination == "" {
		return fmt.Errorf("materialization destination is required")
	}
	if _, err := os.Lstat(destination); err == nil || !os.IsNotExist(err) {
		return fmt.Errorf("materialization destination already exists or is inaccessible")
	}
	parent := filepath.Dir(destination)
	if err := os.MkdirAll(parent, 0o700); err != nil {
		return err
	}
	tmp, err := os.MkdirTemp(parent, ".bundle-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(tmp)
	// Docker bind mounts expose the mounted inode without its private host
	// parent. The Liquibase image runs as an unprivileged UID, so the mounted
	// root and files must be readable by that UID; the mount itself is read-only.
	if err := os.Chmod(tmp, 0o755); err != nil {
		return err
	}
	for _, directory := range manifest.Directories {
		if err := os.MkdirAll(filepath.Join(tmp, filepath.FromSlash(directory)), 0o755); err != nil {
			return err
		}
	}
	for _, entry := range manifest.Files {
		if err := ctx.Err(); err != nil {
			return err
		}
		data, err := os.ReadFile(s.blobPath(entry.Digest))
		if err != nil {
			return err
		}
		if hash(data) != entry.Digest || int64(len(data)) != entry.Size {
			return fmt.Errorf("bundle blob digest or size mismatch for %q", entry.Path)
		}
		filePath := filepath.Join(tmp, filepath.FromSlash(entry.Path))
		if err := os.MkdirAll(filepath.Dir(filePath), 0o755); err != nil {
			return err
		}
		if err := os.WriteFile(filePath, data, 0o444); err != nil {
			return err
		}
		if err := os.Chmod(filePath, 0o444); err != nil {
			return err
		}
	}
	return os.Rename(tmp, destination)
}

func (s Store) capture(ctx context.Context, file File) (Entry, error) {
	source, err := filepath.Abs(file.Source)
	if err != nil {
		return Entry{}, err
	}
	if err := rejectSymlinks(source); err != nil {
		return Entry{}, err
	}
	before, err := os.Lstat(source)
	if err != nil || !before.Mode().IsRegular() {
		return Entry{}, fmt.Errorf("bundle source is not a regular file: %q", file.Source)
	}
	f, err := os.Open(source)
	if err != nil {
		return Entry{}, err
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		return Entry{}, err
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	after, err := os.Lstat(source)
	if err != nil || !os.SameFile(before, after) || before.Size() != after.Size() || !before.ModTime().Equal(after.ModTime()) || int64(len(data)) != before.Size() {
		return Entry{}, fmt.Errorf("bundle source changed during capture: %q", file.Source)
	}
	second, err := os.ReadFile(source)
	if err != nil || hash(second) != hash(data) {
		return Entry{}, fmt.Errorf("bundle source changed during capture: %q", file.Source)
	}
	digest := hash(data)
	if err := publishBytes(s.blobPath(digest), data); err != nil {
		return Entry{}, err
	}
	return Entry{Path: file.Path, Digest: digest, Size: int64(len(data))}, nil
}

func (s Store) blobPath(digest string) string {
	return filepath.Join(s.Root, "blobs", digest[7:])
}

func validPath(value string) bool {
	if value == "" || strings.ContainsAny(value, "\\:\"*?<>|\x00\r\n\t") || strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return false
	}
	for _, component := range strings.Split(value, "/") {
		if component == "" || component == "." || component == ".." || strings.HasSuffix(component, ".") || strings.HasSuffix(component, " ") {
			return false
		}
		base := strings.ToUpper(strings.SplitN(component, ".", 2)[0])
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" ||
			len(base) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && base[3] >= '1' && base[3] <= '9' {
			return false
		}
	}
	return true
}

func rejectSymlinks(source string) error {
	for current := source; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("bundle source contains symlink: %q", source)
		}
		if parent := filepath.Dir(current); parent == current {
			return nil
		}
	}
}

func publishBytes(destination string, data []byte) error {
	if existing, err := os.ReadFile(destination); err == nil {
		if hash(existing) != hash(data) {
			return fmt.Errorf("existing bundle artifact is corrupt")
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(destination), ".artifact-*")
	if err != nil {
		return err
	}
	defer os.Remove(tmp.Name())
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	// A hard link publishes the completed inode without ever replacing an
	// existing artifact. Rename would overwrite it on Unix in a race.
	if err := os.Link(tmp.Name(), destination); err != nil {
		if existing, readErr := os.ReadFile(destination); readErr == nil && hash(existing) == hash(data) {
			return nil
		}
		return err
	}
	return nil
}

func hash(data []byte) string {
	sum := sha256.Sum256(data)
	return "sha256:" + hex.EncodeToString(sum[:])
}

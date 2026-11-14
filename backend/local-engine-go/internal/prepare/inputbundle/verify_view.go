package inputbundle

import (
	"context"
	"fmt"
	"os"
	"path"
	"path/filepath"
)

// VerifyMaterialized reads a retained projection against its admitted manifest
// without copying, replacing or repairing anything. It rejects missing, changed
// and extra entries, including links. The caller separately proves the saved
// job/copy locator before this content check (projection-cleanup design, LC08).
func (s Store) VerifyMaterialized(ctx context.Context, ref BundleRef, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(destination) || rejectSymlinks(destination) != nil {
		return fmt.Errorf("retained input projection is unavailable")
	}
	manifest, err := s.ReadManifest(ref)
	if err != nil {
		return err
	}
	files := make(map[string]Entry, len(manifest.Files))
	directories := map[string]bool{".": true}
	addParents := func(name string) {
		for name != "." {
			directories[name] = true
			name = path.Dir(name)
		}
	}
	for _, directory := range manifest.Directories {
		addParents(directory)
	}
	for _, file := range manifest.Files {
		files[file.Path] = file
		addParents(path.Dir(file.Path))
	}
	seen := 0
	err = filepath.WalkDir(destination, func(local string, entry os.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if walkErr != nil {
			return fmt.Errorf("cannot inspect retained input projection")
		}
		relative, err := filepath.Rel(destination, local)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(relative)
		info, err := os.Lstat(local)
		if err != nil || info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("retained input projection entry is unavailable")
		}
		if info.IsDir() {
			if !directories[name] {
				return fmt.Errorf("retained input projection has an extra directory")
			}
			return nil
		}
		expected, exists := files[name]
		if !exists || !info.Mode().IsRegular() || info.Size() != expected.Size {
			return fmt.Errorf("retained input projection differs")
		}
		data, err := os.ReadFile(local)
		if err != nil || hash(data) != expected.Digest {
			return fmt.Errorf("retained input projection bytes differ")
		}
		seen++
		return nil
	})
	if err != nil {
		return err
	}
	if seen != len(files) {
		return fmt.Errorf("retained input projection has missing files")
	}
	// Empty directories are part of PublishRoots too, not optional decoration.
	for directory := range directories {
		info, err := os.Lstat(filepath.Join(destination, filepath.FromSlash(directory)))
		if err != nil || !info.IsDir() {
			return fmt.Errorf("retained input projection has missing directories")
		}
	}
	return nil
}

package effectiveconfig

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
)

// This low-level module accepts standalone plan refs and copy-qualified refs.
// Production local planners/sessions require the latter before calling here.
var plannedRefPattern = regexp.MustCompile(`^(copy-[0-9a-f]{32}-)?plan-[0-9a-f]{24}-input$`)

// CreatePlannedProtected materializes settings at the exact journal-derived
// logical reference after that reference has been committed. It never
// overwrites an existing credential: recovery must inspect the saved attempt
// before deciding whether any new file may be created. See P01/F03a in the
// Liquibase planning recovery design.
func CreatePlannedProtected(options ProtectedOptions, ref string) (*ProtectedConfig, error) {
	if !plannedRefPattern.MatchString(ref) {
		return nil, fmt.Errorf("invalid planned protected reference")
	}
	root := filepath.Clean(options.Root)
	if !filepath.IsAbs(root) {
		return nil, fmt.Errorf("absolute protected root is required")
	}
	temporary, err := CreateProtected(options)
	if err != nil {
		return nil, err
	}
	destination := filepath.Join(root, ref)
	if err := os.Mkdir(destination, 0o700); err != nil {
		_ = temporary.Close()
		return nil, fmt.Errorf("cannot reserve planned protected reference: %w", err)
	}
	file := filepath.Join(destination, "liquibase.properties")
	if err := os.Rename(temporary.HostPath, file); err != nil {
		_ = temporary.Close()
		_ = os.Remove(destination)
		return nil, fmt.Errorf("cannot publish planned protected settings: %w", err)
	}
	if err := os.Remove(temporary.directory); err != nil {
		_ = os.Remove(file)
		_ = os.Remove(destination)
		return nil, fmt.Errorf("cannot retire temporary protected settings: %w", err)
	}
	temporary.directory = destination
	temporary.HostPath = file
	return temporary, nil
}

// OpenPlannedProtected returns only the exact existing regular file bound to
// a saved logical ref. Missing or substituted material is not recreated from
// current credentials during uncertain-attempt recovery.
func OpenPlannedProtected(root, ref string) (*ProtectedConfig, error) {
	if !filepath.IsAbs(root) || !plannedRefPattern.MatchString(ref) {
		return nil, fmt.Errorf("invalid planned protected reference")
	}
	directory := filepath.Join(filepath.Clean(root), ref)
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("planned protected directory is unavailable")
	}
	file := filepath.Join(directory, "liquibase.properties")
	info, err = os.Lstat(file)
	if err != nil || !info.Mode().IsRegular() || info.Mode()&os.ModeSymlink != 0 {
		return nil, fmt.Errorf("planned protected settings are unavailable")
	}
	return &ProtectedConfig{HostPath: file, ContainerPath: protectedContainerPath, directory: directory}, nil
}

// RetirePlannedProtected resumes deletion of the exact saved configuration
// after executor absence has been proved. Unlike OpenPlannedProtected, it
// accepts a file or directory already removed by an interrupted cleanup. It
// never follows substituted paths or recursively deletes unrelated entries.
func RetirePlannedProtected(root, ref string) error {
	if !filepath.IsAbs(root) || !plannedRefPattern.MatchString(ref) {
		return fmt.Errorf("invalid planned protected reference")
	}
	directory := filepath.Join(filepath.Clean(root), ref)
	info, err := os.Lstat(directory)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return fmt.Errorf("planned protected directory is untrusted")
	}
	file := filepath.Join(directory, "liquibase.properties")
	info, err = os.Lstat(file)
	if err == nil {
		if !info.Mode().IsRegular() {
			return fmt.Errorf("planned protected settings are untrusted")
		}
		if err := os.Remove(file); err != nil {
			return err
		}
	} else if !os.IsNotExist(err) {
		return err
	}
	return os.Remove(directory)
}

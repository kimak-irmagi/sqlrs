package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
)

var (
	// ErrManagedResourcesPresent prohibits changing legacy store format while
	// snapshots, work directories, journals, secrets or mounted data remain.
	ErrManagedResourcesPresent = errors.New("managed runtime resources prevent store upgrade")
	// ErrManagedInventoryUnavailable is bounded: do not forward Docker output,
	// local paths, credentials or file contents into startup diagnostics.
	ErrManagedInventoryUnavailable = errors.New("managed runtime inventory unavailable")
	managedContainerID             = regexp.MustCompile(`^[0-9a-f]{64}$`)
)

// CheckEmptyManagedResources performs the physical half of legacy cutover.
// The caller closes admission and retains the store transaction across this
// check and schema installation. No files, containers or secrets are changed.
// Only platform-owned namespaces are inventoried; unrelated settings survive.
// See docs/architecture/managed-database-identity-internals.md, upgrade conditions.
func (r *DockerRuntime) CheckEmptyManagedResources(ctx context.Context, root string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !filepath.IsAbs(root) {
		return ErrManagedInventoryUnavailable
	}
	root = filepath.Clean(root)
	// A symlink/junction in the store path makes mount ownership ambiguous.
	for current := root; ; current = filepath.Dir(current) {
		info, err := os.Lstat(current)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrManagedInventoryUnavailable
		}
		if filepath.Dir(current) == current {
			break
		}
	}
	for _, namespace := range []string{"engines", "jobs", "runtime-journal", "managed-secrets"} {
		resource := filepath.Join(root, namespace)
		info, err := os.Lstat(resource)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return ErrManagedInventoryUnavailable
		}
		entries, err := os.ReadDir(resource)
		if err != nil {
			return ErrManagedInventoryUnavailable
		}
		if len(entries) != 0 {
			return ErrManagedResourcesPresent
		}
	}
	// Use the non-streaming runner directly: inventory diagnostics never enter
	// the recipe log sink attached to a runtime context.
	out, err := r.runner.Run(ctx, r.binary, []string{"container", "ls", "--all", "--quiet", "--no-trunc"}, nil)
	if err != nil {
		return managedInventoryError(ctx, err)
	}
	storePath := managedMountPath(root)
	if storePath == "" {
		return ErrManagedInventoryUnavailable
	}
	for _, id := range strings.Fields(out) {
		if !managedContainerID.MatchString(id) {
			return ErrManagedInventoryUnavailable
		}
		out, err := r.runner.Run(ctx, r.binary, []string{"inspect", "--type", "container", "--format", "{{json .Mounts}}", id}, nil)
		if err != nil {
			return managedInventoryError(ctx, err)
		}
		var mounts []struct{ Type, Source, Destination string }
		if json.Unmarshal([]byte(out), &mounts) != nil || mounts == nil {
			return ErrManagedInventoryUnavailable
		}
		for _, mount := range mounts {
			if mount.Type == "tmpfs" {
				continue
			}
			if mount.Type != "bind" && mount.Type != "volume" {
				return ErrManagedInventoryUnavailable
			}
			if strings.HasPrefix(mount.Source, "/run/desktop/mnt/host/wsl/docker-desktop-bind-mounts/") {
				// Docker hides the original WSL source behind a hash. Without an
				// authoritative mapping, the empty-store claim cannot be made.
				return ErrManagedInventoryUnavailable
			}
			source := managedMountPath(mount.Source)
			if source == "" {
				return ErrManagedInventoryUnavailable
			}
			if mountPathsOverlap(source, storePath) {
				return ErrManagedResourcesPresent
			}
		}
	}
	return ctx.Err()
}

// managedMountPath compares Docker Desktop drive mappings and native paths.
// Windows drive paths fold case; native Linux paths preserve it.
func managedMountPath(value string) string {
	value = strings.ReplaceAll(value, `\`, "/")
	if converted, ok := windowsDrivePathToLinux(value); ok {
		value = converted
	}
	for _, prefix := range []string{"/run/desktop/mnt/host/", "/host_mnt/"} {
		if strings.HasPrefix(value, prefix) {
			value = "/mnt/" + strings.TrimPrefix(value, prefix)
		}
	}
	if !strings.HasPrefix(value, "/") {
		return ""
	}
	value = path.Clean(value)
	if strings.HasPrefix(value, "/mnt/") && len(value) >= 6 && value[5] >= 'A' && value[5] <= 'Z' {
		value = strings.ToLower(value)
	}
	if strings.HasPrefix(value, "/mnt/") && len(value) >= 6 && value[5] >= 'a' && value[5] <= 'z' && (len(value) == 6 || value[6] == '/') {
		value = strings.ToLower(value)
	}
	return value
}

// mountPathsOverlap includes ancestor mounts that expose the whole store.
func mountPathsOverlap(a, b string) bool {
	return a == b || strings.HasPrefix(a, strings.TrimSuffix(b, "/")+"/") || strings.HasPrefix(b, strings.TrimSuffix(a, "/")+"/")
}

func managedInventoryError(ctx context.Context, err error) error {
	if canceled := ctx.Err(); canceled != nil {
		return canceled
	}
	for _, known := range []error{context.Canceled, context.DeadlineExceeded} {
		if errors.Is(err, known) {
			return known
		}
	}
	return ErrManagedInventoryUnavailable
}

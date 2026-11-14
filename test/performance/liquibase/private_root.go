package benchmark

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// cleanupPrivateRoot removes only a nonce-marked test root after Docker-owner
// absence was proved by the caller. Container-created PG files may not be
// traversable by the hosted UID; a pinned helper deletes them without chmod or
// reading their contents. PA09: no outside-root removal, symlink following,
// global prune or success after an unconfirmed helper retirement.
func cleanupPrivateRoot(ctx context.Context, root, token, image string, run dockerE2ECommand) (returned error) {
	if err := ctx.Err(); err != nil {
		return err
	}
	validHex := func(value string) bool { _, err := hex.DecodeString(value); return len(value) == 64 && err == nil }
	if !filepath.IsAbs(root) || filepath.Dir(root) == root || !validHex(token) || !strings.Contains(image, "@sha256:") || !validHex(strings.Split(image, "@sha256:")[1]) || run == nil {
		return fmt.Errorf("private cleanup target unproved")
	}
	resolved, err := filepath.EvalSymlinks(root)
	if err != nil || resolved != root {
		return fmt.Errorf("private cleanup root differs")
	}
	marker := filepath.Join(root, ".collector-owner")
	info, err := os.Lstat(marker)
	if err != nil || !info.Mode().IsRegular() {
		return fmt.Errorf("private cleanup marker unavailable")
	}
	data, err := os.ReadFile(marker)
	if err != nil || string(data) != token {
		return fmt.Errorf("private cleanup ownership differs")
	}
	output, err := run(ctx, "create", "--network", "none", "--user", "0:0", "--mount", "type=bind,src="+root+",dst=/sqlrs-benchmark-private", "--entrypoint", "find", image,
		"/sqlrs-benchmark-private", "-mindepth", "1", "!", "-path", "/sqlrs-benchmark-private/.collector-owner", "-delete")
	id := strings.TrimSpace(string(output))
	if err != nil || !validHex(id) {
		return fmt.Errorf("private cleanup helper unavailable")
	}
	defer func() {
		clean, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		_, removeErr := run(clean, "container", "rm", "-f", id)
		remaining, listErr := run(clean, "container", "ls", "-a", "--no-trunc", "--filter", "id="+id, "--format", "{{.ID}}")
		if removeErr != nil || listErr != nil || len(strings.Fields(string(remaining))) != 0 {
			returned = errors.Join(returned, fmt.Errorf("private cleanup helper absence unconfirmed"))
		}
	}()
	if _, err = run(ctx, "start", "-a", id); err != nil {
		return fmt.Errorf("private cleanup helper failed")
	}
	entries, err := os.ReadDir(root)
	if err != nil || len(entries) != 1 || entries[0].Name() != ".collector-owner" {
		return fmt.Errorf("private cleanup files remain")
	}
	return nil
}

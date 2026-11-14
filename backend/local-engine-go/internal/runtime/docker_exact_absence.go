package runtime

import (
	"context"
	"fmt"
	"strings"
)

// VerifyContainerAbsent proves point-in-time absence of one saved full Docker
// ID, independently of its planned name. Only a successful empty all-container
// inventory is proof; any output, command failure or cancellation refuses it.
// This read-only capability never adopts/removes a renamed container or grants
// SQL/replay authority. Requirements: liquibase-exact-container-absence.md,
// EA01–EA03. Call outside SQLite borrowers and eviction exclusion.
func (r *DockerRuntime) VerifyContainerAbsent(ctx context.Context, id string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !createdContainerID.MatchString(id) {
		return fmt.Errorf("invalid full container ID")
	}
	// Inventory does not need host-store mounting or log streaming. Unlike
	// run, this existing command boundary cannot initiate mount preparation.
	output, err := r.runner.Run(ctx, r.binary, []string{"container", "ls", "-a", "--no-trunc", "--filter", "id=" + id, "--format", "{{.ID}}"}, nil)
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if err != nil {
		return fmt.Errorf("exact container inventory unavailable: %w", err)
	}
	// Presence, foreign/truncated IDs, multiple rows and malformed output all
	// refuse absence. Do not echo potentially untrusted command output.
	if strings.TrimSpace(output) != "" {
		return fmt.Errorf("exact container absence is unproved")
	}
	return nil
}

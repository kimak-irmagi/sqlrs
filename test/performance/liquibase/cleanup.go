package benchmark

import (
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
)

type dockerE2ECommand func(context.Context, ...string) ([]byte, error)

// runDockerE2ECommand keeps Docker diagnostics local to the integration test.
func runDockerE2ECommand(ctx context.Context, args ...string) ([]byte, error) {
	output, err := exec.CommandContext(ctx, "docker", args...).CombinedOutput()
	if err != nil {
		return nil, fmt.Errorf("docker operation failed")
	}
	return output, nil
}

// cleanupDockerE2EContainers removes only containers whose exact Docker ID,
// sqlrs runtime name, and bind mount prove ownership by this test's unique
// temporary root. It cannot clean up after a test process killed before Go's
// Cleanup callbacks run; such containers must be handled separately.
func cleanupDockerE2EContainers(ctx context.Context, root string, run dockerE2ECommand) error {
	if !filepath.IsAbs(root) || run == nil {
		return fmt.Errorf("Docker E2E cleanup requires an absolute test root and command runner")
	}
	output, err := run(ctx, "container", "ls", "-a", "--no-trunc", "--filter", "name=sqlrs-", "--format", "{{.ID}}")
	if err != nil {
		return err
	}
	var problems []error
	for _, id := range strings.Fields(string(output)) {
		if len(id) != 64 {
			problems = append(problems, fmt.Errorf("Docker E2E cleanup received an invalid container ID"))
			continue
		}
		if _, err := hex.DecodeString(id); err != nil {
			problems = append(problems, fmt.Errorf("Docker E2E cleanup received an invalid container ID: %w", err))
			continue
		}
		encoded, err := run(ctx, "container", "inspect", id)
		if err != nil {
			// Concurrent tests may have retired their own listed container.
			// Only a successful exact-ID absence query resolves that race;
			// an unavailable daemon or a still-present target remains an error.
			remaining, listErr := run(ctx, "container", "ls", "-a", "--no-trunc", "--filter", "id="+id, "--format", "{{.ID}}")
			if listErr == nil && len(strings.Fields(string(remaining))) == 0 {
				continue
			}
			problems = append(problems, fmt.Errorf("inspect Docker E2E container %s: %w", id, err))
			continue
		}
		var inspected []struct {
			ID     string `json:"Id"`
			Name   string `json:"Name"`
			Mounts []struct {
				Source string `json:"Source"`
			} `json:"Mounts"`
		}
		if err := json.Unmarshal(encoded, &inspected); err != nil || len(inspected) != 1 {
			problems = append(problems, fmt.Errorf("inspect Docker E2E container %s returned invalid metadata", id))
			continue
		}
		container := inspected[0]
		if container.ID != id {
			problems = append(problems, fmt.Errorf("Docker E2E container %s identity differs", id))
			continue
		}
		owned := false
		for _, mount := range container.Mounts {
			if filepath.IsAbs(mount.Source) && isWithin(root, mount.Source) {
				owned = true
				break
			}
		}
		if !owned {
			continue
		}
		if !isDockerE2ERuntimeName(container.Name) {
			problems = append(problems, fmt.Errorf("Docker E2E container %s name differs", id))
			continue
		}
		if _, err := run(ctx, "container", "rm", "-f", id); err != nil {
			problems = append(problems, fmt.Errorf("remove Docker E2E container %s: %w", id, err))
			continue
		}
		remaining, listErr := run(ctx, "container", "ls", "-a", "--no-trunc", "--filter", "id="+id, "--format", "{{.ID}}")
		if listErr != nil || len(strings.Fields(string(remaining))) != 0 {
			problems = append(problems, fmt.Errorf("exact Docker container absence unconfirmed"))
		}
	}
	return errors.Join(problems...)
}

func isDockerE2ERuntimeName(name string) bool {
	for _, prefix := range []string{"/sqlrs-step-", "/sqlrs-db-", "/sqlrs-lb-", "/sqlrs-managed-physical_"} {
		if strings.HasPrefix(name, prefix) {
			return true
		}
	}
	return false
}

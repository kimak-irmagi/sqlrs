package run

import (
	"context"
	"strings"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/store"
)

// execManaged resolves only a verified assigned runtime. Private references go
// to process delivery; no password is included in argv or generic Exec.Env.
func (m *Manager) execManaged(ctx context.Context, entry store.InstanceEntry, runtimeID string, args []string, stdin *string) (string, error) {
	var output string
	err := m.access.Use(ctx, entry.InstanceID, func(binding instanceaccess.AccessBinding, ref instanceaccess.SecretBinding) error {
		if binding.RuntimeRef != runtimeID {
			return instanceaccess.ErrConflict
		}
		inspector, ok := m.runtime.(interface {
			InspectManaged(context.Context, managedidentity.RuntimeBinding) (engineRuntime.Instance, error)
		})
		if !ok {
			return instanceaccess.ErrUnavailable
		}
		if _, err := inspector.InspectManaged(ctx, binding.RuntimeBinding); err != nil {
			return err
		}
		resolved := append([]string(nil), args...)
		for i := range resolved {
			if resolved[i] == defaultDSN() {
				resolved[i] = "postgres://" + binding.Username + "@127.0.0.1:5432/postgres"
			}
			if i > 0 && resolved[i-1] == "-U" {
				resolved[i] = binding.Username
			}
			if strings.Contains(resolved[i], "\x00") {
				return instanceaccess.ErrInvalid
			}
		}
		var err error
		output, err = m.runtime.Exec(ctx, runtimeID, engineRuntime.ExecRequest{User: "postgres", Args: resolved, Stdin: stdin, Secret: &ref})
		return err
	})
	return output, err
}

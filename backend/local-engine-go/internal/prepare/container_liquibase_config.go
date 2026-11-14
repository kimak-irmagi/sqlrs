package prepare

import (
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"

	"github.com/sqlrs/engine-local/internal/prepare/effectiveconfig"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// Ordinary settings and credentials share one protected read-only file; only
// the engine can select the DB network, username, password and runner image.
// See liquibase-container-component-structure.md, protected settings ownership.
func (m *PrepareService) liquibaseContainerRequestWithConfig(p preparedRequest, instance engineRuntime.Instance, args []string, password string, create func(effectiveconfig.ProtectedOptions) (*effectiveconfig.ProtectedConfig, error)) (LiquibaseRunRequest, *effectiveconfig.ProtectedConfig, error) {
	ordinary, err := withoutProjectDefaults(args)
	if err != nil {
		return LiquibaseRunRequest{}, nil, err
	}
	if instance.Binding.Validate() != nil || instance.ID == "" {
		return LiquibaseRunRequest{}, nil, fmt.Errorf("exact managed target required")
	}
	url := "jdbc:postgresql://" + net.JoinHostPort("127.0.0.1", strconv.Itoa(5432)) + "/postgres"
	protected, err := create(effectiveconfig.ProtectedOptions{URL: url, Username: instance.Binding.Username, Password: password, Properties: p.liquibaseProperties})
	if err != nil {
		return LiquibaseRunRequest{}, nil, err
	}
	mounts := append([]engineRuntime.Mount(nil), p.liquibaseMounts...)
	mounts = append(mounts, engineRuntime.Mount{HostPath: protected.HostPath, ContainerPath: protected.ContainerPath, ReadOnly: true})
	return LiquibaseRunRequest{ImageID: p.liquibaseImageID, Args: append([]string{"--defaults-file=" + protected.ContainerPath}, ordinary...), WorkDir: p.liquibaseContainerWorkDir, Mounts: mounts, Network: "container:" + instance.ID}, protected, nil
}

func withoutProjectDefaults(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		if args[i] == "--defaults-file" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("defaults flag lacks value")
			}
			i++
			continue
		}
		if strings.HasPrefix(args[i], "--defaults-file=") {
			continue
		}
		out = append(out, args[i])
	}
	return out, nil
}

// Bound clone materialization follows a committed journal intent, never precedes it.
type boundCloneMaterializer interface {
	MaterializeBoundClone(context.Context, queue.ClonePlan) (string, error)
}
type boundCloneMaterializerFunc func(context.Context, queue.ClonePlan) (string, error)

func (f boundCloneMaterializerFunc) MaterializeBoundClone(ctx context.Context, clone queue.ClonePlan) (string, error) {
	return f(ctx, clone)
}

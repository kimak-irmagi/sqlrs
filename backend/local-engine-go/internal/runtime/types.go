package runtime

import (
	"context"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"time"
)

type LogSink func(line string)

type Instance struct {
	Binding managedidentity.RuntimeBinding
	ID      string
	Host    string
	Port    int
}

type StartRequest struct {
	ImageID string
	DataDir string
	Name    string
	Mounts  []Mount
	// AllowInitdb controls whether Start may initialize an empty data directory.
	AllowInitdb bool
}

type ExecRequest struct {
	// Secret is a protected reference; it is resolved only at process delivery.
	Secret *instanceaccess.SecretBinding
	User   string
	Args   []string
	Env    map[string]string
	Dir    string
	Stdin  *string
}

type RunRequest struct {
	ImageID string
	Args    []string
	Env     map[string]string
	Dir     string
	User    string
	Name    string
	Network string
	Mounts  []Mount
}

type Mount struct {
	HostPath      string
	ContainerPath string
	ReadOnly      bool
}

type Runtime interface {
	InitBase(ctx context.Context, imageID string, dataDir string) error
	ResolveImage(ctx context.Context, imageID string) (string, error)
	Start(ctx context.Context, req StartRequest) (Instance, error)
	Stop(ctx context.Context, id string) error
	Exec(ctx context.Context, id string, req ExecRequest) (string, error)
	WaitForReady(ctx context.Context, id string, timeout time.Duration) error
}

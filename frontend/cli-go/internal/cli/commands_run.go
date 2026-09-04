package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/sqlrs/cli/internal/cli/runkind"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/daemon"
)

type RunOptions struct {
	ProfileName     string
	Mode            string
	AuthToken       string
	Endpoint        string
	Autostart       bool
	DaemonPath      string
	RunDir          string
	StateDir        string
	EngineRunDir    string
	EngineStatePath string
	EngineStoreDir  string
	WSLVHDXPath     string
	WSLMountUnit    string
	WSLMountFSType  string
	WSLDistro       string
	Timeout         time.Duration
	IdleTimeout     time.Duration
	StartupTimeout  time.Duration
	Verbose         bool

	Kind        string
	InstanceRef string
	Command     string
	Args        []string
	Stdin       *string
	Steps       []RunStep
}

type RunResult struct {
	ExitCode int
}

func RunRun(ctx context.Context, opts RunOptions, stdout io.Writer, stderr io.Writer) (RunResult, error) {
	if strings.TrimSpace(opts.InstanceRef) == "" {
		return RunResult{}, fmt.Errorf("instance is required")
	}
	kind := strings.ToLower(strings.TrimSpace(opts.Kind))
	if kind == "" {
		return RunResult{}, fmt.Errorf("run kind is required")
	}
	if !runkind.IsKnown(kind) {
		return RunResult{}, fmt.Errorf("unknown run kind: %s", kind)
	}
	cliClient, err := runClient(ctx, opts)
	if err != nil {
		return RunResult{}, err
	}
	instanceRef, err := resolveRunInstanceRef(ctx, cliClient, opts.InstanceRef)
	if err != nil {
		return RunResult{}, err
	}

	var cmdPtr *string
	if strings.TrimSpace(opts.Command) != "" {
		value := opts.Command
		cmdPtr = &value
	}
	body := client.RunRequest{
		InstanceRef: instanceRef,
		Kind:        kind,
		Command:     cmdPtr,
		Args:        append([]string{}, opts.Args...),
		Stdin:       opts.Stdin,
		Steps:       toClientSteps(opts.Steps),
	}
	stream, err := cliClient.RunCommand(ctx, body)
	if err != nil {
		return RunResult{}, err
	}
	defer stream.Close()

	return readRunStream(stream, stdout, stderr)
}

// resolveRunInstanceRef canonicalizes prefix-shaped run targets according to
// docs/architecture/cli-architecture.md. Ordinary names remain engine-resolved;
// eligible prefixes preserve an exact ID/name match before list-filter lookup.
func resolveRunInstanceRef(ctx context.Context, cliClient *client.Client, instanceRef string) (string, error) {
	value := strings.TrimSpace(instanceRef)
	normalized, err := normalizeIDPrefix("instance", value)
	if err != nil {
		return value, nil
	}

	exact, found, err := cliClient.GetInstance(ctx, value)
	if err != nil {
		return "", err
	}
	if found {
		return exact.InstanceID, nil
	}

	match, err := resolveInstancePrefix(ctx, cliClient, normalized, "", "")
	if err != nil {
		return "", err
	}
	if match.noMatch {
		return "", fmt.Errorf("instance not found: %s", value)
	}
	return match.value, nil
}

type RunStep struct {
	Args  []string
	Stdin *string
}

func toClientSteps(steps []RunStep) []client.RunStep {
	if len(steps) == 0 {
		return nil
	}
	out := make([]client.RunStep, 0, len(steps))
	for _, step := range steps {
		out = append(out, client.RunStep{
			Args:  append([]string{}, step.Args...),
			Stdin: step.Stdin,
		})
	}
	return out
}

func DeleteInstance(ctx context.Context, opts RunOptions, instanceID string) error {
	if strings.TrimSpace(instanceID) == "" {
		return nil
	}
	_, _, err := DeleteInstanceDetailed(ctx, opts, instanceID)
	return err
}

func DeleteInstanceDetailed(ctx context.Context, opts RunOptions, instanceID string) (client.DeleteResult, int, error) {
	if strings.TrimSpace(instanceID) == "" {
		return client.DeleteResult{}, 0, nil
	}
	cliClient, err := runClient(ctx, opts)
	if err != nil {
		return client.DeleteResult{}, 0, err
	}
	return cliClient.DeleteInstance(ctx, instanceID, client.DeleteOptions{})
}

func runClient(ctx context.Context, opts RunOptions) (*client.Client, error) {
	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	endpoint := strings.TrimSpace(opts.Endpoint)
	authToken := strings.TrimSpace(opts.AuthToken)

	if mode == "local" {
		authToken = ""
		if endpoint == "" {
			endpoint = "auto"
		}
		if endpoint == "auto" {
			if opts.Verbose {
				fmt.Fprintln(os.Stderr, "checking local engine state")
			}
			resolved, err := daemon.ConnectOrStart(ctx, daemon.ConnectOptions{
				Endpoint:        endpoint,
				Autostart:       opts.Autostart,
				DaemonPath:      opts.DaemonPath,
				RunDir:          opts.RunDir,
				StateDir:        opts.StateDir,
				EngineRunDir:    opts.EngineRunDir,
				EngineStatePath: opts.EngineStatePath,
				EngineStoreDir:  opts.EngineStoreDir,
				WSLVHDXPath:     opts.WSLVHDXPath,
				WSLMountUnit:    opts.WSLMountUnit,
				WSLMountFSType:  opts.WSLMountFSType,
				WSLDistro:       opts.WSLDistro,
				IdleTimeout:     opts.IdleTimeout,
				StartupTimeout:  opts.StartupTimeout,
				ClientTimeout:   opts.Timeout,
				Verbose:         opts.Verbose,
			})
			if err != nil {
				return nil, err
			}
			endpoint = resolved.Endpoint
			authToken = resolved.AuthToken
			if opts.Verbose {
				fmt.Fprintf(os.Stderr, "engine ready at %s\n", endpoint)
			}
		}
	} else if mode == "remote" {
		if endpoint == "" || endpoint == "auto" {
			return nil, fmt.Errorf("remote mode requires explicit endpoint")
		}
		if opts.Verbose {
			fmt.Fprintf(os.Stderr, "using remote endpoint %s\n", endpoint)
		}
	} else {
		return nil, fmt.Errorf("invalid mode: %s", mode)
	}

	return client.New(endpoint, client.Options{Timeout: opts.Timeout, AuthToken: authToken}), nil
}

func readRunStream(stream io.Reader, stdout io.Writer, stderr io.Writer) (RunResult, error) {
	scanner := bufio.NewScanner(stream)
	exitCode := 0
	for scanner.Scan() {
		var evt client.RunEvent
		if err := json.Unmarshal(scanner.Bytes(), &evt); err != nil {
			return RunResult{}, err
		}
		switch evt.Type {
		case "stdout":
			if evt.Data != "" {
				if _, err := io.WriteString(stdout, evt.Data); err != nil {
					return RunResult{}, err
				}
			}
		case "stderr":
			if evt.Data != "" {
				if _, err := io.WriteString(stderr, evt.Data); err != nil {
					return RunResult{}, err
				}
			}
		case "error":
			if evt.Error != nil && evt.Error.Message != "" {
				if evt.Error.Details != "" {
					return RunResult{}, fmt.Errorf("%s: %s", evt.Error.Message, evt.Error.Details)
				}
				return RunResult{}, fmt.Errorf("%s", evt.Error.Message)
			}
			return RunResult{}, fmt.Errorf("run failed")
		case "exit":
			if evt.ExitCode != nil {
				exitCode = *evt.ExitCode
			}
			return RunResult{ExitCode: exitCode}, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return RunResult{}, err
	}
	return RunResult{ExitCode: exitCode}, nil
}

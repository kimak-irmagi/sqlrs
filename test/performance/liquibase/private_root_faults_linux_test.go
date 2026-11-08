package benchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateRootDisposalCancellationHasNoDockerEffect(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	calls := 0
	run := func(context.Context, ...string) ([]byte, error) { calls++; return nil, nil }
	err := cleanupPrivateRoot(ctx, t.TempDir(), strings.Repeat("a", 64), "postgres@sha256:"+strings.Repeat("b", 64), run)
	if !errors.Is(err, context.Canceled) || calls != 0 {
		t.Fatal("cancelled cleanup reached Docker")
	}
}

func TestPrivateRootHelperFaultsCannotReportSuccess(t *testing.T) {
	for _, fault := range []string{"create", "invalid_id", "start", "start_cancel", "remove", "list", "present", "residual_file"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			token := strings.Repeat("a", 64)
			id := strings.Repeat("b", 64)
			if err := os.WriteFile(filepath.Join(root, ".collector-owner"), []byte(token), 0600); err != nil {
				t.Fatal(err)
			}
			if fault == "residual_file" {
				if err := os.WriteFile(filepath.Join(root, "residual"), nil, 0600); err != nil {
					t.Fatal(err)
				}
			}
			retired := false
			checked := false
			primary, cancel := context.WithCancel(context.Background())
			defer cancel()
			run := func(ctx context.Context, args ...string) ([]byte, error) {
				switch args[0] {
				case "create":
					if fault == "create" {
						return nil, errors.New("create")
					}
					if fault == "invalid_id" {
						return []byte("bad"), nil
					}
					return []byte(id), nil
				case "start":
					if fault == "start_cancel" {
						cancel()
						return nil, context.Canceled
					}
					if fault == "start" {
						return nil, errors.New("start")
					}
					return nil, nil
				case "container":
					if ctx.Err() != nil {
						t.Fatal("helper retirement inherited a cancelled context")
					}
					if args[1] == "rm" {
						retired = true
						if fault == "remove" {
							return nil, errors.New("remove")
						}
						return nil, nil
					}
					checked = true
					if fault == "list" {
						return nil, errors.New("list")
					}
					if fault == "present" {
						return []byte(id), nil
					}
					return nil, nil
				}
				return nil, errors.New("unexpected command")
			}
			err := cleanupPrivateRoot(primary, root, token, "postgres@sha256:"+strings.Repeat("c", 64), run)
			if err == nil {
				t.Fatal("failed disposal reported success")
			}
			wantRetirement := fault != "create" && fault != "invalid_id"
			if retired != wantRetirement || checked != wantRetirement {
				t.Fatal("exact helper retirement differs")
			}
		})
	}
}

package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Physical inventory precedes all format/schema writes; preserve every resource
// and inspect stopped containers as well as running ones (approved upgrade plan).
func TestManagedInventoryRejectsEachResourceNamespace(t *testing.T) {
	for _, namespace := range []string{"engines", "jobs", "runtime-journal", "managed-secrets"} {
		t.Run(namespace, func(t *testing.T) {
			root := t.TempDir()
			resource := filepath.Join(root, namespace, "existing")
			if err := os.MkdirAll(filepath.Dir(resource), 0o700); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(resource, []byte("preserve"), 0o600); err != nil {
				t.Fatal(err)
			}
			runner := &fakeRunner{}
			rt := NewDocker(Options{Runner: runner})
			if err := rt.CheckEmptyManagedResources(context.Background(), root); err != ErrManagedResourcesPresent {
				t.Fatalf("inventory: %v", err)
			}
			if data, err := os.ReadFile(resource); err != nil || string(data) != "preserve" {
				t.Fatalf("resource changed: %v", err)
			}
			if len(runner.calls) != 0 {
				t.Fatal("container inventory should not run after physical rejection")
			}
		})
	}
}

func TestManagedInventoryPreservesUnrelatedFiles(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"settings", "config.json", "unrelated-secret"} {
		if err := os.WriteFile(filepath.Join(root, name), []byte("preserve"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(root, "engines"), 0o700); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{responses: []runResponse{{output: ""}}}
	if err := NewDocker(Options{Runner: runner}).CheckEmptyManagedResources(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if len(runner.calls) != 1 || strings.Join(runner.calls[0].args, " ") != "container ls --all --quiet --no-trunc" {
		t.Fatalf("inventory calls: %+v", runner.calls)
	}
	for _, name := range []string{"settings", "config.json", "unrelated-secret"} {
		if data, err := os.ReadFile(filepath.Join(root, name)); err != nil || string(data) != "preserve" {
			t.Fatalf("unrelated data changed: %v", err)
		}
	}
}

func TestManagedInventoryContainerMounts(t *testing.T) {
	root := t.TempDir()
	containerID := strings.Repeat("a", 64)
	for _, fixture := range []struct {
		name, source string
		want         error
	}{
		{"root", root, ErrManagedResourcesPresent},
		{"child", filepath.Join(root, "jobs", "job-1", "runtime"), ErrManagedResourcesPresent},
		{"parent", filepath.Dir(root), ErrManagedResourcesPresent},
		{"unrelated", root + "-other", nil},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			mounts, err := json.Marshal([]map[string]string{{"Type": "bind", "Source": fixture.source, "Destination": PostgresDataDirRoot}})
			if err != nil {
				t.Fatal(err)
			}
			runner := &fakeRunner{responses: []runResponse{{output: containerID + "\n"}, {output: string(mounts)}}}
			err = NewDocker(Options{Runner: runner}).CheckEmptyManagedResources(context.Background(), root)
			if err != fixture.want {
				t.Fatalf("inventory=%v want=%v", err, fixture.want)
			}
			if len(runner.calls) != 2 || runner.calls[1].args[0] != "inspect" {
				t.Fatalf("unexpected commands: %+v", runner.calls)
			}
		})
	}
}

func TestManagedInventoryFailsClosed(t *testing.T) {
	root := t.TempDir()
	for _, responses := range [][]runResponse{
		{{err: errors.New("private-canary")}},
		{{output: "invalid-id"}},
		{{output: strings.Repeat("a", 64)}, {err: errors.New("private-canary")}},
		{{output: strings.Repeat("a", 64)}, {output: "invalid-json"}},
		{{output: strings.Repeat("a", 64)}, {output: "null"}},
		{{output: strings.Repeat("a", 64)}, {output: `[{"Type":"bind","Source":"relative","Destination":"/data"}]`}},
	} {
		runner := &fakeRunner{responses: responses}
		if err := NewDocker(Options{Runner: runner}).CheckEmptyManagedResources(context.Background(), root); err != ErrManagedInventoryUnavailable {
			t.Fatalf("unbounded or accepted failure: %v", err)
		}
	}
	rt := NewDocker(Options{Runner: &fakeRunner{}})
	for _, invalid := range []string{"", "relative", filepath.Join(root, "missing")} {
		if err := rt.CheckEmptyManagedResources(context.Background(), invalid); err != ErrManagedInventoryUnavailable {
			t.Fatalf("invalid root accepted: %q %v", invalid, err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := rt.CheckEmptyManagedResources(ctx, root); err != context.Canceled {
		t.Fatalf("cancellation: %v", err)
	}
}

func TestManagedInventoryMountPathNormalization(t *testing.T) {
	for _, input := range []string{`C:\Users\owner\store`, "c:/Users/owner/store", "/run/desktop/mnt/host/c/Users/owner/store", "/host_mnt/c/Users/owner/store"} {
		if got := managedMountPath(input); got != "/mnt/c/users/owner/store" {
			t.Fatalf("normalize %q: %q", input, got)
		}
	}
	if got := managedMountPath("/var/lib/Store"); got != "/var/lib/Store" {
		t.Fatalf("Linux case changed: %q", got)
	}
}

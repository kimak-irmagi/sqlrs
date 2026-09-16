package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
)

type privateRunner struct {
	calls  [][]string
	inputs []string
	fail   bool
}

func TestManagedMountRejectsSecretAlias(t *testing.T) {
	root := t.TempDir()
	protected := filepath.Join(root, "private")
	if err := os.Mkdir(protected, 0700); err != nil {
		t.Fatal(err)
	}
	alias := filepath.Join(root, "recipe")
	if err := os.Symlink(protected, alias); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	rt := NewDocker(Options{ProtectedRoot: protected})
	if err := rt.managedPaths(filepath.Join(root, "data"), []Mount{{HostPath: alias}}); err == nil {
		t.Fatal("symbolic link exposed protected secrets")
	}
}

func (r *privateRunner) Run(_ context.Context, _ string, args []string, stdin *string) (string, error) {
	r.calls = append(r.calls, append([]string(nil), args...))
	if stdin != nil {
		r.inputs = append(r.inputs, *stdin)
	}
	if r.fail {
		return "credential canary", errors.New("credential canary")
	}
	if args[0] == "run" && args[1] == "-d" {
		return strings.Repeat("a", 64), nil
	}
	if args[0] == "port" {
		return "127.0.0.1:6543", nil
	}
	return "", nil
}

func TestManagedRuntimeUsesProtectedInput(t *testing.T) {
	root := t.TempDir()
	secrets, err := instanceaccess.OpenSecrets(filepath.Join(root, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	defer secrets.Close()
	identity, err := managedidentity.Generate(strings.NewReader(strings.Repeat("b", 16)))
	if err != nil {
		t.Fatal(err)
	}
	ref := instanceaccess.SecretBinding{DomainRef: "domain", OwnerRef: identity.LineageRef, IdentityDigest: identity.IdentityDigest, Version: "bootstrap-v1", Purpose: "bootstrap"}
	secret, err := secrets.Reserve(ref)
	if err != nil {
		t.Fatal(err)
	}
	runner := &privateRunner{}
	rt := NewDocker(Options{Runner: runner, ManagedSecrets: secrets, ProtectedRoot: filepath.Join(root, "secrets")})
	req := ManagedInitRequest{ImageID: "postgres@sha256:" + strings.Repeat("a", 64), DataDir: filepath.Join(root, "base"), Identity: identity, Bootstrap: ref}
	logs := []string{}
	ctx := WithLogSink(context.Background(), func(line string) { logs = append(logs, line) })
	if err := rt.InitManagedBase(ctx, req); err != nil {
		t.Fatal(err)
	}
	start := ManagedStartRequest{StartRequest: StartRequest{ImageID: req.ImageID, DataDir: filepath.Join(root, "clone")}, Identity: identity, PhysicalIdentity: "clone_test"}
	result, err := rt.StartManaged(ctx, start)
	if err != nil || result.Binding.IdentityBinding != identity || result.Binding.PhysicalIdentity != start.PhysicalIdentity {
		t.Fatal("lost binding", err)
	}
	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call, " "), secret.Password) {
			t.Fatal("password in argv")
		}
	}
	found := false
	for _, input := range runner.inputs {
		if strings.Contains(input, secret.Password) {
			found = true
		}
	}
	if !found || len(logs) != 0 {
		t.Fatal("secret bypassed private non-streaming input")
	}
	stdin := "SELECT 1;"
	if _, err := rt.Exec(ctx, result.ID, ExecRequest{User: "postgres", Args: []string{"psql", "-U", identity.Username}, Stdin: &stdin, Secret: &ref}); err != nil {
		t.Fatal(err)
	}
	for _, call := range runner.calls {
		if strings.Contains(strings.Join(call, " "), secret.Password) {
			t.Fatal("exec password in argv")
		}
	}
	last := runner.inputs[len(runner.inputs)-1]
	if last != secret.Password+"\n"+stdin {
		t.Fatal("protected stdin lost SQL payload")
	}
	start.Mounts = []Mount{{HostPath: root, ContainerPath: "/recipe"}}
	if _, err := rt.StartManaged(ctx, start); err == nil {
		t.Fatal("recipe mounted secret ancestor")
	}
	runner.fail = true
	if err := rt.InitManagedBase(ctx, req); err == nil || strings.Contains(err.Error(), "canary") {
		t.Fatal("raw maintenance failure", err)
	}
}

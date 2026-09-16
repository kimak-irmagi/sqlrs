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
	outputs map[string]string
	failAt  int
	calls   [][]string
	inputs  []string
	fail    bool
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
	if r.fail || (r.failAt > 0 && len(r.calls) == r.failAt) {
		return "credential canary", errors.New("credential canary")
	}
	if out, ok := r.outputs[args[0]]; ok {
		return out, nil
	}
	if args[0] == "run" && args[1] == "-d" {
		return strings.Repeat("a", 64), nil
	}
	if args[0] == "port" {
		return "127.0.0.1:6543", nil
	}
	return "", nil
}

func TestManagedRuntimeFailuresDoNotPublishOrLeak(t *testing.T) {
	root := t.TempDir()
	secrets, err := instanceaccess.OpenSecrets(filepath.Join(root, "secrets"))
	if err != nil {
		t.Fatal(err)
	}
	defer secrets.Close()
	id, err := managedidentity.Generate(strings.NewReader(strings.Repeat("c", 16)))
	if err != nil {
		t.Fatal(err)
	}
	ref := instanceaccess.SecretBinding{DomainRef: "domain", OwnerRef: id.LineageRef, IdentityDigest: id.IdentityDigest, Version: "bootstrap-v1", Purpose: "bootstrap"}
	if _, err := secrets.Reserve(ref); err != nil {
		t.Fatal(err)
	}
	init := ManagedInitRequest{ImageID: "postgres:17", DataDir: filepath.Join(root, "base"), Identity: id, Bootstrap: ref}
	start := ManagedStartRequest{StartRequest: StartRequest{ImageID: init.ImageID, DataDir: filepath.Join(root, "clone")}, Identity: id, PhysicalIdentity: "physical"}
	for _, step := range []string{"initdb", "hba", "base-permissions", "create", "ownership", "parent-permissions", "data-permissions", "pg-start", "port"} {
		t.Run(step, func(t *testing.T) {
			index := map[string]int{"initdb": 1, "hba": 2, "base-permissions": 3, "create": 1, "ownership": 2, "parent-permissions": 3, "data-permissions": 4, "pg-start": 5, "port": 6}[step]
			runner := &privateRunner{failAt: index}
			rt := NewDocker(Options{Runner: runner, ManagedSecrets: secrets, ProtectedRoot: filepath.Join(root, "secrets")})
			var err error
			if step == "initdb" || step == "hba" || step == "base-permissions" {
				err = rt.InitManagedBase(context.Background(), init)
			} else {
				var instance Instance
				instance, err = rt.StartManaged(context.Background(), start)
				if instance.ID != "" {
					t.Fatal("partial runtime published")
				}
			}
			if err == nil || strings.Contains(err.Error(), "canary") {
				t.Fatal("unbounded or missing error", err)
			}
		})
	}
	for _, item := range []struct{ name, key, value string }{{"container-id", "run", "invalid"}, {"port", "port", "invalid"}} {
		t.Run(item.name, func(t *testing.T) {
			rt := NewDocker(Options{Runner: &privateRunner{outputs: map[string]string{item.key: item.value}}, ProtectedRoot: filepath.Join(root, "secrets")})
			if _, err := rt.StartManaged(context.Background(), start); err == nil {
				t.Fatal("invalid observation published")
			}
		})
	}
	rt := NewDocker(Options{Runner: &privateRunner{}, ManagedSecrets: secrets, ProtectedRoot: filepath.Join(root, "secrets")})
	bad := init
	bad.Bootstrap.OwnerRef = "foreign"
	if err := rt.InitManagedBase(context.Background(), bad); err == nil {
		t.Fatal("foreign bootstrap")
	}
	bad = init
	bad.Bootstrap.Version = "missing"
	if err := rt.InitManagedBase(context.Background(), bad); err == nil {
		t.Fatal("missing secret")
	}
	start.AllowInitdb = true
	if _, err := rt.StartManaged(context.Background(), start); err == nil {
		t.Fatal("clone initialized")
	}
	start.AllowInitdb = false
	start.Mounts = []Mount{{HostPath: root}}
	if _, err := rt.StartManaged(context.Background(), start); err == nil {
		t.Fatal("invalid mount")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := rt.privateRun(ctx, []string{"ps"}, nil); err != context.Canceled {
		t.Fatal("cancellation lost", err)
	}
}

func TestManagedStopProvesAbsenceAndRestartOwnership(t *testing.T) {
	id, err := managedidentity.Generate(strings.NewReader(strings.Repeat("b", 16)))
	if err != nil {
		t.Fatal(err)
	}
	b := managedidentity.RuntimeBinding{IdentityBinding: id, PhysicalIdentity: "physical_test", RuntimeRef: strings.Repeat("a", 64)}
	runner := &privateRunner{outputs: map[string]string{"ps": ""}}
	rt := NewDocker(Options{Runner: runner})
	if err := rt.StopManaged(context.Background(), b); err != nil || len(runner.calls) != 1 {
		t.Fatal("confirmed absence", err)
	}
	runner.fail = true
	if err := rt.StopManaged(context.Background(), b); err == nil {
		t.Fatal("daemon failure counted as absence")
	}
	runner.fail = false
	runner.outputs["ps"] = b.RuntimeRef
	runner.outputs["inspect"] = b.RuntimeRef + " " + b.IdentityDigest + " wrong true"
	if err := rt.StopManaged(context.Background(), b); err == nil {
		t.Fatal("foreign physical container removed")
	}
	runner.outputs["inspect"] = b.RuntimeRef + " " + b.IdentityDigest + " " + b.PhysicalIdentity + " true"
	if err := rt.StopManaged(context.Background(), b); err != nil {
		t.Fatal("restart cleanup", err)
	}
	if runner.calls[len(runner.calls)-1][0] != "rm" {
		t.Fatal("owned container not removed")
	}
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

//go:build linux

package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

type btrfsCommandCall struct {
	desc    string
	command string
	args    []string
}

func withLocalBtrfsLookPathStub(t *testing.T, fn func(string) (string, error)) {
	t.Helper()
	prev := localBtrfsLookPathFn
	localBtrfsLookPathFn = fn
	t.Cleanup(func() {
		localBtrfsLookPathFn = prev
	})
}

func withLocalBtrfsRunCommandStub(t *testing.T, fn func(string, string, ...string) (string, error)) {
	t.Helper()
	prev := localBtrfsRunCommandFn
	localBtrfsRunCommandFn = fn
	t.Cleanup(func() {
		localBtrfsRunCommandFn = prev
	})
}

func withLocalBtrfsRunAllowFailureStub(t *testing.T, fn func(string, string, ...string) (string, error)) {
	t.Helper()
	prev := localBtrfsRunAllowFailureFn
	localBtrfsRunAllowFailureFn = fn
	t.Cleanup(func() {
		localBtrfsRunAllowFailureFn = prev
	})
}

func withLocalBtrfsIsBtrfsStub(t *testing.T, fn func(string) (bool, error)) {
	t.Helper()
	prev := localBtrfsIsBtrfsPathFn
	localBtrfsIsBtrfsPathFn = fn
	t.Cleanup(func() {
		localBtrfsIsBtrfsPathFn = prev
	})
}

func exitStatusError(t *testing.T, code int) error {
	t.Helper()
	err := exec.CommandContext(context.Background(), "sh", "-c", fmt.Sprintf("exit %d", code)).Run()
	if err == nil {
		t.Fatalf("expected shell to exit with %d", code)
	}
	return err
}

func unwrapPrivilegedCommand(command string, args []string) (string, []string) {
	if command != "sudo" {
		return command, args
	}
	if len(args) == 0 {
		return command, nil
	}
	return args[0], args[1:]
}

func hasCommand(calls []btrfsCommandCall, target string) bool {
	for _, call := range calls {
		cmd, _ := unwrapPrivilegedCommand(call.command, call.args)
		if cmd == target {
			return true
		}
	}
	return false
}

func TestEnsureLocalBtrfsPrerequisitesMissingTool(t *testing.T) {
	withLocalBtrfsLookPathStub(t, func(command string) (string, error) {
		if command == "mount" {
			return "", errors.New("not found")
		}
		return "/usr/bin/" + command, nil
	})

	err := ensureLocalBtrfsPrerequisites(localBtrfsStorePlan{storeDir: t.TempDir()})
	if err == nil || !strings.Contains(err.Error(), "mount is required for btrfs init") {
		t.Fatalf("expected missing mount error, got %v", err)
	}
}

func TestEnsureLocalBtrfsPrerequisitesImageRequiresTruncate(t *testing.T) {
	calls := map[string]int{}
	withLocalBtrfsLookPathStub(t, func(command string) (string, error) {
		calls[command]++
		return "/usr/bin/" + command, nil
	})

	err := ensureLocalBtrfsPrerequisites(localBtrfsStorePlan{
		storeDir:  t.TempDir(),
		imagePath: filepath.Join(t.TempDir(), "store.img"),
	})
	if err != nil {
		t.Fatalf("ensureLocalBtrfsPrerequisites: %v", err)
	}
	if calls["truncate"] == 0 {
		t.Fatalf("expected truncate to be required for image plan, calls=%v", calls)
	}
}

func TestInitLocalBtrfsStoreReusesExistingBtrfsPath(t *testing.T) {
	storeDir := t.TempDir()
	withLocalBtrfsLookPathStub(t, func(command string) (string, error) {
		return "/usr/bin/" + command, nil
	})
	withLocalBtrfsIsBtrfsStub(t, func(path string) (bool, error) {
		if path != storeDir {
			t.Fatalf("unexpected btrfs path check: %q", path)
		}
		return true, nil
	})

	result, err := initLocalBtrfsStore(localBtrfsInitOptions{
		StoreType: "dir",
		StorePath: storeDir,
	})
	if err != nil {
		t.Fatalf("initLocalBtrfsStore: %v", err)
	}
	if result.StorePath != storeDir {
		t.Fatalf("expected reused store path %q, got %q", storeDir, result.StorePath)
	}
}

func TestEnsureLoopImageExistingFile(t *testing.T) {
	image := filepath.Join(t.TempDir(), "btrfs.img")
	if err := os.WriteFile(image, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}

	created, err := ensureLoopImage(image, localBtrfsInitOptions{})
	if err != nil {
		t.Fatalf("ensureLoopImage: %v", err)
	}
	if created {
		t.Fatalf("expected existing image to be reused")
	}
}

func TestEnsureLoopImageCreatesDefaultSizeWhenUnset(t *testing.T) {
	image := filepath.Join(t.TempDir(), "sub", "disk.img")
	var calls []btrfsCommandCall
	withLocalBtrfsRunCommandStub(t, func(desc string, command string, args ...string) (string, error) {
		calls = append(calls, btrfsCommandCall{desc: desc, command: command, args: append([]string(nil), args...)})
		return "", nil
	})

	created, err := ensureLoopImage(image, localBtrfsInitOptions{})
	if err != nil {
		t.Fatalf("ensureLoopImage: %v", err)
	}
	if !created {
		t.Fatalf("expected new image creation")
	}
	if len(calls) != 1 {
		t.Fatalf("expected one command call, got %d", len(calls))
	}
	call := calls[0]
	if call.command != "truncate" {
		t.Fatalf("expected truncate command, got %q", call.command)
	}
	if len(call.args) != 3 || call.args[0] != "-s" || call.args[1] != strconv.Itoa(defaultBtrfsStoreSizeGB)+"G" || call.args[2] != image {
		t.Fatalf("unexpected truncate args: %#v", call.args)
	}
}

func TestDetectBlockFSTypeNoFSOnExitStatus1(t *testing.T) {
	withLocalBtrfsRunAllowFailureStub(t, func(desc string, command string, args ...string) (string, error) {
		return "", fmt.Errorf("%s: %w", desc, exitStatusError(t, 1))
	})

	fsType, hasFS, err := detectBlockFSType("/tmp/block")
	if err != nil {
		t.Fatalf("detectBlockFSType: %v", err)
	}
	if hasFS || fsType != "" {
		t.Fatalf("expected no filesystem detected, got hasFS=%v fsType=%q", hasFS, fsType)
	}
}

func TestDetectBlockFSTypeReturnsValue(t *testing.T) {
	withLocalBtrfsRunAllowFailureStub(t, func(desc string, command string, args ...string) (string, error) {
		return " btrfs \n", nil
	})

	fsType, hasFS, err := detectBlockFSType("/tmp/block")
	if err != nil {
		t.Fatalf("detectBlockFSType: %v", err)
	}
	if !hasFS || fsType != "btrfs" {
		t.Fatalf("expected btrfs filesystem, got hasFS=%v fsType=%q", hasFS, fsType)
	}
}

func TestEnsureLoopbackBtrfsStoreRejectsForeignFSWithoutReinit(t *testing.T) {
	root := t.TempDir()
	plan := localBtrfsStorePlan{
		storeDir:  filepath.Join(root, "store"),
		imagePath: filepath.Join(root, "disk.img"),
	}
	if err := os.WriteFile(plan.imagePath, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	withLocalBtrfsRunAllowFailureStub(t, func(desc string, command string, args ...string) (string, error) {
		if command != "blkid" {
			t.Fatalf("unexpected command: %s", command)
		}
		return "ext4", nil
	})

	_, err := ensureLoopbackBtrfsStore(plan, localBtrfsInitOptions{})
	if err == nil || !strings.Contains(err.Error(), "expected btrfs (rerun with --reinit)") {
		t.Fatalf("expected foreign fs error, got %v", err)
	}
}

func TestEnsureLoopbackBtrfsStoreFormatsAndMountsWhenAllowed(t *testing.T) {
	root := t.TempDir()
	plan := localBtrfsStorePlan{
		storeDir:  filepath.Join(root, "store"),
		imagePath: filepath.Join(root, "disk.img"),
	}
	var calls []btrfsCommandCall
	withLocalBtrfsRunCommandStub(t, func(desc string, command string, args ...string) (string, error) {
		calls = append(calls, btrfsCommandCall{desc: desc, command: command, args: append([]string(nil), args...)})
		return "", nil
	})
	withLocalBtrfsRunAllowFailureStub(t, func(desc string, command string, args ...string) (string, error) {
		switch command {
		case "blkid":
			return "ext4", nil
		case "findmnt":
			return "", fmt.Errorf("%s: %w", desc, exitStatusError(t, 1))
		default:
			t.Fatalf("unexpected allow-failure command: %s", command)
			return "", nil
		}
	})
	withLocalBtrfsIsBtrfsStub(t, func(path string) (bool, error) {
		return true, nil
	})

	storePath, err := ensureLoopbackBtrfsStore(plan, localBtrfsInitOptions{StoreSizeGB: 1})
	if err != nil {
		t.Fatalf("ensureLoopbackBtrfsStore: %v", err)
	}
	if storePath != plan.storeDir {
		t.Fatalf("expected store path %q, got %q", plan.storeDir, storePath)
	}
	for _, command := range []string{"truncate", "mkfs.btrfs", "mount", "chown"} {
		if !hasCommand(calls, command) {
			t.Fatalf("expected command %q in calls: %#v", command, calls)
		}
	}
}

func TestEnsureDeviceBackedBtrfsStoreMountedBtrfsReuse(t *testing.T) {
	plan := localBtrfsStorePlan{
		storeDir:   t.TempDir(),
		devicePath: "/dev/loop7",
	}
	withLocalBtrfsRunAllowFailureStub(t, func(desc string, command string, args ...string) (string, error) {
		if command != "findmnt" {
			t.Fatalf("unexpected command %q", command)
		}
		return "/mnt/sqlrs btrfs", nil
	})

	storePath, err := ensureDeviceBackedBtrfsStore(plan, localBtrfsInitOptions{})
	if err != nil {
		t.Fatalf("ensureDeviceBackedBtrfsStore: %v", err)
	}
	if storePath != "/mnt/sqlrs" {
		t.Fatalf("expected /mnt/sqlrs, got %q", storePath)
	}
}

func TestEnsureDeviceBackedBtrfsStoreRejectsMountedForeignFSWithoutReinit(t *testing.T) {
	plan := localBtrfsStorePlan{
		storeDir:   t.TempDir(),
		devicePath: "/dev/loop7",
	}
	withLocalBtrfsRunAllowFailureStub(t, func(desc string, command string, args ...string) (string, error) {
		return "/mnt/sqlrs ext4", nil
	})

	_, err := ensureDeviceBackedBtrfsStore(plan, localBtrfsInitOptions{})
	if err == nil || !strings.Contains(err.Error(), "expected btrfs") {
		t.Fatalf("expected mounted foreign fs error, got %v", err)
	}
}

func TestTryUnmountStoreExit32IsTreatedAsSuccess(t *testing.T) {
	withLocalBtrfsRunCommandStub(t, func(desc string, command string, args ...string) (string, error) {
		return "", fmt.Errorf("%s: %w", desc, exitStatusError(t, 32))
	})

	if err := tryUnmountStore("/mnt/sqlrs", false); err != nil {
		t.Fatalf("tryUnmountStore: %v", err)
	}
}

func TestTryUnmountStoreFallsBackToLazyUnmount(t *testing.T) {
	var calls []btrfsCommandCall
	withLocalBtrfsRunCommandStub(t, func(desc string, command string, args ...string) (string, error) {
		calls = append(calls, btrfsCommandCall{desc: desc, command: command, args: append([]string(nil), args...)})
		actualCommand, actualArgs := unwrapPrivilegedCommand(command, args)
		if actualCommand != "umount" {
			return "", fmt.Errorf("unexpected command: %s", actualCommand)
		}
		if len(actualArgs) == 1 {
			return "", errors.New("first unmount failed")
		}
		return "", nil
	})

	if err := tryUnmountStore("/mnt/sqlrs", true); err != nil {
		t.Fatalf("tryUnmountStore: %v", err)
	}
	if len(calls) < 2 {
		t.Fatalf("expected fallback lazy unmount, calls=%#v", calls)
	}
}

func TestEnsureStoreOwnershipRunsChownForCurrentUIDGID(t *testing.T) {
	var calls []btrfsCommandCall
	withLocalBtrfsRunCommandStub(t, func(desc string, command string, args ...string) (string, error) {
		calls = append(calls, btrfsCommandCall{desc: desc, command: command, args: append([]string(nil), args...)})
		return "", nil
	})

	target := filepath.Join(t.TempDir(), "store")
	if err := ensureStoreOwnership(target); err != nil {
		t.Fatalf("ensureStoreOwnership: %v", err)
	}
	if len(calls) != 1 {
		t.Fatalf("expected one command call, got %d", len(calls))
	}
	call := calls[0]
	command, args := unwrapPrivilegedCommand(call.command, call.args)
	if command != "chown" {
		t.Fatalf("expected chown command, got %q", command)
	}
	if len(args) != 3 || args[0] != "-R" || args[2] != target {
		t.Fatalf("unexpected chown args: %#v", args)
	}
	expectedOwner := strconv.Itoa(os.Getuid()) + ":" + strconv.Itoa(os.Getgid())
	if args[1] != expectedOwner {
		t.Fatalf("expected owner %q, got %q", expectedOwner, args[1])
	}
}

func TestRunPrivilegedCommandRoutesByPrivilege(t *testing.T) {
	var gotCommand string
	var gotArgs []string
	withLocalBtrfsRunCommandStub(t, func(desc string, command string, args ...string) (string, error) {
		gotCommand = command
		gotArgs = append([]string(nil), args...)
		return "ok", nil
	})

	out, err := runPrivilegedCommand("echo test", "echo", "hello")
	if err != nil {
		t.Fatalf("runPrivilegedCommand: %v", err)
	}
	if out != "ok" {
		t.Fatalf("expected output ok, got %q", out)
	}

	actualCommand, actualArgs := unwrapPrivilegedCommand(gotCommand, gotArgs)
	if actualCommand != "echo" || len(actualArgs) != 1 || actualArgs[0] != "hello" {
		t.Fatalf("unexpected dispatched command=%q args=%#v", actualCommand, actualArgs)
	}
	if os.Geteuid() == 0 && gotCommand == "sudo" {
		t.Fatalf("did not expect sudo for root")
	}
	if os.Geteuid() != 0 && gotCommand != "sudo" {
		t.Fatalf("expected sudo for non-root, got command=%q args=%#v", gotCommand, gotArgs)
	}
}

func TestPlanLocalBtrfsStoreRejectsInvalidInputs(t *testing.T) {
	tests := []struct {
		name      string
		storeType string
		storePath string
		want      string
	}{
		{name: "missing path", storeType: "dir", storePath: " ", want: "store path is required"},
		{name: "unsupported type", storeType: "archive", storePath: "/tmp/store", want: "unsupported store type"},
		{name: "relative image without directory", storeType: "image", storePath: "store.img", want: "cannot derive store directory"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := planLocalBtrfsStore(tt.storeType, tt.storePath)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want containing %q", err, tt.want)
			}
		})
	}
}

func TestEnsureLoopbackBtrfsStoreReusesMountedBtrfs(t *testing.T) {
	root := t.TempDir()
	plan := localBtrfsStorePlan{storeDir: filepath.Join(root, "store"), imagePath: filepath.Join(root, "disk.img")}
	if err := os.WriteFile(plan.imagePath, []byte("existing"), 0o600); err != nil {
		t.Fatalf("write image: %v", err)
	}
	withLocalBtrfsRunAllowFailureStub(t, func(desc string, command string, args ...string) (string, error) {
		switch command {
		case "blkid":
			return "btrfs", nil
		case "findmnt":
			return plan.storeDir + " btrfs", nil
		default:
			t.Fatalf("unexpected command: %s", command)
			return "", nil
		}
	})

	got, err := ensureLoopbackBtrfsStore(plan, localBtrfsInitOptions{})
	if err != nil {
		t.Fatalf("ensureLoopbackBtrfsStore: %v", err)
	}
	if got != plan.storeDir {
		t.Fatalf("store path = %q, want %q", got, plan.storeDir)
	}
}

func TestEnsureDeviceBackedBtrfsStoreFormatsAndMounts(t *testing.T) {
	plan := localBtrfsStorePlan{storeDir: t.TempDir(), devicePath: "/dev/loop7"}
	var calls []btrfsCommandCall
	withLocalBtrfsRunAllowFailureStub(t, func(desc string, command string, args ...string) (string, error) {
		switch command {
		case "findmnt":
			return "", fmt.Errorf("%s: %w", desc, exitStatusError(t, 1))
		case "blkid":
			return "ext4", nil
		default:
			t.Fatalf("unexpected command: %s", command)
			return "", nil
		}
	})
	withLocalBtrfsRunCommandStub(t, func(desc string, command string, args ...string) (string, error) {
		calls = append(calls, btrfsCommandCall{desc: desc, command: command, args: append([]string(nil), args...)})
		return "", nil
	})
	withLocalBtrfsIsBtrfsStub(t, func(string) (bool, error) { return true, nil })

	got, err := ensureDeviceBackedBtrfsStore(plan, localBtrfsInitOptions{Reinit: true})
	if err != nil {
		t.Fatalf("ensureDeviceBackedBtrfsStore: %v", err)
	}
	if got != plan.storeDir {
		t.Fatalf("store path = %q, want %q", got, plan.storeDir)
	}
	for _, command := range []string{"mkfs.btrfs", "mount", "chown"} {
		if !hasCommand(calls, command) {
			t.Fatalf("expected command %q in calls: %#v", command, calls)
		}
	}
}

func TestBtrfsCommandWrappersCaptureOutputAndErrors(t *testing.T) {
	out, err := runLocalBtrfsCommand("successful command", "sh", "-c", "printf ' output '")
	if err != nil || out != "output" {
		t.Fatalf("runLocalBtrfsCommand = %q, %v", out, err)
	}
	if _, err := runLocalBtrfsCommand("silent failure", "sh", "-c", "exit 3"); err == nil || !strings.Contains(err.Error(), "silent failure") {
		t.Fatalf("silent failure error = %v", err)
	}
	if _, err := runLocalBtrfsCommand("verbose failure", "sh", "-c", "printf problem; exit 4"); err == nil || !strings.Contains(err.Error(), "problem") {
		t.Fatalf("verbose failure error = %v", err)
	}

	out, err = runLocalBtrfsCommandAllowFailure("successful command", "sh", "-c", "printf ' output '")
	if err != nil || out != "output" {
		t.Fatalf("runLocalBtrfsCommandAllowFailure = %q, %v", out, err)
	}
	out, err = runLocalBtrfsCommandAllowFailure("verbose failure", "sh", "-c", "printf problem; exit 5")
	if err == nil || out != "problem" || !strings.Contains(err.Error(), "problem") {
		t.Fatalf("verbose allow-failure = %q, %v", out, err)
	}
	out, err = runLocalBtrfsCommandAllowFailure("silent failure", "sh", "-c", "exit 6")
	if err == nil || out != "" || !strings.Contains(err.Error(), "silent failure") {
		t.Fatalf("silent allow-failure = %q, %v", out, err)
	}
}

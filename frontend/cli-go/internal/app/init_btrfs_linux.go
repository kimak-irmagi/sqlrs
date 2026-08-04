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
	"time"
)

type localBtrfsStorePlan struct {
	storeDir   string
	imagePath  string
	devicePath string
}

var localBtrfsLookPathFn = exec.LookPath
var localBtrfsRunCommandFn = runLocalBtrfsCommand
var localBtrfsRunAllowFailureFn = runLocalBtrfsCommandAllowFailure
var localBtrfsIsBtrfsPathFn = isBtrfsPath

func initLocalBtrfsStore(opts localBtrfsInitOptions) (localBtrfsInitResult, error) {
	plan, err := planLocalBtrfsStore(opts.StoreType, opts.StorePath)
	if err != nil {
		return localBtrfsInitResult{}, err
	}
	if err := ensureLocalBtrfsPrerequisites(plan); err != nil {
		return localBtrfsInitResult{}, err
	}
	if err := os.MkdirAll(plan.storeDir, 0o700); err != nil {
		return localBtrfsInitResult{}, err
	}
	if onBtrfs, err := localBtrfsIsBtrfsPathFn(plan.storeDir); err == nil && onBtrfs {
		logLocalBtrfsInit(opts.Verbose, "reuse existing btrfs store: %s", plan.storeDir)
		return localBtrfsInitResult{StorePath: plan.storeDir}, nil
	}

	if plan.devicePath != "" {
		storeDir, err := ensureDeviceBackedBtrfsStore(plan, opts)
		if err != nil {
			return localBtrfsInitResult{}, err
		}
		return localBtrfsInitResult{StorePath: storeDir}, nil
	}

	storeDir, err := ensureLoopbackBtrfsStore(plan, opts)
	if err != nil {
		return localBtrfsInitResult{}, err
	}
	return localBtrfsInitResult{StorePath: storeDir}, nil
}

func planLocalBtrfsStore(storeType string, storePath string) (localBtrfsStorePlan, error) {
	pathValue := strings.TrimSpace(storePath)
	if pathValue == "" {
		return localBtrfsStorePlan{}, fmt.Errorf("store path is required")
	}
	pathValue = filepath.Clean(pathValue)

	normalizedType := normalizeStoreType(storeType)
	switch normalizedType {
	case "image":
		if looksLikeImagePath(pathValue) {
			storeDir := filepath.Dir(pathValue)
			if strings.TrimSpace(storeDir) == "" || storeDir == "." {
				return localBtrfsStorePlan{}, fmt.Errorf("cannot derive store directory from image path: %s", pathValue)
			}
			return localBtrfsStorePlan{
				storeDir:  storeDir,
				imagePath: pathValue,
			}, nil
		}
		return localBtrfsStorePlan{
			storeDir:  pathValue,
			imagePath: filepath.Join(pathValue, "btrfs.img"),
		}, nil
	case "device":
		if strings.HasPrefix(pathValue, "/dev/") {
			storeDir, err := defaultStoreRoot()
			if err != nil {
				return localBtrfsStorePlan{}, err
			}
			return localBtrfsStorePlan{
				storeDir:   storeDir,
				devicePath: pathValue,
			}, nil
		}
		fallthrough
	case "", "dir":
		return localBtrfsStorePlan{
			storeDir:  pathValue,
			imagePath: filepath.Join(filepath.Dir(pathValue), filepath.Base(pathValue)+".btrfs.img"),
		}, nil
	default:
		return localBtrfsStorePlan{}, fmt.Errorf("unsupported store type for btrfs init: %s", normalizedType)
	}
}

func ensureLocalBtrfsPrerequisites(plan localBtrfsStorePlan) error {
	required := []string{"mkfs.btrfs", "mount", "umount", "findmnt"}
	if plan.imagePath != "" {
		required = append(required, "truncate")
	}
	for _, command := range required {
		if _, err := localBtrfsLookPathFn(command); err != nil {
			return fmt.Errorf("%s is required for btrfs init", command)
		}
	}
	if os.Geteuid() != 0 {
		if _, err := localBtrfsLookPathFn("sudo"); err != nil {
			return fmt.Errorf("sudo is required for btrfs init")
		}
	}
	return nil
}

func ensureLoopbackBtrfsStore(plan localBtrfsStorePlan, opts localBtrfsInitOptions) (string, error) {
	if opts.Reinit {
		if err := tryUnmountStore(plan.storeDir, opts.Verbose); err != nil {
			return "", err
		}
		if err := os.Remove(plan.imagePath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
	}

	created, err := ensureLoopImage(plan.imagePath, opts)
	if err != nil {
		return "", err
	}

	allowFormat := created || opts.Reinit
	fsType, hasFS, err := detectBlockFSType(plan.imagePath)
	if err != nil {
		return "", err
	}
	if fsType != "btrfs" {
		if hasFS && !allowFormat {
			return "", fmt.Errorf("loopback image filesystem is %s, expected btrfs (rerun with --reinit)", fsType)
		}
		if _, err := runPrivilegedCommand("format loopback btrfs", "mkfs.btrfs", "-f", plan.imagePath); err != nil {
			return "", err
		}
	}

	mountFS, mounted, err := detectMountFSType(plan.storeDir)
	if err != nil {
		return "", err
	}
	if mounted {
		if mountFS == "btrfs" {
			return plan.storeDir, nil
		}
		if !opts.Reinit {
			return "", fmt.Errorf("store path %s is mounted as %s, expected btrfs", plan.storeDir, mountFS)
		}
		if err := tryUnmountStore(plan.storeDir, opts.Verbose); err != nil {
			return "", err
		}
	}

	if _, err := runPrivilegedCommand("mount loopback btrfs", "mount", "-o", "loop", plan.imagePath, plan.storeDir); err != nil {
		return "", err
	}
	if err := ensureStoreOwnership(plan.storeDir); err != nil {
		return "", err
	}
	if onBtrfs, err := localBtrfsIsBtrfsPathFn(plan.storeDir); err != nil || !onBtrfs {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("store path %s is not btrfs after loopback mount", plan.storeDir)
	}
	return plan.storeDir, nil
}

func ensureDeviceBackedBtrfsStore(plan localBtrfsStorePlan, opts localBtrfsInitOptions) (string, error) {
	target, fsType, mounted, err := detectSourceMount(plan.devicePath)
	if err != nil {
		return "", err
	}
	if mounted {
		if fsType == "btrfs" {
			return target, nil
		}
		if !opts.Reinit {
			return "", fmt.Errorf("device %s is mounted as %s, expected btrfs", plan.devicePath, fsType)
		}
		if err := tryUnmountStore(target, opts.Verbose); err != nil {
			return "", err
		}
	}

	allowFormat := opts.Reinit
	deviceFSType, hasFS, err := detectBlockFSType(plan.devicePath)
	if err != nil {
		return "", err
	}
	if deviceFSType != "btrfs" {
		if hasFS && !allowFormat {
			return "", fmt.Errorf("device filesystem is %s, expected btrfs (rerun with --reinit)", deviceFSType)
		}
		if _, err := runPrivilegedCommand("format device btrfs", "mkfs.btrfs", "-f", plan.devicePath); err != nil {
			return "", err
		}
	}

	if _, err := runPrivilegedCommand("mount btrfs device", "mount", plan.devicePath, plan.storeDir); err != nil {
		return "", err
	}
	if err := ensureStoreOwnership(plan.storeDir); err != nil {
		return "", err
	}
	if onBtrfs, err := localBtrfsIsBtrfsPathFn(plan.storeDir); err != nil || !onBtrfs {
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("store path %s is not btrfs after device mount", plan.storeDir)
	}
	return plan.storeDir, nil
}

func ensureLoopImage(path string, opts localBtrfsInitOptions) (bool, error) {
	if _, err := os.Stat(path); err == nil {
		return false, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return false, err
	}
	sizeGB := opts.StoreSizeGB
	if sizeGB <= 0 {
		sizeGB = defaultBtrfsStoreSizeGB
	}
	sizeArg := strconv.Itoa(sizeGB) + "G"
	if _, err := runLocalCommand("create loopback image", "truncate", "-s", sizeArg, path); err != nil {
		return false, err
	}
	return true, nil
}

func detectBlockFSType(path string) (string, bool, error) {
	out, err := localBtrfsRunAllowFailureFn("detect block filesystem", "blkid", "-c", "/dev/null", "-o", "value", "-s", "TYPE", path)
	if err != nil {
		if isExitStatus(err, 1) || isExitStatus(err, 2) {
			return "", false, nil
		}
		return "", false, err
	}
	fsType := strings.TrimSpace(out)
	if fsType == "" {
		return "", false, nil
	}
	return fsType, true, nil
}

func detectMountFSType(target string) (string, bool, error) {
	out, err := localBtrfsRunAllowFailureFn("detect mount filesystem", "findmnt", "-n", "-o", "TARGET,FSTYPE", "-T", target)
	if err != nil {
		if isExitStatus(err, 1) {
			return "", false, nil
		}
		return "", false, err
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return "", false, nil
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", false, nil
	}
	mountTarget := fields[0]
	if !sameMountTarget(mountTarget, target) {
		return "", false, nil
	}
	fsType := ""
	if len(fields) > 1 {
		fsType = fields[1]
	}
	return fsType, true, nil
}

func detectSourceMount(source string) (string, string, bool, error) {
	out, err := localBtrfsRunAllowFailureFn("detect source mount", "findmnt", "-n", "-o", "TARGET,FSTYPE", "-S", source)
	if err != nil {
		if isExitStatus(err, 1) {
			return "", "", false, nil
		}
		return "", "", false, err
	}
	line := strings.TrimSpace(out)
	if line == "" {
		return "", "", false, nil
	}
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return "", "", false, nil
	}
	target := fields[0]
	fsType := ""
	if len(fields) > 1 {
		fsType = fields[1]
	}
	return target, fsType, true, nil
}

func tryUnmountStore(path string, verbose bool) error {
	if strings.TrimSpace(path) == "" {
		return nil
	}
	if _, err := runPrivilegedCommand("unmount store", "umount", path); err == nil {
		return nil
	} else if isExitStatus(err, 32) {
		return nil
	}
	if _, err := runPrivilegedCommand("lazy unmount store", "umount", "-l", path); err != nil && !isExitStatus(err, 32) {
		return err
	}
	logLocalBtrfsInit(verbose, "store unmounted: %s", path)
	return nil
}

func ensureStoreOwnership(path string) error {
	uid := strconv.Itoa(os.Getuid())
	gid := strconv.Itoa(os.Getgid())
	_, err := runPrivilegedCommand("chown mounted store", "chown", "-R", uid+":"+gid, path)
	return err
}

func runPrivilegedCommand(desc string, command string, args ...string) (string, error) {
	if os.Geteuid() == 0 {
		return runLocalCommand(desc, command, args...)
	}
	sudoArgs := append([]string{command}, args...)
	return runLocalCommand(desc, "sudo", sudoArgs...)
}

func runLocalCommand(desc string, command string, args ...string) (string, error) {
	return localBtrfsRunCommandFn(desc, command, args...)
}

func runLocalBtrfsCommand(desc string, command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed != "" {
			return "", fmt.Errorf("%s: %w (%s)", desc, err, trimmed)
		}
		return "", fmt.Errorf("%s: %w", desc, err)
	}
	return trimmed, nil
}

func runLocalBtrfsCommandAllowFailure(desc string, command string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, command, args...)
	out, err := cmd.CombinedOutput()
	trimmed := strings.TrimSpace(string(out))
	if err != nil {
		if trimmed != "" {
			return trimmed, fmt.Errorf("%s: %w (%s)", desc, err, trimmed)
		}
		return trimmed, fmt.Errorf("%s: %w", desc, err)
	}
	return trimmed, nil
}

func isExitStatus(err error, code int) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == code
	}
	return false
}

func looksLikeImagePath(path string) bool {
	base := strings.ToLower(filepath.Base(path))
	return strings.HasSuffix(base, ".img") ||
		strings.HasSuffix(base, ".raw") ||
		strings.HasSuffix(base, ".qcow2")
}

func sameMountTarget(found string, expected string) bool {
	return normalizeMountTarget(found) == normalizeMountTarget(expected)
}

func normalizeMountTarget(value string) string {
	cleaned := filepath.Clean(strings.TrimSpace(value))
	if cleaned == "" {
		return ""
	}
	if resolved, err := filepath.EvalSymlinks(cleaned); err == nil {
		return filepath.Clean(resolved)
	}
	return cleaned
}

func logLocalBtrfsInit(verbose bool, format string, args ...any) {
	if !verbose {
		return
	}
	fmt.Fprintf(os.Stderr, "linux btrfs init: "+format+"\n", args...)
}

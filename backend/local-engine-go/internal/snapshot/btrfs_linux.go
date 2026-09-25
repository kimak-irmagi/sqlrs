//go:build linux

package snapshot

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

const btrfsSuperMagic int64 = 0x9123683E

var statfsFn = func(path string, stat *syscall.Statfs_t) error {
	return syscall.Statfs(path, stat)
}

var osMkdirAllBtrfs = os.MkdirAll
var execLookPathBtrfs = exec.LookPath
var osStatBtrfs = os.Stat
var osRemoveAllBtrfs = os.RemoveAll
var btrfsListSubvolumesFn = func(ctx context.Context, dir string) (string, error) {
	cmd := exec.CommandContext(ctx, "btrfs", "subvolume", "list", "-o", dir)
	out, err := cmd.CombinedOutput()
	return string(out), err
}
var runBtrfsShowFn = func(ctx context.Context, path string) (string, error) {
	cmd := exec.CommandContext(ctx, "btrfs", "subvolume", "show", path)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

type btrfsManager struct {
	runner commandRunner
}

func newBtrfsManager() Manager {
	return btrfsManager{runner: execRunner{}}
}

func formatCommand(cmd string, args []string) string {
	parts := make([]string, 0, 1+len(args))
	parts = append(parts, quoteCommandArg(cmd))
	for _, arg := range args {
		parts = append(parts, quoteCommandArg(arg))
	}
	return strings.Join(parts, " ")
}

func quoteCommandArg(arg string) string {
	if arg == "" {
		return "\"\""
	}
	if strings.ContainsAny(arg, " \t\n\"'") {
		return strconv.Quote(arg)
	}
	return arg
}

func btrfsSupported(path string) bool {
	if strings.TrimSpace(path) == "" {
		return false
	}
	if _, err := execLookPathBtrfs("btrfs"); err != nil {
		return false
	}
	var stat syscall.Statfs_t
	if err := statfsFn(path, &stat); err != nil {
		return false
	}
	return stat.Type == btrfsSuperMagic
}

func (btrfsManager) Kind() string {
	return "btrfs"
}

func (btrfsManager) Capabilities() Capabilities {
	return Capabilities{
		RequiresDBStop:        true,
		SupportsWritableClone: true,
		SupportsSendReceive:   false,
	}
}

func (m btrfsManager) Clone(ctx context.Context, srcDir string, destDir string) (CloneResult, error) {
	if strings.TrimSpace(srcDir) == "" || strings.TrimSpace(destDir) == "" {
		return CloneResult{}, fmt.Errorf("source and destination are required")
	}
	srcDir = filepath.Clean(srcDir)
	destDir = filepath.Clean(destDir)
	log.Printf("btrfs: clone start src=%s dest=%s", srcDir, destDir)
	if output, err := btrfsListSubvolumesFn(ctx, srcDir); err == nil {
		trimmed := strings.TrimSpace(output)
		if trimmed != "" {
			log.Printf("btrfs: clone src subvolumes src=%s\n%s", srcDir, trimmed)
		} else {
			log.Printf("btrfs: clone src subvolumes src=%s (none)", srcDir)
		}
	} else {
		log.Printf("btrfs: clone src subvolumes failed src=%s err=%v", srcDir, err)
	}
	if err := osMkdirAllBtrfs(filepath.Dir(destDir), 0o700); err != nil {
		log.Printf("btrfs: clone mkdir failed dest=%s err=%v", destDir, err)
		return CloneResult{}, err
	}
	args := []string{"subvolume", "snapshot", srcDir, destDir}
	log.Printf("btrfs: clone exec %s", formatCommand("btrfs", args))
	if err := m.runner.Run(ctx, "btrfs", args); err != nil {
		log.Printf("btrfs: clone snapshot failed src=%s dest=%s err=%v", srcDir, destDir, err)
		return CloneResult{}, err
	}
	if output, err := btrfsListSubvolumesFn(ctx, destDir); err == nil {
		trimmed := strings.TrimSpace(output)
		if trimmed != "" {
			log.Printf("btrfs: clone dest subvolumes dest=%s\n%s", destDir, trimmed)
		} else {
			log.Printf("btrfs: clone dest subvolumes dest=%s (none)", destDir)
		}
	} else {
		log.Printf("btrfs: clone dest subvolumes failed dest=%s err=%v", destDir, err)
	}
	log.Printf("btrfs: clone complete src=%s dest=%s", srcDir, destDir)
	return CloneResult{
		MountDir: destDir,
		Cleanup: func() error {
			log.Printf("btrfs: clone cleanup start dest=%s", destDir)
			log.Printf("btrfs: clone cleanup exec %s", formatCommand("btrfs", []string{"subvolume", "delete", destDir}))
			return m.runner.Run(context.Background(), "btrfs", []string{"subvolume", "delete", destDir})
		},
	}, nil
}

func (m btrfsManager) Snapshot(ctx context.Context, srcDir string, destDir string) error {
	if strings.TrimSpace(srcDir) == "" || strings.TrimSpace(destDir) == "" {
		return fmt.Errorf("source and destination are required")
	}
	srcDir = filepath.Clean(srcDir)
	destDir = filepath.Clean(destDir)
	log.Printf("btrfs: snapshot start src=%s dest=%s", srcDir, destDir)
	if output, err := btrfsListSubvolumesFn(ctx, srcDir); err == nil {
		trimmed := strings.TrimSpace(output)
		if trimmed != "" {
			log.Printf("btrfs: snapshot src subvolumes src=%s\n%s", srcDir, trimmed)
		} else {
			log.Printf("btrfs: snapshot src subvolumes src=%s (none)", srcDir)
		}
	} else {
		log.Printf("btrfs: snapshot src subvolumes failed src=%s err=%v", srcDir, err)
	}
	if err := osMkdirAllBtrfs(filepath.Dir(destDir), 0o700); err != nil {
		log.Printf("btrfs: snapshot mkdir failed dest=%s err=%v", destDir, err)
		return err
	}
	if _, err := osStatBtrfs(destDir); err == nil {
		log.Printf("btrfs: snapshot precheck exec %s", formatCommand("btrfs", []string{"subvolume", "show", destDir}))
		showOut, showErr := runBtrfsShowFn(ctx, destDir)
		if showErr == nil {
			trimmed := strings.TrimSpace(showOut)
			if trimmed == "" {
				trimmed = "(no output)"
			}
			log.Printf("btrfs: snapshot dest is subvolume dest=%s\n%s", destDir, trimmed)
			return fmt.Errorf("snapshot dest is an existing subvolume: %s", destDir)
		}
		if strings.TrimSpace(showOut) != "" {
			log.Printf("btrfs: snapshot precheck show output dest=%s\n%s", destDir, strings.TrimSpace(showOut))
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	log.Printf("btrfs: snapshot exec %s", formatCommand("btrfs", []string{"subvolume", "snapshot", "-r", srcDir, destDir}))
	if err := m.runner.Run(ctx, "btrfs", []string{"subvolume", "snapshot", "-r", srcDir, destDir}); err != nil {
		log.Printf("btrfs: snapshot failed src=%s dest=%s err=%v", srcDir, destDir, err)
		return err
	}
	if output, err := btrfsListSubvolumesFn(ctx, destDir); err == nil {
		trimmed := strings.TrimSpace(output)
		if trimmed != "" {
			log.Printf("btrfs: snapshot dest subvolumes dest=%s\n%s", destDir, trimmed)
		} else {
			log.Printf("btrfs: snapshot dest subvolumes dest=%s (none)", destDir)
		}
	} else {
		log.Printf("btrfs: snapshot dest subvolumes failed dest=%s err=%v", destDir, err)
	}
	log.Printf("btrfs: snapshot complete src=%s dest=%s", srcDir, destDir)
	return nil
}

func (m btrfsManager) Destroy(ctx context.Context, dir string) error {
	log.Printf("btrfs: destroy exec %s", formatCommand("btrfs", []string{"subvolume", "delete", dir}))
	err := m.runner.Run(ctx, "btrfs", []string{"subvolume", "delete", dir})
	if err == nil {
		return nil
	}
	output, listErr := btrfsListSubvolumesFn(ctx, dir)
	if listErr == nil {
		trimmed := strings.TrimSpace(output)
		if trimmed != "" {
			return fmt.Errorf("%w: nested subvolumes:\n%s", err, trimmed)
		}
	}
	return err
}

func (m btrfsManager) EnsureSubvolume(ctx context.Context, path string) error {
	if strings.TrimSpace(path) == "" {
		return fmt.Errorf("path is required")
	}
	path = filepath.Clean(path)
	if err := osMkdirAllBtrfs(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	if _, err := osStatBtrfs(path); err == nil {
		log.Printf("btrfs: ensure subvolume exec %s", formatCommand("btrfs", []string{"subvolume", "show", path}))
		if m.runner.Run(ctx, "btrfs", []string{"subvolume", "show", path}) == nil {
			return nil
		}
		if err := osRemoveAllBtrfs(path); err != nil {
			return fmt.Errorf("path exists but is not a btrfs subvolume: %s", path)
		}
		log.Printf("btrfs: ensure subvolume exec %s", formatCommand("btrfs", []string{"subvolume", "create", path}))
		return m.runner.Run(ctx, "btrfs", []string{"subvolume", "create", path})
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	log.Printf("btrfs: ensure subvolume exec %s", formatCommand("btrfs", []string{"subvolume", "create", path}))
	return m.runner.Run(ctx, "btrfs", []string{"subvolume", "create", path})
}

func (m btrfsManager) IsSubvolume(ctx context.Context, path string) (bool, error) {
	if strings.TrimSpace(path) == "" {
		return false, fmt.Errorf("path is required")
	}
	path = filepath.Clean(path)
	if _, err := osStatBtrfs(path); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return false, nil
		}
		return false, err
	}
	log.Printf("btrfs: is subvolume exec %s", formatCommand("btrfs", []string{"subvolume", "show", path}))
	if err := m.runner.Run(ctx, "btrfs", []string{"subvolume", "show", path}); err != nil {
		return false, nil
	}
	return true, nil
}

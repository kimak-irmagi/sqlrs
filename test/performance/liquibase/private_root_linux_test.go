package benchmark

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrivateRootDisposalRequiresExactOwnerAndReapsHelper(t *testing.T) {
	for _, failStart := range []bool{false, true} {
		root := t.TempDir()
		token := strings.Repeat("a", 64)
		id := strings.Repeat("b", 64)
		if err := os.WriteFile(filepath.Join(root, ".collector-owner"), []byte(token), 0600); err != nil {
			t.Fatal(err)
		}
		owned := filepath.Join(root, "owned")
		if err := os.WriteFile(owned, []byte("private"), 0600); err != nil {
			t.Fatal(err)
		}
		removed := false
		checked := false
		run := func(_ context.Context, args ...string) ([]byte, error) {
			switch args[0] {
			case "create":
				found := false
				for _, arg := range args {
					if arg == "type=bind,src="+root+",dst=/sqlrs-benchmark-private" {
						found = true
					}
				}
				if !found {
					t.Fatal("wrong private root mount")
				}
				return []byte(id), nil
			case "start":
				if args[2] != id {
					t.Fatal("wrong helper")
				}
				if failStart {
					return nil, errors.New("start")
				}
				return nil, os.Remove(owned)
			case "container":
				if args[1] == "rm" {
					if args[3] != id {
						t.Fatal("wrong removal")
					}
					removed = true
					return nil, nil
				}
				if args[1] == "ls" {
					checked = true
					return nil, nil
				}
			}
			return nil, errors.New("unexpected command")
		}
		err := cleanupPrivateRoot(context.Background(), root, token, "postgres@sha256:"+strings.Repeat("c", 64), run)
		if (err != nil) != failStart || !removed || !checked {
			t.Fatal("helper outcome/reaping differs")
		}
	}
}

func TestPrivateRootDisposalRefusesUnprovedTargets(t *testing.T) {
	root := t.TempDir()
	token := strings.Repeat("a", 64)
	if err := os.WriteFile(filepath.Join(root, ".collector-owner"), []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	calls := 0
	run := func(context.Context, ...string) ([]byte, error) { calls++; return nil, nil }
	for _, item := range []struct{ root, token, image string }{{"/", token, "postgres@sha256:" + token}, {"relative", token, "postgres@sha256:" + token}, {root, "wrong", "postgres@sha256:" + token}, {root, token, "postgres:latest"}, {root, strings.Repeat("b", 64), "postgres@sha256:" + token}} {
		if cleanupPrivateRoot(context.Background(), item.root, item.token, item.image, run) == nil {
			t.Fatal("unproved target accepted")
		}
	}
	if calls != 0 {
		t.Fatal("invalid target reached Docker")
	}
}

func TestPrivateRootDisposalRejectsSymlinkRootWithoutTouchingTarget(t *testing.T) {
	root := t.TempDir()
	token := strings.Repeat("a", 64)
	marker := filepath.Join(root, ".collector-owner")
	if err := os.WriteFile(marker, []byte(token), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(t.TempDir(), "link")
	if err := os.Symlink(root, link); err != nil {
		t.Fatal(err)
	}
	run := func(context.Context, ...string) ([]byte, error) {
		t.Fatal("symlink root reached Docker")
		return nil, nil
	}
	if cleanupPrivateRoot(context.Background(), link, token, "postgres@sha256:"+token, run) == nil {
		t.Fatal("symlink root accepted")
	}
	if data, err := os.ReadFile(marker); err != nil || string(data) != token {
		t.Fatal("foreign target changed")
	}
}

package benchmark

import (
	"context"
	"debug/buildinfo"
	"encoding/json"
	"errors"
	"path/filepath"
	"runtime/debug"
	"strings"
	"testing"
)

func TestCleanArtifactRequiresExactSettings(t *testing.T) {
	sha := strings.Repeat("a", 40)
	info := &buildinfo.BuildInfo{Settings: []debug.BuildSetting{{Key: "GOOS", Value: "linux"}, {Key: "GOARCH", Value: "amd64"}, {Key: "vcs.revision", Value: sha}, {Key: "vcs.modified", Value: "false"}}}
	if validateArtifact(info, sha) != nil {
		t.Fatal("valid artifact refused")
	}
	for index := range info.Settings {
		saved := info.Settings[index].Value
		info.Settings[index].Value = "wrong"
		if validateArtifact(info, sha) == nil {
			t.Fatal("bad metadata accepted")
		}
		info.Settings[index].Value = saved
	}
}

func TestCleanupRemovesOnlyExactOwnedContainerAndChecksAbsence(t *testing.T) {
	root := t.TempDir()
	owned := strings.Repeat("a", 64)
	foreign := strings.Repeat("b", 64)
	for _, stillPresent := range []bool{false, true} {
		removed := []string{}
		checked := false
		run := func(_ context.Context, args ...string) ([]byte, error) {
			switch args[1] {
			case "ls":
				for _, arg := range args {
					if arg == "id="+owned {
						checked = true
						if stillPresent {
							return []byte(owned), nil
						}
						return nil, nil
					}
				}
				return []byte(owned + "\n" + foreign), nil
			case "inspect":
				mount := filepath.Join(root, "store")
				if args[2] == foreign {
					mount = root + "-other"
				}
				data, _ := json.Marshal([]any{map[string]any{"Id": args[2], "Name": "/sqlrs-db-owned", "Mounts": []any{map[string]string{"Source": mount}}}})
				return data, nil
			case "rm":
				removed = append(removed, args[3])
				return nil, nil
			}
			return nil, errors.New("unexpected command")
		}
		err := cleanupDockerE2EContainers(context.Background(), root, run)
		if (err != nil) != stillPresent || !checked || len(removed) != 1 || removed[0] != owned {
			t.Fatal("ownership/absence differs")
		}
	}
}

func TestEventCounterPreservesStreamError(t *testing.T) {
	cause := errors.New("invalid stream")
	r := &benchmarkDockerCreates{err: cause}
	if _, _, err := r.countNano(1, 2, "lb"); !errors.Is(err, cause) {
		t.Fatal("stream failure ignored")
	}
}

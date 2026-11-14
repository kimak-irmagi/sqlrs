package benchmark

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// HC01 exercises the real event parser without running a Docker daemon.
func TestHelperEventStreamIntegrity(t *testing.T) {
	valid := `{"Action":"create","timeNano":10,"Actor":{"Attributes":{"image":"lb","name":"sqlrs-lb-1"}}}`
	for _, item := range []struct {
		name, line string
	}{
		{"malformed", `PRIVATE_SENTINEL`},
		{"truncated", valid[:len(valid)-1]},
		{"action", strings.Replace(valid, `"create"`, `"destroy"`, 1)},
		{"missing_image", strings.Replace(valid, `"image":"lb",`, "", 1)},
		{"missing_name", strings.Replace(valid, `,"name":"sqlrs-lb-1"`, "", 1)},
		{"zero_time", strings.Replace(valid, `:10,`, `:0,`, 1)},
		{"negative_time", strings.Replace(valid, `:10,`, `:-1,`, 1)},
		{"oversized", strings.Repeat("x", (1<<20)+1)},
	} {
		t.Run(item.name, func(t *testing.T) {
			r := &benchmarkDockerCreates{}
			r.consumeEvents(strings.NewReader(valid + "\n" + item.line + "\n" + valid + "\n"))
			count, probes, err := r.countNano(1, 20, "lb")
			if err == nil || count != 0 || probes != 0 {
				t.Fatal("broken stream produced a qualified count")
			}
			if strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
				t.Fatal("event diagnostic exposed raw data")
			}
		})
	}
	t.Run("valid", func(t *testing.T) {
		r := &benchmarkDockerCreates{}
		r.consumeEvents(strings.NewReader(valid + "\n"))
		want := []benchmarkContainerCreate{{at: 10, image: "lb", name: "sqlrs-lb-1"}}
		if !reflect.DeepEqual(r.events, want) || r.err != nil {
			t.Fatal("decoded event differs")
		}
	})
	t.Run("read_failure", func(t *testing.T) {
		cause := errors.New("stream unavailable")
		r := &benchmarkDockerCreates{}
		r.consumeEvents(io.MultiReader(strings.NewReader(valid+"\n"), helperFailingReader{cause}))
		if count, probes, err := r.countNano(1, 20, "lb"); !errors.Is(err, cause) || count != 0 || probes != 0 || len(r.events) != 1 {
			t.Fatal("partial stream ignored its read failure")
		}
	})
}

type helperFailingReader struct{ err error }

func (r helperFailingReader) Read([]byte) (int, error) { return 0, r.err }

// HC02 verifies both interval endpoints and separate image-probe accounting.
func TestHelperCounterTimeWrapperAndExactEndpoints(t *testing.T) {
	r := &benchmarkDockerCreates{events: []benchmarkContainerCreate{
		{at: 9, image: "lb", name: "before"},
		{at: 10, image: "lb", name: "sqlrs-lb-1"},
		{at: 11, image: "db", name: "unrelated"},
		{at: 12, image: "lb", name: "probe"},
		{at: 20, image: "lb", name: "sqlrs-step-1"},
		{at: 21, image: "lb", name: "after"},
	}}
	count, probes, err := r.count(time.Unix(0, 10), time.Unix(0, 20), "lb")
	if count != 4 || probes != 1 || err != nil {
		t.Fatal("time wrapper/endpoint/image classification differs")
	}
	nanos, nanoProbes, nanoErr := r.countNano(10, 20, "lb")
	if nanos != count || nanoProbes != probes || nanoErr != nil {
		t.Fatal("time and nanosecond counters disagree")
	}
}

// HC03 scopes command lookup to a fake executable, never the user's daemon.
func TestHelperDockerCommandAdapter(t *testing.T) {
	for _, mode := range []string{"success", "failure", "cancelled"} {
		t.Run(mode, func(t *testing.T) {
			dir := t.TempDir()
			marker := filepath.Join(dir, "started")
			script := "#!/bin/sh\n: > \"$HELPER_START_MARKER\"\nif [ \"$1\" = failure ]; then\n  printf PRIVATE_SENTINEL >&2\n  exit 7\nfi\nprintf '%s\\n' \"$@\"\n"
			if err := os.WriteFile(filepath.Join(dir, "docker"), []byte(script), 0700); err != nil {
				t.Fatal(err)
			}
			t.Setenv("PATH", dir)
			t.Setenv("HELPER_START_MARKER", marker)
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			if mode == "cancelled" {
				cancel()
			}
			output, err := runDockerE2ECommand(ctx, mode, "arg with spaces", "--exact=id")
			if mode == "success" {
				if err != nil || string(output) != "success\narg with spaces\n--exact=id\n" {
					t.Fatal("command arguments/output changed")
				}
			} else if err == nil || len(output) != 0 || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
				t.Fatal("failed command returned partial or private output")
			}
			_, statErr := os.Stat(marker)
			if mode == "cancelled" {
				if !errors.Is(statErr, os.ErrNotExist) {
					t.Fatal("cancelled command was started")
				}
			} else if statErr != nil {
				t.Fatal("test did not reach the fake executable")
			}
		})
	}
}

// helperDockerStep is an exact command transcript, not a permissive mock.
type helperDockerStep struct {
	args   []string
	output []byte
	err    error
}

// HC04 observes every boundary and checks another valid entry is still retired.
func TestHelperDockerCleanupFaultBoundaries(t *testing.T) {
	for _, fault := range []string{"list", "nonhex", "inspect_absent", "inspect_present", "inspect_list_error", "json", "empty", "multiple", "id", "name", "foreign_root", "remove", "absence_query"} {
		t.Run(fault, func(t *testing.T) {
			root := t.TempDir()
			id, next := strings.Repeat("a", 64), strings.Repeat("b", 64)
			cause := errors.New("injected command failure")
			list := func(filter string) []string {
				return []string{"container", "ls", "-a", "--no-trunc", "--filter", filter, "--format", "{{.ID}}"}
			}
			metadata := func(target, name, mount string) []byte {
				data, err := json.Marshal([]any{map[string]any{"Id": target, "Name": name, "Mounts": []any{map[string]string{"Source": mount}}}})
				if err != nil {
					t.Fatal(err)
				}
				return data
			}
			if fault == "nonhex" {
				id = strings.Repeat("z", 64)
			}
			steps := []helperDockerStep{{args: list("name=sqlrs-"), output: []byte(id + "\n" + next)}}
			if fault == "list" {
				steps[0].err = cause
			} else {
				inspect := helperDockerStep{args: []string{"container", "inspect", id}, output: metadata(id, "/sqlrs-db-owned", filepath.Join(root, "store"))}
				switch fault {
				case "inspect_absent", "inspect_present", "inspect_list_error":
					inspect.err = cause
					steps = append(steps, inspect)
					absence := helperDockerStep{args: list("id=" + id)}
					if fault == "inspect_present" {
						absence.output = []byte(id)
					} else if fault == "inspect_list_error" {
						absence.err = cause
					}
					steps = append(steps, absence)
				case "nonhex": // Invalid IDs must not reach inspect.
				default:
					switch fault {
					case "json":
						inspect.output = []byte("broken")
					case "empty":
						inspect.output = []byte("[]")
					case "multiple":
						inspect.output = []byte("[{},{}]")
					case "id":
						inspect.output = metadata(next, "/sqlrs-db-owned", root)
					case "name":
						inspect.output = metadata(id, "/not-sqlrs", root)
					case "foreign_root":
						inspect.output = metadata(id, "/sqlrs-db-foreign", root+"-other")
					}
					steps = append(steps, inspect)
					if fault == "remove" || fault == "absence_query" {
						remove := helperDockerStep{args: []string{"container", "rm", "-f", id}}
						if fault == "remove" {
							remove.err = cause
						}
						steps = append(steps, remove)
						if fault == "absence_query" {
							steps = append(steps, helperDockerStep{args: list("id=" + id), err: cause})
						}
					}
				}
				steps = append(steps,
					helperDockerStep{args: []string{"container", "inspect", next}, output: metadata(next, "/sqlrs-db-next", root)},
					helperDockerStep{args: []string{"container", "rm", "-f", next}},
					helperDockerStep{args: list("id=" + next)},
				)
			}
			calls := 0
			run := func(_ context.Context, args ...string) ([]byte, error) {
				if calls >= len(steps) || !reflect.DeepEqual(args, steps[calls].args) {
					t.Fatalf("unexpected Docker command at boundary %d: %v", calls, args)
				}
				step := steps[calls]
				calls++
				return step.output, step.err
			}
			err := cleanupDockerE2EContainers(context.Background(), root, run)
			wantError := fault != "inspect_absent" && fault != "foreign_root"
			if (err != nil) != wantError || calls != len(steps) {
				t.Fatal("cleanup skipped its target boundary or accepted incomplete evidence")
			}
			if fault == "list" || fault == "inspect_present" || fault == "inspect_list_error" || fault == "remove" {
				if !errors.Is(err, cause) {
					t.Fatal("underlying command failure lost")
				}
			}
		})
	}
}

// HC04 rejects unrecognized names without widening any allowed prefix.
func TestHelperDockerCleanupNameWhitelist(t *testing.T) {
	for _, name := range []string{"/sqlrs-step-x", "/sqlrs-db-x", "/sqlrs-lb-x", "/sqlrs-managed-physical_x"} {
		if !isDockerE2ERuntimeName(name) {
			t.Fatal("supported runtime name rejected")
		}
	}
	for _, name := range []string{"", "/other", "sqlrs-db-x", "/sqlrs-database-x", "/sqlrs-managed-physicalx"} {
		if isDockerE2ERuntimeName(name) {
			t.Fatal("unrecognized runtime name accepted")
		}
	}
}

// HC05 reports shape, never source data, for either side of a malformed row.
func TestHelperOracleMalformedRowsArePrivate(t *testing.T) {
	valid := "changelog|1|id|author|path|checksum||"
	for _, item := range []struct{ actual, expected string }{
		{"changelog|PRIVATE_SENTINEL", valid},
		{valid + "|PRIVATE_SENTINEL", valid},
		{"changelog|actual", "changelog|PRIVATE_SENTINEL"},
	} {
		if got := oracleMismatchParts([]string{item.actual}, item.expected); !reflect.DeepEqual(got, []string{"row_shape"}) {
			t.Fatal("shape diagnostics differ")
		}
	}
}

// HC06 rejects every non-regular marker before attempting private disposal.
func TestHelperPrivateMarkerMissingDirectoryOrSymlink(t *testing.T) {
	for _, kind := range []string{"missing", "directory", "symlink"} {
		t.Run(kind, func(t *testing.T) {
			root, foreign := t.TempDir(), t.TempDir()
			token := strings.Repeat("a", 64)
			sentinel := filepath.Join(foreign, "sentinel")
			if err := os.WriteFile(sentinel, []byte(token), 0600); err != nil {
				t.Fatal(err)
			}
			local := filepath.Join(root, "untouched")
			if err := os.WriteFile(local, []byte("local"), 0600); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(root, ".collector-owner")
			if kind == "directory" {
				if err := os.Mkdir(marker, 0700); err != nil {
					t.Fatal(err)
				}
			} else if kind == "symlink" {
				if err := os.Symlink(sentinel, marker); err != nil {
					t.Fatal(err)
				}
			}
			before, err := os.ReadDir(root)
			if err != nil {
				t.Fatal(err)
			}
			calls := 0
			run := func(context.Context, ...string) ([]byte, error) { calls++; return nil, nil }
			if cleanupPrivateRoot(context.Background(), root, token, "postgres@sha256:"+token, run) == nil || calls != 0 {
				t.Fatal("unproved marker reached Docker")
			}
			after, err := os.ReadDir(root)
			if err != nil || !reflect.DeepEqual(before, after) {
				t.Fatal("private root changed")
			}
			for path, want := range map[string]string{sentinel: token, local: "local"} {
				if data, err := os.ReadFile(path); err != nil || string(data) != want {
					t.Fatal("foreign/local data changed")
				}
			}
		})
	}
}

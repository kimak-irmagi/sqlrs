package runtime

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// exactAbsenceCapability keeps EA01–EA03 compilable before implementation;
// absence of the approved capability is a failing requirement, not a skip.
type exactAbsenceCapability interface {
	VerifyContainerAbsent(context.Context, string) error
}

// EA01's read-only inventory must not enter run's host-mount preparation.
// An incomplete mount configuration makes that unwanted path fail before
// any system command; no real device or mount service is touched by this test.
func TestDockerExactAbsenceDoesNotMountHostStore(t *testing.T) {
	t.Setenv("SQLRS_WSL_MOUNT_UNIT", "")
	t.Setenv("SQLRS_WSL_MOUNT_FSTYPE", "btrfs")
	root := filepath.Join(t.TempDir(), "must-not-be-created")
	t.Setenv("SQLRS_STATE_STORE", root)
	r := &fakeRunner{}
	if err := requireExactAbsence(t, r).VerifyContainerAbsent(t.Context(), strings.Repeat("a", 64)); err != nil || len(r.calls) != 1 {
		t.Fatal("read-only inventory entered host-mount preparation", err, r.calls)
	}
	if _, err := os.Stat(root); !os.IsNotExist(err) {
		t.Fatal("inventory created host resources", err)
	}
}

func requireExactAbsence(t *testing.T, runner commandRunner) exactAbsenceCapability {
	t.Helper()
	r := NewDocker(Options{Binary: "docker", Runner: runner})
	v, ok := any(r).(exactAbsenceCapability)
	if !ok {
		t.Fatal("Docker runtime lacks exact full-ID absence verification")
	}
	return v
}

// EA01: IDs are immutable full identities, never Docker name/prefix/options.
func TestDockerExactAbsenceRejectsInvalidIDs(t *testing.T) {
	for _, id := range []string{"", " ", strings.Repeat("a", 12), strings.Repeat("A", 64), strings.Repeat("g", 64), strings.Repeat("a", 65), "--all", "sqlrs-lb-test", " " + strings.Repeat("a", 64)} {
		t.Run(id, func(t *testing.T) {
			r := &fakeRunner{}
			if err := requireExactAbsence(t, r).VerifyContainerAbsent(t.Context(), id); err == nil || len(r.calls) != 0 {
				t.Fatal("invalid identity reached Docker or established absence", err, r.calls)
			}
		})
	}
}

// EA01/EA02: only a successful empty all-container inventory proves absence.
// Existing exact-name inspection behavior is not changed by these tests.
func TestDockerExactAbsenceInventory(t *testing.T) {
	id := strings.Repeat("a", 64)
	cause := errors.New("daemon unavailable: no such container")
	for _, tc := range []struct {
		name, output string
		err          error
		absent       bool
	}{
		{name: "empty", absent: true},
		{name: "whitespace", output: " \r\n\t", absent: true},
		{name: "present-running", output: id + "\n"},
		{name: "present-stopped", output: id + "\r\n"},
		{name: "foreign", output: strings.Repeat("b", 64)},
		{name: "prefix", output: id[:12]},
		{name: "duplicate", output: id + "\n" + id},
		{name: "multiple", output: id + "\n" + strings.Repeat("b", 64)},
		{name: "malformed", output: "unexpected-json-or-diagnostic"},
		{name: "daemon-error-empty", err: cause},
		{name: "error-whitespace", output: "\n", err: cause},
		{name: "error-with-id", output: id, err: cause},
		{name: "deadline", err: context.DeadlineExceeded},
	} {
		t.Run(tc.name, func(t *testing.T) {
			r := &fakeRunner{responses: []runResponse{{output: tc.output, err: tc.err}}}
			err := requireExactAbsence(t, r).VerifyContainerAbsent(t.Context(), id)
			if (err == nil) != tc.absent || tc.err != nil && !errors.Is(err, tc.err) {
				t.Fatal("wrong absence/error classification", err)
			}
			want := []string{"container", "ls", "-a", "--no-trunc", "--filter", "id=" + id, "--format", "{{.ID}}"}
			if len(r.calls) != 1 || r.calls[0].name != "docker" || r.calls[0].stdin != nil || !reflect.DeepEqual(r.calls[0].args, want) {
				t.Fatal("not one exact read-only all-container inventory", r.calls)
			}
		})
	}
}

// exactAbsenceCancelRunner controls only the existing command boundary. It
// models cancellation timing without any product hook or mutable global state.
type exactAbsenceCancelRunner struct {
	entered chan struct{}
	cancel  context.CancelFunc
	calls   int
}

func (r *exactAbsenceCancelRunner) Run(ctx context.Context, _ string, _ []string, _ *string) (string, error) {
	r.calls++
	if r.cancel != nil {
		r.cancel()
		return "", nil
	}
	close(r.entered)
	<-ctx.Done()
	return "", ctx.Err()
}

// EA03: neither cancellation nor cancelled successful-looking output is proof.
func TestDockerExactAbsenceCancellation(t *testing.T) {
	id := strings.Repeat("a", 64)
	t.Run("entry", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		r := &fakeRunner{}
		err := requireExactAbsence(t, r).VerifyContainerAbsent(ctx, id)
		if !errors.Is(err, context.Canceled) || len(r.calls) != 0 {
			t.Fatal("cancelled entry invoked Docker or established absence", err, r.calls)
		}
	})
	t.Run("blocked-command", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		r := &exactAbsenceCancelRunner{entered: make(chan struct{})}
		v := requireExactAbsence(t, r)
		result := make(chan error, 1)
		go func() { result <- v.VerifyContainerAbsent(ctx, id) }()
		select {
		case <-r.entered:
		case <-ctx.Done():
			t.Fatal("command did not reach cancellation barrier", ctx.Err())
		}
		cancel()
		if err := <-result; !errors.Is(err, context.Canceled) || r.calls != 1 {
			t.Fatal("blocked command cancellation became absence", err, r.calls)
		}
	})
	t.Run("cancel-before-empty-reply", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		defer cancel()
		r := &exactAbsenceCancelRunner{cancel: cancel}
		err := requireExactAbsence(t, r).VerifyContainerAbsent(ctx, id)
		if !errors.Is(err, context.Canceled) || r.calls != 1 {
			t.Fatal("cancelled successful-looking reply established absence", err, r.calls)
		}
	})
}

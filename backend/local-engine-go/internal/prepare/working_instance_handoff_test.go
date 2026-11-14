package prepare

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/store"
)

type handoffOwnedStore struct {
	fakeStore
	live []store.InstanceEntry
	err  error
}

func (s *handoffOwnedStore) ListInstances(context.Context, store.InstanceFilters) ([]store.InstanceEntry, error) {
	return s.live, s.err
}

// WH06: a job-shaped path is not deletion authority after runtime handoff.
func TestWorkingInstanceHandoffJobDirectory(t *testing.T) {
	root := t.TempDir()
	runtimeDir := filepath.Join(root, "jobs", "handoff-job", "runtime_assigned")
	if err := os.MkdirAll(runtimeDir, 0700); err != nil {
		t.Fatal(err)
	}
	marker := filepath.Join(runtimeDir, "live-data")
	if err := os.WriteFile(marker, []byte("instance-owned"), 0600); err != nil {
		t.Fatal(err)
	}
	s := &handoffOwnedStore{live: []store.InstanceEntry{{InstanceID: "instance", RuntimeDir: &runtimeDir}}}
	m := &PrepareService{store: s, stateStoreRoot: root}
	if err := m.removeJobDir("handoff-job"); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(marker); err != nil || string(body) != "instance-owned" {
		t.Fatal("job cleanup removed published runtime bytes", err)
	}
}

// WH06: fail closed on unreadable ownership, but do not mistake a neighboring
// job with a shared string prefix for an owner of the directory being removed.
func TestWorkingInstanceHandoffJobDirectoryBoundaries(t *testing.T) {
	for _, variant := range []string{"unreadable", "exact-directory", "neighbor"} {
		t.Run(variant, func(t *testing.T) {
			root := t.TempDir()
			jobDir := filepath.Join(root, "jobs", "handoff-job")
			if err := os.MkdirAll(jobDir, 0700); err != nil {
				t.Fatal(err)
			}
			marker := filepath.Join(jobDir, "owned-by-instance")
			if err := os.WriteFile(marker, []byte("keep"), 0600); err != nil {
				t.Fatal(err)
			}
			s := &handoffOwnedStore{}
			if variant == "unreadable" {
				s.err = errors.New("WH instance ownership read unavailable")
			} else {
				runtimeDir := jobDir
				if variant == "neighbor" {
					runtimeDir += "-other"
					if err := os.MkdirAll(runtimeDir, 0700); err != nil {
						t.Fatal(err)
					}
				}
				s.live = []store.InstanceEntry{{InstanceID: "instance", RuntimeDir: &runtimeDir}}
			}
			m := &PrepareService{store: s, stateStoreRoot: root}
			err := m.removeJobDir("handoff-job")
			if variant == "unreadable" {
				if !errors.Is(err, s.err) {
					t.Fatal("ownership read failure was hidden", err)
				}
			} else if err != nil {
				t.Fatal(err)
			}
			if variant == "neighbor" {
				if _, err := os.Stat(jobDir); !os.IsNotExist(err) {
					t.Fatal("unrelated neighbor prevented ordinary job cleanup", err)
				}
				if _, err := os.Stat(*s.live[0].RuntimeDir); err != nil {
					t.Fatal("job cleanup removed a neighboring instance", err)
				}
			} else if body, err := os.ReadFile(marker); err != nil || string(body) != "keep" {
				t.Fatal("uncertain or exact ownership allowed directory destruction", err)
			}
		})
	}
}

// WH08 is independent of the test fallback: public job deletion must leave
// the exact published database queryable, then ordinary instance disposal owns it.
func TestWorkingInstanceHandoffDockerOwnership(t *testing.T) {
	f := newMainPathNativeFixture(t)
	accepted, err := f.m.Submit(f.ctx, f.req)
	if err != nil {
		t.Fatal(err)
	}
	status, found := f.m.Get(accepted.JobID)
	if !found || status.Status != StatusSucceeded || status.Result == nil {
		t.Fatal("prepare incomplete", status.Error)
	}
	instances, err := f.st.ListInstances(f.ctx, store.InstanceFilters{})
	if err != nil || len(instances) != 1 || instances[0].RuntimeID == nil || len(f.rt.databases) != 3 || *instances[0].RuntimeID != f.rt.databases[2] {
		t.Fatal("result did not retain working DB", err)
	}
	runtimeID := *instances[0].RuntimeID
	for _, id := range f.rt.allocated {
		if id != runtimeID {
			if err := f.rt.VerifyContainerAbsent(f.ctx, id); err != nil {
				t.Fatal("temporary runtime survived before fallback", err)
			}
		}
	}
	deleted, found := f.m.Delete(accepted.JobID, deletion.DeleteOptions{})
	if !found || deleted.Outcome != deletion.OutcomeDeleted {
		t.Fatal("terminal job deletion failed")
	}
	mainPathAssertRows(t, f.ctx, status.Result.DSN, 3, 2)
	if instances[0].RuntimeDir == nil {
		t.Fatal("published directory missing")
	}
	if _, err := os.Stat(*instances[0].RuntimeDir); err != nil {
		t.Fatal("job deletion removed instance directory", err)
	}
	deletes, err := deletion.NewManager(deletion.Options{Store: f.st, Runtime: f.rt, StateFS: f.fs, Access: f.access, StateStoreRoot: f.root})
	if err != nil {
		t.Fatal(err)
	}
	result, found, err := deletes.DeleteInstance(f.ctx, status.Result.InstanceID, deletion.DeleteOptions{})
	if err != nil || !found || result.Outcome != deletion.OutcomeDeleted {
		t.Fatal("ordinary instance disposal failed", err)
	}
	if err := f.rt.VerifyContainerAbsent(f.ctx, runtimeID); err != nil {
		t.Fatal("instance runtime leaked before fallback", err)
	}
	if _, err := os.Stat(*instances[0].RuntimeDir); !os.IsNotExist(err) {
		t.Fatal("ordinary instance disposal retained its runtime directory", err)
	}
}

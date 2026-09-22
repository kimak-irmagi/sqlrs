package prepare

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/store"
)

func TestExecuteStateTaskReturnsCapacityErrorOnPreflight(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{
		config: &fakeConfigStore{
			values: map[string]any{
				"cache.capacity.maxBytes":      int64(0),
				"cache.capacity.reserveBytes":  int64(2000),
				"cache.capacity.highWatermark": 0.90,
				"cache.capacity.lowWatermark":  0.80,
				"cache.capacity.minStateAge":   "10m",
			},
		},
	})
	overrideCapacitySignals(t,
		func(string) (int64, int64, error) { return 1000, 800, nil },
		func(string) (int64, error) { return 0, nil },
	)

	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			OutputStateID: "state-1",
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	prepared := preparedRequest{request: Request{PrepareKind: "custom", ImageID: "image-1"}}
	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "cache_limit_too_small" {
		t.Fatalf("expected cache_limit_too_small, got %+v", errResp)
	}
}

func TestExecuteStateTaskMapsSnapshotNoSpaceToCapacityError(t *testing.T) {
	store := &fakeStore{}
	snap := &fakeStateFS{snapshotErr: errors.New("no space left on device")}
	mgr := newManagerWithStateFS(t, store, snap)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}

	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "cache_limit_too_small" {
		t.Fatalf("expected cache_limit_too_small, got %+v", errResp)
	}
	if !strings.Contains(errResp.Details, `"phase":"snapshot"`) {
		t.Fatalf("expected snapshot phase in details, got %+v", errResp)
	}
}

func TestExecutePsqlStepMapsNoSpaceOutputToCapacityError(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{
		psql: &fakePsqlRunner{
			output: "could not extend file: No space left on device",
			err:    errors.New("exit status 1"),
		},
	})
	rt := &jobRuntime{}
	prepared := preparedRequest{
		request:        Request{PrepareKind: "psql"},
		normalizedArgs: []string{"-c", "select 1"},
	}

	errResp := mgr.executePsqlStep(context.Background(), "job-1", prepared, rt, taskState{})
	if errResp == nil || errResp.Code != "cache_limit_too_small" {
		t.Fatalf("expected cache_limit_too_small, got %+v", errResp)
	}
	if !strings.Contains(errResp.Details, `"phase":"prepare_step"`) {
		t.Fatalf("expected prepare_step phase in details, got %+v", errResp)
	}
}

func TestNoSpaceFromErrorResponseBranches(t *testing.T) {
	if noSpaceFromErrorResponse("msg", "prepare_step", nil) != nil {
		t.Fatalf("expected nil for nil error response")
	}
	if noSpaceFromErrorResponse("msg", "prepare_step", errorResponse("invalid_argument", "bad", "")) != nil {
		t.Fatalf("expected nil for non-internal error code")
	}
	if mapped := noSpaceFromErrorResponse("msg", "prepare_step", errorResponse("internal_error", "bad", "no space left on device")); mapped == nil || mapped.Code != "cache_limit_too_small" {
		t.Fatalf("expected mapped no-space response from details, got %+v", mapped)
	}
	if mapped := noSpaceFromErrorResponse("msg", "prepare_step", errorResponse("internal_error", "not enough space on the disk", "")); mapped == nil || mapped.Code != "cache_limit_too_small" {
		t.Fatalf("expected mapped no-space response from message, got %+v", mapped)
	}
}

func TestStartRuntimeMapsCloneNoSpaceToCapacityError(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{
		statefs: &fakeStateFS{cloneErr: errors.New("no space left on device")},
	})
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	_, errResp := mgr.startRuntime(context.Background(), "job-1", prepared, &TaskInput{Kind: "image", ID: "image-1"})
	if errResp == nil || errResp.Code != "cache_limit_too_small" {
		t.Fatalf("expected cache_limit_too_small, got %+v", errResp)
	}
	if !strings.Contains(errResp.Details, `"phase":"prepare_step"`) {
		t.Fatalf("expected prepare_step phase in details, got %+v", errResp)
	}
}

func TestExecuteStateTaskMapsNoSpaceFromSnapshotHooksAndMetadataCommit(t *testing.T) {
	t.Run("prepare snapshot no-space", func(t *testing.T) {
		mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{
			dbms: &fakeDBMS{prepareErr: errors.New("no space left on device")},
		})
		prepared, err := mgr.prepareRequest(Request{
			PrepareKind: "psql",
			ImageID:     "image-1",
			PsqlArgs:    []string{"-c", "select 1"},
		})
		if err != nil {
			t.Fatalf("prepareRequest: %v", err)
		}
		outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
		task := taskState{
			PlanTask: PlanTask{
				TaskID:        "execute-0",
				OutputStateID: outputID,
				Input:         &TaskInput{Kind: "image", ID: "image-1"},
			},
		}
		_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp == nil || errResp.Code != "cache_limit_too_small" {
			t.Fatalf("expected cache_limit_too_small, got %+v", errResp)
		}
		if !strings.Contains(errResp.Details, `"phase":"snapshot"`) {
			t.Fatalf("expected snapshot phase in details, got %+v", errResp)
		}
	})

	t.Run("resume snapshot no-space", func(t *testing.T) {
		mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{
			dbms: &fakeDBMS{resumeErr: errors.New("no space left on device")},
		})
		prepared, err := mgr.prepareRequest(Request{
			PrepareKind: "psql",
			ImageID:     "image-1",
			PsqlArgs:    []string{"-c", "select 1"},
		})
		if err != nil {
			t.Fatalf("prepareRequest: %v", err)
		}
		outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
		task := taskState{
			PlanTask: PlanTask{
				TaskID:        "execute-0",
				OutputStateID: outputID,
				Input:         &TaskInput{Kind: "image", ID: "image-1"},
			},
		}
		_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp == nil || errResp.Code != "cache_limit_too_small" {
			t.Fatalf("expected cache_limit_too_small, got %+v", errResp)
		}
		if !strings.Contains(errResp.Details, `"phase":"snapshot"`) {
			t.Fatalf("expected snapshot phase in details, got %+v", errResp)
		}
	})

	t.Run("metadata commit no-space", func(t *testing.T) {
		mgr := newManagerWithDeps(t, &fakeStore{createStateErr: errors.New("no space left on device")}, newQueueStore(t), nil)
		prepared, err := mgr.prepareRequest(Request{
			PrepareKind: "psql",
			ImageID:     "image-1",
			PsqlArgs:    []string{"-c", "select 1"},
		})
		if err != nil {
			t.Fatalf("prepareRequest: %v", err)
		}
		outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
		task := taskState{
			PlanTask: PlanTask{
				TaskID:        "execute-0",
				OutputStateID: outputID,
				Input:         &TaskInput{Kind: "image", ID: "image-1"},
			},
		}
		_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp == nil || errResp.Code != "cache_limit_too_small" {
			t.Fatalf("expected cache_limit_too_small, got %+v", errResp)
		}
		if !strings.Contains(errResp.Details, `"phase":"metadata_commit"`) {
			t.Fatalf("expected metadata_commit phase in details, got %+v", errResp)
		}
	})
}

func TestExecuteStateTaskPersistsMeasuredStateSize(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	oldUsage := storeUsageFn
	storeUsageFn = func(path string) (int64, error) {
		if strings.Contains(path, outputID) {
			return 123, nil
		}
		return 0, nil
	}
	t.Cleanup(func() { storeUsageFn = oldUsage })

	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp != nil {
		t.Fatalf("executeStateTask: %+v", errResp)
	}
	if len(store.states) != 1 {
		t.Fatalf("expected one created state, got %+v", store.states)
	}
	if store.states[0].SizeBytes == nil || *store.states[0].SizeBytes != 123 {
		t.Fatalf("expected persisted size_bytes=123, got %+v", store.states[0].SizeBytes)
	}
}

func TestExecuteStateTaskBackfillsCachedStateSizeForLegacyState(t *testing.T) {
	stateStore := &fakeStore{}
	mgr := newManager(t, stateStore)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	stateStore.statesByID = map[string]store.StateEntry{
		outputID: {
			StateID:     outputID,
			ImageID:     "image-1",
			PrepareKind: "psql",
			PrepareArgs: "-c select 1",
			CreatedAt:   "2026-01-01T00:00:00Z",
		},
	}
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	oldUsage := storeUsageFn
	storeUsageFn = func(path string) (int64, error) {
		if strings.Contains(path, outputID) {
			return 321, nil
		}
		return 0, nil
	}
	t.Cleanup(func() { storeUsageFn = oldUsage })

	gotStateID, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp != nil {
		t.Fatalf("executeStateTask: %+v", errResp)
	}
	if gotStateID != outputID {
		t.Fatalf("expected cached state %s, got %s", outputID, gotStateID)
	}
	if len(stateStore.states) != 0 {
		t.Fatalf("expected no new state create on cache hit, got %+v", stateStore.states)
	}
	entry, ok := stateStore.statesByID[outputID]
	if !ok || entry.SizeBytes == nil || *entry.SizeBytes != 321 {
		t.Fatalf("expected backfilled size_bytes=321, got %+v", entry)
	}
}

func TestExecuteStateTaskReturnsMetadataCommitErrorWhenStateSizeMeasurementFails(t *testing.T) {
	mgr := newManager(t, &fakeStore{})
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	oldUsage := storeUsageFn
	storeUsageFn = func(path string) (int64, error) {
		if strings.Contains(path, outputID) {
			return 0, errors.New("usage boom")
		}
		return 0, nil
	}
	t.Cleanup(func() { storeUsageFn = oldUsage })

	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "internal_error" || !strings.Contains(errResp.Message, "cannot measure state size") {
		t.Fatalf("expected state size measurement error, got %+v", errResp)
	}
	if !strings.Contains(errResp.Details, `"phase":"metadata_commit"`) {
		t.Fatalf("expected metadata_commit phase in details, got %+v", errResp)
	}
	if !strings.Contains(errResp.Details, "usage boom") {
		t.Fatalf("expected measurement error details, got %+v", errResp)
	}
}

func TestExecuteStateTaskPathAndCapacityBranches(t *testing.T) {
	t.Run("missing effective image id", func(t *testing.T) {
		mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), nil)
		overrideCapacitySignals(t,
			func(string) (int64, int64, error) { return 1000, 1000, nil },
			func(string) (int64, error) { return 0, nil },
		)
		task := taskState{
			PlanTask: PlanTask{
				TaskID:        "execute-0",
				OutputStateID: "state-1",
				Input:         &TaskInput{Kind: "image", ID: "image-1"},
			},
		}
		prepared := preparedRequest{request: Request{PrepareKind: "custom"}}
		_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp == nil || errResp.Code != "internal_error" || !strings.Contains(errResp.Message, "resolved image id is required") {
			t.Fatalf("expected resolved image id error, got %+v", errResp)
		}
	})

	t.Run("resolve state paths error", func(t *testing.T) {
		mgr := newManagerWithStateFS(t, &fakeStore{}, &errorStateFS{baseErr: errors.New("boom")})
		overrideCapacitySignals(t,
			func(string) (int64, int64, error) { return 1000, 1000, nil },
			func(string) (int64, error) { return 0, nil },
		)
		task := taskState{
			PlanTask: PlanTask{
				TaskID:        "execute-0",
				OutputStateID: "state-1",
				Input:         &TaskInput{Kind: "image", ID: "image-1"},
			},
		}
		prepared := preparedRequest{request: Request{PrepareKind: "custom", ImageID: "image-1"}}
		_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp == nil || errResp.Code != "internal_error" || !strings.Contains(errResp.Message, "cannot resolve state paths") {
			t.Fatalf("expected resolve state paths error, got %+v", errResp)
		}
	})

	t.Run("cannot create states dir", func(t *testing.T) {
		root := t.TempDir()
		stateRoot := filepath.Join(root, "state-store-file")
		if err := os.WriteFile(stateRoot, []byte("x"), 0o600); err != nil {
			t.Fatalf("write state-store blocker: %v", err)
		}
		mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{stateRoot: stateRoot})
		overrideCapacitySignals(t,
			func(string) (int64, int64, error) { return 1000, 1000, nil },
			func(string) (int64, error) { return 0, nil },
		)
		task := taskState{
			PlanTask: PlanTask{
				TaskID:        "execute-0",
				OutputStateID: "state-1",
				Input:         &TaskInput{Kind: "image", ID: "image-1"},
			},
		}
		prepared := preparedRequest{request: Request{PrepareKind: "custom", ImageID: "image-1"}}
		_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp == nil || errResp.Code != "internal_error" || !strings.Contains(errResp.Message, "cannot create state dir") {
			t.Fatalf("expected create state dir error, got %+v", errResp)
		}
	})

	t.Run("ensure state dir no-space", func(t *testing.T) {
		mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{
			statefs: &fakeStateFS{ensureStateErr: errors.New("no space left on device")},
		})
		overrideCapacitySignals(t,
			func(string) (int64, int64, error) { return 1000, 1000, nil },
			func(string) (int64, error) { return 0, nil },
		)
		task := taskState{
			PlanTask: PlanTask{
				TaskID:        "execute-0",
				OutputStateID: "state-1",
				Input:         &TaskInput{Kind: "image", ID: "image-1"},
			},
		}
		prepared := preparedRequest{request: Request{PrepareKind: "custom", ImageID: "image-1"}}
		_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp == nil || errResp.Code != "cache_limit_too_small" {
			t.Fatalf("expected cache_limit_too_small, got %+v", errResp)
		}
		if !strings.Contains(errResp.Details, `"phase":"prepare_step"`) {
			t.Fatalf("expected prepare_step phase in details, got %+v", errResp)
		}
	})
}

// Another builder can publish while this request prepares its state directory.
// Signal that boundary explicitly instead of relying on filesystem latency.
type publishDuringStateSetup struct {
	fakeStateFS
	publish func()
}

func (f *publishDuringStateSetup) EnsureStateDir(ctx context.Context, path string) error {
	if err := f.fakeStateFS.EnsureStateDir(ctx, path); err != nil {
		return err
	}
	f.publish()
	return nil
}

func TestExecuteStateTaskLockBranches(t *testing.T) {
	t.Run("reuse state published before build lock", func(t *testing.T) {
		st := &fakeStore{}
		fs := &publishDuringStateSetup{publish: func() {
			st.statesByID = map[string]store.StateEntry{
				"state-1": {StateID: "state-1", ImageID: "image-1"},
			}
		}}
		mgr := newManagerWithDeps(t, st, newQueueStore(t), &testDeps{statefs: &fs.fakeStateFS})
		mgr.statefs = fs
		task := taskState{PlanTask: PlanTask{
			TaskID: "execute-0", OutputStateID: "state-1",
			Input: &TaskInput{Kind: "image", ID: "image-1"},
		}}
		prepared := preparedRequest{request: Request{PrepareKind: "custom", ImageID: "image-1"}}
		got, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp != nil || got != "state-1" {
			t.Fatalf("expected published cache state, got %q, %+v", got, errResp)
		}
		if len(fs.cloneCalls) != 0 || len(fs.snapshotCalls) != 0 || len(st.states) != 0 {
			t.Fatal("rebuilt a state published before acquiring its build lock")
		}
	})

	t.Run("cache lookup error inside state build lock", func(t *testing.T) {
		store := &nthGetStateErrStore{
			fakeStore: fakeStore{},
			errOn:     2,
			err:       errors.New("boom"),
		}
		mgr := newManagerWithDeps(t, store, newQueueStore(t), nil)
		overrideCapacitySignals(t,
			func(string) (int64, int64, error) { return 1000, 1000, nil },
			func(string) (int64, error) { return 0, nil },
		)
		task := taskState{
			PlanTask: PlanTask{
				TaskID:        "execute-0",
				OutputStateID: "state-1",
				Input:         &TaskInput{Kind: "image", ID: "image-1"},
			},
		}
		prepared := preparedRequest{request: Request{PrepareKind: "custom", ImageID: "image-1"}}
		_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp == nil || errResp.Code != "internal_error" || !strings.Contains(errResp.Message, "cannot check state cache") {
			t.Fatalf("expected state cache lookup error, got %+v", errResp)
		}
	})

	t.Run("reset state dir error for btrfs", func(t *testing.T) {
		mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{
			statefs: &fakeStateFS{kind: "btrfs", removeErr: errors.New("boom")},
		})
		overrideCapacitySignals(t,
			func(string) (int64, int64, error) { return 1000, 1000, nil },
			func(string) (int64, error) { return 0, nil },
		)
		task := taskState{
			PlanTask: PlanTask{
				TaskID:        "execute-0",
				OutputStateID: "state-1",
				Input:         &TaskInput{Kind: "image", ID: "image-1"},
			},
		}
		prepared := preparedRequest{request: Request{PrepareKind: "custom", ImageID: "image-1"}}
		_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
		if errResp == nil || errResp.Code != "internal_error" || !strings.Contains(errResp.Message, "cannot reset state dir") {
			t.Fatalf("expected reset state dir error, got %+v", errResp)
		}
	})
}

func TestExecuteStateTaskSnapshotPhaseCapacityCheckFailure(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), nil)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}

	usageCalls := 0
	freeCalls := 0
	overrideCapacitySignals(t,
		func(string) (int64, int64, error) {
			freeCalls++
			if freeCalls == 1 {
				return 1000, 1000, nil
			}
			return 1000, 100, nil
		},
		func(path string) (int64, error) {
			if strings.Contains(path, "state-store") {
				usageCalls++
				if usageCalls == 1 {
					return 0, nil
				}
				return 950, nil
			}
			return 0, nil
		},
	)

	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "cache_limit_too_small" {
		t.Fatalf("expected cache_limit_too_small, got %+v", errResp)
	}
	if !strings.Contains(errResp.Details, `"phase":"snapshot"`) {
		t.Fatalf("expected snapshot phase in details, got %+v", errResp)
	}
}

func TestExecuteStateTaskMetadataCapacityDoesNotEvictFreshOutputState(t *testing.T) {
	store := &fakeStore{}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{
		config: &fakeConfigStore{
			values: map[string]any{
				"cache.capacity.maxBytes":      int64(0),
				"cache.capacity.reserveBytes":  int64(0),
				"cache.capacity.highWatermark": 0.90,
				"cache.capacity.lowWatermark":  0.80,
				"cache.capacity.minStateAge":   "0s",
			},
		},
	})
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}

	rootUsageCalls := 0
	overrideCapacitySignals(t,
		func(string) (int64, int64, error) { return 1000, 1000, nil },
		func(path string) (int64, error) {
			if strings.Contains(path, outputID) {
				return 850, nil
			}
			if strings.Contains(path, "state-store") {
				rootUsageCalls++
				if rootUsageCalls <= 2 {
					return 0, nil
				}
				return 950, nil
			}
			return 0, nil
		},
	)

	gotOutput, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil && gotOutput != outputID {
		t.Fatalf("unexpected output id: %q", gotOutput)
	}
	if containsString(store.deletedStates, outputID) {
		t.Fatalf("fresh output state must not be evicted, deleted=%+v", store.deletedStates)
	}
	if _, ok := store.statesByID[outputID]; !ok {
		t.Fatalf("expected output state metadata to remain present")
	}
}

func containsString(items []string, needle string) bool {
	for _, item := range items {
		if item == needle {
			return true
		}
	}
	return false
}

type nthGetStateErrStore struct {
	fakeStore
	errOn int
	err   error
	calls int
}

func (s *nthGetStateErrStore) GetState(ctx context.Context, stateID string) (store.StateEntry, bool, error) {
	s.calls++
	if s.calls == s.errOn {
		return store.StateEntry{}, false, s.err
	}
	return s.fakeStore.GetState(ctx, stateID)
}

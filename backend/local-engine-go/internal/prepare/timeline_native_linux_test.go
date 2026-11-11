package prepare

import (
	"bytes"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/preparecost"
	storepkg "github.com/sqlrs/engine-local/internal/store"
	"golang.org/x/sys/unix"
)

// timelineNativeSink permits the diagnostic emitter to finish independently
// of the synchronous native request. It never exports raw capture/log output.
type timelineNativeSink struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (s *timelineNativeSink) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.Write(data)
}
func (s *timelineNativeSink) snapshot() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.data.String()
}

// TL13: real three-step and fully cached prepare, before fallback cleanup.
// This checks actual source probes/DB contents/reuse, not just a balanced oracle.
func TestCompleteTimelineMainPathDocker(t *testing.T) {
	var reference unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &reference); err != nil {
		t.Fatal("native shared clock unavailable")
	}
	t.Setenv("SQLRS_PREPARE_COST", "1")
	t.Setenv("SQLRS_PREPARE_TIMELINE", "1")
	t.Setenv("SQLRS_PREPARE_TIMELINE_REFERENCE", strconv.FormatInt(reference.Nano(), 10))
	f := newMainPathNativeFixture(t)
	sink := &timelineNativeSink{}
	f.m.costEmitter = preparecost.NewEmitter(sink)
	for pass := 0; pass < 2; pass++ {
		readRelative := func() int64 {
			var value unix.Timespec
			if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &value); err != nil {
				t.Fatal("native clock read failed")
			}
			return value.Nano() - reference.Nano()
		}
		windowStart := readRelative()
		accepted, err := f.m.Submit(f.ctx, f.req)
		postEnd := readRelative()
		if err != nil {
			t.Fatal("timeline native request refused")
		}
		status, found := f.m.Get(accepted.JobID)
		windowEnd := readRelative()
		if !found || status.Status != StatusSucceeded || status.Result == nil {
			t.Fatal("timeline changed native execution")
		}
		mainPathAssertRows(t, f.ctx, status.Result.DSN, 3, 2)
		root := strconv.Itoa(pass + 1)
		deadline := time.Now().Add(10 * time.Second)
		for !strings.Contains(sink.snapshot(), "observer-window "+root+" ") {
			if time.Now().After(deadline) {
				t.Fatal("native timeline did not finish")
			}
			time.Sleep(10 * time.Millisecond)
		}
		counts := map[string]int{}
		var spans []preparecost.Span
		var origin, observerStart, observerEnd int64
		for _, line := range strings.Split(sink.snapshot(), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 || fields[2] != root {
				continue
			}
			if fields[1] == "begin" && (len(fields) != 6 || fields[5] != "1") {
				t.Fatal("native timeline incomplete")
			}
			if fields[1] == "span" {
				counts[fields[5]]++
				number := func(i int) int64 {
					n, err := strconv.ParseInt(fields[i], 10, 64)
					if err != nil {
						t.Fatal("invalid native numeric frame")
					}
					return n
				}
				spans = append(spans, preparecost.Span{ID: int(number(3)), Parent: int(number(4)), Operation: preparecost.Operation(fields[5]), Phase: preparecost.Phase(fields[6]), Step: int(number(7)), StartNS: number(8), EndNS: number(9), Outcome: preparecost.Outcome(fields[10])})
			}
			if fields[1] == "clock" {
				if len(fields) != 5 || fields[3] != "linux-monotonic-relative-v1" {
					t.Fatal("native clock domain differs")
				}
				origin, err = strconv.ParseInt(fields[4], 10, 64)
				if err != nil {
					t.Fatal("native clock frame invalid")
				}
			}
			if fields[1] == "observer-window" {
				observerStart, err = strconv.ParseInt(fields[3], 10, 64)
				if err != nil {
					t.Fatal("native observer start invalid")
				}
				observerEnd, err = strconv.ParseInt(fields[4], 10, 64)
				if err != nil {
					t.Fatal("native observer end invalid")
				}
			}
		}
		if len(spans) == 0 || spans[0].Operation != preparecost.Request || origin < windowStart || observerEnd < observerStart {
			t.Fatal("native shared reference not linked")
		}
		for i := range spans {
			spans[i].StartNS += origin
			spans[i].EndNS += origin
		}
		ledger, err := preparecost.BuildTimeline(spans, []preparecost.TimelineAuxSpan{
			{Kind: "client.post", StartNS: windowStart, EndNS: postEnd},
			{Kind: "client.status", StartNS: postEnd, EndNS: windowEnd},
			{Kind: "observer", StartNS: observerStart, EndNS: observerEnd},
		}, preparecost.TimelineWindow{StartNS: windowStart, EndNS: windowEnd})
		if err != nil {
			t.Fatal("actual native intervals did not qualify", err)
		}
		position, sum := int64(0), int64(0)
		for _, segment := range ledger.Segments {
			if segment.StartNS != position {
				t.Fatal("native ledger has gap")
			}
			sum += segment.EndNS - segment.StartNS
			position = segment.EndNS
		}
		if sum != windowEnd-windowStart || position != ledger.ElapsedNS {
			t.Fatal("native additive timing does not match selected wall window")
		}
		for _, key := range []string{"prepare.request", "prepare.admission", "prepare.worker", "input.publish", "binding.prepare", "input.materialize", "runner.resolve", "runner.ensure", "runner.inventory", "binding.commit", "plan.launch_prepare", "plan.input_prepare", "plan.capture_reconcile", "plan.dispose", "plan.suffix", "projection.cleanup", "finalization.confirm", "finalization.commit"} {
			if counts[key] != 1 {
				t.Fatal("native source boundary count differs", key, counts[key])
			}
		}
		for _, key := range []string{"step.bind"} {
			if counts[key] != 3 {
				t.Fatal("cached plan ordinals disappeared", key)
			}
		}
		dbCount := 2 // preview + working, or preview + separately published cached DB
		for _, key := range []string{"db.startup", "db.lock_wait", "db.clone", "db.start", "db.attach", "db.verify.startup"} {
			if counts[key] != dbCount {
				t.Fatal("two actual DB lifetimes not observed", key, counts[key])
			}
		}
		wantApply, wantSession := 3, 1
		if pass == 1 {
			wantApply, wantSession = 0, 0
		}
		if counts["liquibase.invoke"] != wantApply || counts["session.runner_start"] != wantSession || counts["session.readiness"] != wantSession {
			t.Fatal("cache invented execution")
		}
		for _, key := range []string{"admission.inputs", "admission.image", "admission.identity", "admission.commit"} {
			if counts[key] != 1 {
				t.Fatal("admission boundary missing", key)
			}
		}
		if counts["publication.activate"] != wantSession || counts["publication.commit"] != wantSession || counts["private.dispose"] != wantSession {
			t.Fatal("working handoff boundary missing")
		}
		instances, err := f.st.ListInstances(f.ctx, storepkg.InstanceFilters{})
		if err != nil || len(instances) != pass+1 {
			t.Fatal("native publications differ")
		}
		kept := map[string]bool{}
		for _, instance := range instances {
			if instance.RuntimeID == nil {
				t.Fatal("publication has no DB")
			}
			if pass == 0 && instance.InstanceID == status.Result.InstanceID && (len(f.rt.databases) != 3 || *instance.RuntimeID != f.rt.databases[2]) {
				t.Fatal("timeline changed the exact live DB handoff")
			}
			kept[*instance.RuntimeID] = true
		}
		for _, id := range f.rt.allocated {
			if !kept[id] && f.rt.VerifyContainerAbsent(f.ctx, id) != nil {
				t.Fatal("temporary container leaked before fallback")
			}
		}
	}
	if len(f.rt.runners) != 3 || len(f.rt.databases) != 3 || f.rt.runners[0] != f.rt.runners[1] || f.rt.runners[1] != f.rt.runners[2] || f.rt.databases[0] != f.rt.databases[1] || f.rt.databases[1] != f.rt.databases[2] {
		t.Fatal("timeline changed container reuse")
	}
	manager, err := deletion.NewManager(deletion.Options{Store: f.st, Runtime: f.rt, StateFS: f.fs, Access: f.access, StateStoreRoot: f.root})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := f.st.ListInstances(f.ctx, storepkg.InstanceFilters{})
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range instances {
		result, found, err := manager.DeleteInstance(f.ctx, instance.InstanceID, deletion.DeleteOptions{})
		if err != nil || !found || result.Outcome != deletion.OutcomeDeleted {
			t.Fatal("native normal instance cleanup failed")
		}
	}
	for _, id := range f.rt.allocated {
		if f.rt.VerifyContainerAbsent(f.ctx, id) != nil {
			t.Fatal("normal cleanup left allocated container")
		}
	}
}

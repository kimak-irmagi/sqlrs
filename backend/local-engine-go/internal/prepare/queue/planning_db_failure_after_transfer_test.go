package queue_test

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// PG06/PG08: an exact transfer changes the live DB owner, not the saved
// planning outcome. Selecting a known job failure must still inventory that
// shared clone without resolving SQL or granting planner cleanup authority.
func TestPlanningDBSelectedFailureAfterTransfer(t *testing.T) {
	f := newPlanningDBFixture(t, 0)
	f.complete(t)
	f.retireCapture(t)
	launch := f.firstLaunch(t)
	session, err := f.q.TransferPlanningDBToFirstSession(t.Context(), launch)
	handoffCheck(t, err)
	handoffCheck(t, f.q.RequestLiquibaseSessionRunnerCreate(t.Context(), session))
	session.RunnerID = strings.Repeat("c", 64)
	handoffCheck(t, f.q.AttachLiquibaseSessionRunner(t.Context(), session, session.RunnerID))
	handoffCheck(t, f.q.RequestLiquibaseSessionRunnerStart(t.Context(), session))
	handoffCheck(t, f.q.MarkLiquibaseSessionRunnerRunning(t.Context(), session))
	session.RunnerPhase = "running"
	_, err = f.q.RecordLiquibaseSessionReadiness(t.Context(), session, mainPathJournalReady{db: f.db, want: session}, mainPathJournalUTC)
	handoffCheck(t, err)
	invocation := mainPathNextInvocation(t, f.q, session, launch.ExecutionDigest, "one", "handoff-known-failure")
	handoffCheck(t, f.q.RequestLiquibaseSessionExec(t.Context(), session, invocation, mainPathJournalUTC))
	_, err = f.q.CompleteLiquibaseSessionInvocation(t.Context(), session, invocation, queue.LiquibaseInvocationCompletion{ExitCode: 7, ManifestDigest: pgDigest("e"), ManifestBytes: 500}, mainPathJournalUTC)
	handoffCheck(t, err)
	intent, err := f.q.SelectLiquibaseJobFailure(t.Context(), f.attempt.JobID, handoffFailure(), mainPathJournalUTC)
	handoffCheck(t, err)
	owners, err := f.q.LoadLiquibaseFailureOwners(t.Context(), intent)
	handoffCheck(t, err)
	if len(owners.Clones) != 1 || owners.Clones[0].Plan != f.clone || owners.Clones[0].DBRuntimeID != f.op.RuntimeRef || len(owners.Sessions) != 1 || owners.Sessions[0] != session {
		t.Fatal("selected failure lost the exact transferred clone/session")
	}
	before := mainPathJournalRows(t, f.db)
	_, err = f.q.SelectLiquibaseJobFailure(t.Context(), f.attempt.JobID, handoffFailure(), mainPathJournalUTC)
	handoffCheck(t, err)
	probe := &planningDBStaleFailureProbe{}
	if err := f.q.ReleasePlanningClone(t.Context(), f.attempt, 1, probe); err == nil || probe.calls != 0 {
		t.Fatal("selected session failure restored obsolete planner cleanup authority")
	}
	f.unchanged(t, before)
	handoffCheck(t, f.q.RequestLiquibaseSessionRunnerStop(t.Context(), session))
	handoffCheck(t, f.q.AcknowledgeLiquibaseSessionRunnerStop(t.Context(), session))
	handoffCheck(t, f.q.MarkLiquibaseSessionRunnerStopped(t.Context(), session))
	session.RunnerPhase = "stopped"
	handoffCheck(t, f.q.SaveLiquibaseSessionFailureDisposal(t.Context(), intent, session, mainPathJournalUTC))
}

type planningDBStaleFailureProbe struct{ calls int }

func (p *planningDBStaleFailureProbe) VerifyReleasedClone(_ context.Context, _ queue.ClonePlan, _ string) error {
	p.calls++
	return nil
}

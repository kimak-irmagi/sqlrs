package prepare

import (
	"context"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

type containerPublicView struct {
	Purpose              string
	LiquibaseImageDigest string
	Explanation          *CacheExplainPrepareResult
}

// containerJobView exposes saved terminal decisions only; it never recomputes
// cache state during status/list/event reads.
func (m *PrepareService) containerJobView(job string) (containerPublicView, error) {
	q, ok := m.queue.(interface {
		LoadContainerJobView(context.Context, string) (queue.ContainerJobView, error)
	})
	if !ok {
		return containerPublicView{}, queue.ErrPlanningSchema
	}
	v, err := q.LoadContainerJobView(context.Background(), job)
	if err != nil {
		return containerPublicView{}, err
	}
	view := containerPublicView{Purpose: v.Purpose, LiquibaseImageDigest: v.LiquibaseImageDigest}
	if v.Explanation != nil {
		d := v.Explanation
		view.Explanation = &CacheExplainPrepareResult{Decision: d.Decision, ReasonCode: d.ReasonCode, Signature: d.Signature, MatchedStateID: d.MatchedStateID, ResolvedImageID: d.ResolvedImageID, LiquibaseImageDigest: d.LiquibaseImageDigest}
	}
	return view, nil
}

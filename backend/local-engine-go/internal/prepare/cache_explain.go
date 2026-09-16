package prepare

import (
	"context"
	"fmt"
	"os"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

const cacheExplainLiquibaseJobID = "cache-explain"

type cacheExplainQueueStore struct {
	queue.Store
}

func (s cacheExplainQueueStore) AppendEvent(ctx context.Context, event queue.EventRecord) (int64, error) {
	return 0, nil
}

func (m *PrepareService) CacheExplain(ctx context.Context, req Request) (result CacheExplainPrepareResult, err error) {
	prepared, err := m.prepareRequest(req)
	if err != nil {
		return CacheExplainPrepareResult{}, err
	}
	if errResp := m.ensureResolvedImageID(ctx, "", &prepared, nil); errResp != nil {
		return CacheExplainPrepareResult{}, errorFromExplainResponse(errResp)
	}
	if m.identity != nil {
		if err := m.bindManagedRequest(ctx, &prepared, nil); err != nil {
			return CacheExplainPrepareResult{}, err
		}
	}

	planner := m
	jobID := ""
	if prepared.request.PrepareKind == "lb" {
		var cleanup func() error
		planner, jobID, cleanup, err = m.newCacheExplainPlanner()
		if err != nil {
			return CacheExplainPrepareResult{}, err
		}
		defer func() {
			if cleanupErr := cleanup(); err == nil && cleanupErr != nil {
				err = cleanupErr
			}
		}()
	}

	tasks, stateID, errResp := planner.buildPlan(ctx, jobID, prepared)
	if errResp != nil {
		return CacheExplainPrepareResult{}, errorFromExplainResponse(errResp)
	}

	signature := ""
	if prepared.request.PrepareKind == "lb" {
		signature, errResp = m.computeJobSignatureFromPlan(prepared, tasks)
	} else {
		signature, errResp = m.computeJobSignature(prepared)
	}
	if errResp != nil {
		return CacheExplainPrepareResult{}, errorFromExplainResponse(errResp)
	}

	cached, err := m.isManagedStateCached(stateID, prepared)
	if err != nil {
		return CacheExplainPrepareResult{}, err
	}

	result = CacheExplainPrepareResult{
		Decision:        "miss",
		ReasonCode:      "no_matching_state",
		Signature:       signature,
		ResolvedImageID: prepared.effectiveImageID(),
	}
	if cached {
		result.Decision = "hit"
		result.ReasonCode = "exact_state_match"
		result.MatchedStateID = stateID
	}
	return result, nil
}

func (m *PrepareService) newCacheExplainPlanner() (*PrepareService, string, func() error, error) {
	if err := os.MkdirAll(m.stateStoreRoot, 0o700); err != nil {
		return nil, "", nil, err
	}
	tempRoot, err := os.MkdirTemp(m.stateStoreRoot, "cache-explain-*")
	if err != nil {
		return nil, "", nil, err
	}
	planner, err := NewPrepareService(Options{
		Identity:       m.identity,
		Access:         m.access,
		Store:          m.store,
		Queue:          cacheExplainQueueStore{Store: m.queue},
		Runtime:        m.runtime,
		StateFS:        m.statefs,
		DBMS:           m.dbms,
		StateStoreRoot: tempRoot,
		Config:         m.config,
		Psql:           m.psql,
		Liquibase:      m.liquibase,
		Version:        m.version,
		ValidateStore:  m.validateStore,
		Now:            m.now,
		IDGen:          m.idGen,
		Async:          false,
		HeartbeatEvery: m.heartbeatEvery,
	})
	if err != nil {
		_ = os.RemoveAll(tempRoot)
		return nil, "", nil, err
	}
	planner.identity = m.identity
	planner.managedRoot = m.managedRoot
	return planner, cacheExplainLiquibaseJobID, func() error {
		return os.RemoveAll(tempRoot)
	}, nil
}

func errorFromExplainResponse(resp *ErrorResponse) error {
	if resp == nil {
		return fmt.Errorf("internal error")
	}
	if resp.Code == "invalid_argument" {
		return ValidationError{
			Code:    resp.Code,
			Message: resp.Message,
			Details: resp.Details,
		}
	}
	if resp.Details != "" {
		return fmt.Errorf("%s: %s", resp.Message, resp.Details)
	}
	if resp.Message != "" {
		return fmt.Errorf("%s", resp.Message)
	}
	return fmt.Errorf("internal error")
}

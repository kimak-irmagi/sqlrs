package runtime

import (
	"context"
	"fmt"
	"time"
)

// SessionReadinessIdentity contains only immutable saved public bindings. The
// actual full runner ID is inspected independently; no ID supplied inside JSON
// can replace that observation. See private-capture ownership §3.1 and schema §4.
type SessionReadinessIdentity struct {
	SessionID, RunnerID, RunnerConfigDigest, HelperDigest string
	WorkerGeneration                                      int
}

// SessionReadiness is evidence about exact verified bytes, not permission to run
// SQL. The journal must independently bind the observed identity and atomically
// admit readiness before it admits a first invocation.
type SessionReadiness struct {
	ReadyDigest string
	ReadyBytes  int64
}

type sessionReadinessMarker struct {
	Protocol                string `json:"protocol"`
	SessionID               string `json:"session_id"`
	WorkerGeneration        int    `json:"worker_generation"`
	RunnerConfigDigest      string `json:"runner_config_digest"`
	HelperDigest            string `json:"helper_digest"`
	CaptureProtocolRevision string `json:"capture_protocol_revision"`
}

// ReadLiquibaseSessionReadiness makes one read-only observation of a live exact
// parent. The coordinator may recheck marker absence within its shared startup
// budget; this method never initializes a parent, retries a child, or treats an
// unavailable marker as proof of non-start. One bounded copy plus both native
// inspections must succeed before any hash is returned. No protected bytes or
// Docker diagnostics escape through an error. Requirements: PC05/PC16.
func (r *DockerRuntime) ReadLiquibaseSessionReadiness(ctx context.Context, name string, identity SessionReadinessIdentity) (SessionReadiness, error) {
	if r == nil || !stepCaptureToken.MatchString(identity.SessionID) ||
		!createdContainerID.MatchString(identity.RunnerID) || identity.WorkerGeneration <= 0 ||
		!validCaptureDigest(identity.RunnerConfigDigest) || !validCaptureDigest(identity.HelperDigest) {
		return SessionReadiness{}, fmt.Errorf("invalid Liquibase readiness binding")
	}
	inspect := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		view, err := r.inspectPresentContainer(ctx, name, identity.RunnerID)
		if err != nil || view.ID != identity.RunnerID || view.Status != "running" {
			return fmt.Errorf("exact Liquibase readiness parent is not running")
		}
		return nil
	}
	if err := inspect(); err != nil {
		return SessionReadiness{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	data, err := r.readPrivateCaptureFile(ctx, identity.RunnerID, "", "ready.complete", 8192)
	if err != nil {
		return SessionReadiness{}, fmt.Errorf("Liquibase readiness marker unavailable")
	}
	var marker sessionReadinessMarker
	if !decodeCaptureObject(data, &marker, "protocol session_id worker_generation runner_config_digest helper_digest capture_protocol_revision", "") ||
		marker.Protocol != "lb-capture-ready-v1" || marker.SessionID != identity.SessionID ||
		marker.WorkerGeneration != identity.WorkerGeneration || marker.RunnerConfigDigest != identity.RunnerConfigDigest ||
		marker.HelperDigest != identity.HelperDigest || marker.CaptureProtocolRevision != LiquibaseStepCaptureProtocol {
		return SessionReadiness{}, fmt.Errorf("Liquibase readiness marker binding differs")
	}
	if err := inspect(); err != nil {
		return SessionReadiness{}, err
	}
	if err := ctx.Err(); err != nil {
		return SessionReadiness{}, err
	}
	return SessionReadiness{ReadyDigest: captureDigest(data), ReadyBytes: int64(len(data))}, nil
}

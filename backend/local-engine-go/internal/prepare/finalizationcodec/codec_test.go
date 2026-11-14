package finalizationcodec

import (
	"crypto/sha256"
	"fmt"
	"reflect"
	"strings"
	"testing"
)

// LC03: all kind/profile pairs and canonical payload defenses are independent
// of journal authority. Recompute the digest for semantic defects so checksum
// rejection cannot masquerade as validation of a different requirement.
// Requirements: liquibase-local-projection-cleanup-design.md, outcome hash.
func TestLiquibaseFinalizationOutcomeCodec(t *testing.T) {
	for _, kind := range []Kind{PrepareSuccess, PlanSuccess, CacheExplainSuccess, Failure, Cancelled} {
		for _, profile := range []EventProfile{StatusResult, Status, Result, Error, StatusError} {
			t.Run("pairs/"+string(kind)+"/"+string(profile), func(t *testing.T) {
				outcome := codecTestOutcome(kind)
				outcome.EventProfile = profile
				allowed := (kind == PrepareSuccess && profile == StatusResult) ||
					(kind == PlanSuccess && profile == Status) || (kind == CacheExplainSuccess && profile == Result) ||
					((kind == Failure || kind == Cancelled) && (profile == Error || profile == StatusError))
				canonical, digest, err := Encode(outcome)
				if allowed {
					if err != nil {
						t.Fatal(err)
					}
					if digest != codecTestDigest(kind, profile, canonical) {
						t.Fatal("digest is not bound to kind/profile/canonical bytes")
					}
					decoded, err := Decode(kind, profile, canonical, digest)
					if err != nil || !reflect.DeepEqual(outcome, decoded) {
						t.Fatalf("round trip changed the outcome: %+v err=%v", decoded, err)
					}
				} else {
					if err == nil || canonical != "" || digest != "" {
						t.Fatal("invalid pair encoded or returned partial evidence")
					}
					// Obtain valid kind-specific bytes from its eligible profile,
					// then recompute the checksum for the invalid pair itself.
					valid, _, err := Encode(codecTestOutcome(kind))
					if err != nil {
						t.Fatal(err)
					}
					assertCodecRejected(t, kind, profile, valid, codecTestDigest(kind, profile, valid))
				}
			})
		}
	}
	const good = `{"instance_id":"instance-1","state_id":"state-1"}`
	for name, payload := range map[string]string{
		"empty": "", "malformed": `{`, "unknown_field": `{"instance_id":"instance-1","state_id":"state-1","other":1}`,
		"duplicate_field": `{"instance_id":"instance-1","state_id":"state-1","instance_id":"instance-1"}`,
		"whitespace":      good + " ", "field_order": `{"state_id":"state-1","instance_id":"instance-1"}`,
		"trailing_value": good + `{}`, "missing_reference": `{"instance_id":"instance-1","state_id":""}`,
		"cross_kind_field": `{"code":"cancelled","message":"Cancelled"}`,
		"secret_field":     `{"instance_id":"instance-1","state_id":"state-1","dsn":"postgres://PRIVATE_CANARY"}`,
		"invalid_utf8":     "{\"instance_id\":\"\xff\",\"state_id\":\"state-1\"}",
	} {
		t.Run(name, func(t *testing.T) {
			assertCodecRejected(t, PrepareSuccess, StatusResult, payload, codecTestDigest(PrepareSuccess, StatusResult, payload))
		})
	}
	t.Run("wrong_checksum", func(t *testing.T) {
		assertCodecRejected(t, PrepareSuccess, StatusResult, good, "sha256:"+strings.Repeat("a", 64))
	})
	t.Run("unknown_kind", func(t *testing.T) {
		assertCodecRejected(t, "other", StatusResult, good, codecTestDigest("other", StatusResult, good))
		if _, _, err := Encode(Outcome{Kind: "other", EventProfile: StatusResult, Prepare: &PrepareReference{InstanceID: "instance-1", StateID: "state-1"}}); err == nil {
			t.Fatal("unknown kind encoded")
		}
	})
	t.Run("unknown_profile", func(t *testing.T) {
		assertCodecRejected(t, PrepareSuccess, "other", good, codecTestDigest(PrepareSuccess, "other", good))
	})
	t.Run("hash_domain", func(t *testing.T) {
		canonical, digest, err := Encode(codecTestOutcome(PrepareSuccess))
		if err != nil || canonical != good || digest != "sha256:0ad68be80fb45fd87fc0a7c7af8e061697db85ef6ad435ae1f31ad84bbb4d089" {
			t.Fatalf("fixed domain vector: canonical=%q digest=%q err=%v", canonical, digest, err)
		}
	})
	t.Run("changed_valid_reference", func(t *testing.T) {
		changed := `{"instance_id":"instance-2","state_id":"state-2"}`
		decoded, err := Decode(PrepareSuccess, StatusResult, changed, codecTestDigest(PrepareSuccess, StatusResult, changed))
		if err != nil || decoded.Prepare == nil || decoded.Prepare.InstanceID != "instance-2" || decoded.Prepare.StateID != "state-2" {
			t.Fatal("codec confused opaque reference validation with saved journal authority", err)
		}
	})
	for _, kind := range []Kind{PrepareSuccess, PlanSuccess, CacheExplainSuccess, Failure, Cancelled} {
		t.Run("missing_payload/"+string(kind), func(t *testing.T) {
			outcome := codecTestOutcome(kind)
			outcome.Prepare, outcome.Plan, outcome.Explanation, outcome.Problem = nil, nil, nil, nil
			if _, _, err := Encode(outcome); err == nil {
				t.Fatal("missing typed payload accepted")
			}
		})
	}
	t.Run("mixed_payloads", func(t *testing.T) {
		outcome := codecTestOutcome(PrepareSuccess)
		outcome.Problem = &PublicError{Code: "failed", Message: "Failed"}
		if _, _, err := Encode(outcome); err == nil {
			t.Fatal("mixed-kind fields accepted")
		}
	})
	for _, tc := range []struct {
		name string
		bad  Outcome
	}{
		{"invalid_error", Outcome{Kind: Failure, EventProfile: Error, Problem: &PublicError{Code: "", Message: "Failed"}}},
		{"invalid_plan_digest", Outcome{Kind: PlanSuccess, EventProfile: Status, Plan: &PlanReference{PlanDigest: "bad", ProjectionDigest: "sha256:" + strings.Repeat("a", 64)}}},
		{"invalid_decision_digest", Outcome{Kind: CacheExplainSuccess, EventProfile: Result, Explanation: &ExplanationReference{PlanDigest: "sha256:" + strings.Repeat("a", 64), DecisionDigest: "bad"}}},
		{"invalid_reference_encoding", Outcome{Kind: PrepareSuccess, EventProfile: StatusResult, Prepare: &PrepareReference{InstanceID: "\xff", StateID: "state"}}},
		{"invalid_error_encoding", Outcome{Kind: Failure, EventProfile: Error, Problem: &PublicError{Code: "failed", Message: "\xff"}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if _, _, err := Encode(tc.bad); err == nil {
				t.Fatal("invalid typed fields encoded")
			}
		})
	}
}

func codecTestOutcome(kind Kind) Outcome {
	digest := "sha256:" + strings.Repeat("a", 64)
	outcome := Outcome{Kind: kind}
	switch kind {
	case PrepareSuccess:
		outcome.EventProfile, outcome.Prepare = StatusResult, &PrepareReference{InstanceID: "instance-1", StateID: "state-1"}
	case PlanSuccess:
		outcome.EventProfile, outcome.Plan = Status, &PlanReference{PlanDigest: digest, ProjectionDigest: digest}
	case CacheExplainSuccess:
		outcome.EventProfile, outcome.Explanation = Result, &ExplanationReference{PlanDigest: digest, DecisionDigest: digest}
	case Failure, Cancelled:
		outcome.EventProfile, outcome.Problem = Error, &PublicError{Code: "failed", Message: "Operation failed", Details: "Public detail"}
	}
	return outcome
}

func codecTestDigest(kind Kind, profile EventProfile, payload string) string {
	return fmt.Sprintf("sha256:%x", sha256.Sum256([]byte("sqlrs-liquibase-job-finalization-v1\n"+string(kind)+"\n"+string(profile)+"\n"+payload)))
}

func assertCodecRejected(t *testing.T, kind Kind, profile EventProfile, payload, digest string) {
	t.Helper()
	decoded, err := Decode(kind, profile, payload, digest)
	if err == nil || !reflect.DeepEqual(decoded, Outcome{}) {
		t.Fatal("invalid bytes yielded success or partial authority")
	}
	if strings.Contains(err.Error(), "PRIVATE_CANARY") || strings.Contains(err.Error(), payload) && payload != "" {
		t.Fatal("codec error disclosed protected input")
	}
}

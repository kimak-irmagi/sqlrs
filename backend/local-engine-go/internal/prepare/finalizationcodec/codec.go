// Package finalizationcodec owns version-1 canonical Liquibase outcome bytes.
// It binds syntax and kind/profile to a digest; queue authority must separately
// match saved references and retirement facts before authorizing any effect.
// See docs/architecture/liquibase-local-projection-cleanup-design.md.
package finalizationcodec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
)

type Kind string

const (
	PrepareSuccess      Kind = "prepare_success"
	PlanSuccess         Kind = "plan_success"
	CacheExplainSuccess Kind = "cache_explain_success"
	Failure             Kind = "failure"
	Cancelled           Kind = "cancelled"
)

type EventProfile string

const (
	StatusResult EventProfile = "status_result"
	Status       EventProfile = "status"
	Result       EventProfile = "result"
	Error        EventProfile = "error"
	StatusError  EventProfile = "status_error"
)

// ErrInvalid never echoes stored bytes, fields or decoder errors. These bytes
// are private recovery data and may have been corrupted to contain secrets.
var ErrInvalid = errors.New("invalid Liquibase finalization outcome")

// PrepareReference preserves only immutable logical IDs, never a live DSN.
type PrepareReference struct {
	InstanceID string `json:"instance_id"`
	StateID    string `json:"state_id"`
}

// PlanReference identifies the already saved plan and public plan projection.
type PlanReference struct {
	PlanDigest       string `json:"plan_digest"`
	ProjectionDigest string `json:"projection_digest"`
}

// ExplanationReference binds the point-in-time decision to its saved plan.
type ExplanationReference struct {
	PlanDigest     string `json:"plan_digest"`
	DecisionDigest string `json:"decision_digest"`
}

// PublicError contains the existing public error fields. Callers must supply
// their already sanitized public error: this codec cannot infer secrets inside
// free-form messages. It admits no host-path/credential/SQL-specific fields.
type PublicError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	Details string `json:"details,omitempty"`
}

// Outcome is a tagged union with exactly one kind-specific payload. Cancelled
// and Failure share PublicError but retain distinct hash domains and authority.
type Outcome struct {
	Kind         Kind
	EventProfile EventProfile
	Prepare      *PrepareReference
	Plan         *PlanReference
	Explanation  *ExplanationReference
	Problem      *PublicError
}

// Encode validates fields and emits the unique JSON representation and its
// domain-separated SHA-256. It returns no partial evidence on invalid input.
func Encode(outcome Outcome) (string, string, error) {
	if !validPair(outcome.Kind, outcome.EventProfile) {
		return "", "", ErrInvalid
	}
	count := 0
	for _, present := range []bool{outcome.Prepare != nil, outcome.Plan != nil, outcome.Explanation != nil, outcome.Problem != nil} {
		if present {
			count++
		}
	}
	if count != 1 {
		return "", "", ErrInvalid
	}
	var payload any
	switch outcome.Kind {
	case PrepareSuccess:
		p := outcome.Prepare
		if p == nil || !validText(p.InstanceID) || !validText(p.StateID) {
			return "", "", ErrInvalid
		}
		payload = p
	case PlanSuccess:
		p := outcome.Plan
		if p == nil || !planningcodec.ValidDigest(p.PlanDigest) || !planningcodec.ValidDigest(p.ProjectionDigest) {
			return "", "", ErrInvalid
		}
		payload = p
	case CacheExplainSuccess:
		p := outcome.Explanation
		if p == nil || !planningcodec.ValidDigest(p.PlanDigest) || !planningcodec.ValidDigest(p.DecisionDigest) {
			return "", "", ErrInvalid
		}
		payload = p
	case Failure, Cancelled:
		p := outcome.Problem
		if p == nil || !validText(p.Code) || !validText(p.Message) || !utf8.ValidString(p.Details) {
			return "", "", ErrInvalid
		}
		payload = p
	}
	// The closed switch above admits only acyclic scalar-string structs with
	// no custom marshaler. Unsupported values and encoding errors are impossible.
	encoded, _ := json.Marshal(payload)
	canonical := string(encoded)
	return canonical, digest(outcome.Kind, outcome.EventProfile, canonical), nil
}

// Decode rejects unknown/duplicate fields, alternate JSON spellings, trailing
// values and mismatched hashes by comparing typed decode/re-encode bytes.
// A valid digest is not proof that these references match the queue's authority.
func Decode(kind Kind, profile EventProfile, encoded, hash string) (Outcome, error) {
	if !validPair(kind, profile) || !utf8.ValidString(encoded) {
		return Outcome{}, ErrInvalid
	}
	outcome := Outcome{Kind: kind, EventProfile: profile}
	var payload any
	switch kind {
	case PrepareSuccess:
		outcome.Prepare = &PrepareReference{}
		payload = outcome.Prepare
	case PlanSuccess:
		outcome.Plan = &PlanReference{}
		payload = outcome.Plan
	case CacheExplainSuccess:
		outcome.Explanation = &ExplanationReference{}
		payload = outcome.Explanation
	case Failure, Cancelled:
		outcome.Problem = &PublicError{}
		payload = outcome.Problem
	}
	if err := json.Unmarshal([]byte(encoded), payload); err != nil {
		return Outcome{}, ErrInvalid
	}
	canonical, expected, err := Encode(outcome)
	if err != nil || encoded != canonical || hash != expected {
		return Outcome{}, ErrInvalid
	}
	return outcome, nil
}

func validPair(kind Kind, profile EventProfile) bool {
	switch kind {
	case PrepareSuccess:
		return profile == StatusResult
	case PlanSuccess:
		return profile == Status
	case CacheExplainSuccess:
		return profile == Result
	case Failure, Cancelled:
		return profile == Error || profile == StatusError
	}
	return false
}

func validText(value string) bool {
	return strings.TrimSpace(value) != "" && utf8.ValidString(value)
}

func digest(kind Kind, profile EventProfile, encoded string) string {
	sum := sha256.Sum256([]byte("sqlrs-liquibase-job-finalization-v1\n" + string(kind) + "\n" + string(profile) + "\n" + encoded))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// Package planningcodec owns the versioned, canonical evidence bytes shared
// by the planner and durable queue. See the planning recovery storage schema.
package planningcodec

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"
)

const PlanRevision = "lb-plan-v1"
const CaptureProtocolRevision = "lb-capture-v2"

// Changeset contains only durable metadata, never raw SQL.
type Changeset struct {
	ID       string `json:"id"`
	Author   string `json:"author"`
	Path     string `json:"path"`
	SQLHash  string `json:"sql_hash"`
	Checksum string `json:"checksum"`
}

type planDocument struct {
	CodecRevision      string      `json:"codec_revision"`
	ParsedOutputDigest string      `json:"parsed_output_digest"`
	ExecutionDigest    string      `json:"execution_digest"`
	Changesets         []Changeset `json:"changesets"`
}

// PlanDigest is the single canonical fingerprint for ordered planning
// metadata. Neither SQL text nor credentials are accepted as fields.
func PlanDigest(revision, outputDigest, executionDigest string, changesets []Changeset) (string, error) {
	if revision != PlanRevision {
		return "", fmt.Errorf("unsupported planning codec revision")
	}
	if !ValidDigest(outputDigest) || !ValidDigest(executionDigest) {
		return "", fmt.Errorf("invalid planning output or execution digest")
	}
	for _, item := range changesets {
		if !validHex(item.SQLHash) {
			return "", fmt.Errorf("invalid planning changeset SQL hash")
		}
		for _, value := range []string{item.ID, item.Author, item.Path, item.Checksum} {
			if !utf8.ValidString(value) {
				return "", fmt.Errorf("invalid planning changeset metadata encoding")
			}
		}
	}
	doc := planDocument{CodecRevision: revision, ParsedOutputDigest: outputDigest, ExecutionDigest: executionDigest, Changesets: append([]Changeset{}, changesets...)}
	encoded, err := json.Marshal(doc)
	if err != nil {
		return "", err
	}
	return digest(encoded), nil
}

// Decision uses the same ordered JSON fields as the public explanation.
type Decision struct {
	Decision             string `json:"decision"`
	ReasonCode           string `json:"reason_code,omitempty"`
	Signature            string `json:"signature,omitempty"`
	MatchedStateID       string `json:"matched_state_id,omitempty"`
	ResolvedImageID      string `json:"resolved_image_id,omitempty"`
	LiquibaseImageDigest string `json:"liquibase_image_digest,omitempty"`
}

// CanonicalDecision returns the saved public bytes and their SHA-256 digest.
func CanonicalDecision(result Decision) (string, string, error) {
	if (result.Decision != "hit" && result.Decision != "miss") || strings.TrimSpace(result.ReasonCode) == "" || strings.TrimSpace(result.Signature) == "" ||
		(result.Decision == "hit" && strings.TrimSpace(result.MatchedStateID) == "") || (result.Decision == "miss" && result.MatchedStateID != "") {
		return "", "", fmt.Errorf("invalid cache explanation decision")
	}
	for _, field := range []string{result.Decision, result.ReasonCode, result.Signature, result.MatchedStateID, result.ResolvedImageID, result.LiquibaseImageDigest} {
		if !utf8.ValidString(field) {
			return "", "", fmt.Errorf("invalid cache explanation decision encoding")
		}
	}
	encoded, err := json.Marshal(result)
	if err != nil {
		return "", "", err
	}
	return string(encoded), digest(encoded), nil
}

// DecodeDecision refuses parseable but noncanonical or mismatched evidence.
func DecodeDecision(encoded, digestValue string) (Decision, error) {
	var result Decision
	if err := json.Unmarshal([]byte(encoded), &result); err != nil {
		return Decision{}, fmt.Errorf("invalid saved cache explanation decision")
	}
	canonical, expected, err := CanonicalDecision(result)
	if err != nil || encoded != canonical || digestValue != expected {
		return Decision{}, fmt.Errorf("saved cache explanation decision does not match its evidence")
	}
	return result, nil
}

func ValidDigest(value string) bool {
	return strings.HasPrefix(value, "sha256:") && validHex(strings.TrimPrefix(value, "sha256:"))
}

func validHex(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, ch := range value {
		if (ch < '0' || ch > '9') && (ch < 'a' || ch > 'f') {
			return false
		}
	}
	return true
}

func digest(value []byte) string {
	sum := sha256.Sum256(value)
	return "sha256:" + hex.EncodeToString(sum[:])
}

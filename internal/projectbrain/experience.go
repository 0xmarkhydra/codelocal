package projectbrain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
	"time"
)

type ExperienceKind string
type ExperienceStatus string

const (
	ExperienceFact            ExperienceKind = "fact"
	ExperienceWorkflow        ExperienceKind = "workflow"
	ExperienceFailureRecovery ExperienceKind = "failure_recovery"

	ExperienceCandidateStatus ExperienceStatus = "candidate"
	ExperiencePromoted        ExperienceStatus = "promoted"
	ExperienceRejected        ExperienceStatus = "rejected"
)

var ErrInvalidExperience = errors.New("invalid project experience")

type ExperienceCandidate struct {
	ID               string         `json:"id"`
	Kind             ExperienceKind `json:"kind"`
	Statement        string         `json:"statement"`
	Trigger          string         `json:"trigger,omitempty"`
	Preconditions    []string       `json:"preconditions,omitempty"`
	EvidenceRefs     []string       `json:"evidenceRefs"`
	VerificationRefs []string       `json:"verificationRefs"`
	FailureSignature string         `json:"failureSignature,omitempty"`
	RecoveryAction   string         `json:"recoveryAction,omitempty"`
	BranchScope      string         `json:"branchScope,omitempty"`
	Confidence       float64        `json:"confidence"`
	CreatedAt        time.Time      `json:"createdAt"`
}

type VerifiedOutcome struct {
	TaskID              string `json:"taskId"`
	VerificationPassed  bool   `json:"verificationPassed"`
	SecurityPassed      bool   `json:"securityPassed"`
	RegressionFree      bool   `json:"regressionFree"`
	RequiredChecks      int    `json:"requiredChecks"`
	PassedRequiredChecks int   `json:"passedRequiredChecks"`
}

type ExperienceDecision struct {
	Status     ExperienceStatus `json:"status"`
	Candidate  ExperienceCandidate `json:"candidate"`
	Trust      string           `json:"trust,omitempty"`
	ReasonCode string           `json:"reasonCode"`
}

// EvaluateExperience is the promotion gate between runtime outcomes and Project
// Brain. A model statement alone can never become durable project knowledge.
func EvaluateExperience(input ExperienceCandidate, outcome VerifiedOutcome) (ExperienceDecision, error) {
	candidate, err := normalizeExperience(input)
	if err != nil {
		return ExperienceDecision{}, err
	}
	decision := ExperienceDecision{Status: ExperienceCandidateStatus, Candidate: candidate, ReasonCode: "awaiting_verified_evidence"}
	if !outcome.VerificationPassed || outcome.RequiredChecks <= 0 || outcome.PassedRequiredChecks < outcome.RequiredChecks {
		decision.Status = ExperienceRejected
		decision.ReasonCode = "verification_not_proven"
		return decision, nil
	}
	if !outcome.SecurityPassed {
		decision.Status = ExperienceRejected
		decision.ReasonCode = "security_gate_failed"
		return decision, nil
	}
	if !outcome.RegressionFree {
		decision.Status = ExperienceRejected
		decision.ReasonCode = "regression_detected"
		return decision, nil
	}
	if len(candidate.EvidenceRefs) == 0 || len(candidate.VerificationRefs) == 0 {
		decision.Status = ExperienceRejected
		decision.ReasonCode = "evidence_missing"
		return decision, nil
	}
	if candidate.Confidence < .8 {
		decision.ReasonCode = "confidence_below_promotion_threshold"
		return decision, nil
	}
	if candidate.Kind == ExperienceFailureRecovery && (candidate.FailureSignature == "" || candidate.RecoveryAction == "") {
		decision.Status = ExperienceRejected
		decision.ReasonCode = "failure_recovery_evidence_incomplete"
		return decision, nil
	}
	decision.Status = ExperiencePromoted
	decision.Trust = "verified"
	decision.ReasonCode = "verified_experience_promoted"
	return decision, nil
}

// SelectExperiences returns only promoted, branch-compatible experience. It is
// intentionally bounded and deterministic so retrieval can feed Context Surface
// without turning the whole historical corpus into prompt context.
func SelectExperiences(decisions []ExperienceDecision, branch, trigger string, limit int) []ExperienceCandidate {
	branch = strings.TrimSpace(branch)
	trigger = strings.ToLower(strings.TrimSpace(trigger))
	if limit <= 0 { limit = 8 }
	if limit > 32 { limit = 32 }
	out := []ExperienceCandidate{}
	for _, decision := range decisions {
		if decision.Status != ExperiencePromoted || decision.Trust != "verified" { continue }
		candidate := decision.Candidate
		if candidate.BranchScope != "" && branch != "" && candidate.BranchScope != branch { continue }
		if trigger != "" && candidate.Trigger != "" && !strings.Contains(trigger, strings.ToLower(candidate.Trigger)) && !strings.Contains(strings.ToLower(candidate.Trigger), trigger) { continue }
		out = append(out, candidate)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Confidence != out[j].Confidence { return out[i].Confidence > out[j].Confidence }
		return out[i].ID < out[j].ID
	})
	if len(out) > limit { out = out[:limit] }
	return out
}

func normalizeExperience(candidate ExperienceCandidate) (ExperienceCandidate, error) {
	candidate.ID = strings.TrimSpace(candidate.ID)
	candidate.Statement = strings.Join(strings.Fields(candidate.Statement), " ")
	candidate.Trigger = strings.Join(strings.Fields(candidate.Trigger), " ")
	candidate.FailureSignature = strings.TrimSpace(candidate.FailureSignature)
	candidate.RecoveryAction = strings.ToLower(strings.TrimSpace(candidate.RecoveryAction))
	candidate.BranchScope = strings.TrimSpace(candidate.BranchScope)
	candidate.Preconditions = experienceStrings(candidate.Preconditions)
	candidate.EvidenceRefs = experienceStrings(candidate.EvidenceRefs)
	candidate.VerificationRefs = experienceStrings(candidate.VerificationRefs)
	if candidate.ID == "" { candidate.ID = "experience:" + experienceDigest(string(candidate.Kind), candidate.Statement, candidate.BranchScope) }
	if candidate.Statement == "" || !validExperienceKind(candidate.Kind) || candidate.Confidence < 0 || candidate.Confidence > 1 { return ExperienceCandidate{}, ErrInvalidExperience }
	if len(candidate.Statement) > 4096 { candidate.Statement = candidate.Statement[:4096] }
	if candidate.CreatedAt.IsZero() { candidate.CreatedAt = time.Now().UTC() } else { candidate.CreatedAt = candidate.CreatedAt.UTC() }
	return candidate, nil
}

func experienceStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.Join(strings.Fields(value), " ")
		if value == "" { continue }
		if len(value) > 256 { value = value[:256] }
		if _, ok := seen[value]; ok { continue }
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func validExperienceKind(kind ExperienceKind) bool { return kind == ExperienceFact || kind == ExperienceWorkflow || kind == ExperienceFailureRecovery }
func experienceDigest(parts ...string) string { h := sha256.New(); for _, part := range parts { _, _ = h.Write([]byte(strings.TrimSpace(part))); _, _ = h.Write([]byte{0}) }; return hex.EncodeToString(h.Sum(nil))[:20] }

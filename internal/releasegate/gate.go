package releasegate

import (
	"sort"
	"strings"
)

type Metrics struct {
	VerifiedSuccessRate         float64 `json:"verifiedSuccessRate"`
	MedianTokensPerVerifiedTask int64   `json:"medianTokensPerVerifiedTask"`
	P95LatencyMillis            int64   `json:"p95LatencyMillis"`
	HumanInterventionsPerTask   float64 `json:"humanInterventionsPerTask"`
	ResumeSuccessRate           float64 `json:"resumeSuccessRate"`
	SecurityRegressions         int64   `json:"securityRegressions"`
	LostUpdates                 int64   `json:"lostUpdates"`
	PolicyBypasses              int64   `json:"policyBypasses"`
}

type Infrastructure struct {
	CIStarted      bool `json:"ciStarted"`
	CIHealthy      bool `json:"ciHealthy"`
	CrossPlatform  bool `json:"crossPlatform"`
	MigrationReady bool `json:"migrationReady"`
	RollbackReady  bool `json:"rollbackReady"`
}

type ChaosResult struct {
	ID       string `json:"id"`
	Passed   bool   `json:"passed"`
	Evidence string `json:"evidence,omitempty"`
}

type Config struct {
	MinResumeSuccessRate float64 `json:"minResumeSuccessRate"`
	MaxSuccessRegression float64 `json:"maxSuccessRegression"`
	MaxTokenRatio        float64 `json:"maxTokenRatio"`
	MaxLatencyRatio      float64 `json:"maxLatencyRatio"`
}

type Gate struct {
	Passed   bool     `json:"passed"`
	Blockers []string `json:"blockers,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
}

// Evaluate refuses production promotion unless correctness/security/concurrency
// are proven and the validation infrastructure itself is healthy. Token wins are
// never allowed to compensate for a correctness regression.
func Evaluate(baseline, candidate Metrics, infra Infrastructure, chaos []ChaosResult, cfg Config) Gate {
	cfg = normalizeConfig(cfg)
	blockers, warnings := []string{}, []string{}
	if !infra.CIStarted {
		blockers = append(blockers, "ci_not_started")
	}
	if infra.CIStarted && !infra.CIHealthy {
		blockers = append(blockers, "ci_not_healthy")
	}
	if !infra.CrossPlatform {
		blockers = append(blockers, "cross_platform_validation_missing")
	}
	if !infra.MigrationReady {
		blockers = append(blockers, "migration_not_ready")
	}
	if !infra.RollbackReady {
		blockers = append(blockers, "rollback_not_ready")
	}
	if candidate.SecurityRegressions != 0 {
		blockers = append(blockers, "security_regression")
	}
	if candidate.LostUpdates != 0 {
		blockers = append(blockers, "lost_update_detected")
	}
	if candidate.PolicyBypasses != 0 {
		blockers = append(blockers, "policy_bypass_detected")
	}
	if candidate.VerifiedSuccessRate+cfg.MaxSuccessRegression < baseline.VerifiedSuccessRate {
		blockers = append(blockers, "verified_success_regression")
	}
	if candidate.ResumeSuccessRate < cfg.MinResumeSuccessRate {
		blockers = append(blockers, "resume_reliability_below_gate")
	}
	if baseline.MedianTokensPerVerifiedTask > 0 && candidate.MedianTokensPerVerifiedTask > 0 {
		ratio := float64(candidate.MedianTokensPerVerifiedTask) / float64(baseline.MedianTokensPerVerifiedTask)
		if ratio > cfg.MaxTokenRatio {
			blockers = append(blockers, "token_efficiency_gate_failed")
		}
	}
	if baseline.P95LatencyMillis > 0 && candidate.P95LatencyMillis > 0 {
		ratio := float64(candidate.P95LatencyMillis) / float64(baseline.P95LatencyMillis)
		if ratio > cfg.MaxLatencyRatio {
			warnings = append(warnings, "latency_regression")
		}
	}
	if candidate.HumanInterventionsPerTask > baseline.HumanInterventionsPerTask && baseline.HumanInterventionsPerTask > 0 {
		warnings = append(warnings, "human_intervention_regression")
	}
	for _, result := range chaos {
		id := strings.TrimSpace(result.ID)
		if id == "" || !result.Passed {
			blockers = append(blockers, "chaos_failed:"+id)
		}
	}
	blockers = uniqueSorted(blockers)
	warnings = uniqueSorted(warnings)
	return Gate{Passed: len(blockers) == 0, Blockers: blockers, Warnings: warnings}
}

func normalizeConfig(cfg Config) Config {
	if cfg.MinResumeSuccessRate <= 0 || cfg.MinResumeSuccessRate > 1 {
		cfg.MinResumeSuccessRate = .99
	}
	if cfg.MaxSuccessRegression < 0 || cfg.MaxSuccessRegression > .1 {
		cfg.MaxSuccessRegression = 0
	}
	if cfg.MaxTokenRatio <= 0 {
		cfg.MaxTokenRatio = 1
	}
	if cfg.MaxLatencyRatio <= 0 {
		cfg.MaxLatencyRatio = 1.25
	}
	return cfg
}
func uniqueSorted(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, v := range values {
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

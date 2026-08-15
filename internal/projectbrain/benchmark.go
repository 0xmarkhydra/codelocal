package projectbrain

import "math"

type BenchmarkScenario struct {
	ID          string   `json:"id"`
	Name        string   `json:"name"`
	Description string   `json:"description,omitempty"`
	Tags        []string `json:"tags,omitempty"`
}

type BenchmarkMetrics struct {
	VerifiedSuccess       bool    `json:"verifiedSuccess"`
	ContextBytes          int64   `json:"contextBytes"`
	RuleInputChars        int64   `json:"ruleInputChars,omitempty"`
	RuleDeduplicatedChars int64   `json:"ruleDeduplicatedChars,omitempty"`
	RuleDuplicateCount    int     `json:"ruleDuplicateCount,omitempty"`
	ToolCalls             int     `json:"toolCalls"`
	FileReads             int     `json:"fileReads"`
	RoundTrips            int     `json:"roundTrips"`
	CompletionMS          int64   `json:"completionMs"`
	RuleAccuracy          float64 `json:"ruleAccuracy,omitempty"`
	SkillHit              bool    `json:"skillHit,omitempty"`
	CrossDeviceContinuity bool    `json:"crossDeviceContinuity,omitempty"`
	ConflictsDetected     int     `json:"conflictsDetected,omitempty"`
	UnrelatedFilesTouched int     `json:"unrelatedFilesTouched,omitempty"`
}

type BenchmarkResult struct {
	Scenario BenchmarkScenario `json:"scenario"`
	Variant  string            `json:"variant"`
	Metrics  BenchmarkMetrics  `json:"metrics"`
}

type BenchmarkComparison struct {
	ScenarioID              string  `json:"scenarioId"`
	BaselineVariant         string  `json:"baselineVariant"`
	CandidateVariant        string  `json:"candidateVariant"`
	ContextReduction        float64 `json:"contextReduction"`
	ToolCallReduction       float64 `json:"toolCallReduction"`
	FileReadReduction       float64 `json:"fileReadReduction"`
	RoundTripReduction      float64 `json:"roundTripReduction"`
	CompletionTimeReduction float64 `json:"completionTimeReduction"`
	RuleDeduplicationDelta  float64 `json:"ruleDeduplicationDelta,omitempty"`
	CandidateRuleCharsSaved int64   `json:"candidateRuleCharsSaved,omitempty"`
	CandidateDuplicateRules int     `json:"candidateDuplicateRules,omitempty"`
	VerifiedSuccessDelta    int     `json:"verifiedSuccessDelta"`
	RuleAccuracyDelta       float64 `json:"ruleAccuracyDelta"`
	CandidateRegressed      bool    `json:"candidateRegressed"`
}

func reduction(before, after float64) float64 {
	if before <= 0 {
		return 0
	}
	value := (before - after) / before
	if math.IsNaN(value) || math.IsInf(value, 0) {
		return 0
	}
	return value
}

func ruleDeduplicationRatio(metrics BenchmarkMetrics) float64 {
	if metrics.RuleInputChars <= 0 || metrics.RuleDeduplicatedChars <= 0 {
		return 0
	}
	return reduction(float64(metrics.RuleInputChars), float64(metrics.RuleInputChars-metrics.RuleDeduplicatedChars))
}

func CompareBenchmark(baseline, candidate BenchmarkResult) BenchmarkComparison {
	comparison := BenchmarkComparison{
		ScenarioID: baseline.Scenario.ID, BaselineVariant: baseline.Variant, CandidateVariant: candidate.Variant,
		ContextReduction:        reduction(float64(baseline.Metrics.ContextBytes), float64(candidate.Metrics.ContextBytes)),
		ToolCallReduction:       reduction(float64(baseline.Metrics.ToolCalls), float64(candidate.Metrics.ToolCalls)),
		FileReadReduction:       reduction(float64(baseline.Metrics.FileReads), float64(candidate.Metrics.FileReads)),
		RoundTripReduction:      reduction(float64(baseline.Metrics.RoundTrips), float64(candidate.Metrics.RoundTrips)),
		CompletionTimeReduction: reduction(float64(baseline.Metrics.CompletionMS), float64(candidate.Metrics.CompletionMS)),
		RuleDeduplicationDelta:  ruleDeduplicationRatio(candidate.Metrics) - ruleDeduplicationRatio(baseline.Metrics),
		CandidateRuleCharsSaved: candidate.Metrics.RuleDeduplicatedChars,
		CandidateDuplicateRules: candidate.Metrics.RuleDuplicateCount,
		RuleAccuracyDelta:       candidate.Metrics.RuleAccuracy - baseline.Metrics.RuleAccuracy,
	}
	if candidate.Metrics.VerifiedSuccess && !baseline.Metrics.VerifiedSuccess {
		comparison.VerifiedSuccessDelta = 1
	} else if !candidate.Metrics.VerifiedSuccess && baseline.Metrics.VerifiedSuccess {
		comparison.VerifiedSuccessDelta = -1
	}
	comparison.CandidateRegressed = comparison.VerifiedSuccessDelta < 0 || comparison.RuleAccuracyDelta < -.05 || candidate.Metrics.UnrelatedFilesTouched > baseline.Metrics.UnrelatedFilesTouched
	return comparison
}

func DefaultBenchmarkScenarios() []BenchmarkScenario {
	return []BenchmarkScenario{
		{ID: "fresh-project", Name: "Fresh project task", Tags: []string{"baseline", "context"}},
		{ID: "repeat-task", Name: "Repeated verified task", Tags: []string{"memory", "skill", "token"}},
		{ID: "new-machine", Name: "Same logical project on new machine", Tags: []string{"cross-device"}},
		{ID: "monorepo-scope", Name: "Nested repository/module rules", Tags: []string{"monorepo", "rules"}},
		{ID: "rule-conflict", Name: "Conflicting project instructions", Tags: []string{"conflict", "rules"}},
		{ID: "branch-applicability", Name: "Branch-specific historical memory", Tags: []string{"memory", "branch"}},
		{ID: "skill-invalidation", Name: "Workflow changed after learned skill", Tags: []string{"skill", "safety"}},
		{ID: "malicious-config", Name: "Repository instruction attempts permission escalation", Tags: []string{"security", "rules"}},
	}
}

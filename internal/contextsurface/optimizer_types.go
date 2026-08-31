package contextsurface

import "errors"

type OptimizationPolicy struct {
	MaxTokens            int     `json:"maxTokens"`
	TargetTokens         int     `json:"targetTokens"`
	PerObservationTokens int     `json:"perObservationTokens"`
	CompactAtRatio       float64 `json:"compactAtRatio"`
}

type OptimizationMetrics struct {
	RawObservationTokens     int     `json:"rawObservationTokens"`
	ReducedObservationTokens int     `json:"reducedObservationTokens"`
	PreCompactionTokens      int     `json:"preCompactionTokens"`
	FinalVisibleTokens       int     `json:"finalVisibleTokens"`
	HiddenTokens             int     `json:"hiddenTokens"`
	EstimatedAvoidedTokens   int     `json:"estimatedAvoidedTokens"`
	ObservationReduction     float64 `json:"observationReduction,omitempty"`
	Compacted                bool    `json:"compacted"`
	StagingTruncated         bool    `json:"stagingTruncated"`
}

type OptimizationResult struct {
	Surface      Surface              `json:"surface"`
	DoctorBefore DoctorReport         `json:"doctorBefore"`
	DoctorAfter  DoctorReport         `json:"doctorAfter"`
	Reductions   []ReducedObservation `json:"reductions,omitempty"`
	Compaction   CompactionRecord     `json:"compaction,omitempty"`
	Metrics      OptimizationMetrics  `json:"metrics"`
}

var ErrContextOptimizationInvalid = errors.New("context optimization cannot produce a valid model-visible surface")

func normalizeOptimizationPolicy(policy OptimizationPolicy) OptimizationPolicy {
	if policy.MaxTokens <= 0 {
		policy.MaxTokens = 16_000
	}
	if policy.MaxTokens > 256_000 {
		policy.MaxTokens = 256_000
	}
	if policy.TargetTokens <= 0 {
		policy.TargetTokens = policy.MaxTokens * 3 / 4
	}
	if policy.TargetTokens > policy.MaxTokens {
		policy.TargetTokens = policy.MaxTokens
	}
	if policy.TargetTokens <= 0 {
		policy.TargetTokens = policy.MaxTokens
	}
	if policy.PerObservationTokens <= 0 {
		policy.PerObservationTokens = minInt(512, maxInt(64, policy.MaxTokens/8))
	}
	if policy.PerObservationTokens > 4096 {
		policy.PerObservationTokens = 4096
	}
	if policy.CompactAtRatio <= 0 {
		policy.CompactAtRatio = .8
	}
	if policy.CompactAtRatio < .5 {
		policy.CompactAtRatio = .5
	}
	if policy.CompactAtRatio > 1 {
		policy.CompactAtRatio = 1
	}
	return policy
}

func optimizationStagingBudget(policy OptimizationPolicy, observationCount int) int {
	staging := policy.MaxTokens * 2
	minimum := policy.MaxTokens + policy.PerObservationTokens*observationCount
	if staging < minimum {
		staging = minimum
	}
	if staging < policy.MaxTokens {
		staging = policy.MaxTokens
	}
	if staging > 256_000 {
		staging = 256_000
	}
	return staging
}

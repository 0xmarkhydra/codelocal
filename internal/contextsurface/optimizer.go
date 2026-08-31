package contextsurface

import "errors"

// CompileOptimized is the model-free token-economy pipeline:
// semantic reduction -> staging surface -> pressure diagnosis -> reversible
// compaction -> final diagnosis. Mandatory evidence is never removed to fit.
func CompileOptimized(input Input, rawObservations []ToolObservation, policy OptimizationPolicy) (OptimizationResult, error) {
	policy = normalizeOptimizationPolicy(policy)
	result := OptimizationResult{Reductions: []ReducedObservation{}}
	for _, raw := range rawObservations {
		reduced := ReduceObservation(raw, policy.PerObservationTokens)
		result.Reductions = append(result.Reductions, reduced)
		input.Observations = append(input.Observations, reduced.Item)
		result.Metrics.RawObservationTokens += reduced.OriginalTokens
		result.Metrics.ReducedObservationTokens += reduced.ReducedTokens
	}
	if result.Metrics.RawObservationTokens > 0 {
		saved := result.Metrics.RawObservationTokens - result.Metrics.ReducedObservationTokens
		if saved < 0 {
			saved = 0
		}
		result.Metrics.ObservationReduction = float64(saved) / float64(result.Metrics.RawObservationTokens)
	}

	staging := Compile(input, optimizationStagingBudget(policy, len(rawObservations)))
	result.Metrics.PreCompactionTokens = staging.Budget.EstimatedTokens
	result.Metrics.StagingTruncated = staging.Truncated
	pressureView := staging
	pressureView.Budget.MaxTokens = policy.MaxTokens
	result.DoctorBefore = Diagnose(pressureView)
	if staging.MandatoryOverflow || len(staging.OmittedItemIDs) > 0 {
		return OptimizationResult{}, ErrContextOptimizationInvalid
	}

	mustCompact := staging.Budget.EstimatedTokens > policy.TargetTokens
	if policy.MaxTokens > 0 && float64(staging.Budget.EstimatedTokens)/float64(policy.MaxTokens) >= policy.CompactAtRatio {
		mustCompact = true
	}
	finalSurface := staging
	if mustCompact {
		compacted, record, err := Compact(staging, policy.TargetTokens)
		if err != nil {
			if errors.Is(err, ErrCompactionMandatoryOverflow) {
				return OptimizationResult{}, ErrContextOptimizationInvalid
			}
			return OptimizationResult{}, err
		}
		finalSurface = compacted
		result.Compaction = record
		result.Metrics.Compacted = record.ID != ""
		result.Metrics.HiddenTokens = record.HiddenTokens
	}
	if finalSurface.Budget.EstimatedTokens > policy.MaxTokens || finalSurface.MandatoryOverflow {
		return OptimizationResult{}, ErrContextOptimizationInvalid
	}
	finalSurface = applyFinalBudget(finalSurface, policy.MaxTokens)
	result.Surface = finalSurface
	result.DoctorAfter = Diagnose(finalSurface)
	result.Metrics.FinalVisibleTokens = finalSurface.Budget.EstimatedTokens
	observationSaved := result.Metrics.RawObservationTokens - result.Metrics.ReducedObservationTokens
	if observationSaved < 0 {
		observationSaved = 0
	}
	result.Metrics.EstimatedAvoidedTokens = observationSaved + result.Metrics.HiddenTokens
	return result, nil
}

func applyFinalBudget(surface Surface, maxTokens int) Surface {
	surface.Budget.MaxTokens = maxTokens
	surface.Budget.EstimatedTokens = 0
	surface.Budget.BrainTokens = 0
	surface.Budget.ActiveTokens = 0
	surface.Budget.RecentTokens = 0
	surface.Budget.ObservationTokens = 0
	for _, item := range surface.Items {
		cost := itemTokens(item)
		surface.Budget.EstimatedTokens += cost
		switch item.Lane {
		case LaneBrainMandatory, LaneBrainRelevant:
			surface.Budget.BrainTokens += cost
		case LaneActive:
			surface.Budget.ActiveTokens += cost
		case LaneRecent:
			surface.Budget.RecentTokens += cost
		case LaneObservation:
			surface.Budget.ObservationTokens += cost
		}
	}
	return surface
}

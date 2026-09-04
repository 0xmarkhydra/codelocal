package agentruntime

import (
	"context"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/usage"
)

var ErrNoRoutableEngine = errors.New("no routable agent engine")

type EngineHistory struct {
	EngineID        string  `json:"engineId"`
	Quality         float64 `json:"quality,omitempty"`
	HistoricSuccess float64 `json:"historicSuccess,omitempty"`
	TokenEfficiency float64 `json:"tokenEfficiency,omitempty"`
	Reliability     float64 `json:"reliability,omitempty"`
	CostEfficiency  float64 `json:"costEfficiency,omitempty"`
	LatencyScore    float64 `json:"latencyScore,omitempty"`
	Samples         int64   `json:"samples,omitempty"`
}

type RouteRequest struct {
	Mode          Mode                 `json:"mode"`
	EngineProfile string               `json:"engineProfile,omitempty"`
	RequireResume bool                 `json:"requireResume,omitempty"`
	RequireMCP    bool                 `json:"requireMcp,omitempty"`
	RequirePlan   bool                 `json:"requirePlan,omitempty"`
	Budget        usage.BudgetDecision `json:"budget"`
	History       []EngineHistory      `json:"history,omitempty"`
}

type EngineCandidate struct {
	EngineID      string        `json:"engineId"`
	Score         float64       `json:"score"`
	CapabilityFit float64       `json:"capabilityFit"`
	Probe         ProbeResult   `json:"probe"`
	History       EngineHistory `json:"history"`
	Reasons       []string      `json:"reasons,omitempty"`
}

// RankEngines is provider-neutral. It rejects incompatible engines first, then
// ranks viable engines using capability fit plus evidence from prior verified
// tasks. Unknown engines receive neutral priors instead of artificial 100% scores.
func RankEngines(probes []ProbeResult, input RouteRequest) []EngineCandidate {
	history := map[string]EngineHistory{}
	for _, item := range input.History {
		id := strings.ToLower(strings.TrimSpace(item.EngineID))
		if id != "" {
			item.EngineID = id
			history[id] = normalizeEngineHistory(item)
		}
	}
	out := []EngineCandidate{}
	for _, probe := range probes {
		id := strings.ToLower(strings.TrimSpace(probe.EngineID))
		if id == "" || !probe.Installed || !probe.Compatible || probe.Auth == AuthRequired {
			continue
		}
		fit, reasons, ok := routeCapabilityFit(probe.Capabilities, input)
		if !ok {
			continue
		}
		h := history[id]
		if h.EngineID == "" {
			h = neutralEngineHistory(id)
		}
		weights := routeWeights(input.Budget)
		score := weights[0]*fit + weights[1]*h.Quality + weights[2]*h.HistoricSuccess + weights[3]*h.TokenEfficiency + weights[4]*h.Reliability + weights[5]*h.CostEfficiency + weights[6]*h.LatencyScore
		out = append(out, EngineCandidate{EngineID: id, Score: score, CapabilityFit: fit, Probe: probe, History: h, Reasons: reasons})
	}
	sort.SliceStable(out, func(i, j int) bool {
		if math.Abs(out[i].Score-out[j].Score) > 1e-9 {
			return out[i].Score > out[j].Score
		}
		return out[i].EngineID < out[j].EngineID
	})
	return out
}

func SelectEngine(ctx context.Context, registry *Registry, input RouteRequest) (EngineCandidate, error) {
	if registry == nil {
		return EngineCandidate{}, ErrNoRoutableEngine
	}
	ranked := RankEngines(registry.ProbeAll(ctx), input)
	if len(ranked) == 0 {
		return EngineCandidate{}, ErrNoRoutableEngine
	}
	return ranked[0], nil
}

func routeCapabilityFit(cap Capabilities, input RouteRequest) (float64, []string, bool) {
	mode := input.Mode
	if mode == "" {
		mode = ModeReview
	}
	if mode == ModeMutate && (!cap.FileEditing || !mutationIsolationAllowed(cap)) {
		return 0, nil, false
	}
	if input.RequireResume && !cap.Resume {
		return 0, nil, false
	}
	if input.RequireMCP && !cap.MCP {
		return 0, nil, false
	}
	if input.RequirePlan && !cap.PlanMode {
		return 0, nil, false
	}
	fit := .55
	reasons := []string{}
	if cap.StructuredOutput {
		fit += .1
		reasons = append(reasons, "structured")
	}
	if cap.Streaming {
		fit += .05
		reasons = append(reasons, "streaming")
	}
	if cap.Resume {
		fit += .05
		reasons = append(reasons, "resume")
	}
	profile := strings.ToLower(strings.TrimSpace(input.EngineProfile))
	switch profile {
	case "fast":
		if cap.NonInteractive {
			fit += .15
		}
	case "coding":
		if cap.FileEditing {
			fit += .15
		}
		if cap.ShellExecution {
			fit += .1
		}
	case "reasoning":
		if cap.PlanMode {
			fit += .1
		}
		if cap.ReviewMode {
			fit += .1
		}
	case "strong":
		if cap.StructuredOutput {
			fit += .1
		}
		if cap.PlanMode {
			fit += .1
		}
	}
	if fit > 1 {
		fit = 1
	}
	return fit, reasons, true
}

func routeWeights(budget usage.BudgetDecision) [7]float64 {
	// capability, quality, success, token, reliability, cost, latency
	weights := [7]float64{.24, .16, .18, .12, .14, .09, .07}
	if budget.State == usage.BudgetSoft {
		weights = [7]float64{.23, .12, .17, .18, .13, .12, .05}
	}
	if budget.State == usage.BudgetHard {
		weights = [7]float64{.28, .08, .14, .22, .12, .13, .03}
	}
	return weights
}

func neutralEngineHistory(id string) EngineHistory {
	return EngineHistory{EngineID: id, Quality: .5, HistoricSuccess: .5, TokenEfficiency: .5, Reliability: .5, CostEfficiency: .5, LatencyScore: .5}
}

func normalizeEngineHistory(value EngineHistory) EngineHistory {
	value.EngineID = strings.ToLower(strings.TrimSpace(value.EngineID))
	value.Quality = clamp01(value.Quality)
	value.HistoricSuccess = clamp01(value.HistoricSuccess)
	value.TokenEfficiency = clamp01(value.TokenEfficiency)
	value.Reliability = clamp01(value.Reliability)
	value.CostEfficiency = clamp01(value.CostEfficiency)
	value.LatencyScore = clamp01(value.LatencyScore)
	return value
}

func clamp01(value float64) float64 {
	if value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

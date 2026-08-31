package usage

import (
	"math"
	"strings"
)

// TokenBreakdown is provider-neutral accounting for one task or activation.
// Providers may supply exact billing tokens; callers may also populate estimated
// values when exact usage is unavailable. Estimated must remain explicit so
// routing and billing logic never mistakes an approximation for provider truth.
type TokenBreakdown struct {
	System      int64 `json:"system"`
	Brain       int64 `json:"brain"`
	Code        int64 `json:"code"`
	Tool        int64 `json:"tool"`
	History     int64 `json:"history"`
	Output      int64 `json:"output"`
	CachedInput int64 `json:"cachedInput,omitempty"`
}

func (b TokenBreakdown) Input() int64 {
	return nonNegative(b.System) + nonNegative(b.Brain) + nonNegative(b.Code) + nonNegative(b.Tool) + nonNegative(b.History)
}

func (b TokenBreakdown) Total() int64 { return b.Input() + nonNegative(b.Output) }

type TaskUsage struct {
	TaskID       string         `json:"taskId"`
	AgentID      string         `json:"agentId,omitempty"`
	ActivationID string         `json:"activationId,omitempty"`
	EngineID     string         `json:"engineId,omitempty"`
	ModelID      string         `json:"modelId,omitempty"`
	Tokens       TokenBreakdown `json:"tokens"`
	CostMicros   int64          `json:"costMicros,omitempty"` // micro-USD unless Currency says otherwise.
	Currency     string         `json:"currency,omitempty"`
	ModelCalls   int64          `json:"modelCalls,omitempty"`
	ToolCalls    int64          `json:"toolCalls,omitempty"`
	Estimated    bool           `json:"estimated,omitempty"`
}

func (u TaskUsage) Normalize() TaskUsage {
	u.TaskID = strings.TrimSpace(u.TaskID)
	u.AgentID = strings.TrimSpace(u.AgentID)
	u.ActivationID = strings.TrimSpace(u.ActivationID)
	u.EngineID = strings.TrimSpace(u.EngineID)
	u.ModelID = strings.TrimSpace(u.ModelID)
	u.Currency = strings.ToUpper(strings.TrimSpace(u.Currency))
	if u.Currency == "" && u.CostMicros > 0 {
		u.Currency = "USD"
	}
	u.Tokens.System = nonNegative(u.Tokens.System)
	u.Tokens.Brain = nonNegative(u.Tokens.Brain)
	u.Tokens.Code = nonNegative(u.Tokens.Code)
	u.Tokens.Tool = nonNegative(u.Tokens.Tool)
	u.Tokens.History = nonNegative(u.Tokens.History)
	u.Tokens.Output = nonNegative(u.Tokens.Output)
	u.Tokens.CachedInput = nonNegative(u.Tokens.CachedInput)
	u.CostMicros = nonNegative(u.CostMicros)
	u.ModelCalls = nonNegative(u.ModelCalls)
	u.ToolCalls = nonNegative(u.ToolCalls)
	return u
}

func Add(left, right TaskUsage) TaskUsage {
	left, right = left.Normalize(), right.Normalize()
	out := left
	if out.TaskID == "" {
		out.TaskID = right.TaskID
	}
	if out.EngineID == "" {
		out.EngineID = right.EngineID
	} else if right.EngineID != "" && out.EngineID != right.EngineID {
		out.EngineID = "mixed"
	}
	if out.ModelID == "" {
		out.ModelID = right.ModelID
	} else if right.ModelID != "" && out.ModelID != right.ModelID {
		out.ModelID = "mixed"
	}
	if out.Currency == "" {
		out.Currency = right.Currency
	}
	if out.Currency != right.Currency && right.CostMicros > 0 {
		// A caller must convert currency before aggregating. Zeroing the aggregate
		// is safer than silently adding incomparable money values.
		out.CostMicros = 0
		out.Currency = "MIXED"
	} else {
		out.CostMicros += right.CostMicros
	}
	out.Tokens.System += right.Tokens.System
	out.Tokens.Brain += right.Tokens.Brain
	out.Tokens.Code += right.Tokens.Code
	out.Tokens.Tool += right.Tokens.Tool
	out.Tokens.History += right.Tokens.History
	out.Tokens.Output += right.Tokens.Output
	out.Tokens.CachedInput += right.Tokens.CachedInput
	out.ModelCalls += right.ModelCalls
	out.ToolCalls += right.ToolCalls
	out.Estimated = out.Estimated || right.Estimated
	return out
}

type Budget struct {
	MaxInputTokens  int64   `json:"maxInputTokens,omitempty"`
	MaxOutputTokens int64   `json:"maxOutputTokens,omitempty"`
	MaxTotalTokens  int64   `json:"maxTotalTokens,omitempty"`
	MaxCostMicros   int64   `json:"maxCostMicros,omitempty"`
	Currency        string  `json:"currency,omitempty"`
	SoftRatio       float64 `json:"softRatio,omitempty"`
}

type BudgetState string

const (
	BudgetWithin BudgetState = "within"
	BudgetSoft   BudgetState = "soft_limit"
	BudgetHard   BudgetState = "hard_limit"
)

type BudgetDecision struct {
	State          BudgetState `json:"state"`
	InputRatio     float64     `json:"inputRatio,omitempty"`
	OutputRatio    float64     `json:"outputRatio,omitempty"`
	TotalRatio     float64     `json:"totalRatio,omitempty"`
	CostRatio      float64     `json:"costRatio,omitempty"`
	DominantRatio  float64     `json:"dominantRatio,omitempty"`
	RemainingTotal int64       `json:"remainingTotal,omitempty"`
	RemainingCost  int64       `json:"remainingCost,omitempty"`
	Reason         string      `json:"reason,omitempty"`
}

func EvaluateBudget(budget Budget, current TaskUsage) BudgetDecision {
	current = current.Normalize()
	soft := budget.SoftRatio
	if soft <= 0 || soft >= 1 {
		soft = .8
	}

	decision := BudgetDecision{State: BudgetWithin}
	decision.InputRatio = ratio(current.Tokens.Input(), budget.MaxInputTokens)
	decision.OutputRatio = ratio(current.Tokens.Output, budget.MaxOutputTokens)
	decision.TotalRatio = ratio(current.Tokens.Total(), budget.MaxTotalTokens)
	decision.CostRatio = ratio(current.CostMicros, budget.MaxCostMicros)
	decision.DominantRatio = max4(decision.InputRatio, decision.OutputRatio, decision.TotalRatio, decision.CostRatio)
	if budget.MaxTotalTokens > 0 {
		decision.RemainingTotal = max64(0, budget.MaxTotalTokens-current.Tokens.Total())
	}
	if budget.MaxCostMicros > 0 {
		decision.RemainingCost = max64(0, budget.MaxCostMicros-current.CostMicros)
	}

	currency := strings.ToUpper(strings.TrimSpace(budget.Currency))
	if budget.MaxCostMicros > 0 && currency != "" && current.Currency != "" && current.Currency != currency {
		decision.State = BudgetHard
		decision.Reason = "usage currency does not match budget currency"
		return decision
	}
	if decision.DominantRatio >= 1 {
		decision.State = BudgetHard
		decision.Reason = "hard task budget reached"
		return decision
	}
	if decision.DominantRatio >= soft {
		decision.State = BudgetSoft
		decision.Reason = "soft task budget reached"
	}
	return decision
}

func nonNegative(value int64) int64 {
	if value < 0 {
		return 0
	}
	return value
}

func ratio(value, limit int64) float64 {
	if limit <= 0 {
		return 0
	}
	result := float64(nonNegative(value)) / float64(limit)
	if math.IsNaN(result) || math.IsInf(result, 0) {
		return 0
	}
	return result
}

func max4(a, b, c, d float64) float64 { return math.Max(math.Max(a, b), math.Max(c, d)) }
func max64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}

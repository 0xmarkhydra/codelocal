package usage

import "testing"

func TestTokenBreakdownTotals(t *testing.T) {
	breakdown := TokenBreakdown{System: 100, Brain: 200, Code: 300, Tool: 400, History: 500, Output: 600, CachedInput: 700}
	if got, want := breakdown.Input(), int64(1500); got != want {
		t.Fatalf("Input()=%d want %d", got, want)
	}
	if got, want := breakdown.Total(), int64(2100); got != want {
		t.Fatalf("Total()=%d want %d", got, want)
	}
}

func TestEvaluateBudgetSoftAndHard(t *testing.T) {
	budget := Budget{MaxTotalTokens: 1000, MaxCostMicros: 1_000_000, Currency: "USD", SoftRatio: .8}
	soft := EvaluateBudget(budget, TaskUsage{Tokens: TokenBreakdown{Code: 700, Output: 100}, CostMicros: 400_000, Currency: "usd"})
	if soft.State != BudgetSoft || soft.DominantRatio != .8 || soft.RemainingTotal != 200 {
		t.Fatalf("unexpected soft decision: %+v", soft)
	}

	hard := EvaluateBudget(budget, TaskUsage{Tokens: TokenBreakdown{Code: 900, Output: 101}, CostMicros: 400_000, Currency: "USD"})
	if hard.State != BudgetHard || hard.RemainingTotal != 0 {
		t.Fatalf("unexpected hard decision: %+v", hard)
	}
}

func TestEvaluateBudgetRejectsCurrencyMismatch(t *testing.T) {
	decision := EvaluateBudget(Budget{MaxCostMicros: 100, Currency: "USD"}, TaskUsage{CostMicros: 10, Currency: "EUR"})
	if decision.State != BudgetHard || decision.Reason == "" {
		t.Fatalf("expected currency mismatch to fail closed: %+v", decision)
	}
}

func TestAddUsagePreservesFirstEngineIdentity(t *testing.T) {
	combined := Add(TaskUsage{}, TaskUsage{TaskID: "task", EngineID: "codex", ModelID: "gpt", Tokens: TokenBreakdown{Code: 10}})
	if combined.EngineID != "codex" || combined.ModelID != "gpt" {
		t.Fatalf("single-engine aggregate must keep its identity: %+v", combined)
	}
}

func TestAddUsageMarksMixedAndPreservesEstimated(t *testing.T) {
	combined := Add(
		TaskUsage{TaskID: "task", EngineID: "codex", ModelID: "a", Currency: "USD", CostMicros: 3, Tokens: TokenBreakdown{Brain: 10}, ModelCalls: 1},
		TaskUsage{TaskID: "task", EngineID: "claude", ModelID: "b", Currency: "USD", CostMicros: 4, Tokens: TokenBreakdown{Tool: 20, Output: 5}, ToolCalls: 2, Estimated: true},
	)
	if combined.EngineID != "mixed" || combined.ModelID != "mixed" {
		t.Fatalf("expected mixed routing metadata: %+v", combined)
	}
	if combined.CostMicros != 7 || combined.Tokens.Total() != 35 || !combined.Estimated {
		t.Fatalf("unexpected aggregate: %+v", combined)
	}
}

func TestNormalizeClampsNegativeValues(t *testing.T) {
	usage := (TaskUsage{CostMicros: -1, ModelCalls: -2, Tokens: TokenBreakdown{Code: -3, Output: 4}}).Normalize()
	if usage.CostMicros != 0 || usage.ModelCalls != 0 || usage.Tokens.Code != 0 || usage.Tokens.Output != 4 {
		t.Fatalf("negative values not clamped: %+v", usage)
	}
}

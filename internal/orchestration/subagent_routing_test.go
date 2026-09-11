package orchestration

import (
	"strings"
	"testing"
)

func routingTestDefs() []SubagentDefinition {
	return []SubagentDefinition{
		{Name: "explorer", Description: "Survey codebase, find related files and symbols. Read-only.", Mode: SubagentModeSubagent, Role: SpecialistInvestigator, EngineProfile: "balanced", Tools: []string{"read_*", "search_*", "lsp_*"}, Permission: map[string]string{"edit_*": "deny"}, TokenBudget: 24000, MaxDepth: 2, MaxConcurrent: 3, ReadOnly: true, Source: SubagentSourceProject, Prompt: "Survey."},
		{Name: "builder", Description: "Implement fixes and refactors with scoped file writes.", Mode: SubagentModeSubagent, Role: SpecialistImplementer, EngineProfile: "coding", Tools: []string{"read_*", "edit_*"}, TokenBudget: 16000, MaxDepth: 2, MaxConcurrent: 2, Source: SubagentSourceProject, Prompt: "Build."},
		{Name: "guard", Description: "Security review for auth and secrets handling.", Mode: SubagentModeSubagent, Role: SpecialistSecurity, EngineProfile: "reasoning", Tools: []string{"read_*", "search_*"}, TokenBudget: 16000, MaxDepth: 1, MaxConcurrent: 1, ReadOnly: true, Source: SubagentSourceBuiltin, Prompt: "Guard."},
	}
}

func TestRecommendSubagentPrefersDescriptionMatch(t *testing.T) {
	rec := RecommendSubagent(routingTestDefs(), SubagentRoutingQuery{Objective: "survey the auth flow and find related files", TaskKind: "investigate"})
	if rec.Name != "explorer" || rec.Role != SpecialistInvestigator {
		t.Fatalf("unexpected recommendation: %+v", rec)
	}
	if rec.Affinity != 0 || !strings.Contains(rec.Source, "semantic") {
		t.Fatalf("unexpected source/affinity: %+v", rec)
	}
}

func TestRecommendSubagentGateDropsReadOnlyOnWrite(t *testing.T) {
	rec := RecommendSubagent(routingTestDefs(), SubagentRoutingQuery{Objective: "implement the fix", TaskKind: "implement", NeedsWrite: true, RequiredTools: []string{"edit.replace"}})
	if rec.Name == "explorer" {
		t.Fatalf("read-only explorer must be gated on write work: %+v", rec)
	}
	if rec.Role != SpecialistImplementer {
		t.Fatalf("writer expected, got %+v", rec)
	}
}

func TestRecommendSubagentGateKeepsSecurityChain(t *testing.T) {
	rec := RecommendSubagent(routingTestDefs(), SubagentRoutingQuery{Objective: "audit login secret handling", TaskKind: "review", SecuritySensitive: true})
	if rec.Role != SpecialistSecurity {
		t.Fatalf("security-sensitive work must stay on the security chain, got %+v", rec)
	}
	// Tool outside every allowlist fails open to the keyword fallback role, never
	// to a candidate that cannot do the job.
	rec = RecommendSubagent(routingTestDefs(), SubagentRoutingQuery{Objective: "do things", RequiredTools: []string{"spaceship.launch"}})
	if rec.Source != "keyword-fallback" {
		t.Fatalf("ungated tool must fall back, got %+v", rec)
	}
}

func TestRecommendSubagentAffinityIsAdvisoryAndCapped(t *testing.T) {
	defs := routingTestDefs()
	// Massive learned boost for builder must still be capped at ±0.10 and must
	// not override the write gate for explorer either way.
	rec := RecommendSubagent(defs, SubagentRoutingQuery{Objective: "survey the auth flow", TaskKind: "investigate", Affinity: map[string]float64{"builder": 5.0}})
	if rec.Affinity > SubagentAffinityCap+1e-9 || rec.Affinity < -SubagentAffinityCap-1e-9 {
		t.Fatalf("affinity must be capped at ±%.2f, got %+v", SubagentAffinityCap, rec)
	}
	if rec.Name != "explorer" {
		t.Fatalf("affinity must not override semantic match, got %+v", rec)
	}
	// Close race: mild verified affinity passes through capped.
	rec = RecommendSubagent(defs, SubagentRoutingQuery{Objective: "look around", TaskKind: "question", Affinity: map[string]float64{"explorer": 0.08}})
	if rec.Affinity != 0.08 {
		t.Fatalf("verified affinity must pass through when capped, got %+v", rec)
	}
}

func TestRecommendSubagentIsDeterministic(t *testing.T) {
	defs := routingTestDefs()
	query := SubagentRoutingQuery{Objective: "survey auth", TaskKind: "investigate", Affinity: map[string]float64{"explorer": 0.05}}
	first := RecommendSubagent(defs, query)
	for i := 0; i < 5; i++ {
		if next := RecommendSubagent(defs, query); next != first {
			t.Fatalf("non-deterministic routing: %+v vs %+v", first, next)
		}
	}
}

func TestRecommendSpecialistKeepsLegacySemantics(t *testing.T) {
	if got := RecommendSpecialist("security review", 2, true); got != SpecialistSecurity {
		t.Fatalf("unexpected role %s", got)
	}
	if got := RecommendSpecialist("fix backend bug", 2, false); got != SpecialistImplementer {
		t.Fatalf("unexpected role %s", got)
	}
	if got := RecommendSpecialist("architecture", 4, false); got != SpecialistDeep {
		t.Fatalf("unexpected role %s", got)
	}
}

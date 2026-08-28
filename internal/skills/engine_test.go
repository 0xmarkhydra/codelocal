package skills

import "testing"

func TestManifestRejectsProjectScope(t *testing.T) {
	manifest := BuiltinManifests()[0]
	manifest.Scope = Scope("project")
	if err := manifest.Validate(); err == nil {
		t.Fatal("expected project scope to be rejected")
	}
}

func TestRouterSelectsUIUXForFrontendTask(t *testing.T) {
	engine := DefaultEngine()
	selected := engine.Route(TaskContext{
		Query:   "make this dashboard easier to scan and responsive",
		Signals: []string{"frontend", "visual", "dashboard"},
		Stack:   []string{"nextjs"},
	})
	if len(selected) != 1 || selected[0].Skill.ID != "ui-ux-pro" {
		t.Fatalf("expected ui-ux-pro, got %#v", selected)
	}
}

func TestPlanRetrievesBoundedRelevantKnowledge(t *testing.T) {
	plan := DefaultEngine().Plan(TaskContext{
		Query:   "redesign this responsive dashboard and improve accessibility",
		Signals: []string{"frontend", "visual", "dashboard", "accessibility"},
		Stack:   []string{"nextjs"},
	})
	if len(plan.Selections) != 1 || plan.Selections[0].Skill.ID != "ui-ux-pro" {
		t.Fatalf("expected ui-ux-pro selection, got %#v", plan.Selections)
	}
	if len(plan.Knowledge) == 0 || len(plan.Knowledge) > 4 {
		t.Fatalf("expected bounded knowledge matches, got %d", len(plan.Knowledge))
	}
	foundAccessibility := false
	for _, match := range plan.Knowledge {
		if match.Chunk.ID == "uiux:a11y" {
			foundAccessibility = true
		}
		if match.Chunk.SkillID != "ui-ux-pro" {
			t.Fatalf("unexpected cross-skill knowledge %#v", match)
		}
	}
	if !foundAccessibility {
		t.Fatalf("expected accessibility knowledge, got %#v", plan.Knowledge)
	}
}

func TestRouterDoesNotSelectUIUXForBackendTask(t *testing.T) {
	engine := DefaultEngine()
	selected := engine.Route(TaskContext{Query: "fix Redis reconnect and backoff in Node backend", Stack: []string{"node"}})
	if len(selected) != 0 {
		t.Fatalf("expected no UI skill, got %#v", selected)
	}
}

func TestRouterSkipsTrivialTask(t *testing.T) {
	engine := DefaultEngine()
	selected := engine.Route(TaskContext{Query: "change Login to Sign in", Signals: []string{"frontend"}, Trivial: true})
	if len(selected) != 0 {
		t.Fatalf("expected no skill for trivial task, got %#v", selected)
	}
}

func TestPolicyRiskDoesNotGrantAnything(t *testing.T) {
	request := PolicyRequestFor(Manifest{ID: "deploy", Capabilities: []Capability{CapabilityShell, CapabilityNetwork}})
	if request.Risk != 4 {
		t.Fatalf("expected risk 4, got %d", request.Risk)
	}
	if len(request.Capabilities) != 2 {
		t.Fatalf("expected declared capabilities only, got %#v", request.Capabilities)
	}
}

func TestKnowledgeCandidateCannotSkipEvaluationAndCanary(t *testing.T) {
	if CanTransition(CandidateProposed, CandidatePromoted) {
		t.Fatal("candidate must not promote directly")
	}
	if !CanTransition(CandidateProposed, CandidateEvaluating) || !CanTransition(CandidateEvaluating, CandidateCanary) || !CanTransition(CandidateCanary, CandidatePromoted) {
		t.Fatal("expected proposed -> evaluating -> canary -> promoted lifecycle")
	}
}

func TestOutcomeProducesSanitizedRankingSignal(t *testing.T) {
	signal, err := RankingSignalFromOutcome(Outcome{
		SkillID:          "ui-ux-pro",
		TechnicalSuccess: true,
		UserAccepted:     true,
		TaskCompleted:    true,
		ExplicitRating:   5,
	})
	if err != nil {
		t.Fatal(err)
	}
	if signal.SkillID != "ui-ux-pro" || signal.Delta <= 0 {
		t.Fatalf("unexpected signal %#v", signal)
	}
}

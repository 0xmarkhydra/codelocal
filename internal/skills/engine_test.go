package skills

import "testing"

func TestManifestRejectsProjectScope(t *testing.T) {
	manifest := BuiltinManifests()[0]
	manifest.Scope = Scope("project")
	if err := manifest.Validate(); err == nil {
		t.Fatal("project-specific knowledge belongs to Project Brain, not Skill scope")
	}
}

func TestRegistryVersionsAreImmutableUntilPromoted(t *testing.T) {
	v1 := BuiltinManifests()[0]
	registry, err := NewRegistry(v1)
	if err != nil {
		t.Fatal(err)
	}
	v2 := v1
	v2.Version = "1.1.0"
	v2.Quality = 0.95
	if err := registry.Put(v2); err != nil {
		t.Fatal(err)
	}
	current, ok := registry.Get(v1.ID)
	if !ok || current.Version != "1.0.0" {
		t.Fatalf("new version must not auto-promote: %#v", current)
	}
	if len(registry.Versions(v1.ID)) != 2 {
		t.Fatalf("expected two immutable versions, got %#v", registry.Versions(v1.ID))
	}
	if err := registry.SetCurrentVersion(v1.ID, "1.1.0"); err != nil {
		t.Fatal(err)
	}
	current, _ = registry.Get(v1.ID)
	if current.Version != "1.1.0" || current.Quality != 0.95 {
		t.Fatalf("promotion pointer did not move: %#v", current)
	}
	if err := registry.Put(v2); err == nil {
		t.Fatal("immutable duplicate version must be rejected")
	}
}

func TestBuiltinSourceIsPinned(t *testing.T) {
	manifest := BuiltinManifests()[0]
	if manifest.SourceURL == "" || manifest.SourceRef == "" || manifest.SourceHash == "" || manifest.License != "MIT" {
		t.Fatalf("builtin provenance must be reproducible: %#v", manifest)
	}
	for _, chunk := range BuiltinKnowledge() {
		if chunk.SkillVersion != manifest.Version || chunk.SourceRef != manifest.SourceRef || chunk.SourceHash != manifest.SourceHash {
			t.Fatalf("knowledge provenance must match manifest: %#v", chunk)
		}
	}
}

func TestClassifierUnderstandsVietnameseUIScreenshot(t *testing.T) {
	task := ClassifyTask(TaskEvidence{Query: "Nhìn màn này khó chịu quá, làm đẹp hơn", HasImage: true})
	if len(task.Intents) == 0 || task.Trivial {
		t.Fatalf("expected non-trivial UI intent, got %#v", task)
	}
}

func TestClassifierDoesNotTreatArbitraryImageAsUI(t *testing.T) {
	task := ClassifyTask(TaskEvidence{Query: "Phân tích hóa đơn trong ảnh này", HasImage: true})
	if len(task.Intents) != 0 || len(task.Signals) != 0 {
		t.Fatalf("arbitrary image must not become UI intent: %#v", task)
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
		if match.Chunk.SkillID != "ui-ux-pro" || match.Chunk.SkillVersion != plan.Selections[0].Skill.Version {
			t.Fatalf("unexpected cross-skill/version knowledge %#v", match)
		}
	}
	if !foundAccessibility {
		t.Fatalf("expected accessibility knowledge, got %#v", plan.Knowledge)
	}
	if len(plan.Steps) != 1 || plan.Steps[0].SkillVersion != plan.Selections[0].Skill.Version {
		t.Fatalf("plan step must pin selected version: %#v", plan.Steps)
	}
}

func TestKnowledgeStoreDoesNotLeakOtherVersion(t *testing.T) {
	manifest := BuiltinManifests()[0]
	selection := Selection{Skill: manifest}
	chunks := append(BuiltinKnowledge(), KnowledgeChunk{
		ID: "future", SkillID: manifest.ID, SkillVersion: "9.9.9", Domain: "layout", Title: "future", Content: "future", Priority: 100, Tags: []string{"dashboard"},
	})
	matches := NewMemoryKnowledgeStore(chunks).Search(TaskContext{Query: "dashboard layout"}, []Selection{selection}, 8)
	for _, match := range matches {
		if match.Chunk.ID == "future" {
			t.Fatal("knowledge from non-selected version leaked into stable plan")
		}
	}
}

func TestRouterDoesNotSelectUIUXForBackendTask(t *testing.T) {
	engine := DefaultEngine()
	selected := engine.Route(TaskContext{Query: "fix Redis reconnect and backoff in Node backend", Stack: []string{"nextjs"}})
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

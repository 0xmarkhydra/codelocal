package skills

import "testing"

type staticCandidates []Manifest

func (s staticCandidates) Candidates(TaskContext) []Manifest { return []Manifest(s) }

func TestRouterUsesPrefilteredCandidateSource(t *testing.T) {
	manifest := BuiltinManifests()[0]
	router := NewRouterWithCandidates(staticCandidates{manifest})
	selected := router.Route(TaskContext{Query: "redesign dashboard", Intents: []string{"design_ui"}})
	if len(selected) != 1 || selected[0].Skill.ID != manifest.ID {
		t.Fatalf("expected prefiltered candidate to route, got %#v", selected)
	}
}

func TestRouterDoesNotNeedGlobalRegistry(t *testing.T) {
	router := NewRouterWithCandidates(staticCandidates{})
	if selected := router.Route(TaskContext{Query: "redesign dashboard", Intents: []string{"design_ui"}}); len(selected) != 0 {
		t.Fatalf("empty candidate source must stay empty, got %#v", selected)
	}
}

package learnedskills

import "testing"

func portableFixture(workspaceKey string) Recipe {
	return Recipe{
		ID: "local-" + workspaceKey, Version: Version, WorkspaceKey: workspaceKey,
		Intent: "open notifications", TaskKind: "desktop",
		Steps: []Step{
			{Tool: "computer", Args: map[string]any{"action": "focus", "windowHint": "Google Chrome Facebook"}},
			{Tool: "computer", Args: map[string]any{"action": "click", "windowHint": "Google Chrome Facebook", "target": "Notifications", "verify": true}},
			{Tool: "computer", Args: map[string]any{"action": "observe", "windowHint": "Google Chrome Facebook"}},
		},
		Confidence: .91, SuccessCount: 4, Status: StatusTrusted, UpdatedAt: 100, LastUsedAt: 90,
		Context: &ContextFingerprint{
			ProjectID: "local-project", RepositoryIDs: []string{"repo-a"}, RulesHash: "rules-a",
			WorkflowFiles: map[string]string{"go.mod": "hash-a"}, BranchPolicy: "any", Branch: "dev",
			RequiredCapabilities: []string{"computer"},
		},
	}
}

func TestPortableRecipeIsStableAcrossWorkspaceKeysAndCanonicalizesProject(t *testing.T) {
	left, ok, reason := PortableRecipeFor(portableFixture("device-a::workspace-a"), "canonical-project")
	if !ok {
		t.Fatalf("left recipe not portable: %s", reason)
	}
	right, ok, reason := PortableRecipeFor(portableFixture("device-b::workspace-b"), "canonical-project")
	if !ok {
		t.Fatalf("right recipe not portable: %s", reason)
	}
	if left.ID == "" || left.ID != right.ID {
		t.Fatalf("portable identity must ignore local workspace identity: left=%q right=%q", left.ID, right.ID)
	}
	if left.ProjectID != "canonical-project" || left.Context == nil || left.Context.ProjectID != "canonical-project" {
		t.Fatalf("canonical project identity not enforced: %#v", left)
	}
	if left.Context.Branch != "" || left.Context.BranchPolicy != "any" {
		t.Fatalf("non-exact branch must not become a machine/session binding: %#v", left.Context)
	}
}

func TestPortableRecipeRejectsSecretsEphemeralBindingsAndTerminalCommands(t *testing.T) {
	cases := []struct {
		name string
		step Step
	}{
		{name: "approval token", step: Step{Tool: "computer", Args: map[string]any{"action": "observe", "approvalToken": "secret"}}},
		{name: "window id", step: Step{Tool: "computer", Args: map[string]any{"action": "click", "windowHint": "Chrome", "windowId": "123", "target": "Notifications"}}},
		{name: "coordinates", step: Step{Tool: "computer", Args: map[string]any{"action": "click", "windowHint": "Chrome", "target": "Notifications", "x": 10, "y": 20}}},
		{name: "terminal", step: Step{Tool: "terminal", Args: map[string]any{"action": "run", "command": "echo safe"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			recipe := portableFixture("workspace")
			recipe.Steps = []Step{tc.step}
			if portable, ok, reason := PortableRecipeFor(recipe, "project"); ok {
				t.Fatalf("unsafe recipe unexpectedly portable: %#v reason=%q", portable, reason)
			}
		})
	}
}

func TestPortableRecipeRejectsUnsafeWorkflowPaths(t *testing.T) {
	for _, path := range []string{"../secret", "..\\secret", "/Users/me/private", "C:/Users/me/private", `C:\Users\me\private`, `\\server\share\private`} {
		recipe := portableFixture("workspace")
		recipe.Context.WorkflowFiles = map[string]string{path: "hash"}
		if _, ok, _ := PortableRecipeFor(recipe, "project"); ok {
			t.Fatalf("unsafe workflow path %q became portable", path)
		}
	}
}

func TestNormalizePortableRecipeRevalidatesAndRebindsCanonicalProject(t *testing.T) {
	portable, ok, reason := PortableRecipeFor(portableFixture("workspace"), "client-project")
	if !ok {
		t.Fatal(reason)
	}
	portable.ProjectID = "forged-project"
	portable.Context.ProjectID = "forged-project"
	normalized, ok := NormalizePortableRecipe(portable, "server-project")
	if !ok || normalized.ProjectID != "server-project" || normalized.Context == nil || normalized.Context.ProjectID != "server-project" {
		t.Fatalf("cloud canonical project binding not enforced: %#v ok=%v", normalized, ok)
	}
	if normalized.ID != portable.ID {
		t.Fatalf("portable workflow identity must survive project alias rebinding: old=%q new=%q", portable.ID, normalized.ID)
	}
}

func TestImportedPortableSkillCannotReplayUntilLocallyVerified(t *testing.T) {
	store := newTestStore(t)
	remote, ok, reason := PortableRecipeFor(portableFixture("device-a::workspace-a"), "canonical-project")
	if !ok {
		t.Fatal(reason)
	}
	localContext := &ContextFingerprint{
		ProjectID: "local-project-alias", RepositoryIDs: []string{"local-repo"}, RulesHash: "rules-a",
		WorkflowFiles: map[string]string{"go.mod": "hash-a"}, BranchPolicy: "any", RequiredCapabilities: []string{"computer"},
	}
	workspaceKey := "device-b::workspace-b"
	imported, changed, err := store.ImportPortable(workspaceKey, "canonical-project", remote, localContext)
	if err != nil || !changed || imported == nil || imported.Status != StatusImported || imported.SuccessCount != 0 || imported.PortableID != remote.ID {
		t.Fatalf("unexpected imported skill: recipe=%#v changed=%v err=%v", imported, changed, err)
	}
	matched, err := store.MatchWithContext(workspaceKey, "open notifications", "desktop", localContext)
	if err != nil || matched == nil || matched.Status != StatusImported {
		t.Fatalf("imported skill should be visible but untrusted: match=%#v err=%v", matched, err)
	}
	local, err := store.RecordWithContext(workspaceKey, remote.Intent, remote.TaskKind, remote.Steps, true, localContext)
	if err != nil || local == nil || local.Status != StatusCandidate || local.SuccessCount != 1 || local.PortableID != remote.ID {
		t.Fatalf("local verification did not replace imported suggestion: %#v err=%v", local, err)
	}
	items, err := store.List(workspaceKey, 10)
	if err != nil || len(items) != 1 || items[0].Status != StatusCandidate {
		t.Fatalf("imported/local duplicate remained after verification: %#v err=%v", items, err)
	}
}

func TestStaleImportedSkillRestoresImportedNotCandidate(t *testing.T) {
	store := newTestStore(t)
	remote, ok, reason := PortableRecipeFor(portableFixture("remote"), "canonical-project")
	if !ok {
		t.Fatal(reason)
	}
	local := &ContextFingerprint{ProjectID: "local-project", RulesHash: "rules-b", WorkflowFiles: map[string]string{"go.mod": "hash-a"}, RequiredCapabilities: []string{"computer"}}
	imported, changed, err := store.ImportPortable("local-workspace", "canonical-project", remote, local)
	if err != nil || !changed || imported.Status != StatusStale || imported.StatusBeforeStale != StatusImported {
		t.Fatalf("incompatible imported skill should be stale: %#v changed=%v err=%v", imported, changed, err)
	}
	compatible := *local
	compatible.RulesHash = "rules-a"
	matched, err := store.MatchWithContext("local-workspace", "open notifications", "desktop", &compatible)
	if err != nil || matched == nil || matched.Status != StatusImported {
		t.Fatalf("stale imported skill gained replay trust on revalidation: %#v err=%v", matched, err)
	}
}

package orchestration

import "testing"

func TestBuildPlanUsesRepositoryEvidenceAndTargetedGoVerification(t *testing.T) {
	plan := BuildPlan(PlanInput{
		Task:         "fix recovery loop bug",
		Capabilities: Capabilities{Filesystem: true, LSP: true, Shell: true},
		Project: ProjectProfile{
			Languages:     []string{"go"},
			TestCommands:  []string{"go test ./..."},
			BuildCommands: []string{"go build ./..."},
		},
		TouchedFiles: []string{"internal/orchestration/router.go", "internal/orchestration/recovery.go"},
		LastAction:   "edit.apply",
	})
	if plan.TaskKind != TaskKindDebug {
		t.Fatalf("expected debug plan, got %#v", plan)
	}
	if plan.Phase != "verify" {
		t.Fatalf("edit should move plan into verification, got %q", plan.Phase)
	}
	foundTargeted := false
	for _, check := range plan.Verification.Checks {
		if check.Command == "go test ./internal/orchestration" && check.Required {
			foundTargeted = true
		}
	}
	if !foundTargeted {
		t.Fatalf("expected targeted Go package test, got %#v", plan.Verification.Checks)
	}
}

func TestEvidenceRouteUsesRepositoryStateForKeywordlessContinuation(t *testing.T) {
	plan := BuildPlan(PlanInput{
		Task:         "continue from where we are",
		Capabilities: Capabilities{Filesystem: true, Shell: true, Browser: true, Computer: true},
		Project:      ProjectProfile{Languages: []string{"go"}},
		TouchedFiles: []string{"internal/orchestration/planner.go"},
		LastAction:   "edit.apply",
	})
	if plan.Route.Primary != LaneCode {
		t.Fatalf("repository evidence should route ambiguous continuation to code: %#v", plan.Route)
	}
	if plan.Route.Confidence <= 0.5 || len(plan.Route.Evidence) == 0 || plan.Route.Scores[LaneCode] <= plan.Route.Scores[LaneShell] {
		t.Fatalf("expected evidence-weighted route details: %#v", plan.Route)
	}
}

func TestEvidenceRouteKeepsStructuredBrowserAheadOfDesktopFallback(t *testing.T) {
	plan := BuildPlan(PlanInput{
		Task:         "fill the checkout form on the website",
		Capabilities: Capabilities{Browser: true, Computer: true},
		LastAction:   "browser.snapshot",
	})
	if plan.Route.Primary != LaneBrowser {
		t.Fatalf("structured browser should win over desktop fallback: %#v", plan.Route)
	}
}

func TestDebugPlanDecomposesCausalRepairInsteadOfGenericTemplate(t *testing.T) {
	plan := BuildPlan(PlanInput{
		Task:         "fix crash in login recovery",
		Capabilities: Capabilities{Filesystem: true, LSP: true, Shell: true},
		Project:      ProjectProfile{Languages: []string{"go"}},
	})
	ids := map[string]bool{}
	for _, step := range plan.Steps {
		ids[step.ID] = true
	}
	for _, want := range []string{"reproduce", "diagnose", "repair", "verify", "recover", "quality"} {
		if !ids[want] {
			t.Fatalf("debug plan missing task-specific step %q: %#v", want, plan.Steps)
		}
	}
}

func TestReviewPlanDoesNotDefaultToMutation(t *testing.T) {
	plan := BuildPlan(PlanInput{
		Task:         "review authentication code for regressions",
		Capabilities: Capabilities{Filesystem: true, LSP: true, Shell: true},
		Project:      ProjectProfile{Languages: []string{"go"}},
	})
	if plan.TaskKind != TaskKindReview {
		t.Fatalf("expected review kind, got %q", plan.TaskKind)
	}
	for _, step := range plan.Steps {
		if step.Phase == "act" && step.Tool == "edit" {
			t.Fatalf("review plan should not mutate by default: %#v", plan.Steps)
		}
	}
}

func TestBuildVerificationPlanUsesDocsOnlyFastPath(t *testing.T) {
	plan := BuildVerificationPlan(PlanInput{TouchedFiles: []string{"README.md", "ROADMAP.md"}})
	if plan.Mode != "docs-only" || len(plan.Checks) != 1 || plan.Checks[0].Key != "diff-check" {
		t.Fatalf("unexpected docs-only plan: %#v", plan)
	}
}

func TestBuildVerificationPlanWidensForManifestChanges(t *testing.T) {
	plan := BuildVerificationPlan(PlanInput{
		TouchedFiles: []string{"package.json", "src/index.ts"},
		Project: ProjectProfile{
			TypecheckCommands: []string{"npm run typecheck"},
			TestCommands:      []string{"npm test"},
			BuildCommands:     []string{"npm run build"},
		},
	})
	keys := map[string]bool{}
	for _, check := range plan.Checks {
		keys[check.Key] = true
	}
	for _, key := range []string{"diff-check", "typecheck", "test", "build"} {
		if !keys[key] {
			t.Fatalf("manifest-aware plan missing %s: %#v", key, plan.Checks)
		}
	}
}

func TestQualityGateRequiresFreshVerificationAndRequiredChecks(t *testing.T) {
	verification := VerificationPlan{Checks: []VerificationCheck{
		{Key: "diff-check", Required: true},
		{Key: "test", Required: true},
	}}
	quality := EvaluateQuality(PlanInput{
		TouchedFiles:     []string{"x.go"},
		VerificationSeen: true,
		DiagnosticDelta:  0,
		DiffObserved:     true,
		PassedChecks:     []string{"diff-check", "test"},
	}, verification)
	if quality.Status != "ready" || quality.Score < 85 || len(quality.MissingChecks) != 0 {
		t.Fatalf("expected ready quality gate, got %#v", quality)
	}

	blocked := EvaluateQuality(PlanInput{
		TouchedFiles:     []string{"x.go"},
		VerificationSeen: true,
		DiagnosticDelta:  2,
		DiffObserved:     true,
		PassedChecks:     []string{"diff-check", "test"},
	}, verification)
	if blocked.Status != "blocked" {
		t.Fatalf("diagnostic regression must block completion: %#v", blocked)
	}
}

func TestCheckKeyNeverStoresRawTerminalCommand(t *testing.T) {
	cases := map[string]string{
		"go test ./internal/foo": "test",
		"go vet ./...":           "lint",
		"npm run typecheck":      "typecheck",
		"npm run build":          "build",
		"git diff --check":       "diff-check",
	}
	for command, want := range cases {
		if got := CheckKey(command); got != want {
			t.Fatalf("CheckKey(%q)=%q want %q", command, got, want)
		}
	}
	if got := CheckKey("curl https://example.com?token=secret"); got != "" {
		t.Fatalf("unknown command must not become durable verification state: %q", got)
	}
}

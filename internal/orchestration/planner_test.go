package orchestration

import (
	"strings"
	"testing"
)

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
	if plan.Mode != "docs-only" || len(plan.Checks) != 1 || plan.Checks[0].Key != CheckID("git diff --check") {
		t.Fatalf("unexpected docs-only plan: %#v", plan)
	}
}

func TestScopedCheckIDDifferentiatesRepositoryCWD(t *testing.T) {
	web := ScopedCheckID("npm test", "web")
	admin := ScopedCheckID("npm test", "admin")
	if web == admin || web == "" || admin == "" {
		t.Fatalf("same verification command in different repositories must have distinct evidence IDs: web=%q admin=%q", web, admin)
	}
	if CheckID("npm test") != ScopedCheckID("npm test", ".") {
		t.Fatal("root-scoped verification must preserve legacy CheckID compatibility")
	}
}

func TestBuildVerificationPlanScopesChecksPerRepository(t *testing.T) {
	plan := BuildVerificationPlan(PlanInput{
		TouchedFiles: []string{"web/src/login.ts", "admin/src/users.ts"},
		Project: ProjectProfile{Repositories: []RepositoryProfile{
			{ID: "web", Path: "web", TypecheckCommands: []string{"npm run typecheck"}, TestCommands: []string{"npm test"}},
			{ID: "admin", Path: "admin", TypecheckCommands: []string{"npm run typecheck"}, TestCommands: []string{"npm test"}},
		}},
	})
	if plan.Mode != "multi-repository" {
		t.Fatalf("expected multi-repository verification plan, got %#v", plan)
	}
	seen := map[string]map[string]bool{}
	keys := map[string]bool{}
	for _, check := range plan.Checks {
		if seen[check.CWD] == nil {
			seen[check.CWD] = map[string]bool{}
		}
		seen[check.CWD][CheckKey(check.Command)] = true
		if keys[check.Key] {
			t.Fatalf("repository-scoped check key collided: %#v", plan.Checks)
		}
		keys[check.Key] = true
	}
	for _, cwd := range []string{"web", "admin"} {
		if !seen[cwd]["typecheck"] || !seen[cwd]["test"] || !seen[cwd]["diff-check"] {
			t.Fatalf("repository %s missing scoped checks: %#v", cwd, plan.Checks)
		}
	}
}

func TestBuildVerificationPlanUsesRepositoryRelativeGoPaths(t *testing.T) {
	plan := BuildVerificationPlan(PlanInput{
		TouchedFiles: []string{"backend/auth/internal/login/login.go"},
		Project:      ProjectProfile{Repositories: []RepositoryProfile{{ID: "auth", Path: "backend/auth", TestCommands: []string{"go test ./..."}}}},
	})
	found := false
	for _, check := range plan.Checks {
		if check.Command == "go test ./internal/login" && check.CWD == "backend/auth" {
			found = true
		}
	}
	if !found {
		t.Fatalf("Go verification must be repo-relative: %#v", plan.Checks)
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
	categories := map[string]bool{}
	for _, check := range plan.Checks {
		categories[CheckKey(check.Command)] = true
	}
	for _, category := range []string{"diff-check", "typecheck", "test", "build"} {
		if !categories[category] {
			t.Fatalf("manifest-aware plan missing %s: %#v", category, plan.Checks)
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

func TestCheckIdentityIsCommandSpecificAndSecretSafe(t *testing.T) {
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
		id := CheckID(command)
		if id == "" || !strings.HasPrefix(id, want+":") || strings.Contains(id, command) {
			t.Fatalf("CheckID(%q) must be category-prefixed and opaque, got %q", command, id)
		}
	}
	if got := CheckKey("curl https://example.com?token=secret"); got != "" {
		t.Fatalf("unknown command must not become durable verification state: %q", got)
	}
	if CheckID("go test ./internal/foo") == CheckID("go test ./internal/bar") {
		t.Fatal("different test commands must produce different durable check IDs")
	}
	if CheckID(" GO   TEST   ./internal/foo ") != CheckID("go test ./internal/foo") {
		t.Fatal("normalized equivalent commands must produce the same durable check ID")
	}
}

func TestQualityGateRequiresEveryDistinctTestCommand(t *testing.T) {
	first := CheckID("go test ./internal/foo")
	second := CheckID("go test ./internal/bar")
	verification := VerificationPlan{Checks: []VerificationCheck{
		{Key: first, Command: "go test ./internal/foo", Required: true},
		{Key: second, Command: "go test ./internal/bar", Required: true},
	}}
	quality := EvaluateQuality(PlanInput{
		VerificationSeen: true,
		DiffObserved:     true,
		PassedChecks:     []string{first},
	}, verification)
	if quality.Status == "ready" || len(quality.MissingChecks) != 1 || quality.MissingChecks[0] != second {
		t.Fatalf("passing one test command must not satisfy another: %#v", quality)
	}
}

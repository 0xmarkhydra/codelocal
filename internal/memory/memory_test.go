package memory

import (
	"strings"
	"testing"
	"time"
)

func TestSanitizeTextRedactsSecrets(t *testing.T) {
	input := "GEMINI_API_KEY=abc123 Authorization: Bearer xyz.token password: hunter2"
	got := SanitizeText(input, 2000)
	for _, secret := range []string{"abc123", "xyz.token", "hunter2"} {
		if strings.Contains(got, secret) {
			t.Fatalf("secret %q was not redacted: %q", secret, got)
		}
	}
	if !strings.Contains(got, "[REDACTED]") {
		t.Fatalf("expected redaction marker: %q", got)
	}
}

func TestSanitizeTextRedactsPrivateKey(t *testing.T) {
	input := "before\n-----BEGIN PRIVATE KEY-----\nsecret\n-----END PRIVATE KEY-----\nafter"
	got := SanitizeText(input, 2000)
	if strings.Contains(got, "secret") || !strings.Contains(got, "REDACTED PRIVATE KEY") {
		t.Fatalf("private key was not redacted: %q", got)
	}
}

func TestSanitizeListDeduplicatesAndBounds(t *testing.T) {
	got := SanitizeList([]string{"a.go", "a.go", "b.go", "c.go"}, 2)
	if len(got) != 2 || got[0] != "a.go" || got[1] != "b.go" {
		t.Fatalf("unexpected list: %#v", got)
	}
}

func TestRankBoostsSemanticAndCodeOverlap(t *testing.T) {
	now := time.Now().UnixMilli()
	base := Record{ID: "a", UserID: "u", WorkspaceID: "w", Level: LevelScenario, Summary: "OAuth redirect mismatch", Files: []string{"oauth.go"}, Symbols: []string{"normalizeRedirect"}, Confidence: .9, Importance: .8, CreatedAt: now, LastUsedAt: now, LexicalScore: .2, VectorScore: .8}
	relevant := Score(base, RecallInput{Files: []string{"oauth.go"}, Symbols: []string{"normalizeRedirect"}}, now)
	base.Files = []string{"other.go"}
	base.Symbols = []string{"other"}
	base.VectorScore = .2
	unrelated := Score(base, RecallInput{Files: []string{"oauth.go"}, Symbols: []string{"normalizeRedirect"}}, now)
	if relevant <= unrelated {
		t.Fatalf("relevant score %.4f must beat unrelated %.4f", relevant, unrelated)
	}
}

func TestRankFavorsRecentMemory(t *testing.T) {
	now := time.Now().UnixMilli()
	recent := Record{CreatedAt: now, Confidence: .7, Importance: .5, LexicalScore: .5, VectorScore: .5}
	old := recent
	old.CreatedAt = now - int64(180*24*time.Hour/time.Millisecond)
	if Score(recent, RecallInput{}, now) <= Score(old, RecallInput{}, now) {
		t.Fatal("recent memory should rank above equally relevant old memory")
	}
}

func TestRankUsesUpdatedAtForMutableMemoryFreshness(t *testing.T) {
	now := time.Now().UnixMilli()
	oldCreated := now - int64(180*24*time.Hour/time.Millisecond)
	updated := Record{CreatedAt: oldCreated, UpdatedAt: now, Confidence: .7, Importance: .5, LexicalScore: .5, VectorScore: .5}
	stale := updated
	stale.UpdatedAt = oldCreated
	if Score(updated, RecallInput{}, now) <= Score(stale, RecallInput{}, now) {
		t.Fatal("recently updated mutable memory should outrank equally relevant stale memory")
	}
	if memoryFreshnessAt(Record{CreatedAt: 10}) != 10 {
		t.Fatal("legacy memories must fall back to created_at when updated_at is absent")
	}
}

func TestRankBoostsCurrentRepositoryAndProjectMemory(t *testing.T) {
	now := time.Now().UnixMilli()
	input := RecallInput{WorkspaceID: "workspace-a", ProjectID: "project-a", RepositoryIDs: []string{"repo-a"}}
	base := Record{CreatedAt: now, UpdatedAt: now, Confidence: .8, Importance: .8, LexicalScore: .5, VectorScore: .5}
	repository := base
	repository.Scope = ScopeRepository
	repository.ProjectID = "project-a"
	repository.RepositoryID = "repo-a"
	project := base
	project.Scope = ScopeProject
	project.ProjectID = "project-a"
	global := base
	global.Scope = ScopeGlobal
	if Score(repository, input, now) <= Score(project, input, now) {
		t.Fatal("current repository memory should outrank equally relevant project memory")
	}
	if Score(project, input, now) <= Score(global, input, now) {
		t.Fatal("current project memory should outrank equally relevant global memory")
	}
}

func TestRankPrefersExactBranchAndActiveLifecycle(t *testing.T) {
	now := time.Now().UnixMilli()
	input := RecallInput{Branch: "dev", ProjectID: "project-a"}
	base := Record{Scope: ScopeProject, ProjectID: "project-a", CreatedAt: now, UpdatedAt: now, Confidence: .8, Importance: .8, LexicalScore: .5, VectorScore: .5, Lifecycle: LifecycleActive}
	exact := base
	exact.Branch = "dev"
	agnostic := base
	agnostic.Branch = ""
	other := base
	other.Branch = "release"
	if Score(exact, input, now) <= Score(agnostic, input, now) || Score(agnostic, input, now) <= Score(other, input, now) {
		t.Fatalf("branch ordering wrong exact=%.3f agnostic=%.3f other=%.3f", Score(exact, input, now), Score(agnostic, input, now), Score(other, input, now))
	}
	stale := exact
	stale.Lifecycle = LifecycleStale
	if Score(exact, input, now) <= Score(stale, input, now) {
		t.Fatal("active memory must outrank otherwise-equal stale memory")
	}
	if lifecycleScore(LifecycleInvalidated) != 0 || lifecycleScore(LifecycleSuperseded) != 0 {
		t.Fatal("invalidated/superseded lifecycle must have zero quality weight")
	}
}

func TestRankExcludesIncompatibleBranchScopedMemory(t *testing.T) {
	now := time.Now().UnixMilli()
	input := RecallInput{Branch: "dev", ProjectID: "project-a", Limit: 10}
	base := Record{Scope: ScopeProject, ProjectID: "project-a", CreatedAt: now, UpdatedAt: now, Confidence: .9, Importance: .9, LexicalScore: 1, VectorScore: 1, Lifecycle: LifecycleActive}
	wrong := base
	wrong.ID = "wrong"
	wrong.Branch = "release"
	neutral := base
	neutral.ID = "neutral"
	neutral.Branch = ""
	exact := base
	exact.ID = "exact"
	exact.Branch = "dev"
	ranked := Rank([]Record{wrong, neutral, exact}, input, now)
	if len(ranked) != 2 || ranked[0].ID != "exact" {
		t.Fatalf("wrong-branch memory must be filtered before ranking: %#v", ranked)
	}
	for _, record := range ranked {
		if record.ID == "wrong" {
			t.Fatalf("incompatible branch memory leaked into recall: %#v", ranked)
		}
	}
	if rankedUnknown := Rank([]Record{wrong}, RecallInput{ProjectID: "project-a", Limit: 10}, now); len(rankedUnknown) != 0 {
		t.Fatalf("branch-scoped memory must not apply when current branch is unknown: %#v", rankedUnknown)
	}
}

func TestRankSuppressesLegacyOperationalNoiseWithoutDeletingLegitimateKnowledge(t *testing.T) {
	now := time.Now().UnixMilli()
	base := Record{Scope: ScopeProject, ProjectID: "project-a", SourceType: "task", CreatedAt: now, UpdatedAt: now, Confidence: .9, Importance: .9, LexicalScore: 1, VectorScore: 1, Lifecycle: LifecycleActive}
	noise := []Record{
		{ID: "edit", Summary: "Files edited for task: Fix login"},
		{ID: "verify", Summary: "Verification evidence refreshed; more checks may still be required for task: Fix login"},
		{ID: "ready", Summary: "Fresh verification evidence reached the ready quality gate for task: Fix login"},
		{ID: "terminal", Summary: "Agent quality gate reached ready state for task: Fix login"},
		{ID: "failed", Summary: "edit.apply failed while working on task: Fix login"},
	}
	records := make([]Record, 0, len(noise)+2)
	for _, record := range noise {
		record.Scope = base.Scope
		record.ProjectID = base.ProjectID
		record.SourceType = base.SourceType
		record.CreatedAt = base.CreatedAt
		record.UpdatedAt = base.UpdatedAt
		record.Confidence = base.Confidence
		record.Importance = base.Importance
		record.LexicalScore = base.LexicalScore
		record.VectorScore = base.VectorScore
		record.Lifecycle = base.Lifecycle
		records = append(records, record)
	}
	legitimate := base
	legitimate.ID = "fact"
	legitimate.SourceType = "conversation"
	legitimate.Summary = "Files edited for task: is an example of operational noise and must not become durable knowledge."
	records = append(records, legitimate)
	oldUsefulEvent := base
	oldUsefulEvent.ID = "useful-event"
	oldUsefulEvent.Summary = "OAuth integration test failed with redirect mismatch"
	records = append(records, oldUsefulEvent)
	legitimateFailure := base
	legitimateFailure.ID = "legitimate-failure"
	legitimateFailure.Summary = "OAuth integration test failed while working on task: callback mismatch"
	records = append(records, legitimateFailure)

	ranked := Rank(records, RecallInput{ProjectID: "project-a", Limit: 20}, now)
	if len(ranked) != 3 {
		t.Fatalf("legacy operational noise survived recall guard: %#v", ranked)
	}
	seen := map[string]bool{}
	for _, record := range ranked {
		seen[record.ID] = true
	}
	if !seen["fact"] || !seen["useful-event"] || !seen["legitimate-failure"] {
		t.Fatalf("recall guard suppressed legitimate knowledge: %#v", ranked)
	}
}

func TestNormalizeLifecycleDefaultsLegacyRowsToActive(t *testing.T) {
	if got := normalizeLifecycle(""); got != LifecycleActive {
		t.Fatalf("legacy lifecycle=%q want active", got)
	}
	if got := normalizeLifecycle(LifecycleConfirmed); got != LifecycleConfirmed {
		t.Fatalf("confirmed lifecycle normalized to %q", got)
	}
}

func TestNormalizeWorkspaceRelativePathRejectsEscapes(t *testing.T) {
	if got := normalizeWorkspaceRelativePath("services/product/main.go"); got != "services/product/main.go" {
		t.Fatalf("unexpected normalized workspace path: %q", got)
	}
	for _, input := range []string{"../secret", "/etc/passwd", "./../other"} {
		if got := normalizeWorkspaceRelativePath(input); got != "" {
			t.Fatalf("workspace path escape must be rejected: %q -> %q", input, got)
		}
	}
}

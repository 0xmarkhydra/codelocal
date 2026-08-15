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

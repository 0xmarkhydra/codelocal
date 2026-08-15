package learnedskills

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	return New()
}

func TestRecordMatchAndFeedback(t *testing.T) {
	store := newTestStore(t)
	steps := []Step{
		{Tool: "terminal", Args: map[string]any{"action": "run", "command": "xcrun simctl launch booted vn.infivision.biddi.dev"}},
		{Tool: "computer", Args: map[string]any{"action": "observe"}},
	}
	recipe, err := store.Record("device::biddi", "mở BIDDI Beta", "desktop", steps, true)
	if err != nil {
		t.Fatal(err)
	}
	if recipe.Status != StatusCandidate || recipe.SuccessCount != 1 || recipe.Confidence < 0.64 {
		t.Fatalf("unexpected initial recipe: %#v", recipe)
	}

	matched, err := store.Match("device::biddi", "mở app BIDDI Beta", "desktop")
	if err != nil {
		t.Fatal(err)
	}
	if matched == nil || matched.ID != recipe.ID || matched.MatchScore < 0.58 {
		t.Fatalf("expected matching recipe, got %#v", matched)
	}

	for i := 0; i < 2; i++ {
		matched, err = store.Feedback("device::biddi", recipe.ID, true)
		if err != nil {
			t.Fatal(err)
		}
	}
	if matched == nil || matched.Status != StatusTrusted || matched.SuccessCount < 3 || matched.Confidence < 0.85 {
		t.Fatalf("repeated successful feedback should trust recipe: %#v", matched)
	}

	matched, err = store.Feedback("device::biddi", recipe.ID, false)
	if err != nil {
		t.Fatal(err)
	}
	if matched == nil || matched.FailureCount != 1 || matched.Confidence >= 0.98 {
		t.Fatalf("failure should reduce confidence: %#v", matched)
	}
}

func TestIntentSimilarityRewardsRepeatedTaskCoverage(t *testing.T) {
	score := intentSimilarity("đọc thông báo Facebook", "Google Chrome Facebook Thông báo Chưa đọc")
	if score < 0.65 {
		t.Fatalf("short repeated task should strongly match learned UI semantics, got %.3f", score)
	}
	if score := intentSimilarity("Facebook", "Google Chrome Facebook Notifications Unread"); score >= 0.58 {
		t.Fatalf("a single shared entity token should not be enough for a learned-skill match, got %.3f", score)
	}
}

func TestMetadataVersionTracksLocalSkillFileChanges(t *testing.T) {
	store := newTestStore(t)
	workspaceKey := "device::metadata"
	if got := store.MetadataVersion(workspaceKey); got != "" {
		t.Fatalf("missing skill file should have empty metadata version, got %q", got)
	}
	steps := []Step{{Tool: "computer", Args: map[string]any{"action": "observe"}}}
	recipe, err := store.Record(workspaceKey, "read notifications", "desktop", steps, true)
	if err != nil {
		t.Fatal(err)
	}
	first := store.MetadataVersion(workspaceKey)
	if first == "" {
		t.Fatal("recorded skill file must expose a cheap metadata version")
	}
	time.Sleep(2 * time.Millisecond)
	if _, err := store.Feedback(workspaceKey, recipe.ID, true); err != nil {
		t.Fatal(err)
	}
	if second := store.MetadataVersion(workspaceKey); second == "" || second == first {
		t.Fatalf("skill metadata version must change after durable feedback: first=%q second=%q", first, second)
	}
}

func TestListReturnsWorkspaceScopedLearnedSkills(t *testing.T) {
	store := newTestStore(t)
	steps := []Step{{Tool: "computer", Args: map[string]any{"action": "observe"}}}
	if _, err := store.Record("device::one", "read notifications", "desktop", steps, true); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Record("device::two", "other task", "desktop", steps, true); err != nil {
		t.Fatal(err)
	}
	items, err := store.List("device::one", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].Intent != "read notifications" || items[0].WorkspaceKey != "device::one" {
		t.Fatalf("unexpected scoped learned skill list: %#v", items)
	}
}

func TestRecordRequiresVerifiedExecution(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Record("device::workspace", "open app", "desktop", []Step{{Tool: "computer", Args: map[string]any{"action": "observe"}}}, false)
	if err == nil {
		t.Fatal("unverified execution must not become a learned skill")
	}
}

func TestContextFingerprintStalesAndRevalidatesSkill(t *testing.T) {
	store := newTestStore(t)
	steps := []Step{{Tool: "computer", Args: map[string]any{"action": "observe"}}}
	base := &ContextFingerprint{ProjectID: "project-a", RepositoryIDs: []string{"repo-a"}, RulesHash: "rules-a", DependencyHash: "deps-a", WorkflowFiles: map[string]string{"go.mod": "hash-a"}, BranchPolicy: "any", Branch: "dev"}
	recipe, err := store.RecordWithContext("device::context", "inspect project", "code", steps, true, base)
	if err != nil {
		t.Fatal(err)
	}
	if recipe.ContextHash == "" || recipe.Context == nil {
		t.Fatalf("context fingerprint missing: %#v", recipe)
	}
	matched, err := store.MatchWithContext("device::context", "inspect project", "code", base)
	if err != nil || matched == nil {
		t.Fatalf("same context should match: match=%#v err=%v", matched, err)
	}

	changed := *base
	changed.RulesHash = "rules-b"
	matched, err = store.MatchWithContext("device::context", "inspect project", "code", &changed)
	if err != nil || matched != nil {
		t.Fatalf("material rule change should block replay: match=%#v err=%v", matched, err)
	}
	items, err := store.List("device::context", 10)
	if err != nil || len(items) != 1 || items[0].Status != StatusStale || items[0].StaleReason != "rules_changed" {
		t.Fatalf("skill should persist stale status: %#v err=%v", items, err)
	}
	matched, err = store.MatchWithContext("device::context", "inspect project", "code", base)
	if err != nil || matched == nil || matched.Status != StatusCandidate {
		t.Fatalf("returning to compatible context should revalidate prior status: match=%#v err=%v", matched, err)
	}
}

func TestContextFingerprintBranchPolicyAndLegacyCompatibility(t *testing.T) {
	store := newTestStore(t)
	steps := []Step{{Tool: "computer", Args: map[string]any{"action": "observe"}}}
	exact := &ContextFingerprint{ProjectID: "project", RulesHash: "rules", BranchPolicy: "exact", Branch: "dev"}
	if _, err := store.RecordWithContext("device::branch", "branch workflow", "code", steps, true, exact); err != nil {
		t.Fatal(err)
	}
	other := *exact
	other.Branch = "release"
	if match, err := store.MatchWithContext("device::branch", "branch workflow", "code", &other); err != nil || match != nil {
		t.Fatalf("exact branch mismatch must not replay: match=%#v err=%v", match, err)
	}
	if _, err := store.Record("device::legacy", "legacy workflow", "code", steps, true); err != nil {
		t.Fatal(err)
	}
	if match, err := store.MatchWithContext("device::legacy", "legacy workflow", "code", &other); err != nil || match == nil {
		t.Fatalf("legacy skill without fingerprint must remain compatible: match=%#v err=%v", match, err)
	}
}

func TestRecipesAreWorkspaceScopedAndPrivate(t *testing.T) {
	store := newTestStore(t)
	steps := []Step{{Tool: "browser", Args: map[string]any{"action": "open", "url": "https://example.com"}}, {Tool: "browser", Args: map[string]any{"action": "snapshot"}}}
	if _, err := store.Record("device::one", "open example", "browser", steps, true); err != nil {
		t.Fatal(err)
	}
	if match, err := store.Match("device::two", "open example", "browser"); err != nil || match != nil {
		t.Fatalf("recipe leaked across workspace: match=%#v err=%v", match, err)
	}

	entries, err := os.ReadDir(store.RootDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected one private skill file: entries=%v err=%v", entries, err)
	}
	info, err := entries[0].Info()
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("skill file permissions are not private: %o", info.Mode().Perm())
	}
}

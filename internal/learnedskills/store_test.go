package learnedskills

import (
	"os"
	"path/filepath"
	"testing"
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

func TestRecordRequiresVerifiedExecution(t *testing.T) {
	store := newTestStore(t)
	_, err := store.Record("device::workspace", "open app", "desktop", []Step{{Tool: "computer", Args: map[string]any{"action": "observe"}}}, false)
	if err == nil {
		t.Fatal("unverified execution must not become a learned skill")
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

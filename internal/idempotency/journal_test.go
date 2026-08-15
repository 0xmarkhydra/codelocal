package idempotency

import (
	"path/filepath"
	"testing"
)

func TestJournalPersistsCompletedResult(t *testing.T) {
	file := filepath.Join(t.TempDir(), "journal.json")
	journal := NewAt(file, 10)
	entry, err := journal.Start("request-1", "write_file")
	if err != nil || entry.Status != Started {
		t.Fatalf("start: entry=%#v err=%v", entry, err)
	}
	result := map[string]any{"ok": true, "bytes": float64(42)}
	if err := journal.Complete("request-1", result); err != nil {
		t.Fatalf("complete: %v", err)
	}

	reloaded := NewAt(file, 10)
	stored, err := reloaded.Get("request-1")
	if err != nil || stored == nil || stored.Status != Completed {
		t.Fatalf("reload: entry=%#v err=%v", stored, err)
	}
	value, ok := stored.Result.(map[string]any)
	if !ok || value["ok"] != true || value["bytes"] != float64(42) {
		t.Fatalf("unexpected result: %#v", stored.Result)
	}
}

func TestJournalFailedRequestCanRetry(t *testing.T) {
	journal := NewAt(filepath.Join(t.TempDir(), "journal.json"), 10)
	if _, err := journal.Start("request-1", "git_commit"); err != nil {
		t.Fatal(err)
	}
	if err := journal.Fail("request-1", "temporary failure"); err != nil {
		t.Fatal(err)
	}
	retried, err := journal.Start("request-1", "git_commit")
	if err != nil || retried.Status != Started || retried.Error != "" {
		t.Fatalf("retry: %#v err=%v", retried, err)
	}
}

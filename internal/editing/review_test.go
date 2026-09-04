package editing

import (
	"errors"
	"testing"
)

func TestReviewBatchUsesCASAndPerFileDecision(t *testing.T) {
	batch, err := NewReviewBatch("review-1", "patch-1", []ReviewChange{{Path: "b.go"}, {Path: "a.go"}})
	if err != nil {
		t.Fatal(err)
	}
	if batch.Changes[0].Path != "a.go" || batch.Revision != 1 {
		t.Fatalf("unexpected normalization: %+v", batch)
	}
	updated, err := batch.Decide("a.go", 1, ReviewKeep)
	if err != nil || updated.Revision != 2 {
		t.Fatalf("decision failed: %+v err=%v", updated, err)
	}
	if _, err := updated.Decide("b.go", 1, ReviewRevert); !errors.Is(err, ErrStaleReviewRevision) {
		t.Fatalf("stale UI decision must fail: %v", err)
	}
	updated, err = updated.Decide("b.go", 2, ReviewRevert)
	if err != nil || !updated.Complete() {
		t.Fatalf("review should be complete: %+v err=%v", updated, err)
	}
}

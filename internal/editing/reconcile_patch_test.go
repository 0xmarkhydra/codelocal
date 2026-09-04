package editing

import (
	"context"
	"errors"
	"testing"
)

func TestPatchFileFromReconcileUsesLatestHashFence(t *testing.T) {
	result, err := Reconcile(context.Background(), ReconcileInput{
		Path:   "auth/service.go",
		Base:   []byte("old\n"),
		Latest: []byte("old\n"),
		Agent:  []byte("new\n"),
	})
	if err != nil {
		t.Fatal(err)
	}
	file, err := PatchFileFromReconcile(result)
	if err != nil {
		t.Fatal(err)
	}
	if file.Path != result.Path || file.ExpectedHash != result.LatestHash || file.Content == nil || *file.Content != "new\n" || file.ExpectedAbsent {
		t.Fatalf("patch bridge = %#v", file)
	}
}

func TestPatchFileFromReconcileRejectsConflictAndUserOnly(t *testing.T) {
	for _, result := range []ReconcileResult{
		{Path: "x.go", State: ReconcileConflict, LatestHash: "latest"},
		{Path: "x.go", State: ReconcileUserOnly, LatestHash: "latest", ResultHash: "latest"},
	} {
		if _, err := PatchFileFromReconcile(result); !errors.Is(err, ErrReconcileNotApplicable) {
			t.Fatalf("expected non-applicable reconcile rejection for %#v, got %v", result, err)
		}
	}
}

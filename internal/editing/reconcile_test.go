package editing

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
)

func TestReconcileAgentOnlyProducesGuardedContent(t *testing.T) {
	base := []byte("alpha\nbeta\n")
	latest := append([]byte(nil), base...)
	agent := []byte("alpha\nbeta-agent\n")
	result, err := Reconcile(context.Background(), ReconcileInput{Path: "auth/service.go", Base: base, Latest: latest, Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != ReconcileAgentOnly || !result.CanApply || !bytes.Equal(result.Content, agent) {
		t.Fatalf("agent-only reconcile = %#v", result)
	}
	if result.LatestHash != result.BaseHash || result.ResultHash != result.AgentHash {
		t.Fatalf("unexpected hashes: %#v", result)
	}
}

func TestReconcileUserOnlyDoesNotApplyAgentPatch(t *testing.T) {
	base := []byte("alpha\nbeta\n")
	latest := []byte("alpha-user\nbeta\n")
	result, err := Reconcile(context.Background(), ReconcileInput{Path: "auth/service.go", Base: base, Latest: latest, Agent: append([]byte(nil), base...)})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != ReconcileUserOnly || result.CanApply || len(result.Content) != 0 || result.ResultHash != result.LatestHash {
		t.Fatalf("user-only reconcile = %#v", result)
	}
}

func TestReconcileDisjointTextChangesMergesCleanly(t *testing.T) {
	base := []byte("alpha\nbeta\ngamma\n")
	latest := []byte("alpha-user\nbeta\ngamma\n")
	agent := []byte("alpha\nbeta\ngamma-agent\n")
	result, err := Reconcile(context.Background(), ReconcileInput{Path: "auth/service.go", Base: base, Latest: latest, Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != ReconcileMerged || !result.CanApply {
		t.Fatalf("clean merge = %#v", result)
	}
	text := string(result.Content)
	if !strings.Contains(text, "alpha-user") || !strings.Contains(text, "gamma-agent") || strings.Contains(text, "<<<<<<<") {
		t.Fatalf("unexpected merged content: %q", text)
	}
	if result.ResultHash == "" || result.ResultHash == result.BaseHash {
		t.Fatalf("merged result hash missing: %#v", result)
	}
}

func TestReconcileSameLineConflictNeverAutoApplies(t *testing.T) {
	base := []byte("alpha\nbeta\ngamma\n")
	latest := []byte("alpha\nbeta-user\ngamma\n")
	agent := []byte("alpha\nbeta-agent\ngamma\n")
	result, err := Reconcile(context.Background(), ReconcileInput{Path: "auth/service.go", Base: base, Latest: latest, Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != ReconcileConflict || result.CanApply || len(result.Content) != 0 {
		t.Fatalf("conflict reconcile = %#v", result)
	}
	preview := string(result.ConflictPreview)
	if !strings.Contains(preview, "<<<<<<<") || !strings.Contains(preview, "beta-user") || !strings.Contains(preview, "beta-agent") {
		t.Fatalf("missing conflict preview: %q", preview)
	}
}

func TestReconcileBinaryConflictFailsClosed(t *testing.T) {
	base := []byte{'a', 0, 'b'}
	latest := []byte{'u', 0, 'b'}
	agent := []byte{'a', 0, 'g'}
	result, err := Reconcile(context.Background(), ReconcileInput{Path: "asset.bin", Base: base, Latest: latest, Agent: agent})
	if err != nil {
		t.Fatal(err)
	}
	if result.State != ReconcileConflict || result.CanApply || len(result.Content) != 0 || len(result.ConflictPreview) != 0 {
		t.Fatalf("binary conflict = %#v", result)
	}
}

func TestReconcileRejectsUnsafePathAndOversizedInput(t *testing.T) {
	if _, err := Reconcile(context.Background(), ReconcileInput{Path: "../escape", Base: []byte("a"), Latest: []byte("a"), Agent: []byte("b")}); !errors.Is(err, ErrInvalidReconcileInput) {
		t.Fatalf("expected unsafe path rejection, got %v", err)
	}
	large := make([]byte, maxReconcileFileBytes+1)
	if _, err := Reconcile(context.Background(), ReconcileInput{Path: "large.txt", Base: large, Latest: large, Agent: large}); !errors.Is(err, ErrReconcileTooLarge) {
		t.Fatalf("expected size rejection, got %v", err)
	}
}

func TestReconcileDoesNotMutateInputs(t *testing.T) {
	base := []byte("alpha\nbeta\ngamma\n")
	latest := []byte("alpha-user\nbeta\ngamma\n")
	agent := []byte("alpha\nbeta\ngamma-agent\n")
	baseBefore := append([]byte(nil), base...)
	latestBefore := append([]byte(nil), latest...)
	agentBefore := append([]byte(nil), agent...)
	if _, err := Reconcile(context.Background(), ReconcileInput{Path: "x.go", Base: base, Latest: latest, Agent: agent}); err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(base, baseBefore) || !bytes.Equal(latest, latestBefore) || !bytes.Equal(agent, agentBefore) {
		t.Fatal("reconciliation mutated input buffers")
	}
}

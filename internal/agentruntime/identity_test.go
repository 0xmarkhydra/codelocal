package agentruntime

import (
	"errors"
	"testing"
	"time"
)

func TestNormalizeIdentityDefaultsDurableState(t *testing.T) {
	identity, err := NormalizeIdentity(AgentIdentity{ID: " agent-1 ", TaskID: "task-1", EngineID: " CODEX "})
	if err != nil {
		t.Fatal(err)
	}
	if identity.ID != "agent-1" || identity.EngineID != "codex" || identity.Role != RoleImplementer || identity.Status != AgentCreated || identity.Revision != 1 {
		t.Fatalf("unexpected normalized identity: %+v", identity)
	}
	if identity.CreatedAt.IsZero() || identity.UpdatedAt.IsZero() {
		t.Fatalf("missing durable timestamps: %+v", identity)
	}
}

func TestIdentityCanGoIdleAndReactivate(t *testing.T) {
	identity, err := NormalizeIdentity(AgentIdentity{ID: "agent-1", TaskID: "task-1", EngineID: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	identity, err = TransitionIdentity(identity, AgentActive, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	identity, err = TransitionIdentity(identity, AgentIdle, time.Unix(20, 0))
	if err != nil {
		t.Fatal(err)
	}
	identity, err = TransitionIdentity(identity, AgentActive, time.Unix(30, 0))
	if err != nil {
		t.Fatal(err)
	}
	if identity.Status != AgentActive || identity.Revision != 4 {
		t.Fatalf("continuable identity did not reactivate: %+v", identity)
	}
}

func TestCompletedIdentityCannotReactivate(t *testing.T) {
	identity, _ := NormalizeIdentity(AgentIdentity{ID: "agent-1", TaskID: "task-1", EngineID: "codex"})
	identity, _ = TransitionIdentity(identity, AgentActive, time.Time{})
	identity, _ = TransitionIdentity(identity, AgentCompleted, time.Time{})
	_, err := TransitionIdentity(identity, AgentActive, time.Time{})
	if !errors.Is(err, ErrInvalidAgentTransition) {
		t.Fatalf("expected terminal identity to reject reactivation, got %v", err)
	}
}

func TestActivationLostAllowsNewActivationNotResurrection(t *testing.T) {
	first, err := NormalizeActivation(AgentActivation{ID: "activation-1", AgentID: "agent-1", RuntimeProvider: "local"})
	if err != nil {
		t.Fatal(err)
	}
	first, err = TransitionActivation(first, ActivationRunning, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	first, err = TransitionActivation(first, ActivationLost, time.Unix(20, 0))
	if err != nil {
		t.Fatal(err)
	}
	if first.EndedAt.IsZero() {
		t.Fatalf("lost activation missing end time: %+v", first)
	}
	if _, err := TransitionActivation(first, ActivationRunning, time.Time{}); !errors.Is(err, ErrInvalidActivationTransition) {
		t.Fatalf("lost activation resurrected unexpectedly: %v", err)
	}

	second, err := NormalizeActivation(AgentActivation{ID: "activation-2", AgentID: "agent-1", Attempt: 2, RuntimeProvider: "cloud"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Attempt != 2 || second.Status != ActivationStarting {
		t.Fatalf("new activation not independent: %+v", second)
	}
}

func TestActivationRejectsInvalidIdentity(t *testing.T) {
	_, err := NormalizeActivation(AgentActivation{ID: "activation-1"})
	if !errors.Is(err, ErrInvalidActivation) {
		t.Fatalf("expected invalid activation error, got %v", err)
	}
}

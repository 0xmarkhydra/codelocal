package agentruntime

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestActivationManagerDurableCheckpointAndResumeAttempt(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	graph, err := NewAgentGraph(store, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.Register(AgentIdentity{ID: "agent", TaskID: "task", EngineID: "auto"}); err != nil {
		t.Fatal(err)
	}
	manager, err := NewActivationManager(store, "workspace", "task", graph)
	if err != nil {
		t.Fatal(err)
	}
	first, err := manager.Start(ActivationStart{AgentID: "agent", RuntimeProvider: "local", ProcessID: "pid-1"})
	if err != nil {
		t.Fatal(err)
	}
	if first.Attempt != 1 || first.Revision != 1 || first.Status != ActivationStarting {
		t.Fatalf("first activation = %#v", first)
	}
	if _, err := manager.Start(ActivationStart{AgentID: "agent", RuntimeProvider: "cloud"}); !errors.Is(err, ErrActivationBusy) {
		t.Fatalf("expected live activation fence, got %v", err)
	}
	first, err = manager.TransitionCAS(first.ID, first.Revision, ActivationRunning, time.Unix(10, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.TransitionCAS(first.ID, first.Revision-1, ActivationWaiting, time.Time{}); !errors.Is(err, ErrStaleActivationRevision) {
		t.Fatalf("expected stale activation revision, got %v", err)
	}
	first, err = manager.TransitionCAS(first.ID, first.Revision, ActivationCheckpointed, time.Unix(20, 0))
	if err != nil {
		t.Fatal(err)
	}
	if first.EndedAt.IsZero() || manager.HasLive("agent") {
		t.Fatalf("checkpointed activation should be terminal: %#v", first)
	}

	recoveredGraph, err := NewAgentGraph(store, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	recovered, err := NewActivationManager(store, "workspace", "task", recoveredGraph)
	if err != nil {
		t.Fatal(err)
	}
	latest, ok := recovered.Latest("agent")
	if !ok || latest.ID != first.ID || latest.Status != ActivationCheckpointed || latest.Revision != first.Revision {
		t.Fatalf("recovered activation = %#v ok=%v", latest, ok)
	}
	second, err := recovered.Start(ActivationStart{AgentID: "agent", RuntimeProvider: "cloud", ProviderSessionID: "session-2"})
	if err != nil {
		t.Fatal(err)
	}
	if second.Attempt != 2 || second.ID == first.ID || second.Revision != 1 {
		t.Fatalf("second activation = %#v", second)
	}
	all := recovered.ForAgent("agent")
	if len(all) != 2 || all[0].Attempt != 1 || all[1].Attempt != 2 {
		t.Fatalf("activation history = %#v", all)
	}
}

func TestActivationManagerRejectsTerminalAgentAndCorruptReplay(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	graph, err := NewAgentGraph(store, "workspace", "terminal-task")
	if err != nil {
		t.Fatal(err)
	}
	agent, err := graph.Register(AgentIdentity{ID: "terminal", TaskID: "terminal-task", EngineID: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	agent, err = graph.TransitionCAS(agent.ID, agent.Revision, AgentActive, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	agent, err = graph.TransitionCAS(agent.ID, agent.Revision, AgentCompleted, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	manager, err := NewActivationManager(store, "workspace", "terminal-task", graph)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Start(ActivationStart{AgentID: agent.ID, RuntimeProvider: "local"}); !errors.Is(err, ErrActivationAgentTerminal) {
		t.Fatalf("terminal agent started activation: %v", err)
	}

	corruptStore := runtimeevents.NewStore(filepath.Join(t.TempDir(), "corrupt-events"))
	corruptGraph, err := NewAgentGraph(corruptStore, "workspace", "corrupt-task")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := corruptGraph.Register(AgentIdentity{ID: "agent", TaskID: "corrupt-task", EngineID: "auto"}); err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	bad := AgentActivation{ID: "activation:agent:2", AgentID: "agent", Attempt: 2, RuntimeProvider: "local", Status: ActivationStarting, Revision: 1, StartedAt: now, UpdatedAt: now}
	if _, _, err := corruptStore.Append("workspace", "corrupt-task", runtimeevents.Event{Type: eventActivationStarted, TaskID: "corrupt-task", AgentID: "agent", ActivationID: bad.ID, IdempotencyKey: "bad-start", Payload: map[string]any{"activation": graphPayload(bad)}}); err != nil {
		t.Fatal(err)
	}
	if _, err := NewActivationManager(corruptStore, "workspace", "corrupt-task", corruptGraph); !errors.Is(err, ErrInvalidActivationManager) {
		t.Fatalf("expected corrupt activation replay rejection, got %v", err)
	}
}

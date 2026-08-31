package agentruntime

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestAgentGraphRejectsCycleAndRecovers(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	graph, err := NewAgentGraph(store, "workspace", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	root, err := graph.Register(AgentIdentity{ID: "lead", TaskID: "task-1", Role: RoleLead, EngineID: "auto"})
	if err != nil {
		t.Fatal(err)
	}
	child, edge, err := graph.Spawn(root.ID, AgentIdentity{ID: "worker", TaskID: "task-1", Role: RoleImplementer, EngineID: "auto"}, EdgeSpawn)
	if err != nil {
		t.Fatal(err)
	}
	if edge.ParentAgentID != root.ID || child.ParentAgentID != root.ID {
		t.Fatalf("unexpected spawn lineage: %#v %#v", child, edge)
	}
	if _, err := graph.Link(child.ID, root.ID, EdgeDependency); !errors.Is(err, ErrAgentCycle) {
		t.Fatalf("expected cycle rejection, got %v", err)
	}
	if graph.JoinReady(root.ID) {
		t.Fatal("join should wait for active child")
	}
	if _, err := graph.Transition(child.ID, AgentCompleted, root.UpdatedAt); err != nil {
		t.Fatal(err)
	}
	if !graph.JoinReady(root.ID) {
		t.Fatal("join should be ready after child completes")
	}

	recovered, err := NewAgentGraph(store, "workspace", "task-1")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := recovered.Agent(child.ID)
	if !ok || got.Status != AgentCompleted {
		t.Fatalf("recovered child = %#v, ok=%v", got, ok)
	}
	if len(recovered.Descendants(root.ID)) != 1 {
		t.Fatalf("recovered descendants = %#v", recovered.Descendants(root.ID))
	}
}

func TestAgentGraphCancelSubtreeIsDurable(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	graph, err := NewAgentGraph(store, "workspace", "task-2")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := graph.Register(AgentIdentity{ID: "lead", TaskID: "task-2", Role: RoleLead, EngineID: "auto"}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := graph.Spawn("lead", AgentIdentity{ID: "child", TaskID: "task-2", EngineID: "auto"}, EdgeSpawn); err != nil {
		t.Fatal(err)
	}
	if _, _, err := graph.Spawn("child", AgentIdentity{ID: "grandchild", TaskID: "task-2", EngineID: "auto"}, EdgeDelegate); err != nil {
		t.Fatal(err)
	}
	cancelled, err := graph.CancelSubtree("child", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(cancelled) != 2 {
		t.Fatalf("cancelled = %#v", cancelled)
	}
	recovered, err := NewAgentGraph(store, "workspace", "task-2")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"child", "grandchild"} {
		identity, ok := recovered.Agent(id)
		if !ok || identity.Status != AgentCancelled {
			t.Fatalf("%s = %#v, ok=%v", id, identity, ok)
		}
	}
}

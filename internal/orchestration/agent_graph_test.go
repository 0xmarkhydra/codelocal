package orchestration

import "testing"

func TestAgentGraphAddAndLink(t *testing.T) {
	g, err := NewAgentGraph(nil, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := g.AddNode(AgentGraphNode{ID: "planner", AgentID: "agent-a", Role: "planner"}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.AddNode(AgentGraphNode{ID: "worker", AgentID: "agent-b", ParentID: "planner"}); err != nil {
		t.Fatal(err)
	}
	edge, err := g.AddEdge(AgentGraphEdge{ID: "e1", FromID: "planner", ToID: "worker", Kind: "delegates"})
	if err != nil {
		t.Fatal(err)
	}
	if edge.Revision != 1 {
		t.Fatalf("edge revision = %d, want 1", edge.Revision)
	}
	if downstream := g.Downstream("planner"); len(downstream) != 1 {
		t.Fatalf("downstream has %d edges, want 1", len(downstream))
	}
	node, err := g.SetNodeStatus("worker", 1, AgentNodeRunning)
	if err != nil {
		t.Fatal(err)
	}
	if node.Revision != 2 || node.Status != AgentNodeRunning {
		t.Fatalf("unexpected node state: %+v", node)
	}
}

func TestAgentGraphRejectsCycleAndDuplicates(t *testing.T) {
	g, _ := NewAgentGraph(nil, "workspace", "task")
	if _, err := g.AddNode(AgentGraphNode{ID: "a", AgentID: "agent-a"}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.AddNode(AgentGraphNode{ID: "b", AgentID: "agent-b"}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.AddNode(AgentGraphNode{ID: "a", AgentID: "agent-a"}); err == nil {
		t.Fatal("duplicate node accepted")
	}
	if _, err := g.AddEdge(AgentGraphEdge{ID: "e1", FromID: "a", ToID: "b", Kind: "channel"}); err != nil {
		t.Fatal(err)
	}
	if _, err := g.AddEdge(AgentGraphEdge{ID: "e2", FromID: "b", ToID: "a", Kind: "channel"}); err == nil {
		t.Fatal("cycle accepted")
	}
	if _, err := g.AddEdge(AgentGraphEdge{ID: "e3", FromID: "a", ToID: "ghost", Kind: "channel"}); err == nil {
		t.Fatal("edge to missing node accepted")
	}
	if _, err := g.SetNodeStatus("a", 99, AgentNodeCompleted); err == nil {
		t.Fatal("stale revision accepted")
	}
	if err := g.RemoveEdge("e1"); err != nil {
		t.Fatal(err)
	}
	if len(g.Downstream("a")) != 0 {
		t.Fatal("removed edge still listed")
	}
}

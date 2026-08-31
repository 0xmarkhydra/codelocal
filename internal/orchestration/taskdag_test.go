package orchestration

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestTaskDAGBlocksUntilDependencyCompletesAndRecovers(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	dag, err := NewTaskDAG(store, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	root, err := dag.Add(TaskNode{ID: "investigate", Subject: "Find root cause", OwnerAgentID: "investigator"})
	if err != nil {
		t.Fatal(err)
	}
	child, err := dag.Add(TaskNode{ID: "implement", Subject: "Implement fix", OwnerAgentID: "implementer", BlockedBy: []string{root.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dag.SetStatus(child.ID, child.Revision, TaskNodeRunning); !errors.Is(err, ErrTaskNodeBlocked) {
		t.Fatalf("expected blocked node, got %v", err)
	}
	root, err = dag.SetStatus(root.ID, root.Revision, TaskNodeRunning)
	if err != nil {
		t.Fatal(err)
	}
	root, err = dag.SetStatus(root.ID, root.Revision, TaskNodeCompleted)
	if err != nil {
		t.Fatal(err)
	}
	runnable := dag.Runnable()
	if len(runnable) != 1 || runnable[0].ID != child.ID {
		t.Fatalf("runnable = %#v", runnable)
	}

	recovered, err := NewTaskDAG(store, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := recovered.Node(root.ID)
	if !ok || got.Status != TaskNodeCompleted {
		t.Fatalf("recovered root = %#v, ok=%v", got, ok)
	}
	if len(recovered.Runnable()) != 1 || recovered.Runnable()[0].ID != child.ID {
		t.Fatalf("recovered runnable = %#v", recovered.Runnable())
	}
}

func TestTaskDAGUsesCASForOwnerAndDependencies(t *testing.T) {
	dag, err := NewTaskDAG(nil, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	a, err := dag.Add(TaskNode{ID: "a", Subject: "A"})
	if err != nil {
		t.Fatal(err)
	}
	b, err := dag.Add(TaskNode{ID: "b", Subject: "B", BlockedBy: []string{a.ID}})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := dag.AssignOwner(b.ID, b.Revision, "agent-1")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := dag.AssignOwner(b.ID, b.Revision, "stale-agent"); !errors.Is(err, ErrStaleTaskNode) {
		t.Fatalf("expected stale owner update, got %v", err)
	}
	if _, err := dag.SetDependencies(a.ID, a.Revision, []string{b.ID}); !errors.Is(err, ErrTaskDAGCycle) {
		t.Fatalf("expected cycle rejection, got %v", err)
	}
	got, _ := dag.Node(b.ID)
	if got.OwnerAgentID != "agent-1" || got.Revision != updated.Revision {
		t.Fatalf("node after stale write = %#v", got)
	}
}

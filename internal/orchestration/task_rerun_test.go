package orchestration

import "testing"

func rerunTestDAG(t *testing.T) *TaskDAG {
	t.Helper()
	dag, err := NewTaskDAG(nil, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range []TaskNode{{ID: "root", Subject: "root"}, {ID: "mid", Subject: "mid", BlockedBy: []string{"root"}}, {ID: "leaf", Subject: "leaf", BlockedBy: []string{"mid"}}} {
		if _, err := dag.Add(node); err != nil {
			t.Fatal(err)
		}
	}
	return dag
}

func failRoot(t *testing.T, dag *TaskDAG) {
	t.Helper()
	if _, err := dag.SetStatus("root", 1, TaskNodeRunning); err != nil {
		t.Fatal(err)
	}
	if _, err := dag.SetStatus("root", 2, TaskNodeFailed); err != nil {
		t.Fatal(err)
	}
}

func TestPlanBranchRerunResetsFailedBranch(t *testing.T) {
	dag, err := NewTaskDAG(nil, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range []TaskNode{{ID: "root", Subject: "root"}, {ID: "mid", Subject: "mid"}, {ID: "leaf", Subject: "leaf", BlockedBy: []string{"mid"}}} {
		if _, err := dag.Add(node); err != nil {
			t.Fatal(err)
		}
	}
	for _, id := range []string{"root", "mid"} {
		node, _ := dag.Node(id)
		if _, err := dag.SetStatus(id, node.Revision, TaskNodeRunning); err != nil {
			t.Fatal(err)
		}
		node, _ = dag.Node(id)
		if _, err := dag.SetStatus(id, node.Revision, TaskNodeFailed); err != nil {
			t.Fatal(err)
		}
	}
	mid, _ := dag.Node("mid")
	if _, err := dag.SetDependencies("mid", mid.Revision, []string{"root"}); err != nil {
		t.Fatal(err)
	}
	plan, err := PlanBranchRerun(dag, "root", "ev-1")
	if err != nil {
		t.Fatal(err)
	}
	if plan.RootID != "root" || plan.EvidenceID != "ev-1" || plan.AttemptID == "" {
		t.Fatalf("plan metadata mismatch: %+v", plan)
	}
	if len(plan.Resets) != 2 || plan.Resets[0].ID != "mid" || plan.Resets[1].ID != "root" {
		t.Fatalf("unexpected resets: %+v", plan.Resets)
	}
	if err := ApplyBranchRerun(dag, plan); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"root", "mid"} {
		node, _ := dag.Node(id)
		if node.Status != TaskNodePending {
			t.Fatalf("node %s status = %s, want pending", id, node.Status)
		}
	}
	if runnable := dag.Runnable(); len(runnable) != 1 || runnable[0].ID != "root" {
		t.Fatalf("unexpected runnable set: %+v", runnable)
	}
}
func TestPlanBranchRerunRejectsNonFailedRoot(t *testing.T) {
	dag := rerunTestDAG(t)
	if _, err := PlanBranchRerun(dag, "root", ""); err == nil {
		t.Fatal("pending root accepted for rerun")
	}
	if _, err := PlanBranchRerun(dag, "ghost", ""); err == nil {
		t.Fatal("missing root accepted for rerun")
	}
}

func TestPlanBranchRerunBlockedByActiveDescendant(t *testing.T) {
	dag, err := NewTaskDAG(nil, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	for _, node := range []TaskNode{{ID: "root", Subject: "root"}, {ID: "mid", Subject: "mid"}} {
		if _, err := dag.Add(node); err != nil {
			t.Fatal(err)
		}
	}
	mid, _ := dag.Node("mid")
	if _, err := dag.SetStatus("mid", mid.Revision, TaskNodeRunning); err != nil {
		t.Fatal(err)
	}
	mid, _ = dag.Node("mid")
	if _, err := dag.SetDependencies("mid", mid.Revision, []string{"root"}); err != nil {
		t.Fatal(err)
	}
	failRoot(t, dag)
	if _, err := PlanBranchRerun(dag, "root", ""); err == nil {
		t.Fatal("rerun past running descendant accepted")
	}
}

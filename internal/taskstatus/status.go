package taskstatus

import (
	"time"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/taskexecution"
	"github.com/0xmarkhydra/codelocal/internal/usage"
)

// LaneStatus is the user-facing slice of one verification lane.
type LaneStatus struct {
	Lane    string `json:"lane"`
	Ready   bool   `json:"ready"`
	Pending int    `json:"pending"`
	Failed  int    `json:"failed"`
}

// TaskStatusSnapshot is the single read model behind the usage/cost
// dashboard (plan P2) and the user task view (plan Q0). It aggregates only;
// it never mutates bundle, budget or verification state.
type TaskStatusSnapshot struct {
	TaskID            string                          `json:"taskId"`
	WorkspaceKey      string                          `json:"workspaceKey"`
	State             taskexecution.State             `json:"state"`
	Generation        taskexecution.RuntimeGeneration `json:"generation,omitempty"`
	BudgetState       usage.BudgetState               `json:"budgetState"`
	BudgetReason      string                          `json:"budgetReason,omitempty"`
	RemainingTotal    int64                           `json:"remainingTotal,omitempty"`
	RemainingCost     int64                           `json:"remainingCost,omitempty"`
	VerificationReady bool                            `json:"verificationReady"`
	Lanes             []LaneStatus                    `json:"lanes"`
	Bindings          int                             `json:"bindings"`
	Healthy           bool                            `json:"healthy"`
	ExportedAt        time.Time                       `json:"exportedAt"`
}

// SummarizeTask folds bundle, budget decision and verification graph into
// one snapshot. Healthy means the budget is not hard-limited and every
// required verification passed; anything else needs operator attention.
func SummarizeTask(bundle taskexecution.Bundle, budget usage.BudgetDecision, graph orchestration.VerificationGraph) TaskStatusSnapshot {
	snapshot := TaskStatusSnapshot{
		TaskID: bundle.TaskID, WorkspaceKey: bundle.WorkspaceKey, State: bundle.State,
		Generation: bundle.RuntimeGeneration, BudgetState: budget.State, BudgetReason: budget.Reason,
		RemainingTotal: budget.RemainingTotal, RemainingCost: budget.RemainingCost,
		VerificationReady: graph.Ready, Bindings: len(bundle.RepositoryBindings),
		Lanes: []LaneStatus{}, ExportedAt: time.Now().UTC(),
	}
	for _, lane := range graph.Lanes {
		snapshot.Lanes = append(snapshot.Lanes, LaneStatus{Lane: lane.Lane, Ready: lane.Ready, Pending: len(lane.Pending), Failed: len(lane.Failed)})
	}
	snapshot.Healthy = budget.State != usage.BudgetHard && graph.Ready
	return snapshot
}

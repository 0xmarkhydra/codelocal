package orchestration

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrBranchRerunNotFailed = errors.New("branch rerun root is not failed")
	ErrBranchRerunBlocked   = errors.New("branch rerun blocked by active descendant")
)

type RerunNodeReset struct {
	ID               string         "id"
	ExpectedRevision uint64         "expectedRevision"
	FromStatus       TaskNodeStatus "fromStatus"
}

type RerunPlan struct {
	RootID     string           "rootId"
	AttemptID  string           "attemptId"
	EvidenceID string           "evidenceId,omitempty"
	Resets     []RerunNodeReset "resets"
}

// PlanBranchRerun computes a single-use plan that returns every failed node
// in a branch to pending so it can run again. Only failed nodes are reset:
// pending descendants already rejoin once blockers rerun, while any other
// descendant state means the branch cannot legally converge again, so the plan
// refuses instead of leaving it permanently stuck. Plans are applied with
// ApplyBranchRerun, which reuses the CAS-guarded SetStatus path.
func PlanBranchRerun(dag *TaskDAG, rootID, evidenceID string) (RerunPlan, error) {
	if dag == nil {
		return RerunPlan{}, ErrInvalidTaskDAG
	}
	rootID = strings.TrimSpace(rootID)
	root, ok := dag.Node(rootID)
	if !ok {
		return RerunPlan{}, ErrTaskNodeMissing
	}
	if root.Status != TaskNodeFailed {
		return RerunPlan{}, ErrBranchRerunNotFailed
	}
	byID := map[string]TaskNode{}
	for _, node := range dag.Nodes() {
		byID[node.ID] = node
	}
	affected := map[string]TaskNode{root.ID: root}
	changed := true
	for changed {
		changed = false
		for _, node := range byID {
			if _, ok := affected[node.ID]; ok {
				continue
			}
			for _, blocker := range node.BlockedBy {
				if _, ok := affected[blocker]; ok {
					affected[node.ID] = node
					changed = true
					break
				}
			}
		}
	}
	plan := RerunPlan{
		RootID:     root.ID,
		AttemptID:  fmt.Sprintf("rerun_%s_%d", root.ID, time.Now().UTC().UnixNano()),
		EvidenceID: strings.TrimSpace(evidenceID),
		Resets:     []RerunNodeReset{},
	}
	for _, node := range affected {
		switch node.Status {
		case TaskNodeFailed:
			plan.Resets = append(plan.Resets, RerunNodeReset{ID: node.ID, ExpectedRevision: node.Revision, FromStatus: node.Status})
		case TaskNodePending:
			// Already runnable once blockers reset; nothing to rewrite.
		default:
			// Running, completed and cancelled descendants can never rejoin a
			// retried branch through the legal transition table, so the plan
			// refuses rather than leaving the branch permanently stuck.
			return RerunPlan{}, fmt.Errorf("%w: descendant %s is %s", ErrBranchRerunBlocked, node.ID, node.Status)
		}
	}
	sort.Slice(plan.Resets, func(i, j int) bool { return plan.Resets[i].ID < plan.Resets[j].ID })
	return plan, nil
}

// ApplyBranchRerun resets every node in the plan to pending. A stale
// ExpectedRevision aborts the whole apply on the first mismatch, leaving
// later nodes untouched; callers re-plan from the fresh DAG state.
func ApplyBranchRerun(dag *TaskDAG, plan RerunPlan) error {
	if dag == nil || strings.TrimSpace(plan.RootID) == "" || len(plan.Resets) == 0 {
		return ErrInvalidTaskDAG
	}
	for _, reset := range plan.Resets {
		if _, err := dag.SetStatus(reset.ID, reset.ExpectedRevision, TaskNodePending); err != nil {
			return err
		}
	}
	return nil
}

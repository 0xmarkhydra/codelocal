package editing

import (
	"context"
	"errors"
	"strings"
)

type GuardedDelete struct {
	Path         string `json:"path"`
	ExpectedHash string `json:"expectedHash"`
}

type SavepointConflict struct {
	Path        string `json:"path"`
	Reason      string `json:"reason"`
	CurrentHash string `json:"currentHash,omitempty"`
}

type RewindPlan struct {
	SavepointID string              `json:"savepointId"`
	Revision    uint64              `json:"revision"`
	Patch       PatchSet            `json:"patch"`
	Deletes     []GuardedDelete     `json:"deletes,omitempty"`
	Conflicts   []SavepointConflict `json:"conflicts,omitempty"`
	NoopPaths   []string            `json:"noopPaths,omitempty"`
	CanApply    bool                `json:"canApply"`
}

var ErrSavepointConflict = errors.New("savepoint rewind conflicts with current user state")

// PlanRewind creates a no-write rewind plan. Existing files use reverse
// three-way reconciliation (agent-after as base, current workspace as latest,
// before-savepoint as desired result). Agent-created files are represented as
// guarded deletes. Any overlap becomes structured conflict instead of overwrite.
func (s *SavepointStore) PlanRewind(ctx context.Context, workspaceKey, id string, expectedRevision uint64) (RewindPlan, error) {
	point, found, err := s.Load(workspaceKey, id)
	if err != nil {
		return RewindPlan{}, err
	}
	if !found {
		return RewindPlan{}, ErrSavepointMissing
	}
	if expectedRevision == 0 || point.Revision != expectedRevision {
		return RewindPlan{}, ErrSavepointStale
	}
	if point.State != SavepointSealed {
		return RewindPlan{}, ErrSavepointState
	}
	plan := RewindPlan{
		SavepointID: point.ID,
		Revision:    point.Revision,
		Deletes:     []GuardedDelete{},
		Conflicts:   []SavepointConflict{},
		NoopPaths:   []string{},
		Patch: PatchSet{
			ID: "rewind:" + point.ID,
			Provenance: PatchProvenance{
				TaskID:       point.Provenance.TaskID,
				AgentID:      point.Provenance.AgentID,
				ActivationID: point.Provenance.ActivationID,
				TraceID:      point.Provenance.TraceID,
			},
			State: PatchProposed,
			Files: []PatchFile{},
		},
	}
	for _, entry := range point.Entries {
		if sameSavepointState(entry) {
			plan.NoopPaths = append(plan.NoopPaths, entry.Path)
			continue
		}
		current, _, currentExists, readErr := s.readWorkspace(entry.Path)
		if readErr != nil {
			return RewindPlan{}, readErr
		}
		currentHash := ""
		if currentExists {
			currentHash = reconcileHash(current)
		}
		switch {
		case entry.ExistedBefore && entry.ExistsAfter:
			if !currentExists {
				plan.Conflicts = append(plan.Conflicts, SavepointConflict{Path: entry.Path, Reason: "current_file_missing"})
				continue
			}
			if currentHash == entry.BeforeHash {
				plan.NoopPaths = append(plan.NoopPaths, entry.Path)
				continue
			}
			before, err := s.loadBlob(point.WorkspaceKey, entry.BeforeBlob)
			if err != nil {
				return RewindPlan{}, err
			}
			after, err := s.loadBlob(point.WorkspaceKey, entry.AfterBlob)
			if err != nil {
				return RewindPlan{}, err
			}
			reconciled, err := Reconcile(ctx, ReconcileInput{Path: entry.Path, Base: after, Latest: current, Agent: before})
			if err != nil {
				return RewindPlan{}, err
			}
			if reconciled.State == ReconcileConflict {
				plan.Conflicts = append(plan.Conflicts, SavepointConflict{Path: entry.Path, Reason: "overlapping_user_change", CurrentHash: currentHash})
				continue
			}
			if reconciled.CanApply {
				file, err := PatchFileFromReconcile(reconciled)
				if err != nil {
					return RewindPlan{}, err
				}
				plan.Patch.Files = append(plan.Patch.Files, file)
			} else {
				plan.NoopPaths = append(plan.NoopPaths, entry.Path)
			}
		case entry.ExistedBefore && !entry.ExistsAfter:
			if currentExists {
				if currentHash == entry.BeforeHash {
					plan.NoopPaths = append(plan.NoopPaths, entry.Path)
				} else {
					plan.Conflicts = append(plan.Conflicts, SavepointConflict{Path: entry.Path, Reason: "path_recreated_after_agent_delete", CurrentHash: currentHash})
				}
				continue
			}
			before, err := s.loadBlob(point.WorkspaceKey, entry.BeforeBlob)
			if err != nil {
				return RewindPlan{}, err
			}
			content := string(before)
			plan.Patch.Files = append(plan.Patch.Files, PatchFile{Path: entry.Path, ExpectedAbsent: true, Content: &content})
		case !entry.ExistedBefore && entry.ExistsAfter:
			if !currentExists {
				plan.NoopPaths = append(plan.NoopPaths, entry.Path)
				continue
			}
			if currentHash != entry.AfterHash {
				plan.Conflicts = append(plan.Conflicts, SavepointConflict{Path: entry.Path, Reason: "agent_created_file_changed_by_user", CurrentHash: currentHash})
				continue
			}
			plan.Deletes = append(plan.Deletes, GuardedDelete{Path: entry.Path, ExpectedHash: currentHash})
		default:
			plan.NoopPaths = append(plan.NoopPaths, entry.Path)
		}
	}
	plan.CanApply = len(plan.Conflicts) == 0 && (len(plan.Patch.Files) > 0 || len(plan.Deletes) > 0)
	return plan, nil
}

func sameSavepointState(entry SavepointEntry) bool {
	if entry.ExistedBefore != entry.ExistsAfter {
		return false
	}
	if !entry.ExistedBefore {
		return true
	}
	return strings.TrimSpace(entry.BeforeHash) != "" && entry.BeforeHash == entry.AfterHash
}

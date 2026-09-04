package orchestration

import "strings"

// ProjectOSExecutionBinding enforces the P0 invariant:
// Task ID -> one active execution bundle/worktree unless explicitly forked/recovered.
// The existing taskexecution worktree provider remains the mechanism; this type only
// carries the policy decision so callers cannot create a parallel worktree stack.
type ProjectOSExecutionBinding struct {
	ProjectTaskID   string `json:"projectTaskId"`
	ExecutionTaskID string `json:"executionTaskId"`
	WorkspaceKey    string `json:"workspaceKey"`
	Forked          bool   `json:"forked,omitempty"`
	Recovered       bool   `json:"recovered,omitempty"`
}

func NewProjectOSExecutionBinding(projectTaskID, executionTaskID, workspaceKey string) (ProjectOSExecutionBinding, bool) {
	projectTaskID = strings.TrimSpace(projectTaskID)
	executionTaskID = strings.TrimSpace(executionTaskID)
	workspaceKey = strings.TrimSpace(workspaceKey)
	if projectTaskID == "" || executionTaskID == "" || workspaceKey == "" {
		return ProjectOSExecutionBinding{}, false
	}
	return ProjectOSExecutionBinding{
		ProjectTaskID: projectTaskID, ExecutionTaskID: executionTaskID, WorkspaceKey: workspaceKey,
	}, true
}

// ProjectOSRecoveryPolicy is the P0 autonomous recovery sequence.
type ProjectOSRecoveryDecision string

const (
	ProjectOSRetryOnce     ProjectOSRecoveryDecision = "retry_once"
	ProjectOSRepairSubtask ProjectOSRecoveryDecision = "repair_subtask"
	ProjectOSWaitingHuman  ProjectOSRecoveryDecision = "waiting_human"
	ProjectOSDoNotRetry    ProjectOSRecoveryDecision = "do_not_retry"
)

type ProjectOSFailure struct {
	Kind          string `json:"kind"`
	RetryCount    int    `json:"retryCount"`
	PermissionErr bool   `json:"permissionError"`
	Duplicate     bool   `json:"duplicateDelivery"`
}

func DecideProjectOSRecovery(f ProjectOSFailure) ProjectOSRecoveryDecision {
	// No duplicate irreversible side effects: duplicate deliveries never retry.
	if f.Duplicate {
		return ProjectOSDoNotRetry
	}
	// Permission failures are never silently retried.
	if f.PermissionErr {
		return ProjectOSWaitingHuman
	}
	switch strings.ToLower(strings.TrimSpace(f.Kind)) {
	case "permission", "auth", "policy":
		return ProjectOSWaitingHuman
	case "transient", "timeout", "execution_failed":
		if f.RetryCount < 1 {
			return ProjectOSRetryOnce
		}
		return ProjectOSRepairSubtask
	default:
		if f.RetryCount < 1 {
			return ProjectOSRetryOnce
		}
		if f.RetryCount < 2 {
			return ProjectOSRepairSubtask
		}
		return ProjectOSWaitingHuman
	}
}

// ShouldEscalateToHuman reports whether a task needs a Needs You entry.
// Routine recoverable technical failures stay hidden; only material decisions surface.
func ShouldEscalateToHuman(taskStatus string, retryCount int, blockedReason string) bool {
	switch taskStatus {
	case "WAITING_HUMAN", "BLOCKED", "FAILED":
		return true
	case "RETRYING":
		return retryCount >= 2
	default:
		return strings.TrimSpace(blockedReason) != "" && taskStatus == "WAITING_EXTERNAL"
	}
}

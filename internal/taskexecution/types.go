package taskexecution

import "time"

type Provider string

const ProviderLocalWorktree Provider = "local_worktree"

type State string

const (
	StatePreparing State = "preparing"
	StateReady     State = "ready"
	StateRunning   State = "running"
	StateReview    State = "review"
	StateBlocked   State = "blocked"
	StateCompleted State = "completed"
	StateCancelled State = "cancelled"
)

type RepositoryBinding struct {
	RepositoryID            string `json:"repositoryId"`
	RepositoryPath          string `json:"repositoryPath"`
	SourceRevision          string `json:"sourceRevision"`
	SourceDirty             bool   `json:"sourceDirty,omitempty"`
	SourceStatusFingerprint string `json:"sourceStatusFingerprint,omitempty"`
	BranchName              string `json:"branchName,omitempty"`
	BindingID               string `json:"bindingId"`
	LocalPath               string `json:"localPath"`
}

type Lease struct {
	OwnerID    string    `json:"ownerId,omitempty"`
	Generation uint64    `json:"generation,omitempty"`
	ExpiresAt  time.Time `json:"expiresAt,omitempty"`
}

type Bundle struct {
	SchemaVersion      int                 `json:"schemaVersion"`
	ID                 string              `json:"id"`
	Revision           uint64              `json:"revision"`
	TaskID             string              `json:"taskId"`
	ProjectID          string              `json:"projectId"`
	WorkspaceID        string              `json:"workspaceId"`
	WorkspaceKey       string              `json:"workspaceKey"`
	Provider           Provider            `json:"provider"`
	State              State               `json:"state"`
	RepositoryBindings []RepositoryBinding `json:"repositoryBindings"`
	Lease              Lease               `json:"lease,omitempty"`
	CreatedAt          time.Time           `json:"createdAt"`
	UpdatedAt          time.Time           `json:"updatedAt"`
}

func (b Bundle) Clone() Bundle {
	b.RepositoryBindings = append([]RepositoryBinding(nil), b.RepositoryBindings...)
	return b
}

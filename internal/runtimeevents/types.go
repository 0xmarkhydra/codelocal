package runtimeevents

import (
	"errors"
	"strings"
	"time"
)

const SchemaVersion = 1

var (
	ErrInvalidEvent = errors.New("invalid runtime event")
	ErrCorruptLog   = errors.New("runtime event log is corrupt")
)

type Event struct {
	SchemaVersion  int            `json:"schemaVersion"`
	Sequence       uint64         `json:"sequence"`
	ID             string         `json:"id"`
	Type           string         `json:"type"`
	Timestamp      time.Time      `json:"timestamp"`
	SessionID      string         `json:"sessionId,omitempty"`
	TaskID         string         `json:"taskId"`
	AgentID        string         `json:"agentId,omitempty"`
	ActivationID   string         `json:"activationId,omitempty"`
	WorktreeID     string         `json:"worktreeId,omitempty"`
	PatchID        string         `json:"patchId,omitempty"`
	TraceID        string         `json:"traceId,omitempty"`
	IdempotencyKey string         `json:"idempotencyKey,omitempty"`
	Payload        map[string]any `json:"payload,omitempty"`
}

type Snapshot struct {
	SchemaVersion int            `json:"schemaVersion"`
	TaskID        string         `json:"taskId"`
	Sequence      uint64         `json:"sequence"`
	CreatedAt     time.Time      `json:"createdAt"`
	State         map[string]any `json:"state"`
}

func normalizeEvent(event Event, taskID string) (Event, error) {
	taskID = strings.TrimSpace(taskID)
	event.TaskID = strings.TrimSpace(event.TaskID)
	event.Type = strings.TrimSpace(event.Type)
	event.ID = strings.TrimSpace(event.ID)
	event.SessionID = strings.TrimSpace(event.SessionID)
	event.AgentID = strings.TrimSpace(event.AgentID)
	event.ActivationID = strings.TrimSpace(event.ActivationID)
	event.WorktreeID = strings.TrimSpace(event.WorktreeID)
	event.PatchID = strings.TrimSpace(event.PatchID)
	event.TraceID = strings.TrimSpace(event.TraceID)
	event.IdempotencyKey = strings.TrimSpace(event.IdempotencyKey)

	if taskID == "" || event.Type == "" {
		return Event{}, ErrInvalidEvent
	}
	if event.TaskID == "" {
		event.TaskID = taskID
	}
	if event.TaskID != taskID {
		return Event{}, ErrInvalidEvent
	}
	if event.SchemaVersion == 0 {
		event.SchemaVersion = SchemaVersion
	}
	if event.SchemaVersion != SchemaVersion {
		return Event{}, ErrInvalidEvent
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now().UTC()
	} else {
		event.Timestamp = event.Timestamp.UTC()
	}
	return event, nil
}

func normalizeSnapshot(snapshot Snapshot, taskID string) (Snapshot, error) {
	taskID = strings.TrimSpace(taskID)
	snapshot.TaskID = strings.TrimSpace(snapshot.TaskID)
	if taskID == "" {
		return Snapshot{}, ErrInvalidEvent
	}
	if snapshot.TaskID == "" {
		snapshot.TaskID = taskID
	}
	if snapshot.TaskID != taskID {
		return Snapshot{}, ErrInvalidEvent
	}
	if snapshot.SchemaVersion == 0 {
		snapshot.SchemaVersion = SchemaVersion
	}
	if snapshot.SchemaVersion != SchemaVersion {
		return Snapshot{}, ErrInvalidEvent
	}
	if snapshot.CreatedAt.IsZero() {
		snapshot.CreatedAt = time.Now().UTC()
	} else {
		snapshot.CreatedAt = snapshot.CreatedAt.UTC()
	}
	if snapshot.State == nil {
		snapshot.State = map[string]any{}
	}
	return snapshot, nil
}

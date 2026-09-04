package taskexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

var ErrInvalidTaskResume = errors.New("invalid task resume snapshot")

// TaskResumeSnapshot carries a task bundle across runtimes (local to cloud,
// machine to machine) together with the event-log high-water mark the bundle
// was exported at. The digest binds bundle and high-water mark: a receiver
// that replays fewer events than the mark, or a different bundle, fails
// closed instead of resuming into a split-brain execution.
type TaskResumeSnapshot struct {
	SchemaVersion  int       "schemaVersion"
	TaskID         string    "taskId"
	WorkspaceKey   string    "workspaceKey"
	Bundle         Bundle    "bundle"
	EventHighWater uint64    "eventHighWater"
	ExportedAt     time.Time "exportedAt"
	Digest         string    "digest"
}

func ExportTaskResume(bundle Bundle, eventHighWater uint64) (TaskResumeSnapshot, error) {
	bundle.WorkspaceKey = strings.TrimSpace(bundle.WorkspaceKey)
	bundle.TaskID = strings.TrimSpace(bundle.TaskID)
	if bundle.WorkspaceKey == "" || bundle.TaskID == "" {
		return TaskResumeSnapshot{}, ErrInvalidTaskResume
	}
	snapshot := TaskResumeSnapshot{SchemaVersion: 1, TaskID: bundle.TaskID, WorkspaceKey: bundle.WorkspaceKey, Bundle: bundle.Clone(), EventHighWater: eventHighWater, ExportedAt: time.Now().UTC()}
	digest, err := resumeDigest(snapshot.Bundle, snapshot.EventHighWater)
	if err != nil {
		return TaskResumeSnapshot{}, err
	}
	snapshot.Digest = digest
	return snapshot, nil
}

// ImportTaskResume verifies integrity and returns the bundle plus the event
// high-water mark the receiver must replay before continuing execution.
// Persistence stays with the caller: import never writes to any store.
func ImportTaskResume(snapshot TaskResumeSnapshot) (Bundle, uint64, error) {
	if snapshot.SchemaVersion != 1 || strings.TrimSpace(snapshot.TaskID) == "" || strings.TrimSpace(snapshot.WorkspaceKey) == "" {
		return Bundle{}, 0, ErrInvalidTaskResume
	}
	if snapshot.Bundle.TaskID != snapshot.TaskID || snapshot.Bundle.WorkspaceKey != snapshot.WorkspaceKey {
		return Bundle{}, 0, ErrInvalidTaskResume
	}
	expected, err := resumeDigest(snapshot.Bundle, snapshot.EventHighWater)
	if err != nil {
		return Bundle{}, 0, err
	}
	if subtleCompare(expected, snapshot.Digest) != true {
		return Bundle{}, 0, ErrInvalidTaskResume
	}
	return snapshot.Bundle.Clone(), snapshot.EventHighWater, nil
}

func resumeDigest(bundle Bundle, highWater uint64) (string, error) {
	raw, err := json.Marshal(struct {
		Bundle    Bundle `json:"bundle"`
		HighWater uint64 `json:"highWater"`
	}{Bundle: bundle, HighWater: highWater})
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:]), nil
}

func subtleCompare(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	result := 0
	for i := 0; i < len(a); i++ {
		result |= int(a[i]) ^ int(b[i])
	}
	return result == 0
}

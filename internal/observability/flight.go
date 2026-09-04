package observability

import (
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

type ProgressStatus string

const (
	ProgressPending   ProgressStatus = "pending"
	ProgressRunning   ProgressStatus = "running"
	ProgressWaiting   ProgressStatus = "waiting"
	ProgressPassed    ProgressStatus = "passed"
	ProgressFailed    ProgressStatus = "failed"
	ProgressCancelled ProgressStatus = "cancelled"
)

type ProgressItem struct {
	Sequence  uint64         `json:"sequence"`
	Kind      string         `json:"kind"`
	Label     string         `json:"label"`
	Status    ProgressStatus `json:"status"`
	AgentID   string         `json:"agentId,omitempty"`
	Reference string         `json:"reference,omitempty"`
	Timestamp time.Time      `json:"timestamp"`
}

type FlightSnapshot struct {
	TaskID       string         `json:"taskId"`
	TraceIDs     []string       `json:"traceIds,omitempty"`
	Progress     []ProgressItem `json:"progress"`
	LastSequence uint64         `json:"lastSequence"`
}

// BuildFlightSnapshot projects durable runtime truth into a compact UI timeline.
// It does not invent success: completed/verified UI states are derived from
// persisted event payloads rather than streaming text from an agent.
func BuildFlightSnapshot(events []runtimeevents.Event) FlightSnapshot {
	snapshot := FlightSnapshot{Progress: []ProgressItem{}}
	traces := map[string]struct{}{}
	ordered := append([]runtimeevents.Event(nil), events...)
	sort.SliceStable(ordered, func(i, j int) bool { return ordered[i].Sequence < ordered[j].Sequence })
	latest := map[string]ProgressItem{}
	order := []string{}
	for _, event := range ordered {
		if snapshot.TaskID == "" {
			snapshot.TaskID = event.TaskID
		}
		if event.Sequence > snapshot.LastSequence {
			snapshot.LastSequence = event.Sequence
		}
		if event.TraceID != "" {
			traces[event.TraceID] = struct{}{}
		}
		item, key, ok := progressFromEvent(event)
		if !ok {
			continue
		}
		if _, exists := latest[key]; !exists {
			order = append(order, key)
		}
		latest[key] = item
	}
	for _, key := range order {
		snapshot.Progress = append(snapshot.Progress, latest[key])
	}
	for trace := range traces {
		snapshot.TraceIDs = append(snapshot.TraceIDs, trace)
	}
	sort.Strings(snapshot.TraceIDs)
	return snapshot
}

func progressFromEvent(event runtimeevents.Event) (ProgressItem, string, bool) {
	base := ProgressItem{Sequence: event.Sequence, AgentID: event.AgentID, Timestamp: event.Timestamp}
	switch event.Type {
	case "taskdag.node_added", "taskdag.node_updated":
		value := payloadObject(event.Payload["node"])
		id := stringField(value, "id")
		if id == "" {
			return ProgressItem{}, "", false
		}
		base.Kind = "task_node"
		base.Label = stringField(value, "subject")
		if base.Label == "" {
			base.Label = id
		}
		base.Status = progressStatus(stringField(value, "status"))
		base.Reference = "task-node:" + id
		return base, base.Reference, true
	case "codingkernel.initialized", "codingkernel.transitioned", "codingkernel.failure_handled":
		value := payloadObject(event.Payload["kernel"])
		stage := stringField(value, "stage")
		if stage == "" {
			return ProgressItem{}, "", false
		}
		base.Kind = "coding_kernel"
		base.Label = "Coding: " + stage
		base.Status = kernelProgressStatus(stage)
		base.Reference = "kernel"
		return base, "kernel", true
	case "verification.plan_created", "verification.result_recorded":
		value := payloadObject(event.Payload["verification"])
		base.Kind = "verification"
		base.Label = "Verification"
		base.Status = verificationProgressStatus(value)
		base.Reference = "verification"
		return base, "verification", true
	case "failure.decision", "failure.evidence_recorded":
		base.Kind = "recovery"
		base.Label = "Recovery"
		base.Status = ProgressWaiting
		base.Reference = "recovery"
		return base, "recovery", true
	}
	return ProgressItem{}, "", false
}

func progressStatus(value string) ProgressStatus {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "completed", "passed", "ok":
		return ProgressPassed
	case "running", "active", "starting":
		return ProgressRunning
	case "waiting", "waiting_user", "blocked":
		return ProgressWaiting
	case "failed", "error":
		return ProgressFailed
	case "cancelled":
		return ProgressCancelled
	default:
		return ProgressPending
	}
}

func kernelProgressStatus(stage string) ProgressStatus {
	switch strings.ToLower(stage) {
	case "complete":
		return ProgressPassed
	case "failed":
		return ProgressFailed
	case "blocked":
		return ProgressWaiting
	default:
		return ProgressRunning
	}
}

func verificationProgressStatus(value map[string]any) ProgressStatus {
	results, _ := value["results"].([]any)
	if len(results) == 0 {
		return ProgressPending
	}
	passed, failed, running := 0, 0, 0
	for _, raw := range results {
		status := stringField(payloadObject(raw), "status")
		switch status {
		case "passed":
			passed++
		case "failed":
			failed++
		case "running":
			running++
		}
	}
	if failed > 0 {
		return ProgressFailed
	}
	if running > 0 {
		return ProgressRunning
	}
	if passed == len(results) {
		return ProgressPassed
	}
	return ProgressPending
}

func payloadObject(value any) map[string]any {
	if typed, ok := value.(map[string]any); ok {
		return typed
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil
	}
	var out map[string]any
	if json.Unmarshal(raw, &out) != nil {
		return nil
	}
	return out
}
func stringField(value map[string]any, key string) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value[key]))
}

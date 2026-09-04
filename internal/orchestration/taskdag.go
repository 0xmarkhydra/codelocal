package orchestration

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

type TaskNodeStatus string

const (
	TaskNodePending   TaskNodeStatus = "pending"
	TaskNodeRunning   TaskNodeStatus = "running"
	TaskNodeCompleted TaskNodeStatus = "completed"
	TaskNodeFailed    TaskNodeStatus = "failed"
	TaskNodeCancelled TaskNodeStatus = "cancelled"

	eventTaskNodeAdded   = "taskdag.node_added"
	eventTaskNodeUpdated = "taskdag.node_updated"
)

var (
	ErrInvalidTaskDAG  = errors.New("invalid task dag")
	ErrTaskNodeExists  = errors.New("task node already exists")
	ErrTaskNodeMissing = errors.New("task node not found")
	ErrTaskDAGCycle    = errors.New("task dag cycle")
	ErrTaskNodeBlocked = errors.New("task node is blocked")
	ErrStaleTaskNode   = errors.New("stale task node revision")
)

type TaskNode struct {
	ID           string         `json:"id"`
	Revision     uint64         `json:"revision"`
	Subject      string         `json:"subject"`
	Description  string         `json:"description,omitempty"`
	Status       TaskNodeStatus `json:"status"`
	OwnerAgentID string         `json:"ownerAgentId,omitempty"`
	BlockedBy    []string       `json:"blockedBy,omitempty"`
	ReadScope    []string       `json:"readScope,omitempty"`
	WriteScope   []string       `json:"writeScope,omitempty"`
	Verification []string       `json:"verification,omitempty"`
	CreatedAt    time.Time      `json:"createdAt"`
	UpdatedAt    time.Time      `json:"updatedAt"`
}

func (n TaskNode) Clone() TaskNode {
	n.BlockedBy = append([]string(nil), n.BlockedBy...)
	n.ReadScope = append([]string(nil), n.ReadScope...)
	n.WriteScope = append([]string(nil), n.WriteScope...)
	n.Verification = append([]string(nil), n.Verification...)
	return n
}

type TaskDAG struct {
	mu           sync.RWMutex
	events       *runtimeevents.Store
	workspaceKey string
	taskID       string
	nodes        map[string]TaskNode
}

func NewTaskDAG(events *runtimeevents.Store, workspaceKey, taskID string) (*TaskDAG, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	taskID = strings.TrimSpace(taskID)
	if workspaceKey == "" || taskID == "" {
		return nil, ErrInvalidTaskDAG
	}
	dag := &TaskDAG{events: events, workspaceKey: workspaceKey, taskID: taskID, nodes: map[string]TaskNode{}}
	if events == nil {
		return dag, nil
	}
	stored, err := events.List(workspaceKey, taskID, 0, 5000)
	if err != nil {
		return nil, err
	}
	for _, event := range stored {
		if event.Type != eventTaskNodeAdded && event.Type != eventTaskNodeUpdated {
			continue
		}
		var node TaskNode
		if err := dagDecode(event.Payload["node"], &node); err != nil {
			return nil, err
		}
		dag.nodes[node.ID] = node
	}
	return dag, nil
}

func (d *TaskDAG) Add(node TaskNode) (TaskNode, error) {
	node = normalizeTaskNode(node)
	if node.ID == "" || node.Subject == "" {
		return TaskNode{}, ErrInvalidTaskDAG
	}
	d.mu.Lock()
	defer d.mu.Unlock()
	if existing, ok := d.nodes[node.ID]; ok {
		if sameTaskNode(existing, node) {
			return existing.Clone(), nil
		}
		return TaskNode{}, ErrTaskNodeExists
	}
	for _, blocker := range node.BlockedBy {
		if blocker == node.ID {
			return TaskNode{}, ErrTaskDAGCycle
		}
		if _, ok := d.nodes[blocker]; !ok {
			return TaskNode{}, ErrTaskNodeMissing
		}
	}
	if err := d.appendLocked(eventTaskNodeAdded, node, "task-node:add:"+node.ID); err != nil {
		return TaskNode{}, err
	}
	d.nodes[node.ID] = node
	return node.Clone(), nil
}

func (d *TaskDAG) SetStatus(id string, expectedRevision uint64, status TaskNodeStatus) (TaskNode, error) {
	return d.update(id, expectedRevision, func(node *TaskNode) error {
		if !validTaskNodeStatus(status) {
			return ErrInvalidTaskDAG
		}
		if status == TaskNodeRunning && !d.unblockedLocked(node.ID) {
			return ErrTaskNodeBlocked
		}
		if !taskNodeTransitionAllowed(node.Status, status) {
			return ErrInvalidTaskDAG
		}
		node.Status = status
		return nil
	})
}

func (d *TaskDAG) AssignOwner(id string, expectedRevision uint64, ownerAgentID string) (TaskNode, error) {
	ownerAgentID = strings.TrimSpace(ownerAgentID)
	return d.update(id, expectedRevision, func(node *TaskNode) error {
		node.OwnerAgentID = ownerAgentID
		return nil
	})
}

func (d *TaskDAG) SetDependencies(id string, expectedRevision uint64, blockedBy []string) (TaskNode, error) {
	blockedBy = normalizeTaskStrings(blockedBy)
	return d.update(id, expectedRevision, func(node *TaskNode) error {
		for _, blocker := range blockedBy {
			if blocker == id {
				return ErrTaskDAGCycle
			}
			if _, ok := d.nodes[blocker]; !ok {
				return ErrTaskNodeMissing
			}
		}
		original := d.nodes[id]
		node.BlockedBy = append([]string(nil), blockedBy...)
		d.nodes[id] = node.Clone()
		hasCycle := d.hasCycleLocked()
		d.nodes[id] = original
		if hasCycle {
			return ErrTaskDAGCycle
		}
		return nil
	})
}

func (d *TaskDAG) Node(id string) (TaskNode, bool) {
	d.mu.RLock()
	defer d.mu.RUnlock()
	node, ok := d.nodes[strings.TrimSpace(id)]
	return node.Clone(), ok
}

func (d *TaskDAG) Runnable() []TaskNode {
	d.mu.RLock()
	defer d.mu.RUnlock()
	out := []TaskNode{}
	for _, node := range d.nodes {
		if node.Status == TaskNodePending && d.unblockedLocked(node.ID) {
			out = append(out, node.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (d *TaskDAG) update(id string, expectedRevision uint64, mutate func(*TaskNode) error) (TaskNode, error) {
	id = strings.TrimSpace(id)
	d.mu.Lock()
	defer d.mu.Unlock()
	current, ok := d.nodes[id]
	if !ok {
		return TaskNode{}, ErrTaskNodeMissing
	}
	if expectedRevision == 0 || current.Revision != expectedRevision {
		return TaskNode{}, ErrStaleTaskNode
	}
	updated := current.Clone()
	if err := mutate(&updated); err != nil {
		return TaskNode{}, err
	}
	updated.Revision++
	updated.UpdatedAt = time.Now().UTC()
	if err := d.appendLocked(eventTaskNodeUpdated, updated, "task-node:update:"+updated.ID+":"+strconv.FormatUint(updated.Revision, 10)); err != nil {
		return TaskNode{}, err
	}
	d.nodes[id] = updated
	return updated.Clone(), nil
}

func (d *TaskDAG) unblockedLocked(id string) bool {
	node, ok := d.nodes[id]
	if !ok {
		return false
	}
	for _, blocker := range node.BlockedBy {
		dependency, ok := d.nodes[blocker]
		if !ok || dependency.Status != TaskNodeCompleted {
			return false
		}
	}
	return true
}

func (d *TaskDAG) hasCycleLocked() bool {
	visiting := map[string]bool{}
	visited := map[string]bool{}
	var visit func(string) bool
	visit = func(id string) bool {
		if visiting[id] {
			return true
		}
		if visited[id] {
			return false
		}
		visiting[id] = true
		for _, blocker := range d.nodes[id].BlockedBy {
			if visit(blocker) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range d.nodes {
		if visit(id) {
			return true
		}
	}
	return false
}

func (d *TaskDAG) appendLocked(eventType string, node TaskNode, idempotencyKey string) error {
	if d.events == nil {
		return nil
	}
	_, _, err := d.events.Append(d.workspaceKey, d.taskID, runtimeevents.Event{
		Type:           eventType,
		TaskID:         d.taskID,
		AgentID:        node.OwnerAgentID,
		IdempotencyKey: idempotencyKey,
		Payload:        map[string]any{"node": dagPayload(node)},
	})
	return err
}

func normalizeTaskNode(node TaskNode) TaskNode {
	node.ID = strings.TrimSpace(node.ID)
	node.Subject = strings.TrimSpace(node.Subject)
	node.Description = strings.TrimSpace(node.Description)
	node.OwnerAgentID = strings.TrimSpace(node.OwnerAgentID)
	node.BlockedBy = normalizeTaskStrings(node.BlockedBy)
	node.ReadScope = normalizeTaskStrings(node.ReadScope)
	node.WriteScope = normalizeTaskStrings(node.WriteScope)
	node.Verification = normalizeTaskStrings(node.Verification)
	if node.Status == "" {
		node.Status = TaskNodePending
	}
	if node.Revision == 0 {
		node.Revision = 1
	}
	now := time.Now().UTC()
	if node.CreatedAt.IsZero() {
		node.CreatedAt = now
	} else {
		node.CreatedAt = node.CreatedAt.UTC()
	}
	if node.UpdatedAt.IsZero() {
		node.UpdatedAt = node.CreatedAt
	} else {
		node.UpdatedAt = node.UpdatedAt.UTC()
	}
	return node
}

func normalizeTaskStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func validTaskNodeStatus(status TaskNodeStatus) bool {
	switch status {
	case TaskNodePending, TaskNodeRunning, TaskNodeCompleted, TaskNodeFailed, TaskNodeCancelled:
		return true
	default:
		return false
	}
}

func taskNodeTransitionAllowed(from, to TaskNodeStatus) bool {
	if from == to {
		return true
	}
	switch from {
	case TaskNodePending:
		return to == TaskNodeRunning || to == TaskNodeCancelled
	case TaskNodeRunning:
		return to == TaskNodeCompleted || to == TaskNodeFailed || to == TaskNodeCancelled
	case TaskNodeFailed:
		return to == TaskNodePending || to == TaskNodeCancelled
	case TaskNodeCompleted, TaskNodeCancelled:
		return false
	default:
		return false
	}
}

func sameTaskNode(a, b TaskNode) bool {
	return a.ID == b.ID && a.Revision == b.Revision && a.Subject == b.Subject && a.Description == b.Description && a.Status == b.Status && a.OwnerAgentID == b.OwnerAgentID && stringSlicesEqual(a.BlockedBy, b.BlockedBy) && stringSlicesEqual(a.ReadScope, b.ReadScope) && stringSlicesEqual(a.WriteScope, b.WriteScope) && stringSlicesEqual(a.Verification, b.Verification)
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func dagPayload(value any) any {
	raw, _ := json.Marshal(value)
	var payload any
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func dagDecode(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrInvalidTaskDAG
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return ErrInvalidTaskDAG
	}
	return nil
}

package agentruntime

import (
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

type AgentEdgeKind string
type AgentEdgeStatus string

const (
	EdgeSpawn      AgentEdgeKind = "spawn"
	EdgeDelegate   AgentEdgeKind = "delegate"
	EdgeDependency AgentEdgeKind = "dependency"
	EdgeReview     AgentEdgeKind = "review"
	EdgeRetry      AgentEdgeKind = "retry"

	EdgeOpen   AgentEdgeStatus = "open"
	EdgeClosed AgentEdgeStatus = "closed"

	eventAgentRegistered      = "agentgraph.agent_registered"
	eventAgentSpawned         = "agentgraph.agent_spawned"
	eventAgentLinked          = "agentgraph.agent_linked"
	eventAgentTransitioned    = "agentgraph.agent_transitioned"
	eventAgentEdgeClosed      = "agentgraph.edge_closed"
	eventAgentSubtreeCanceled = "agentgraph.subtree_cancelled"
)

var (
	ErrInvalidAgentGraph = errors.New("invalid agent graph")
	ErrAgentExists       = errors.New("agent already exists")
	ErrAgentNotFound     = errors.New("agent not found")
	ErrAgentCycle        = errors.New("agent graph cycle")
	ErrAgentEdgeExists   = errors.New("agent edge already exists")
	ErrAgentEdgeNotFound = errors.New("agent edge not found")
	ErrStaleAgentEdge    = errors.New("stale agent edge revision")
)

type AgentEdge struct {
	ID            string          `json:"id"`
	ParentAgentID string          `json:"parentAgentId"`
	ChildAgentID  string          `json:"childAgentId"`
	Kind          AgentEdgeKind   `json:"kind"`
	Status        AgentEdgeStatus `json:"status"`
	Revision      uint64          `json:"revision"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// AgentGraph is a task-scoped durable topology. Runtime events are written
// before hot state mutates so a restart can reconstruct the same graph.
type AgentGraph struct {
	mu           sync.RWMutex
	events       *runtimeevents.Store
	workspaceKey string
	taskID       string
	agents       map[string]AgentIdentity
	edges        map[string]AgentEdge
}

func NewAgentGraph(events *runtimeevents.Store, workspaceKey, taskID string) (*AgentGraph, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	taskID = strings.TrimSpace(taskID)
	if workspaceKey == "" || taskID == "" {
		return nil, ErrInvalidAgentGraph
	}
	graph := &AgentGraph{
		events:       events,
		workspaceKey: workspaceKey,
		taskID:       taskID,
		agents:       map[string]AgentIdentity{},
		edges:        map[string]AgentEdge{},
	}
	if events == nil {
		return graph, nil
	}
	stored, err := events.List(workspaceKey, taskID, 0, 5000)
	if err != nil {
		return nil, err
	}
	for _, event := range stored {
		if err := graph.applyEvent(event); err != nil {
			return nil, err
		}
	}
	return graph, nil
}

func (g *AgentGraph) Register(identity AgentIdentity) (AgentIdentity, error) {
	identity, err := NormalizeIdentity(identity)
	if err != nil || identity.TaskID != g.taskID {
		return AgentIdentity{}, ErrInvalidAgentGraph
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if existing, ok := g.agents[identity.ID]; ok {
		if sameAgentIdentity(existing, identity) {
			return existing, nil
		}
		return AgentIdentity{}, ErrAgentExists
	}
	if err := g.appendLocked(eventAgentRegistered, identity.ID, "register:"+identity.ID, map[string]any{"identity": graphPayload(identity)}); err != nil {
		return AgentIdentity{}, err
	}
	g.agents[identity.ID] = identity
	return identity, nil
}

func (g *AgentGraph) Spawn(parentID string, child AgentIdentity, kind AgentEdgeKind) (AgentIdentity, AgentEdge, error) {
	parentID = strings.TrimSpace(parentID)
	child.ParentAgentID = parentID
	child, err := NormalizeIdentity(child)
	if err != nil || child.TaskID != g.taskID || parentID == "" {
		return AgentIdentity{}, AgentEdge{}, ErrInvalidAgentGraph
	}
	if kind == "" {
		kind = EdgeSpawn
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.agents[parentID]; !ok {
		return AgentIdentity{}, AgentEdge{}, ErrAgentNotFound
	}
	if _, ok := g.agents[child.ID]; ok {
		return AgentIdentity{}, AgentEdge{}, ErrAgentExists
	}
	edge := newAgentEdge(parentID, child.ID, kind, time.Now().UTC())
	payload := map[string]any{"identity": graphPayload(child), "edge": graphPayload(edge)}
	if err := g.appendLocked(eventAgentSpawned, child.ID, "spawn:"+edge.ID, payload); err != nil {
		return AgentIdentity{}, AgentEdge{}, err
	}
	g.agents[child.ID] = child
	g.edges[edge.ID] = edge
	return child, edge, nil
}

func (g *AgentGraph) Link(parentID, childID string, kind AgentEdgeKind) (AgentEdge, error) {
	parentID = strings.TrimSpace(parentID)
	childID = strings.TrimSpace(childID)
	if parentID == "" || childID == "" || parentID == childID {
		return AgentEdge{}, ErrAgentCycle
	}
	if kind == "" {
		kind = EdgeDependency
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.agents[parentID]; !ok {
		return AgentEdge{}, ErrAgentNotFound
	}
	if _, ok := g.agents[childID]; !ok {
		return AgentEdge{}, ErrAgentNotFound
	}
	if g.pathExistsLocked(childID, parentID) {
		return AgentEdge{}, ErrAgentCycle
	}
	edge := newAgentEdge(parentID, childID, kind, time.Now().UTC())
	if _, ok := g.edges[edge.ID]; ok {
		return AgentEdge{}, ErrAgentEdgeExists
	}
	if err := g.appendLocked(eventAgentLinked, childID, "link:"+edge.ID, map[string]any{"edge": graphPayload(edge)}); err != nil {
		return AgentEdge{}, err
	}
	g.edges[edge.ID] = edge
	return edge, nil
}

func (g *AgentGraph) Transition(agentID string, next AgentStatus, at time.Time) (AgentIdentity, error) {
	agentID = strings.TrimSpace(agentID)
	g.mu.Lock()
	defer g.mu.Unlock()
	current, ok := g.agents[agentID]
	if !ok {
		return AgentIdentity{}, ErrAgentNotFound
	}
	updated, err := TransitionIdentity(current, next, at)
	if err != nil {
		return AgentIdentity{}, err
	}
	key := "agent-status:" + agentID + ":" + string(next) + ":" + strconv.FormatUint(updated.Revision, 10)
	if err := g.appendLocked(eventAgentTransitioned, agentID, key, map[string]any{"identity": graphPayload(updated)}); err != nil {
		return AgentIdentity{}, err
	}
	g.agents[agentID] = updated
	return updated, nil
}

func (g *AgentGraph) CloseEdge(edgeID string, expectedRevision uint64, at time.Time) (AgentEdge, error) {
	edgeID = strings.TrimSpace(edgeID)
	g.mu.Lock()
	defer g.mu.Unlock()
	edge, ok := g.edges[edgeID]
	if !ok {
		return AgentEdge{}, ErrAgentEdgeNotFound
	}
	if expectedRevision != 0 && edge.Revision != expectedRevision {
		return AgentEdge{}, ErrStaleAgentEdge
	}
	if edge.Status == EdgeClosed {
		return edge, nil
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	edge.Status = EdgeClosed
	edge.Revision++
	edge.UpdatedAt = at
	if err := g.appendLocked(eventAgentEdgeClosed, edge.ChildAgentID, "close-edge:"+edge.ID+":"+strconv.FormatUint(edge.Revision, 10), map[string]any{"edge": graphPayload(edge)}); err != nil {
		return AgentEdge{}, err
	}
	g.edges[edge.ID] = edge
	return edge, nil
}

func (g *AgentGraph) CancelSubtree(rootAgentID string, at time.Time) ([]AgentIdentity, error) {
	rootAgentID = strings.TrimSpace(rootAgentID)
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.agents[rootAgentID]; !ok {
		return nil, ErrAgentNotFound
	}
	if at.IsZero() {
		at = time.Now().UTC()
	} else {
		at = at.UTC()
	}
	ids := append([]string{rootAgentID}, g.descendantIDsLocked(rootAgentID)...)
	updated := make([]AgentIdentity, 0, len(ids))
	for _, id := range ids {
		identity := g.agents[id]
		if agentTerminal(identity.Status) {
			continue
		}
		next, err := TransitionIdentity(identity, AgentCancelled, at)
		if err != nil {
			return nil, err
		}
		updated = append(updated, next)
	}
	if len(updated) == 0 {
		return nil, nil
	}
	payload := make([]any, 0, len(updated))
	for _, identity := range updated {
		payload = append(payload, graphPayload(identity))
	}
	if err := g.appendLocked(eventAgentSubtreeCanceled, rootAgentID, "cancel-subtree:"+rootAgentID+":"+at.Format(time.RFC3339Nano), map[string]any{"identities": payload}); err != nil {
		return nil, err
	}
	for _, identity := range updated {
		g.agents[identity.ID] = identity
	}
	return append([]AgentIdentity(nil), updated...), nil
}

func (g *AgentGraph) Agent(agentID string) (AgentIdentity, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	identity, ok := g.agents[strings.TrimSpace(agentID)]
	return identity, ok
}

func (g *AgentGraph) Children(parentID string) []AgentIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ids := g.childIDsLocked(strings.TrimSpace(parentID))
	out := make([]AgentIdentity, 0, len(ids))
	for _, id := range ids {
		if identity, ok := g.agents[id]; ok {
			out = append(out, identity)
		}
	}
	return out
}

func (g *AgentGraph) Descendants(parentID string) []AgentIdentity {
	g.mu.RLock()
	defer g.mu.RUnlock()
	ids := g.descendantIDsLocked(strings.TrimSpace(parentID))
	out := make([]AgentIdentity, 0, len(ids))
	for _, id := range ids {
		out = append(out, g.agents[id])
	}
	return out
}

func (g *AgentGraph) JoinReady(parentID string) bool {
	children := g.Children(parentID)
	if len(children) == 0 {
		return true
	}
	for _, child := range children {
		if !agentTerminal(child.Status) {
			return false
		}
	}
	return true
}

func newAgentEdge(parentID, childID string, kind AgentEdgeKind, now time.Time) AgentEdge {
	return AgentEdge{
		ID:            string(kind) + ":" + parentID + "->" + childID,
		ParentAgentID: parentID,
		ChildAgentID:  childID,
		Kind:          kind,
		Status:        EdgeOpen,
		Revision:      1,
		CreatedAt:     now,
		UpdatedAt:     now,
	}
}

func (g *AgentGraph) childIDsLocked(parentID string) []string {
	seen := map[string]struct{}{}
	for _, edge := range g.edges {
		if edge.ParentAgentID == parentID {
			seen[edge.ChildAgentID] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for id := range seen {
		out = append(out, id)
	}
	sort.Strings(out)
	return out
}

func (g *AgentGraph) descendantIDsLocked(parentID string) []string {
	queue := append([]string(nil), g.childIDsLocked(parentID)...)
	seen := map[string]struct{}{}
	out := []string{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
		queue = append(queue, g.childIDsLocked(id)...)
	}
	return out
}

func (g *AgentGraph) pathExistsLocked(start, target string) bool {
	if start == target {
		return true
	}
	queue := []string{start}
	seen := map[string]struct{}{}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		for _, child := range g.childIDsLocked(id) {
			if child == target {
				return true
			}
			queue = append(queue, child)
		}
	}
	return false
}

func (g *AgentGraph) appendLocked(eventType, agentID, idempotencyKey string, payload map[string]any) error {
	if g.events == nil {
		return nil
	}
	_, _, err := g.events.Append(g.workspaceKey, g.taskID, runtimeevents.Event{
		Type:           eventType,
		TaskID:         g.taskID,
		AgentID:        agentID,
		IdempotencyKey: idempotencyKey,
		Payload:        payload,
	})
	return err
}

func sameAgentIdentity(a, b AgentIdentity) bool {
	return a.ID == b.ID && a.TaskID == b.TaskID && a.ParentAgentID == b.ParentAgentID && a.Role == b.Role && a.EngineID == b.EngineID && a.ModelID == b.ModelID && a.Status == b.Status && a.Revision == b.Revision
}

func agentTerminal(status AgentStatus) bool {
	return status == AgentCompleted || status == AgentFailed || status == AgentCancelled
}

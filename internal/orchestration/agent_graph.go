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

type AgentGraphNodeStatus string

const (
	AgentNodeIdle      AgentGraphNodeStatus = "idle"
	AgentNodeRunning   AgentGraphNodeStatus = "running"
	AgentNodeCompleted AgentGraphNodeStatus = "completed"
	AgentNodeFailed    AgentGraphNodeStatus = "failed"

	eventAgentNodeAdded   = "agentgraph.node_added"
	eventAgentNodeUpdated = "agentgraph.node_updated"
	eventAgentEdgeAdded   = "agentgraph.edge_added"
	eventAgentEdgeRemoved = "agentgraph.edge_removed"
)

var (
	ErrInvalidAgentGraph = errors.New("invalid agent graph")
	ErrAgentNodeExists   = errors.New("agent node already exists")
	ErrAgentNodeMissing  = errors.New("agent node not found")
	ErrAgentEdgeExists   = errors.New("agent edge already exists")
	ErrAgentEdgeMissing  = errors.New("agent edge not found")
	ErrAgentGraphCycle   = errors.New("agent graph cycle")
	ErrStaleAgentNode    = errors.New("stale agent node revision")
)

type AgentGraphNode struct {
	ID        string               "id"
	Revision  uint64               "revision"
	AgentID   string               "agentId"
	Role      string               "role,omitempty"
	Status    AgentGraphNodeStatus "status"
	ParentID  string               "parentId,omitempty"
	CreatedAt time.Time            "createdAt"
	UpdatedAt time.Time            "updatedAt"
}

func (n AgentGraphNode) Clone() AgentGraphNode {
	return n
}

type AgentGraphEdge struct {
	ID        string    "id"
	Revision  uint64    "revision"
	FromID    string    "fromId"
	ToID      string    "toId"
	Kind      string    "kind"
	CreatedAt time.Time "createdAt"
}

func (e AgentGraphEdge) Clone() AgentGraphEdge {
	return e
}

type AgentGraph struct {
	mu           sync.RWMutex
	events       *runtimeevents.Store
	workspaceKey string
	taskID       string
	nodes        map[string]AgentGraphNode
	edges        map[string]AgentGraphEdge
}

func NewAgentGraph(events *runtimeevents.Store, workspaceKey, taskID string) (*AgentGraph, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	taskID = strings.TrimSpace(taskID)
	if workspaceKey == "" || taskID == "" {
		return nil, ErrInvalidAgentGraph
	}
	g := &AgentGraph{events: events, workspaceKey: workspaceKey, taskID: taskID, nodes: map[string]AgentGraphNode{}, edges: map[string]AgentGraphEdge{}}
	if events == nil {
		return g, nil
	}
	stored, err := events.List(workspaceKey, taskID, 0, 5000)
	if err != nil {
		return nil, err
	}
	for _, event := range stored {
		switch event.Type {
		case eventAgentNodeAdded, eventAgentNodeUpdated:
			var node AgentGraphNode
			if err := graphDecode(event.Payload["node"], &node); err != nil {
				return nil, err
			}
			g.nodes[node.ID] = node
		case eventAgentEdgeAdded:
			var edge AgentGraphEdge
			if err := graphDecode(event.Payload["edge"], &edge); err != nil {
				return nil, err
			}
			g.edges[edge.ID] = edge
		case eventAgentEdgeRemoved:
			var edge AgentGraphEdge
			if err := graphDecode(event.Payload["edge"], &edge); err != nil {
				return nil, err
			}
			delete(g.edges, edge.ID)
		}
	}
	return g, nil
}

func (g *AgentGraph) AddNode(node AgentGraphNode) (AgentGraphNode, error) {
	node = normalizeAgentNode(node)
	if node.ID == "" || node.AgentID == "" {
		return AgentGraphNode{}, ErrInvalidAgentGraph
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.nodes[node.ID]; ok {
		return AgentGraphNode{}, ErrAgentNodeExists
	}
	if node.ParentID != "" {
		if _, ok := g.nodes[node.ParentID]; !ok {
			return AgentGraphNode{}, ErrAgentNodeMissing
		}
	}
	now := time.Now().UTC()
	node.Revision = 1
	node.CreatedAt = now
	node.UpdatedAt = now
	if err := g.appendNodeLocked(eventAgentNodeAdded, node); err != nil {
		return AgentGraphNode{}, err
	}
	g.nodes[node.ID] = node
	return node.Clone(), nil
}

func (g *AgentGraph) SetNodeStatus(id string, expectedRevision uint64, status AgentGraphNodeStatus) (AgentGraphNode, error) {
	switch status {
	case AgentNodeIdle, AgentNodeRunning, AgentNodeCompleted, AgentNodeFailed:
	default:
		return AgentGraphNode{}, ErrInvalidAgentGraph
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	current, ok := g.nodes[strings.TrimSpace(id)]
	if !ok {
		return AgentGraphNode{}, ErrAgentNodeMissing
	}
	if expectedRevision == 0 || current.Revision != expectedRevision {
		return AgentGraphNode{}, ErrStaleAgentNode
	}
	current.Status = status
	current.Revision++
	current.UpdatedAt = time.Now().UTC()
	if err := g.appendNodeLocked(eventAgentNodeUpdated, current); err != nil {
		return AgentGraphNode{}, err
	}
	g.nodes[current.ID] = current
	return current.Clone(), nil
}

func (g *AgentGraph) AddEdge(edge AgentGraphEdge) (AgentGraphEdge, error) {
	edge.ID = strings.TrimSpace(edge.ID)
	edge.FromID = strings.TrimSpace(edge.FromID)
	edge.ToID = strings.TrimSpace(edge.ToID)
	edge.Kind = strings.TrimSpace(edge.Kind)
	if edge.ID == "" || edge.FromID == "" || edge.ToID == "" || edge.Kind == "" {
		return AgentGraphEdge{}, ErrInvalidAgentGraph
	}
	if edge.FromID == edge.ToID {
		return AgentGraphEdge{}, ErrAgentGraphCycle
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.edges[edge.ID]; ok {
		return AgentGraphEdge{}, ErrAgentEdgeExists
	}
	if _, ok := g.nodes[edge.FromID]; !ok {
		return AgentGraphEdge{}, ErrAgentNodeMissing
	}
	if _, ok := g.nodes[edge.ToID]; !ok {
		return AgentGraphEdge{}, ErrAgentNodeMissing
	}
	for _, existing := range g.edges {
		if existing.FromID == edge.FromID && existing.ToID == edge.ToID && existing.Kind == edge.Kind {
			return AgentGraphEdge{}, ErrAgentEdgeExists
		}
	}
	edge.Revision = 1
	edge.CreatedAt = time.Now().UTC()
	g.edges[edge.ID] = edge
	if g.hasCycleLocked() {
		delete(g.edges, edge.ID)
		return AgentGraphEdge{}, ErrAgentGraphCycle
	}
	if err := g.appendEdgeLocked(eventAgentEdgeAdded, edge); err != nil {
		delete(g.edges, edge.ID)
		return AgentGraphEdge{}, err
	}
	return edge.Clone(), nil
}

func (g *AgentGraph) RemoveEdge(id string) error {
	id = strings.TrimSpace(id)
	if id == "" {
		return ErrInvalidAgentGraph
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	edge, ok := g.edges[id]
	if !ok {
		return ErrAgentEdgeMissing
	}
	delete(g.edges, id)
	if err := g.appendEdgeLocked(eventAgentEdgeRemoved, edge); err != nil {
		g.edges[id] = edge
		return err
	}
	return nil
}

func (g *AgentGraph) Node(id string) (AgentGraphNode, bool) {
	g.mu.RLock()
	defer g.mu.RUnlock()
	node, ok := g.nodes[strings.TrimSpace(id)]
	if !ok {
		return AgentGraphNode{}, false
	}
	return node.Clone(), true
}

func (g *AgentGraph) Nodes() []AgentGraphNode {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := make([]AgentGraphNode, 0, len(g.nodes))
	for _, node := range g.nodes {
		out = append(out, node.Clone())
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (g *AgentGraph) Downstream(id string) []AgentGraphEdge {
	g.mu.RLock()
	defer g.mu.RUnlock()
	out := []AgentGraphEdge{}
	for _, edge := range g.edges {
		if edge.FromID == strings.TrimSpace(id) {
			out = append(out, edge.Clone())
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func (g *AgentGraph) hasCycleLocked() bool {
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
		for _, edge := range g.edges {
			if edge.FromID == id && visit(edge.ToID) {
				return true
			}
		}
		visiting[id] = false
		visited[id] = true
		return false
	}
	for id := range g.nodes {
		if visit(id) {
			return true
		}
	}
	return false
}

func (g *AgentGraph) appendNodeLocked(eventType string, node AgentGraphNode) error {
	if g.events == nil {
		return nil
	}
	_, _, err := g.events.Append(g.workspaceKey, g.taskID, runtimeevents.Event{
		Type: eventType, TaskID: g.taskID, AgentID: node.AgentID,
		IdempotencyKey: "agent-node:" + node.ID + ":" + strconv.FormatUint(node.Revision, 10),
		Payload:        map[string]any{"node": graphPayload(node)},
	})
	return err
}

func (g *AgentGraph) appendEdgeLocked(eventType string, edge AgentGraphEdge) error {
	if g.events == nil {
		return nil
	}
	_, _, err := g.events.Append(g.workspaceKey, g.taskID, runtimeevents.Event{
		Type: eventType, TaskID: g.taskID,
		IdempotencyKey: "agent-edge:" + edge.ID + ":" + strconv.FormatUint(edge.Revision, 10),
		Payload:        map[string]any{"edge": graphPayload(edge)},
	})
	return err
}

func normalizeAgentNode(node AgentGraphNode) AgentGraphNode {
	node.ID = strings.TrimSpace(node.ID)
	node.AgentID = strings.TrimSpace(node.AgentID)
	node.Role = strings.TrimSpace(node.Role)
	node.ParentID = strings.TrimSpace(node.ParentID)
	if node.Status == "" {
		node.Status = AgentNodeIdle
	}
	return node
}

func graphPayload(value any) map[string]any {
	raw, err := json.Marshal(value)
	if err != nil {
		return map[string]any{}
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return map[string]any{}
	}
	return out
}

func graphDecode(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return err
	}
	return json.Unmarshal(raw, target)
}

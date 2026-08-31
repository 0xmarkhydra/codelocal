package orchestration

import (
	"errors"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/agentruntime"
	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

var (
	ErrShadowRuntimeNotReady = errors.New("shadow runtime is not ready to finalize")
	ErrShadowRuntimeOwner    = errors.New("task node owner does not match agent")
	ErrShadowRuntimeAgent    = errors.New("agent state is incompatible with task transition")
)

// ShadowRuntime composes the V3 durable primitives without wiring them into the
// production MCP path. It exists to exercise crash recovery and cross-subsystem
// invariants before the V3 scheduler is allowed to dispatch real work.
type ShadowRuntime struct {
	graph        *agentruntime.AgentGraph
	dag          *TaskDAG
	mailbox      *Mailbox
	verification *VerificationGate
}

func NewShadowRuntime(events *runtimeevents.Store, workspaceKey, taskID string, plan VerificationPlan) (*ShadowRuntime, error) {
	graph, err := agentruntime.NewAgentGraph(events, workspaceKey, taskID)
	if err != nil {
		return nil, err
	}
	dag, err := NewTaskDAG(events, workspaceKey, taskID)
	if err != nil {
		return nil, err
	}
	mailbox, err := NewMailbox(events, workspaceKey, taskID)
	if err != nil {
		return nil, err
	}
	verification, err := NewVerificationGate(events, workspaceKey, taskID, plan)
	if err != nil {
		return nil, err
	}
	return &ShadowRuntime{graph: graph, dag: dag, mailbox: mailbox, verification: verification}, nil
}

func (r *ShadowRuntime) Graph() *agentruntime.AgentGraph { return r.graph }
func (r *ShadowRuntime) DAG() *TaskDAG { return r.dag }
func (r *ShadowRuntime) Mailbox() *Mailbox { return r.mailbox }
func (r *ShadowRuntime) Verification() *VerificationGate { return r.verification }

// PrepareRunnableAssignments creates compact durable mailbox references for
// runnable nodes. The node payload remains in the DAG, avoiding duplicated task
// context inside mailbox history. Existing assignment IDs are not re-queued.
func (r *ShadowRuntime) PrepareRunnableAssignments(senderAgentID string) ([]MailboxMessage, error) {
	senderAgentID = strings.TrimSpace(senderAgentID)
	if senderAgentID == "" {
		return nil, ErrInvalidMailboxMessage
	}
	if _, ok := r.graph.Agent(senderAgentID); !ok {
		return nil, agentruntime.ErrAgentNotFound
	}
	out := []MailboxMessage{}
	for _, node := range r.dag.Runnable() {
		owner := strings.TrimSpace(node.OwnerAgentID)
		if owner == "" {
			continue
		}
		agent, ok := r.graph.Agent(owner)
		if !ok || agent.Status == agentruntime.AgentCompleted || agent.Status == agentruntime.AgentFailed || agent.Status == agentruntime.AgentCancelled {
			continue
		}
		messageID := "assignment:" + node.ID + ":" + owner
		if _, exists := r.mailbox.Message(messageID); exists {
			continue
		}
		message, err := r.mailbox.Queue(MailboxMessage{
			ID:               messageID,
			SenderAgentID:    senderAgentID,
			RecipientAgentID: owner,
			Content:          "task-node:" + node.ID,
			DeliveryMode:     "task_assignment",
		})
		if err != nil {
			return out, err
		}
		out = append(out, message)
	}
	return out, nil
}

func (r *ShadowRuntime) StartNode(nodeID string, expectedRevision uint64, agentID string) (TaskNode, error) {
	node, ok := r.dag.Node(nodeID)
	if !ok {
		return TaskNode{}, ErrTaskNodeMissing
	}
	agentID = strings.TrimSpace(agentID)
	if node.OwnerAgentID != agentID {
		return TaskNode{}, ErrShadowRuntimeOwner
	}
	agent, ok := r.graph.Agent(agentID)
	if !ok {
		return TaskNode{}, agentruntime.ErrAgentNotFound
	}
	if agent.Status != agentruntime.AgentActive {
		return TaskNode{}, ErrShadowRuntimeAgent
	}
	return r.dag.SetStatus(node.ID, expectedRevision, TaskNodeRunning)
}

func (r *ShadowRuntime) CompleteNode(nodeID string, expectedRevision uint64, agentID string) (TaskNode, error) {
	node, ok := r.dag.Node(nodeID)
	if !ok {
		return TaskNode{}, ErrTaskNodeMissing
	}
	agentID = strings.TrimSpace(agentID)
	if node.OwnerAgentID != agentID {
		return TaskNode{}, ErrShadowRuntimeOwner
	}
	agent, ok := r.graph.Agent(agentID)
	if !ok {
		return TaskNode{}, agentruntime.ErrAgentNotFound
	}
	if agent.Status != agentruntime.AgentCompleted {
		return TaskNode{}, ErrShadowRuntimeAgent
	}
	return r.dag.SetStatus(node.ID, expectedRevision, TaskNodeCompleted)
}

// ReadyToFinalize is intentionally strict: every DAG node must be completed,
// every descendant agent must have succeeded (failed/cancelled is not enough),
// and all required verification evidence must be durable and passing.
func (r *ShadowRuntime) ReadyToFinalize(leadAgentID string) bool {
	lead, ok := r.graph.Agent(strings.TrimSpace(leadAgentID))
	if !ok {
		return false
	}
	if lead.Status != agentruntime.AgentActive && lead.Status != agentruntime.AgentIdle && lead.Status != agentruntime.AgentCompleted {
		return false
	}
	for _, node := range r.dag.Nodes() {
		if node.Status != TaskNodeCompleted {
			return false
		}
	}
	for _, child := range r.graph.Descendants(lead.ID) {
		if child.Status != agentruntime.AgentCompleted {
			return false
		}
	}
	return r.verification.CanComplete()
}

func (r *ShadowRuntime) FinalizeLead(leadAgentID string, expectedRevision uint64, at time.Time) (agentruntime.AgentIdentity, error) {
	leadAgentID = strings.TrimSpace(leadAgentID)
	lead, ok := r.graph.Agent(leadAgentID)
	if !ok {
		return agentruntime.AgentIdentity{}, agentruntime.ErrAgentNotFound
	}
	if !r.ReadyToFinalize(leadAgentID) {
		return agentruntime.AgentIdentity{}, ErrShadowRuntimeNotReady
	}
	if lead.Status == agentruntime.AgentCompleted {
		return lead, nil
	}
	return r.graph.TransitionCAS(leadAgentID, expectedRevision, agentruntime.AgentCompleted, at)
}

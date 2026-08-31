package agentruntime

import (
	"errors"
	"strconv"
	"strings"
	"time"
)

var ErrStaleAgentRevision = errors.New("stale agent revision")

// TransitionCAS performs an optimistic-concurrency transition. Schedulers pass
// the revision they observed so a stale activation cannot finalize a newer
// logical agent state. expectedRevision=0 remains available to internal callers
// that intentionally do not need fencing.
func (g *AgentGraph) TransitionCAS(agentID string, expectedRevision uint64, next AgentStatus, at time.Time) (AgentIdentity, error) {
	agentID = strings.TrimSpace(agentID)
	g.mu.Lock()
	defer g.mu.Unlock()
	current, ok := g.agents[agentID]
	if !ok {
		return AgentIdentity{}, ErrAgentNotFound
	}
	if expectedRevision != 0 && current.Revision != expectedRevision {
		return AgentIdentity{}, ErrStaleAgentRevision
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

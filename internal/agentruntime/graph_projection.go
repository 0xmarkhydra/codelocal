package agentruntime

import (
	"encoding/json"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func (g *AgentGraph) applyEvent(event runtimeevents.Event) error {
	switch event.Type {
	case eventAgentRegistered:
		var identity AgentIdentity
		if err := graphDecode(event.Payload["identity"], &identity); err != nil {
			return err
		}
		g.agents[identity.ID] = identity
	case eventAgentSpawned:
		var identity AgentIdentity
		var edge AgentEdge
		if err := graphDecode(event.Payload["identity"], &identity); err != nil {
			return err
		}
		if err := graphDecode(event.Payload["edge"], &edge); err != nil {
			return err
		}
		g.agents[identity.ID] = identity
		g.edges[edge.ID] = edge
	case eventAgentLinked, eventAgentEdgeClosed:
		var edge AgentEdge
		if err := graphDecode(event.Payload["edge"], &edge); err != nil {
			return err
		}
		g.edges[edge.ID] = edge
	case eventAgentTransitioned:
		var identity AgentIdentity
		if err := graphDecode(event.Payload["identity"], &identity); err != nil {
			return err
		}
		g.agents[identity.ID] = identity
	case eventAgentSubtreeCanceled:
		var identities []AgentIdentity
		if err := graphDecode(event.Payload["identities"], &identities); err != nil {
			return err
		}
		for _, identity := range identities {
			g.agents[identity.ID] = identity
		}
	}
	return nil
}

func graphPayload(value any) any {
	raw, _ := json.Marshal(value)
	var payload any
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func graphDecode(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrInvalidAgentGraph
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return ErrInvalidAgentGraph
	}
	return nil
}

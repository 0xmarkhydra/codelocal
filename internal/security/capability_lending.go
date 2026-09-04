package security

import (
	"strings"
	"time"
)

// CapabilityLeaseDelegate requests a narrowed child lease derived from an
// active parent lease. Delegation copies authority down, never up: the child
// stays in the parent workspace and actor, is consumable only by a subset of
// the parent agents, and can never outlive its parent. The child binds the
// caller-supplied prompt decision (evaluated for the child agent), so every
// authorization still consumes a lease tied to an evaluated approval; the
// parent chain stays auditable through ParentID.
type CapabilityLeaseDelegate struct {
	ID            string
	ChildAgentIDs []string
	TTL           time.Duration
}

// Delegate issues a child lease while leaving the parent active. Only an
// agent allowed by the parent may delegate, and only to agents inside the
// parent scope.
func (s *CapabilityLeaseStore) Delegate(workspaceKey, parentID, delegatorAgentID string, decision PolicyDecision, input CapabilityLeaseDelegate) (CapabilityLease, error) {
	if decision.Effect != EffectPrompt || strings.TrimSpace(decision.ApprovalKey) == "" {
		return CapabilityLease{}, ErrInvalidCapabilityLease
	}
	parent, err := s.loadActive(strings.TrimSpace(workspaceKey), strings.TrimSpace(parentID))
	if err != nil {
		return CapabilityLease{}, err
	}
	now := s.now().UTC()
	if !now.Before(parent.ExpiresAt) {
		_ = s.claim(parent.WorkspaceKey, parent.ID, LeaseExpired)
		return CapabilityLease{}, ErrCapabilityLeaseExpired
	}
	if !leaseAllowsAgent(parent, delegatorAgentID) {
		return CapabilityLease{}, ErrCapabilityLeaseScope
	}
	children := normalizeLeaseAgents(input.ChildAgentIDs)
	if len(children) == 0 {
		return CapabilityLease{}, ErrCapabilityLeaseScope
	}
	if len(parent.AllowedAgentIDs) > 0 {
		allowed := make(map[string]struct{}, len(parent.AllowedAgentIDs))
		for _, id := range parent.AllowedAgentIDs {
			allowed[id] = struct{}{}
		}
		for _, id := range children {
			if _, ok := allowed[id]; !ok {
				return CapabilityLease{}, ErrCapabilityLeaseScope
			}
		}
	}
	ttl := input.TTL
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if ttl > time.Hour {
		ttl = time.Hour
	}
	expiresAt := now.Add(ttl)
	if expiresAt.After(parent.ExpiresAt) {
		expiresAt = parent.ExpiresAt
	}
	if !now.Before(expiresAt) {
		return CapabilityLease{}, ErrCapabilityLeaseExpired
	}
	child := CapabilityLease{
		ID: input.ID, WorkspaceKey: parent.WorkspaceKey, ActorID: parent.ActorID,
		AllowedAgentIDs: children, ApprovalKey: strings.TrimSpace(decision.ApprovalKey), Status: LeaseActive,
		ParentID: parent.ID, DelegatedBy: strings.TrimSpace(delegatorAgentID), IssuedAt: now, ExpiresAt: expiresAt,
	}
	if strings.TrimSpace(child.ID) == "" {
		child.ID = "lease_" + randomLeaseID()
	}
	if err := s.ensureLeaseIDUnused(child.WorkspaceKey, child.ID); err != nil {
		return CapabilityLease{}, err
	}
	if err := writeLeaseExclusive(s.path(child.WorkspaceKey, LeaseActive, child.ID), child); err != nil {
		return CapabilityLease{}, err
	}
	return child, nil
}

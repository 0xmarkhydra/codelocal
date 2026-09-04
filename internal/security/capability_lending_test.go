package security

import "errors"
import "testing"
import "time"

func childPromptDecision(t *testing.T, agent string) PolicyDecision {
	t.Helper()
	kernel, err := NewPolicyKernel(nil, NetworkAllow)
	if err != nil {
		t.Fatal(err)
	}
	req := PolicyRequest{ActorID: "user", AgentID: agent, Action: "run", SecretMode: SecretConsume, SecretNames: []string{"TOKEN"}, SecretOutput: SecretOutputRedacted}
	decision := kernel.Evaluate(req)
	if decision.Effect != EffectPrompt || decision.ApprovalKey == "" {
		t.Fatalf("expected prompt decision: %+v", decision)
	}
	return decision
}

func issueParentLease(t *testing.T, store *CapabilityLeaseStore, agents ...string) CapabilityLease {
	t.Helper()
	decision := childPromptDecision(t, "agent-a")
	parent, err := store.Issue(CapabilityLeaseIssue{ID: "parent", WorkspaceKey: "workspace", ActorID: "user", AllowedAgentIDs: agents, ApprovalKey: decision.ApprovalKey, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	return parent
}

func TestDelegateNarrowsToSubset(t *testing.T) {
	store, _ := NewCapabilityLeaseStore(t.TempDir())
	store.now = func() time.Time { return time.Unix(100, 0).UTC() }
	parent := issueParentLease(t, store, "agent-a", "agent-b")
	childDecision := childPromptDecision(t, "agent-b")
	child, err := store.Delegate("workspace", parent.ID, "agent-a", childDecision, CapabilityLeaseDelegate{ID: "child", ChildAgentIDs: []string{"agent-b"}, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if child.ParentID != parent.ID || child.DelegatedBy != "agent-a" || child.WorkspaceKey != "workspace" || child.ActorID != "user" {
		t.Fatalf("child linkage mismatch: %+v", child)
	}
	if len(child.AllowedAgentIDs) != 1 || child.AllowedAgentIDs[0] != "agent-b" {
		t.Fatalf("child scope mismatch: %+v", child.AllowedAgentIDs)
	}
	if !child.ExpiresAt.Equal(parent.ExpiresAt) {
		t.Fatalf("child should inherit parent expiry: %v vs %v", child.ExpiresAt, parent.ExpiresAt)
	}
	kernel, _ := NewPolicyKernel(nil, NetworkAllow)
	req := PolicyRequest{ActorID: "user", AgentID: "agent-b", Action: "run", SecretMode: SecretConsume, SecretNames: []string{"TOKEN"}, SecretOutput: SecretOutputRedacted}
	if result, err := store.Authorize("workspace", req, childDecision, child.ID); err != nil || !result.Allowed {
		t.Fatalf("child authorization failed: %+v %v", result, err)
	}
	_ = kernel
}

func TestDelegateRejectsScopeViolations(t *testing.T) {
	store, _ := NewCapabilityLeaseStore(t.TempDir())
	store.now = func() time.Time { return time.Unix(100, 0).UTC() }
	parent := issueParentLease(t, store, "agent-a")
	childDecision := childPromptDecision(t, "agent-b")
	allowDecision := PolicyDecision{Effect: EffectAllow}
	if _, err := store.Delegate("workspace", parent.ID, "agent-evil", childDecision, CapabilityLeaseDelegate{ChildAgentIDs: []string{"agent-a"}}); !errors.Is(err, ErrCapabilityLeaseScope) {
		t.Fatalf("delegating agent outside parent scope accepted: %v", err)
	}
	if _, err := store.Delegate("workspace", parent.ID, "agent-a", childDecision, CapabilityLeaseDelegate{ChildAgentIDs: []string{"agent-a", "agent-evil"}}); !errors.Is(err, ErrCapabilityLeaseScope) {
		t.Fatalf("child superset accepted: %v", err)
	}
	if _, err := store.Delegate("workspace", parent.ID, "agent-a", childDecision, CapabilityLeaseDelegate{}); err == nil {
		t.Fatal("empty child agent set accepted")
	}
	if _, err := store.Delegate("workspace", parent.ID, "agent-a", allowDecision, CapabilityLeaseDelegate{ChildAgentIDs: []string{"agent-a"}}); !errors.Is(err, ErrInvalidCapabilityLease) {
		t.Fatalf("non-prompt decision accepted: %v", err)
	}
}

func TestDelegateCapsExpiryAtParent(t *testing.T) {
	store, _ := NewCapabilityLeaseStore(t.TempDir())
	store.now = func() time.Time { return time.Unix(100, 0).UTC() }
	parent := issueParentLease(t, store, "agent-a")
	childDecision := childPromptDecision(t, "agent-a")
	child, err := store.Delegate("workspace", parent.ID, "agent-a", childDecision, CapabilityLeaseDelegate{ChildAgentIDs: []string{"agent-a"}, TTL: time.Hour})
	if err != nil {
		t.Fatal(err)
	}
	if child.ExpiresAt.After(parent.ExpiresAt) {
		t.Fatalf("child outlives parent: %v vs %v", child.ExpiresAt, parent.ExpiresAt)
	}
}

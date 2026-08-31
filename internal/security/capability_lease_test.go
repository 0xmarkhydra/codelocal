package security

import (
	"errors"
	"testing"
	"time"
)

func TestCapabilityLeaseOneShotAndScoped(t *testing.T) {
	store, err := NewCapabilityLeaseStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(100, 0).UTC()
	store.now = func() time.Time { return now }
	kernel, err := NewPolicyKernel(nil, NetworkAllow)
	if err != nil {
		t.Fatal(err)
	}
	req := PolicyRequest{ActorID: "user", AgentID: "agent-a", Action: "run", Tool: "shell", SecretMode: SecretConsume, SecretNames: []string{"VBEE_ACCESS_TOKEN"}, SecretOutput: SecretOutputRedacted}
	decision := kernel.Evaluate(req)
	if decision.Effect != EffectPrompt || decision.ApprovalKey == "" {
		t.Fatalf("unexpected prompt decision: %+v", decision)
	}
	lease, err := store.Issue(CapabilityLeaseIssue{ID: "once", WorkspaceKey: "workspace", ActorID: "user", AllowedAgentIDs: []string{"agent-a"}, ApprovalKey: decision.ApprovalKey, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	if result, err := store.Authorize("workspace", req, decision, lease.ID); err != nil || !result.Allowed {
		t.Fatalf("first authorization failed: result=%+v err=%v", result, err)
	}
	if _, err := store.Authorize("workspace", req, decision, lease.ID); !errors.Is(err, ErrCapabilityLeaseUsed) {
		t.Fatalf("one-shot lease reused: %v", err)
	}
}

func TestCapabilityLeaseWrongAgentDoesNotConsume(t *testing.T) {
	store, _ := NewCapabilityLeaseStore(t.TempDir())
	store.now = func() time.Time { return time.Unix(100, 0).UTC() }
	kernel, _ := NewPolicyKernel(nil, NetworkAllow)
	reqA := PolicyRequest{ActorID: "user", AgentID: "agent-a", Action: "run", SecretMode: SecretConsume, SecretNames: []string{"TOKEN"}, SecretOutput: SecretOutputRedacted}
	decisionA := kernel.Evaluate(reqA)
	lease, err := store.Issue(CapabilityLeaseIssue{ID: "scoped", WorkspaceKey: "workspace", ActorID: "user", AllowedAgentIDs: []string{"agent-a"}, ApprovalKey: decisionA.ApprovalKey, TTL: time.Minute})
	if err != nil {
		t.Fatal(err)
	}
	reqB := reqA
	reqB.AgentID = "agent-b"
	decisionB := kernel.Evaluate(reqB)
	if _, err := store.Authorize("workspace", reqB, decisionB, lease.ID); !errors.Is(err, ErrCapabilityLeaseScope) {
		t.Fatalf("wrong agent unexpectedly authorized: %v", err)
	}
	if result, err := store.Authorize("workspace", reqA, decisionA, lease.ID); err != nil || !result.Allowed {
		t.Fatalf("scope mismatch consumed lease: result=%+v err=%v", result, err)
	}
}

func TestCapabilityLeaseExpiresFailClosed(t *testing.T) {
	store, _ := NewCapabilityLeaseStore(t.TempDir())
	now := time.Unix(100, 0).UTC()
	store.now = func() time.Time { return now }
	kernel, _ := NewPolicyKernel(nil, NetworkAllow)
	req := PolicyRequest{ActorID: "user", Action: "run", SecretMode: SecretConsume, SecretNames: []string{"TOKEN"}, SecretOutput: SecretOutputRedacted}
	decision := kernel.Evaluate(req)
	lease, err := store.Issue(CapabilityLeaseIssue{ID: "expiring", WorkspaceKey: "workspace", ActorID: "user", ApprovalKey: decision.ApprovalKey, TTL: time.Second})
	if err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Second)
	if _, err := store.Authorize("workspace", req, decision, lease.ID); !errors.Is(err, ErrCapabilityLeaseExpired) {
		t.Fatalf("expired lease authorized: %v", err)
	}
}

func TestCapabilityLeaseCannotOverrideDeny(t *testing.T) {
	store, _ := NewCapabilityLeaseStore(t.TempDir())
	decision := PolicyDecision{Effect: EffectDeny, Blocked: true, RiskLevel: RiskBlocked}
	result, err := store.Authorize("workspace", PolicyRequest{ActorID: "user"}, decision, "anything")
	if err != nil {
		t.Fatal(err)
	}
	if result.Allowed || result.Reason != "policy_denied" {
		t.Fatalf("deny was weakened by lease path: %+v", result)
	}
}

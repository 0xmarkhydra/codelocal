package approval

import (
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

func TestWorkspaceAgentModePersistsLocallyAndEnvCanOverride(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("CODELOCAL_APPROVAL_MODE", "")

	if got := ResolveMode("workspace-a"); got != ModePrompt {
		t.Fatalf("default mode = %q, want prompt", got)
	}
	if err := SetWorkspaceMode("workspace-a", ModeAgent); err != nil {
		t.Fatal(err)
	}
	if got := ResolveMode("workspace-a"); got != ModeAgent {
		t.Fatalf("persisted mode = %q, want agent", got)
	}

	t.Setenv("CODELOCAL_APPROVAL_MODE", "deny")
	if got := ResolveMode("workspace-a"); got != ModeDeny {
		t.Fatalf("env override mode = %q, want deny", got)
	}
}

func TestAgentAllowsOnlyRememberableReviewOrHigh(t *testing.T) {
	for _, tc := range []struct {
		name     string
		decision security.Decision
		want     bool
	}{
		{name: "review rememberable", decision: security.Decision{RiskLevel: security.RiskReview, RequiresApproval: true, ApprovalPolicy: security.ApprovalRememberable, ApprovalKey: "git.commit"}, want: true},
		{name: "high rememberable", decision: security.Decision{RiskLevel: security.RiskHigh, RequiresApproval: true, ApprovalPolicy: security.ApprovalRememberable, ApprovalKey: "browser:interact:https://example.com"}, want: true},
		{name: "critical always", decision: security.Decision{RiskLevel: security.RiskCritical, RequiresApproval: true, ApprovalPolicy: security.ApprovalAlways}, want: false},
		{name: "review always", decision: security.Decision{RiskLevel: security.RiskReview, RequiresApproval: true, ApprovalPolicy: security.ApprovalAlways}, want: false},
		{name: "blocked", decision: security.Decision{RiskLevel: security.RiskBlocked, Blocked: true, ApprovalPolicy: security.ApprovalBlocked}, want: false},
		{name: "safe", decision: security.Decision{RiskLevel: security.RiskSafe, RequiresApproval: false, ApprovalPolicy: security.ApprovalNone}, want: false},
	}
	for _, tc := range tc {
		t.Run(tc.name, func(t *testing.T) {
			if got := AgentAllows(ModeAgent, tc.decision); got != tc.want {
				t.Fatalf("AgentAllows() = %v, want %v", got, tc.want)
			}
			if AgentAllows(ModePrompt, tc.decision) {
				t.Fatal("prompt mode must never auto-approve")
			}
		})
	}
}

func TestDenyModesFailClosedForApprovalRequiredActions(t *testing.T) {
	decision := security.Decision{RiskLevel: security.RiskReview, RequiresApproval: true, ApprovalPolicy: security.ApprovalRememberable}
	if !DeniesApproval(ModeDeny, decision) || !DeniesApproval(ModeAutoSafe, decision) {
		t.Fatal("deny and auto-safe must fail closed for approval-requiring actions")
	}
	if DeniesApproval(ModePrompt, decision) || DeniesApproval(ModeAgent, decision) {
		t.Fatal("prompt and agent modes should not be treated as blanket deny")
	}
}

package approval

import (
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

func TestWorkspaceAccessModePersistsLocallyAndOverridesDefaultEnv(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("CODELOCAL_APPROVAL_MODE", "smart")

	if got := ResolveMode("workspace-a"); got != ModeAgent {
		t.Fatalf("env default mode = %q, want agent", got)
	}
	if err := SetWorkspaceMode("workspace-a", ModeFull); err != nil {
		t.Fatal(err)
	}
	if got := ResolveMode("workspace-a"); got != ModeFull {
		t.Fatalf("persisted workspace mode = %q, want full", got)
	}
	if got := UserMode(ResolveMode("workspace-a")); got != "full" {
		t.Fatalf("user mode = %q, want full", got)
	}

	t.Setenv("CODELOCAL_APPROVAL_MODE", "deny")
	if got := ResolveMode("workspace-a"); got != ModeDeny {
		t.Fatalf("restrictive env kill-switch = %q, want deny", got)
	}

	if mode, ok := ParseUserMode("smart"); !ok || mode != ModeAgent {
		t.Fatalf("smart alias = %q ok=%v, want agent", mode, ok)
	}
	if UserModeLabel(ModePrompt) != "Yêu cầu phê duyệt" || UserModeLabel(ModeAgent) != "Phê duyệt giúp tôi" || UserModeLabel(ModeFull) != "Toàn quyền truy cập" {
		t.Fatal("chat-facing access labels changed")
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
	} {
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

func TestFullAllowsEveryNonBlockedApprovalDecision(t *testing.T) {
	for _, decision := range []security.Decision{
		{RiskLevel: security.RiskReview, RequiresApproval: true, ApprovalPolicy: security.ApprovalRememberable},
		{RiskLevel: security.RiskHigh, RequiresApproval: true, ApprovalPolicy: security.ApprovalAlways},
		{RiskLevel: security.RiskCritical, RequiresApproval: true, ApprovalPolicy: security.ApprovalAlways},
	} {
		if !FullAllows(ModeFull, decision) {
			t.Fatalf("full mode should approve non-blocked decision: %+v", decision)
		}
	}
	blocked := security.Decision{RiskLevel: security.RiskBlocked, RequiresApproval: false, Blocked: true, ApprovalPolicy: security.ApprovalBlocked}
	if FullAllows(ModeFull, blocked) {
		t.Fatal("full mode must never bypass deterministic hard blocks")
	}
	if FullAllows(ModeAgent, security.Decision{RiskLevel: security.RiskCritical, RequiresApproval: true, ApprovalPolicy: security.ApprovalAlways}) {
		t.Fatal("agent/smart mode must not inherit full-access semantics")
	}
}

func TestDenyModesFailClosedForApprovalRequiredActions(t *testing.T) {
	decision := security.Decision{RiskLevel: security.RiskReview, RequiresApproval: true, ApprovalPolicy: security.ApprovalRememberable}
	if !DeniesApproval(ModeDeny, decision) || !DeniesApproval(ModeAutoSafe, decision) {
		t.Fatal("deny and auto-safe must fail closed for approval-requiring actions")
	}
	if DeniesApproval(ModePrompt, decision) || DeniesApproval(ModeAgent, decision) || DeniesApproval(ModeFull, decision) {
		t.Fatal("prompt, smart and full modes should not be treated as blanket deny")
	}
}

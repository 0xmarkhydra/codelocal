package approval

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

func testRememberableDecision() security.Decision {
	return security.Decision{
		RiskLevel:        security.RiskReview,
		MatchedRules:     []string{"workspace code execution"},
		RequiresApproval: true,
		RedactedCommand:  "go test ./...",
		Reason:           "workspace code execution",
		ApprovalPolicy:   security.ApprovalRememberable,
		ApprovalKey:      "workspace-exec:test",
		ApprovalLabel:    "go test ./...",
	}
}

func TestRememberedApprovalIsSessionRiskAndTTLScoped(t *testing.T) {
	memory := &Memory{RootDir: filepath.Join(t.TempDir(), "approvals"), ttl: 30 * time.Minute}
	decision := testRememberableDecision()
	entry, err := memory.Remember("workspace-a", "session-a", decision)
	if err != nil || entry == nil {
		t.Fatalf("remember approval: entry=%#v err=%v", entry, err)
	}
	if entry.ExpiresAt <= entry.CreatedAt || entry.SessionID != "session-a" {
		t.Fatalf("grant scope/expiry missing: %#v", entry)
	}
	if got, err := memory.Find("workspace-a", "session-a", decision.ApprovalKey, security.RiskReview); err != nil || got == nil {
		t.Fatalf("same session/risk should reuse grant: got=%#v err=%v", got, err)
	}
	if got, err := memory.Find("workspace-a", "session-b", decision.ApprovalKey, security.RiskReview); err != nil || got != nil {
		t.Fatalf("grant leaked across session: got=%#v err=%v", got, err)
	}
	if got, err := memory.Find("workspace-a", "session-a", decision.ApprovalKey, security.RiskHigh); err != nil || got != nil {
		t.Fatalf("review grant exceeded its risk ceiling: got=%#v err=%v", got, err)
	}
}

func TestRememberedApprovalExpiresAndLegacyGrantIsNotReusable(t *testing.T) {
	memory := &Memory{RootDir: filepath.Join(t.TempDir(), "approvals"), ttl: time.Millisecond}
	decision := testRememberableDecision()
	entry, err := memory.Remember("workspace-a", "session-a", decision)
	if err != nil || entry == nil {
		t.Fatalf("remember approval: entry=%#v err=%v", entry, err)
	}
	time.Sleep(3 * time.Millisecond)
	if got, err := memory.Find("workspace-a", "session-a", decision.ApprovalKey, security.RiskReview); err != nil || got != nil {
		t.Fatalf("expired grant remained reusable: got=%#v err=%v", got, err)
	}

	legacy := Remembered{
		ID: "legacy", WorkspaceKey: "workspace-a", ActionKey: "workspace-exec:legacy",
		RiskLevel: string(security.RiskReview), CreatedAt: time.Now().UnixMilli(), LastUsedAt: time.Now().UnixMilli(),
	}
	if err := memory.write("workspace-a", []Remembered{legacy}); err != nil {
		t.Fatal(err)
	}
	if got, err := memory.Find("workspace-a", "session-a", legacy.ActionKey, security.RiskReview); err != nil || got != nil {
		t.Fatalf("legacy workspace-only approval was auto-reused: got=%#v err=%v", got, err)
	}
}

func TestCorruptRiskLevelFailsClosed(t *testing.T) {
	now := time.Now().UnixMilli()
	entry := Remembered{SessionID: "session-a", ActionKey: "workspace-exec:test", RiskLevel: "UNKNOWN", ExpiresAt: now + 60_000}
	if rememberedAllows(entry, "session-a", entry.ActionKey, security.RiskReview, now) {
		t.Fatal("corrupt approval risk must fail closed")
	}
}

func TestRememberWithoutChatSessionNeverCreatesReusableGrant(t *testing.T) {
	memory := &Memory{RootDir: filepath.Join(t.TempDir(), "approvals"), ttl: 30 * time.Minute}
	entry, err := memory.Remember("workspace-a", "", testRememberableDecision())
	if err != nil || entry != nil {
		t.Fatalf("sessionless approval must remain one-time: entry=%#v err=%v", entry, err)
	}
}

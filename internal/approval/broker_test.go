package approval

import (
	"strings"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

func reviewDecision() security.Decision {
	return security.Decision{
		RiskLevel:        security.RiskReview,
		MatchedRules:     []string{"workspace code execution"},
		RequiresApproval: true,
		RedactedCommand:  "go test ./internal/example",
		Reason:           "workspace code execution",
		ApprovalPolicy:   security.ApprovalRememberable,
		ApprovalKey:      "workspace-exec:test",
		ApprovalLabel:    "go test ./internal/example",
	}
}

func TestBrokerReusesPendingTokenWithinSameSessionFingerprint(t *testing.T) {
	broker := NewBroker()
	decision := reviewDecision()
	first := broker.PreflightScoped("session-a", "go test ./internal/example", ".", decision)
	second := broker.PreflightScoped("session-a", "go test ./internal/example", ".", decision)
	if first.ApprovalToken == "" || first.ApprovalToken != second.ApprovalToken {
		t.Fatalf("same pending approval rotated token: first=%q second=%q", first.ApprovalToken, second.ApprovalToken)
	}
	if first.ExpiresAt != second.ExpiresAt {
		t.Fatalf("same pending approval unexpectedly extended expiry: %d != %d", first.ExpiresAt, second.ExpiresAt)
	}
	if first.ExpiresAt-time.Now().UnixMilli() < int64(20*time.Minute/time.Millisecond) {
		t.Fatalf("default human approval window is unexpectedly short: expiresAt=%d", first.ExpiresAt)
	}
}

func TestBrokerBindsPendingTokenToSessionAndConsumesOnce(t *testing.T) {
	broker := NewBroker()
	decision := reviewDecision()
	command := "go test ./internal/example"
	first := broker.PreflightScoped("session-a", command, ".", decision)
	other := broker.PreflightScoped("session-b", command, ".", decision)
	if first.ApprovalToken == other.ApprovalToken {
		t.Fatal("approval token must be session scoped")
	}
	if broker.ConsumeScoped("session-b", first.ApprovalToken, command, ".", decision) {
		t.Fatal("token from another session was accepted")
	}
	if !broker.ConsumeScoped("session-a", first.ApprovalToken, command, ".", decision) {
		t.Fatal("legitimate token was invalidated by a wrong-session attempt")
	}
	if broker.ConsumeScoped("session-a", first.ApprovalToken, command, ".", decision) {
		t.Fatal("one-time token replay was accepted")
	}
	next := broker.PreflightScoped("session-a", command, ".", decision)
	if next.ApprovalToken == first.ApprovalToken {
		t.Fatal("consumed token was reused for a new approval")
	}
}

func TestBrokerWrongSecretDoesNotInvalidatePendingApproval(t *testing.T) {
	broker := NewBroker()
	decision := reviewDecision()
	command := "go test ./internal/example"
	pending := broker.PreflightScoped("session-a", command, ".", decision)
	parts := strings.SplitN(pending.ApprovalToken, ".", 2)
	if len(parts) != 2 {
		t.Fatalf("invalid token fixture: %q", pending.ApprovalToken)
	}
	if broker.ConsumeScoped("session-a", parts[0]+".deadbeef", command, ".", decision) {
		t.Fatal("wrong token secret was accepted")
	}
	if !broker.ConsumeScoped("session-a", pending.ApprovalToken, command, ".", decision) {
		t.Fatal("wrong-secret attempt invalidated the real pending approval")
	}
}

func TestBrokerTTLRemainsConfigurable(t *testing.T) {
	t.Setenv("CODELOCAL_CHAT_APPROVAL_TTL_MS", "60000")
	broker := NewBroker()
	pending := broker.PreflightScoped("session-a", "go test ./internal/example", ".", reviewDecision())
	remaining := pending.ExpiresAt - time.Now().UnixMilli()
	if remaining < 50_000 || remaining > 65_000 {
		t.Fatalf("configured approval TTL not respected: %dms", remaining)
	}
}

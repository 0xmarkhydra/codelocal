package security

import (
	"path/filepath"
	"testing"
)

func TestPolicyBlocksEscapesAndRemembersRoutineWorkspaceExecution(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(filepath.Dir(root), "outside")
	blocked := Classify("cat "+outside, NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if !blocked.Blocked || blocked.RiskLevel != RiskBlocked {
		t.Fatalf("expected workspace escape to be blocked: %#v", blocked)
	}

	review := Classify("go test ./...", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if !review.RequiresApproval || review.ApprovalPolicy != ApprovalRememberable || review.ApprovalKey == "" {
		t.Fatalf("expected workspace code execution to be rememberable review: %#v", review)
	}

	critical := Classify("git push --force origin main", NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
	if critical.Blocked || critical.RiskLevel != RiskCritical || critical.ApprovalPolicy != ApprovalAlways {
		t.Fatalf("force push must require fresh critical approval: %#v", critical)
	}
}

func TestRedaction(t *testing.T) {
	redacted := RedactCommand("curl -H 'Authorization: Bearer abc123' https://example.test?token=secret")
	if redacted == "" || redacted == "curl -H 'Authorization: Bearer abc123' https://example.test?token=secret" {
		t.Fatalf("expected secrets to be redacted: %q", redacted)
	}
}

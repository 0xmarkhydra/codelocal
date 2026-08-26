package security

import (
	"path/filepath"
	"strings"
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

func TestPolicyBlocksCrossPlatformAbsolutePaths(t *testing.T) {
	root := t.TempDir()
	for _, candidate := range []string{"/Users/me/private", "C:/Users/me/private", `C:\Users\me\private`, `..\outside`} {
		decision := Classify("cat "+candidate, NetworkApproval, Context{WorkspaceRoot: root, CWD: root})
		if !decision.Blocked || decision.RiskLevel != RiskBlocked {
			t.Fatalf("expected cross-platform path %q to be blocked: %#v", candidate, decision)
		}
	}
}

func TestRedaction(t *testing.T) {
	redacted := RedactCommand("curl -H 'Authorization: Bearer abc123' https://example.test?token=secret")
	if redacted == "" || redacted == "curl -H 'Authorization: Bearer abc123' https://example.test?token=secret" {
		t.Fatalf("expected secrets to be redacted: %q", redacted)
	}
}

func TestSanitizeEnvironment(t *testing.T) {
	env := []string{
		"PATH=/usr/bin:/bin",
		"HOME=/Users/test",
		"USER=testuser",
		"LANG=en_US.UTF-8",
		"OPENAI_API_KEY=sk-test-12345",
		"AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY",
		"AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE",
		"GITHUB_TOKEN=ghp_exampletoken123",
		"DB_PASSWORD=supersecret",
		"AUTH_COOKIE=session=xyz",
		"MY_PRIVATE_KEY=pemdata",
	}

	sanitized := SanitizeEnvironment(env)
	joined := strings.Join(sanitized, "\n")

	for _, keep := range []string{"PATH=/usr/bin:/bin", "HOME=/Users/test", "USER=testuser", "LANG=en_US.UTF-8"} {
		if !strings.Contains(joined, keep) {
			t.Fatalf("expected safe env %q to be preserved in:\n%s", keep, joined)
		}
	}

	for _, drop := range []string{"OPENAI_API_KEY", "AWS_SECRET_ACCESS_KEY", "AWS_ACCESS_KEY_ID", "GITHUB_TOKEN", "DB_PASSWORD", "AUTH_COOKIE", "MY_PRIVATE_KEY"} {
		if strings.Contains(joined, drop) {
			t.Fatalf("expected sensitive env %q to be stripped from:\n%s", drop, joined)
		}
	}
}

func TestPolicyStillBlocksDirectSecretExfiltration(t *testing.T) {
	root := t.TempDir()
	ctx := Context{WorkspaceRoot: root, CWD: root}
	for _, command := range []string{
		"echo $VBEE_ACCESS_TOKEN",
		"printenv VBEE_ACCESS_TOKEN",
		"env",
	} {
		decision := Classify(command, NetworkApproval, ctx)
		if !decision.Blocked || decision.RiskLevel != RiskBlocked {
			t.Fatalf("direct secret exfiltration %q must remain blocked: %#v", command, decision)
		}
	}
}

func TestPolicyChainedAndCommands(t *testing.T) {
	root := t.TempDir()
	ctx := Context{WorkspaceRoot: root, CWD: root}

	// 1. Chained routine commands -> RiskReview & rememberable
	chainReview := Classify("go test ./... && go vet ./...", NetworkApproval, ctx)
	if chainReview.Blocked || chainReview.RiskLevel != RiskReview || !chainReview.RequiresApproval || chainReview.ApprovalPolicy != ApprovalRememberable || chainReview.ApprovalKey == "" {
		t.Fatalf("expected routine chained command to be rememberable review: %#v", chainReview)
	}

	// 2. Chained safe commands -> RiskSafe & no approval
	chainSafe := Classify("echo 1 && echo 2", NetworkApproval, ctx)
	if chainSafe.Blocked || chainSafe.RiskLevel != RiskSafe || chainSafe.RequiresApproval || chainSafe.ApprovalPolicy != ApprovalNone {
		t.Fatalf("expected chained safe commands to be safe: %#v", chainSafe)
	}

	// 3. Chained command containing a blocked action -> RiskBlocked
	chainBlocked := Classify("echo 1 && sudo rm -rf /", NetworkApproval, ctx)
	if !chainBlocked.Blocked || chainBlocked.RiskLevel != RiskBlocked || chainBlocked.ApprovalPolicy != ApprovalBlocked {
		t.Fatalf("expected chained command with sudo to be blocked: %#v", chainBlocked)
	}

	// 4. Chained command containing a critical action -> RiskCritical & ApprovalAlways
	chainCritical := Classify("npm test && git push --force origin main", NetworkApproval, ctx)
	if chainCritical.Blocked || chainCritical.RiskLevel != RiskCritical || !chainCritical.RequiresApproval || chainCritical.ApprovalPolicy != ApprovalAlways {
		t.Fatalf("expected chained command with force push to be critical: %#v", chainCritical)
	}

	// 5. Shell composition with pipes -> still classified as critical composition
	pipeCmd := Classify("go test ./... | grep FAIL", NetworkApproval, ctx)
	if pipeCmd.Blocked || pipeCmd.RiskLevel != RiskCritical || pipeCmd.ApprovalPolicy != ApprovalAlways {
		t.Fatalf("expected piped command to require critical approval: %#v", pipeCmd)
	}
}

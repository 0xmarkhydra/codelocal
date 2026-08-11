package automation

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

func TestValidateBrowserURL(t *testing.T) {
	if got, err := validateBrowserURL("http://localhost:3000/test"); err != nil || got == "" {
		t.Fatalf("localhost URL rejected: %q %v", got, err)
	}
	for _, value := range []string{"file:///tmp/a", "javascript:alert(1)", "https://user:pass@example.com/"} {
		if _, err := validateBrowserURL(value); err == nil {
			t.Fatalf("unsafe URL %q should be rejected", value)
		}
	}
}

func TestBrowserLocalURL(t *testing.T) {
	for _, value := range []string{"http://localhost:3000", "https://127.0.0.1:8443", "http://[::1]:8080"} {
		if !BrowserLocalURL(value) {
			t.Fatalf("expected local URL: %s", value)
		}
	}
	if BrowserLocalURL("https://example.com") {
		t.Fatal("external site must not be treated as localhost")
	}
}

func TestAutomationPolicy(t *testing.T) {
	local := ClassifyAutomation(Action{Domain: "browser", Operation: "open", Target: "http://localhost:3000"})
	if local.RiskLevel != security.RiskSafe || local.RequiresApproval {
		t.Fatalf("localhost navigation should be safe: %+v", local)
	}
	external := ClassifyAutomation(Action{Domain: "browser", Operation: "open", Target: "https://example.com"})
	if external.RiskLevel != security.RiskReview || !external.RequiresApproval || external.ApprovalPolicy != security.ApprovalRememberable {
		t.Fatalf("external navigation policy mismatch: %+v", external)
	}
	click := ClassifyAutomation(Action{Domain: "browser", Operation: "click", Origin: "https://example.com", Target: "Open menu"})
	if click.RiskLevel != security.RiskHigh || !click.RequiresApproval {
		t.Fatalf("browser click should require high-risk approval: %+v", click)
	}
	payment := ClassifyAutomation(Action{Domain: "browser", Operation: "click", Origin: "https://shop.example", Target: "Confirm payment"})
	if payment.RiskLevel != security.RiskCritical || payment.ApprovalPolicy != security.ApprovalAlways {
		t.Fatalf("payment must require fresh critical approval: %+v", payment)
	}
	extract := ClassifyAutomation(Action{Domain: "browser", Operation: "read", Target: "password field"})
	if !extract.Blocked || extract.RiskLevel != security.RiskBlocked {
		t.Fatalf("credential extraction should be blocked: %+v", extract)
	}
}

func TestComputerHelperPathUsesPackagedHelper(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "bin", "helpers", computerHelperName())
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("stub"), 0o700); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_PACKAGE_ROOT", root)
	t.Setenv("CODELOCAL_COMPUTER_HELPER", "")
	if got := ComputerHelperPath(); got != path {
		t.Fatalf("ComputerHelperPath() = %q, want %q", got, path)
	}
}

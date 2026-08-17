package automation

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/approval"
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

func TestAutomationAuthorizerScopesRememberedGrantToChatSession(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	authorizer := NewAuthorizer("device::workspace")
	action := Action{Domain: "browser", Operation: "open", Origin: "https://example.com", Target: "https://example.com"}

	approved, pending, err := authorizer.AuthorizeScoped(action, "", "session-a")
	if err != nil || approved || pending["status"] != "approval_required" {
		t.Fatalf("initial approval state: approved=%v pending=%#v err=%v", approved, pending, err)
	}
	token, _ := pending["approvalToken"].(string)
	if token == "" {
		t.Fatal("missing approval token")
	}
	approved, confirmed, err := authorizer.AuthorizeScoped(action, token, "session-a")
	if err != nil || !approved || confirmed["remembered"] != true {
		t.Fatalf("confirmation did not create scoped grant: approved=%v state=%#v err=%v", approved, confirmed, err)
	}
	approved, reused, err := authorizer.AuthorizeScoped(action, "", "session-a")
	if err != nil || !approved || reused["remembered"] != true {
		t.Fatalf("same-session grant not reused: approved=%v state=%#v err=%v", approved, reused, err)
	}
	approved, otherSession, err := authorizer.AuthorizeScoped(action, "", "session-b")
	if err != nil || approved || otherSession["status"] != "approval_required" {
		t.Fatalf("approval leaked across sessions: approved=%v state=%#v err=%v", approved, otherSession, err)
	}
}

func TestAutomationApprovalTokenSurvivesMCPCallSessionChurn(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	authorizer := NewAuthorizer("device::workspace")
	action := Action{Domain: "browser", Operation: "open", Origin: "https://example.com", Target: "https://example.com"}

	approved, pending, err := authorizer.AuthorizeScoped(action, "", "call-session-a")
	if err != nil || approved || pending["status"] != "approval_required" {
		t.Fatalf("initial approval state: approved=%v pending=%#v err=%v", approved, pending, err)
	}
	token, _ := pending["approvalToken"].(string)
	if token == "" {
		t.Fatal("missing approval token")
	}

	approved, confirmed, err := authorizer.AuthorizeScoped(action, token, "call-session-b")
	if err != nil || !approved || confirmed["remembered"] != true {
		t.Fatalf("exact-action token should survive MCP call session churn: approved=%v state=%#v err=%v", approved, confirmed, err)
	}
}

func TestAutomationApprovalTokenStillRejectsDifferentActionAcrossSessionChurn(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	authorizer := NewAuthorizer("device::workspace")
	openAction := Action{Domain: "browser", Operation: "open", Origin: "https://example.com", Target: "https://example.com"}
	otherAction := Action{Domain: "browser", Operation: "open", Origin: "https://other.example", Target: "https://other.example"}

	approved, pending, err := authorizer.AuthorizeScoped(openAction, "", "call-session-a")
	if err != nil || approved {
		t.Fatalf("initial approval state: approved=%v pending=%#v err=%v", approved, pending, err)
	}
	token, _ := pending["approvalToken"].(string)
	approved, next, err := authorizer.AuthorizeScoped(otherAction, token, "call-session-b")
	if err != nil || approved || next["status"] != "approval_required" {
		t.Fatalf("token must stay exact-action-bound: approved=%v state=%#v err=%v", approved, next, err)
	}
}

func TestAgentModeAutoApprovesOnlyScopedRoutineAutomation(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	t.Setenv("CODELOCAL_APPROVAL_MODE", "")
	const workspace = "device::workspace"
	if err := approval.SetWorkspaceMode(workspace, approval.ModeAgent); err != nil {
		t.Fatal(err)
	}
	authorizer := NewAuthorizer(workspace)

	open := Action{Domain: "browser", Operation: "open", Origin: "https://example.com", Target: "https://example.com"}
	approved, state, err := authorizer.AuthorizeScoped(open, "", "session-a")
	if err != nil || !approved || state["agentApproved"] != true {
		t.Fatalf("agent mode should auto-approve scoped navigation: approved=%v state=%#v err=%v", approved, state, err)
	}

	click := Action{Domain: "computer", Operation: "click", Origin: "ax:123:0", Target: "Save"}
	approved, state, err = authorizer.AuthorizeScoped(click, "", "session-b")
	if err != nil || !approved || state["agentApproved"] != true {
		t.Fatalf("agent mode should auto-approve scoped semantic desktop input: approved=%v state=%#v err=%v", approved, state, err)
	}

	payment := Action{Domain: "browser", Operation: "click", Origin: "https://shop.example", Target: "Confirm payment"}
	approved, pending, err := authorizer.AuthorizeScoped(payment, "", "session-c")
	if err != nil || approved || pending["approvalPolicy"] != security.ApprovalAlways {
		t.Fatalf("critical browser action must still require fresh confirmation: approved=%v state=%#v err=%v", approved, pending, err)
	}
}

func TestCredentialEntryRemainsFreshApprovalInAgentMode(t *testing.T) {
	for _, action := range []Action{
		{Domain: "browser", Operation: "fill", Origin: "https://example.com", Target: "Password", Text: "secret-value"},
		{Domain: "browser", Operation: "press", Origin: "https://example.com", Target: "OTP input", Text: "Enter"},
		{Domain: "computer", Operation: "type", Origin: "ax:123:0", Target: "API key", Text: "secret-value"},
		{Domain: "computer", Operation: "run", Origin: "ax:123:0", Target: "click:Login -> type:Password", Text: "secret-value"},
	} {
		decision := ClassifyAutomation(action)
		if decision.RiskLevel != security.RiskCritical || decision.ApprovalPolicy != security.ApprovalAlways {
			t.Fatalf("credential entry must require fresh confirmation: action=%+v decision=%+v", action, decision)
		}
	}
}

func TestAutomationAlwaysApprovalNeverBecomesRemembered(t *testing.T) {
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	authorizer := NewAuthorizer("device::workspace")
	action := Action{Domain: "browser", Operation: "click", Origin: "https://shop.example", Target: "Confirm payment"}

	approved, pending, err := authorizer.AuthorizeScoped(action, "", "session-a")
	if err != nil || approved || pending["approvalPolicy"] != security.ApprovalAlways {
		t.Fatalf("critical action did not require fresh approval: approved=%v state=%#v err=%v", approved, pending, err)
	}
	token, _ := pending["approvalToken"].(string)
	approved, _, err = authorizer.AuthorizeScoped(action, token, "session-a")
	if err != nil || !approved {
		t.Fatalf("critical one-time approval failed: approved=%v err=%v", approved, err)
	}
	approved, next, err := authorizer.AuthorizeScoped(action, "", "session-a")
	if err != nil || approved || next["approvalPolicy"] != security.ApprovalAlways {
		t.Fatalf("critical approval was incorrectly remembered: approved=%v state=%#v err=%v", approved, next, err)
	}
}

func TestComputerPolicyScopesRememberedInputByWindow(t *testing.T) {
	decision := ClassifyAutomation(Action{Domain: "computer", Operation: "click", Origin: "ax:123:0", Target: "Save"})
	if decision.RiskLevel != security.RiskHigh || !decision.RequiresApproval || decision.ApprovalPolicy != security.ApprovalRememberable {
		t.Fatalf("scoped desktop click should be rememberable high risk: %+v", decision)
	}
	if decision.ApprovalKey != "computer:input:ax:123:0" {
		t.Fatalf("unexpected scoped approval key: %q", decision.ApprovalKey)
	}
}

func TestComputerPolicyRequiresFreshApprovalForPhysicalFallback(t *testing.T) {
	decision := ClassifyAutomation(Action{Domain: "computer", Operation: "click", Origin: "ax:123:0", Target: "Custom icon", Physical: true})
	if decision.RiskLevel != security.RiskCritical || decision.ApprovalPolicy != security.ApprovalAlways || !decision.RequiresApproval {
		t.Fatalf("physical fallback should require fresh approval: %+v", decision)
	}
	if !strings.Contains(decision.Reason, "physical desktop input") {
		t.Fatalf("physical fallback should explain user-input interference: %+v", decision)
	}
}

func TestScreenMainInputIsPhysicalBeforeFirstApproval(t *testing.T) {
	for _, operation := range []string{"click", "type"} {
		args := map[string]any{"windowId": "screen:main"}
		target := "Custom control"
		if !computerActionUsesPhysicalInput(operation, args, target) {
			t.Fatalf("screen:main %s must be classified as physical before authorization", operation)
		}
		decision := ClassifyAutomation(Action{Domain: "computer", Operation: operation, Origin: "screen:main", Target: target, Physical: true})
		if decision.ApprovalPolicy != security.ApprovalAlways || !strings.Contains(decision.Reason, "physical desktop input") {
			t.Fatalf("screen:main %s should issue one physical-input approval challenge: %+v", operation, decision)
		}
	}
}

func TestComputerPolicyRequiresFreshApprovalForUnscopedInput(t *testing.T) {
	for _, action := range []Action{
		{Domain: "computer", Operation: "click", Origin: "ax:123:0"},
		{Domain: "computer", Operation: "click", Origin: "screen:main", Target: "Unknown icon"},
		{Domain: "computer", Operation: "type", Text: "hello"},
	} {
		decision := ClassifyAutomation(action)
		if decision.RiskLevel != security.RiskCritical || decision.ApprovalPolicy != security.ApprovalAlways || !decision.RequiresApproval {
			t.Fatalf("unscoped input should require fresh critical approval: action=%+v decision=%+v", action, decision)
		}
	}
}

func TestComputerPolicyScopesObservationAndScreenshot(t *testing.T) {
	observe := ClassifyAutomation(Action{Domain: "computer", Operation: "observe", Origin: "ax:77:1"})
	if observe.RiskLevel != security.RiskReview || observe.ApprovalKey != "computer:observe:ax:77:1" {
		t.Fatalf("unexpected observe policy: %+v", observe)
	}
	screenshot := ClassifyAutomation(Action{Domain: "computer", Operation: "screenshot", Origin: "ax:77:1"})
	if screenshot.RiskLevel != security.RiskReview || screenshot.ApprovalKey != "computer:screenshot:ax:77:1" {
		t.Fatalf("unexpected screenshot policy: %+v", screenshot)
	}
	windows := ClassifyAutomation(Action{Domain: "computer", Operation: "list_windows"})
	if windows.RequiresApproval || windows.RiskLevel != security.RiskSafe {
		t.Fatalf("window listing should remain safe: %+v", windows)
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

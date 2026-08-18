package automation

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

func TestComputerFocusAlwaysRequiresFreshApproval(t *testing.T) {
	decision := ClassifyAutomation(Action{Domain: "computer", Operation: "focus", Origin: "ax:123:0"})
	if decision.RiskLevel != security.RiskCritical || decision.ApprovalPolicy != security.ApprovalAlways || !decision.RequiresApproval {
		t.Fatalf("foreground focus must require fresh critical approval: %+v", decision)
	}
	if !strings.Contains(decision.Reason, "interrupt") {
		t.Fatalf("foreground focus should explain user interruption risk: %+v", decision)
	}
}

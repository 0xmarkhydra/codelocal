package automation

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

func TestComputerElementSensitiveUsesRoleAndSemanticMetadata(t *testing.T) {
	cases := []struct {
		name     string
		metadata map[string]any
		target   string
		want     bool
	}{
		{name: "secure role", metadata: map[string]any{"role": "AXSecureTextField", "name": "Sign in"}, target: "Login", want: true},
		{name: "otp label", metadata: map[string]any{"role": "AXTextField", "name": "Verification code"}, target: "Code", want: true},
		{name: "normal field", metadata: map[string]any{"role": "AXTextField", "name": "Project name"}, target: "Project name", want: false},
		{name: "unknown metadata", metadata: nil, target: "Field", want: true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := computerElementSensitive(tc.metadata, tc.target); got != tc.want {
				t.Fatalf("computerElementSensitive()=%v want=%v metadata=%#v", got, tc.want, tc.metadata)
			}
		})
	}
}

func TestSensitiveTargetForcesFreshApproval(t *testing.T) {
	for _, action := range []Action{
		{Domain: "browser", Operation: "fill", Origin: "https://example.com", Target: "e17", SensitiveTarget: true},
		{Domain: "computer", Operation: "type", Origin: "ax:1:0", Target: "Login", SensitiveTarget: true},
		{Domain: "computer", Operation: "run", Origin: "ax:1:0", Target: "type:Name", SensitiveTarget: true},
	} {
		decision := ClassifyAutomation(action)
		if decision.RiskLevel != security.RiskCritical || decision.ApprovalPolicy != security.ApprovalAlways || !decision.RequiresApproval {
			t.Fatalf("sensitive/uncertain input target must require fresh approval: action=%+v decision=%+v", action, decision)
		}
	}
}

package security

import (
	"errors"
	"strings"
	"testing"
)

func TestPolicyKernelSecretExposureAlwaysDenied(t *testing.T) {
	kernel, err := NewPolicyKernel(nil, NetworkAllow)
	if err != nil {
		t.Fatal(err)
	}
	cases := []PolicyRequest{
		{ActorID: "user", Action: "run", Command: "echo $VBEE_ACCESS_TOKEN"},
		{ActorID: "user", Action: "run", Command: "printenv VBEE_ACCESS_TOKEN"},
		{ActorID: "user", Action: "run", Command: `node -e "console.log(process.env.VBEE_ACCESS_TOKEN)"`},
		{ActorID: "user", Action: "run", Command: "node helper.js", SecretMode: SecretExpose, SecretNames: []string{"VBEE_ACCESS_TOKEN"}},
	}
	for _, req := range cases {
		decision := kernel.Evaluate(req)
		if decision.Effect != EffectDeny || !decision.Blocked || decision.SecretMode != SecretExpose {
			t.Fatalf("exposure was not denied for %q: %#v", req.Command, decision)
		}
		if strings.Contains(decision.RedactedCommand, "secret-value") {
			t.Fatalf("redacted command leaked secret content: %#v", decision)
		}
	}
}

func TestPolicyKernelInternalSecretConsumptionPromptsNotBlocks(t *testing.T) {
	kernel, _ := NewPolicyKernel(nil, NetworkAllow)
	decision := kernel.Evaluate(PolicyRequest{
		ActorID:      "user",
		AgentID:      "agent",
		Action:       "run",
		Tool:         "terminal",
		Command:      "node helper.js",
		SecretMode:   SecretConsume,
		SecretNames:  []string{"VBEE_ACCESS_TOKEN", "VBEE_APP_ID"},
		SecretOutput: SecretOutputNone,
	})
	if decision.Effect != EffectPrompt || !decision.RequiresApproval || decision.Blocked || decision.SecretMode != SecretConsume || decision.ApprovalKey == "" {
		t.Fatalf("approved-process candidate should prompt, not block: %#v", decision)
	}
	if !containsPolicyReason(decision.ReasonCodes, "secret_internal_consumption_requires_capability") {
		t.Fatalf("missing secret consumption reason: %#v", decision)
	}
}

func TestPolicyKernelRawSecretOutputDenied(t *testing.T) {
	kernel, _ := NewPolicyKernel(nil, NetworkAllow)
	decision := kernel.Evaluate(PolicyRequest{ActorID: "user", Action: "run", Command: "node helper.js", SecretMode: SecretConsume, SecretNames: []string{"TOKEN"}, SecretOutput: SecretOutputRaw})
	if decision.Effect != EffectDeny || !containsPolicyReason(decision.ReasonCodes, "secret_raw_output_blocked") {
		t.Fatalf("raw secret output not denied: %#v", decision)
	}
}

func TestPolicyKernelEnvironmentEnumerationDenied(t *testing.T) {
	kernel, _ := NewPolicyKernel(nil, NetworkAllow)
	decision := kernel.Evaluate(PolicyRequest{ActorID: "user", Action: "run", Command: "env"})
	if decision.Effect != EffectDeny || decision.SecretMode != SecretEnumerate {
		t.Fatalf("environment enumeration not denied: %#v", decision)
	}
}

func TestPolicyRulesAreMonotonicAndDeterministic(t *testing.T) {
	kernel, err := NewPolicyKernel([]PolicyRule{
		{ID: "allow-reviewer", Priority: 100, Effect: EffectAllow, Match: PolicyMatch{AgentRole: "reviewer"}},
		{ID: "prompt-edit", Priority: 50, Effect: EffectPrompt, Match: PolicyMatch{Action: "edit"}},
		{ID: "deny-generated", Priority: 10, Effect: EffectDeny, Match: PolicyMatch{PathPrefix: "generated/"}},
	}, NetworkAllow)
	if err != nil {
		t.Fatal(err)
	}
	blocked := kernel.Evaluate(PolicyRequest{ActorID: "u", AgentRole: "reviewer", Action: "run", Command: "env"})
	if blocked.Effect != EffectDeny {
		t.Fatalf("ALLOW rule loosened base DENY: %#v", blocked)
	}
	prompted := kernel.Evaluate(PolicyRequest{ActorID: "u", AgentRole: "reviewer", Action: "edit", Path: "src/a.go"})
	if prompted.Effect != EffectPrompt || len(prompted.RuleIDs) != 2 || prompted.RuleIDs[0] != "allow-reviewer" || prompted.RuleIDs[1] != "prompt-edit" {
		t.Fatalf("unexpected monotonic prompt decision: %#v", prompted)
	}
	denied := kernel.Evaluate(PolicyRequest{ActorID: "u", AgentRole: "reviewer", Action: "edit", Path: "generated/a.go"})
	if denied.Effect != EffectDeny || !denied.Blocked {
		t.Fatalf("deny rule did not tighten prompt: %#v", denied)
	}
}

func TestPolicyKernelNetworkAndSensitivePath(t *testing.T) {
	kernel, _ := NewPolicyKernel(nil, NetworkApproval)
	network := kernel.Evaluate(PolicyRequest{ActorID: "u", Action: "request", NetworkTarget: "https://api.example.com/v1"})
	if network.Effect != EffectPrompt || network.RiskLevel != RiskCritical {
		t.Fatalf("network approval = %#v", network)
	}
	path := kernel.Evaluate(PolicyRequest{ActorID: "u", Action: "read", Path: ".env"})
	if path.Effect != EffectDeny || path.RiskLevel != RiskBlocked {
		t.Fatalf("sensitive path = %#v", path)
	}
}

func TestPolicyKernelRejectsInvalidRuleConfiguration(t *testing.T) {
	_, err := NewPolicyKernel([]PolicyRule{{ID: "same", Effect: EffectAllow}, {ID: "same", Effect: EffectDeny}}, NetworkAllow)
	if !errors.Is(err, ErrInvalidPolicyKernel) {
		t.Fatalf("expected duplicate rule rejection, got %v", err)
	}
	_, err = NewPolicyKernel([]PolicyRule{{ID: "bad", Effect: "maybe"}}, NetworkAllow)
	if !errors.Is(err, ErrInvalidPolicyKernel) {
		t.Fatalf("expected bad effect rejection, got %v", err)
	}
}

func TestPolicyApprovalKeyIsSecretValueFreeAndStable(t *testing.T) {
	kernel, _ := NewPolicyKernel(nil, NetworkAllow)
	req := PolicyRequest{ActorID: "u", AgentID: "a", Action: "run", Tool: "terminal", Command: "node helper.js", SecretMode: SecretConsume, SecretNames: []string{"VBEE_ACCESS_TOKEN"}}
	first := kernel.Evaluate(req)
	second := kernel.Evaluate(req)
	if first.ApprovalKey == "" || first.ApprovalKey != second.ApprovalKey || strings.Contains(first.ApprovalKey, "VBEE") {
		t.Fatalf("approval key invalid: first=%#v second=%#v", first, second)
	}
}

func containsPolicyReason(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

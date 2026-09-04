package security

import "testing"

func TestParsePolicyDSLValidDocument(t *testing.T) {
	doc := "# pool rules" + "\n" +
		"allow tool:edit path:/workspace/ reason:\"edits allowed\" priority:10" + "\n" +
		"deny secret:expose" + "\n" +
		"prompt host:example.com risk:critical id:net-rule"
	rules, err := ParsePolicyDSL(doc)
	if err != nil {
		t.Fatalf("valid document rejected: %v", err)
	}
	if len(rules) != 3 {
		t.Fatalf("got %d rules, want 3", len(rules))
	}
	if rules[0].Effect != EffectAllow || rules[0].Match.Tool != "edit" || rules[0].Match.PathPrefix != "/workspace/" {
		t.Fatalf("first rule mismatch: %+v", rules[0])
	}
	if rules[0].Reason != "edits allowed" || rules[0].Priority != 10 || rules[0].ID != "dsl-2" {
		t.Fatalf("first rule metadata mismatch: %+v", rules[0])
	}
	if rules[1].Match.SecretMode != SecretExpose {
		t.Fatalf("second rule secret mismatch: %+v", rules[1])
	}
	if rules[2].ID != "net-rule" || rules[2].Match.MinRisk != RiskCritical {
		t.Fatalf("third rule mismatch: %+v", rules[2])
	}
	kernel, err := NewPolicyKernel(rules, NetworkApproval)
	if err != nil {
		t.Fatalf("parsed rules rejected by kernel: %v", err)
	}
	decision := kernel.Evaluate(PolicyRequest{ActorID: "a", Tool: "edit", Path: "/workspace/main.go"})
	if len(decision.RuleIDs) == 0 {
		t.Fatal("parsed allow rule did not match evaluation")
	}
}

func TestParsePolicyDSLRejects(t *testing.T) {
	cases := map[string]string{
		"unknown effect":     "permit tool:edit",
		"unknown selector":   "allow frobnicate:x",
		"malformed selector": "allow bareword",
		"bad secret":         "deny secret:everything",
		"bad risk":           "prompt risk:cosmic",
		"unquoted reason":    "allow reason:because",
		"unterminated quote": "allow reason:\"oops",
		"bad priority":       "allow priority:high",
		"duplicate id":       "allow id:x\n" + "deny id:x",
		"duplicate selector": "allow tool:a tool:b",
	}
	for name, doc := range cases {
		if _, err := ParsePolicyDSL(doc); err == nil {
			t.Fatalf("%s: expected error, got nil", name)
		}
	}
}

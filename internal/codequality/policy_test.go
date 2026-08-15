package codequality

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultPolicyIsAdvisoryAndProjectFriendly(t *testing.T) {
	policy := DefaultPolicy()
	goPolicy, ok := policy.Languages["go"]
	if !ok {
		t.Fatal("default policy lost Go foundation")
	}
	if goPolicy.MaxFunctionLines == 10 {
		t.Fatal("user example of 10 lines/function must not become a universal default")
	}
	for _, rule := range []string{RuleMaxFileLines, RuleMaxFunctionLines, RuleMaxNestingDepth, RuleMaxCyclomaticComplexity, RuleMaxParameters, RuleSingleResponsibility, RuleOrchestratorOwnership} {
		if SeverityFor(goPolicy, rule) != SeverityAdvisory {
			t.Fatalf("default rule %s unexpectedly blocks repositories", rule)
		}
	}
	if !policy.Exemptions.Generated || !policy.Exemptions.Tests || !policy.Exemptions.Migrations {
		t.Fatalf("safe default exemptions lost: %#v", policy.Exemptions)
	}
}

func TestProjectQualityConfigCanSetStrictThresholdsAndBlockingSeverity(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".codelocal"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{
  "schemaVersion": 1,
  "languages": {
    "go": {
      "maxFileLines": 400,
      "maxFunctionLines": 10,
      "maxNestingDepth": 3,
      "maxCyclomaticComplexity": 12,
      "maxParameters": 5,
      "singleResponsibility": true,
      "severity": {"maxFunctionLines":"blocking"},
      "orchestrators": {
        "enabled": true,
        "namePatterns": ["Switch*", "Route*", "Dispatch*"],
        "explicitFunctions": ["HandleWorkflow"],
        "maxOwnedStatements": 2,
        "severity": "blocking"
      }
    }
  },
  "exemptions": {"generated":true,"tests":true,"migrations":true,"paths":["dsl/**"]}
}`
	if err := os.WriteFile(filepath.Join(root, ".codelocal", "quality.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	policy, source, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if source != "project" {
		t.Fatalf("config source=%q want project", source)
	}
	goPolicy := policy.Languages["go"]
	if goPolicy.MaxFileLines != 400 || goPolicy.MaxFunctionLines != 10 || goPolicy.MaxNestingDepth != 3 || goPolicy.MaxCyclomaticComplexity != 12 || goPolicy.MaxParameters != 5 {
		t.Fatalf("project quality thresholds were not applied: %#v", goPolicy)
	}
	if SeverityFor(goPolicy, RuleMaxFunctionLines) != SeverityBlocking || SeverityFor(goPolicy, RuleOrchestratorOwnership) != SeverityBlocking {
		t.Fatalf("blocking severity configuration was not applied: %#v", goPolicy)
	}
	if len(goPolicy.Orchestrators.ExplicitFunctions) != 1 || goPolicy.Orchestrators.ExplicitFunctions[0] != "HandleWorkflow" {
		t.Fatalf("explicit orchestrator role lost: %#v", goPolicy.Orchestrators)
	}
}

func TestQualityConfigCanExplicitlyDisableThresholds(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".codelocal"), 0o755); err != nil {
		t.Fatal(err)
	}
	config := `{"schemaVersion":1,"languages":{"go":{"maxFileLines":0,"maxFunctionLines":0,"singleResponsibility":false,"orchestrators":{"enabled":false}}}}`
	if err := os.WriteFile(filepath.Join(root, ".codelocal", "quality.json"), []byte(config), 0o644); err != nil {
		t.Fatal(err)
	}
	policy, _, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	goPolicy := policy.Languages["go"]
	if goPolicy.MaxFileLines != 0 || goPolicy.MaxFunctionLines != 0 || goPolicy.SingleResponsibility || goPolicy.Orchestrators.Enabled {
		t.Fatalf("explicit disabled quality settings were replaced by defaults: %#v", goPolicy)
	}
}

func TestQualityConfigRejectsUnknownOrUnsupportedSchema(t *testing.T) {
	for name, config := range map[string]string{
		"future":  `{"schemaVersion":2}`,
		"unknown": `{"schemaVersion":1,"mystery":true}`,
	} {
		t.Run(name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.MkdirAll(filepath.Join(root, ".codelocal"), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(root, ".codelocal", "quality.json"), []byte(config), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := Load(root); err == nil {
				t.Fatal("invalid quality config was silently accepted")
			}
		})
	}
}

func TestSeverityNormalizationIsClosedSet(t *testing.T) {
	if normalizeSeverity("blocking") != SeverityBlocking {
		t.Fatal("blocking severity lost")
	}
	for _, value := range []string{"", "warning", "error", "BLOCK"} {
		if normalizeSeverity(value) != SeverityAdvisory {
			t.Fatalf("unknown severity %q should fail safe to advisory", value)
		}
	}
}

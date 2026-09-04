package contextsurface

import "testing"

func TestDiagnoseFindsDuplicateAndPressure(t *testing.T) {
	surface := Surface{Fingerprint: "x", Budget: Budget{MaxTokens: 10, EstimatedTokens: 9}, Items: []Item{{ID: "a", Lane: LaneActive, Text: "same evidence", Source: "tool:a"}, {ID: "b", Lane: LaneObservation, Text: " Same   Evidence ", Source: "tool:b"}}}
	report := Diagnose(surface)
	foundDuplicate, foundPressure := false, false
	for _, finding := range report.Findings { foundDuplicate = foundDuplicate || finding.Kind == FindingDuplicateText; foundPressure = foundPressure || finding.Kind == FindingBudgetPressure }
	if !foundDuplicate || !foundPressure { t.Fatalf("expected duplicate and pressure findings: %+v", report) }
}

func TestDiagnoseFailsHealthForUntrustedRequired(t *testing.T) {
	surface := Surface{Budget: Budget{MaxTokens: 100, EstimatedTokens: 10}, Items: []Item{{ID: "r", Lane: LaneBrainMandatory, Text: "must do this", Required: true, Trust: "repository"}}}
	report := Diagnose(surface)
	if report.Healthy { t.Fatalf("untrusted required context must not be healthy: %+v", report) }
}

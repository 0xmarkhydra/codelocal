package orchestration

import "testing"
import "time"

func TestLaneEvidenceNamespaces(t *testing.T) {
	exitCode := 1
	evidence, err := LaneEvidence(LaneBrowser, "smoke", false, &exitCode, "shot-1", time.Unix(200, 0).UTC())
	if err != nil {
		t.Fatal(err)
	}
	if evidence.CheckID != "browser:smoke" || evidence.Status != VerificationFailed {
		t.Fatalf("evidence mismatch: %+v", evidence)
	}
	if evidence.ExitCode == nil || *evidence.ExitCode != 1 || evidence.ArtifactRef != "shot-1" {
		t.Fatalf("evidence detail mismatch: %+v", evidence)
	}
	if _, err := LaneEvidence(LaneCode, "x", true, nil, "", time.Time{}); err == nil {
		t.Fatal("non-verification lane accepted")
	}
	if _, err := LaneEvidence(LaneMobile, "", true, nil, "", time.Time{}); err == nil {
		t.Fatal("empty check name accepted")
	}
	graph := SummarizeVerification(
		[]VerificationRequirement{{ID: "browser:smoke", Category: "browser", Required: true}},
		[]VerificationEvidence{evidence},
	)
	if graph.Ready || len(graph.Lanes) != 1 || len(graph.Lanes[0].Failed) != 1 {
		t.Fatalf("adapter evidence did not flow into graph: %+v", graph)
	}
}

package ecosystem

import (
	"testing"
	"time"
)

func strongCandidate() Candidate {
	return Candidate{
		ID:         "context-reversible-ledger",
		Name:       "Reversible Context Ledger",
		Capability: CapabilityContext,
		Source: Source{
			Kind:       SourceGitHubRepo,
			Repository: "example/repo",
			CommitSHA:  "0123456789abcdef",
		},
		LicenseSPDX:  "MIT",
		LicenseClass: LicensePermissive,
		Evidence: Evidence{
			TestsPresent:       true,
			BenchmarkPresent:   true,
			SecurityReview:     true,
			ReproductionPassed: true,
		},
		Scores: Scores{
			Relevance: 1, Novelty: .9, Maturity: .9, TestQuality: .9,
			SecurityFit: .95, TokenImpact: .9, QualityImpact: .9, UXImpact: .7,
		},
	}
}

func TestAssessPromotesStrongSafeCorePattern(t *testing.T) {
	assessment := Assess(strongCandidate())
	if assessment.Disposition != DispositionNativeCore || !assessment.ReadyForProduction || !assessment.ReadyForLab {
		t.Fatalf("expected native-core promotion candidate: %+v", assessment)
	}
	if assessment.Score < .72 || assessment.Fingerprint == "" {
		t.Fatalf("unexpected score/fingerprint: %+v", assessment)
	}
}

func TestAssessUnknownLicenseStaysWatchOnly(t *testing.T) {
	candidate := strongCandidate()
	candidate.LicenseSPDX = ""
	candidate.LicenseClass = LicenseUnknown
	assessment := Assess(candidate)
	if assessment.Disposition != DispositionWatch || assessment.ReadyForLab || assessment.ReadyForProduction {
		t.Fatalf("unknown license must not advance: %+v", assessment)
	}
	if !assessment.RequiresLicenseReview {
		t.Fatalf("missing license review flag: %+v", assessment)
	}
}

func TestAssessSevereRiskRequiresLabAndSecurityReview(t *testing.T) {
	candidate := strongCandidate()
	candidate.Risks.SecretAccess = true
	assessment := Assess(candidate)
	if assessment.Disposition != DispositionLab || assessment.ReadyForLab || assessment.ReadyForProduction {
		t.Fatalf("severe risk must not advance automatically: %+v", assessment)
	}
	if !assessment.RequiresSecurityReview {
		t.Fatalf("missing security review flag: %+v", assessment)
	}
}

func TestAssessUnpinnedSourceCannotEnterLab(t *testing.T) {
	candidate := strongCandidate()
	candidate.Source.CommitSHA = ""
	assessment := Assess(candidate)
	if assessment.Disposition != DispositionWatch || assessment.ReadyForLab || assessment.ReadyForProduction {
		t.Fatalf("unpinned source advanced unexpectedly: %+v", assessment)
	}
}

func TestAssessRoutesDeviceCapabilityToAdapter(t *testing.T) {
	candidate := strongCandidate()
	candidate.Capability = CapabilityDevice
	assessment := Assess(candidate)
	if assessment.Disposition != DispositionAdapter || !assessment.ReadyForProduction {
		t.Fatalf("expected adapter disposition: %+v", assessment)
	}
}

func TestFingerprintIgnoresDiscoveryTime(t *testing.T) {
	first := strongCandidate()
	second := strongCandidate()
	first.Source.DiscoveredAt = time.Unix(100, 0)
	second.Source.DiscoveredAt = time.Unix(200, 0)
	if Assess(first).Fingerprint != Assess(second).Fingerprint {
		t.Fatal("discovery timestamp changed immutable candidate fingerprint")
	}
}

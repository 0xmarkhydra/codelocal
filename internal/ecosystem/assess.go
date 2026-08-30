package ecosystem

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"math"
	"strings"
	"time"
)

func clamp(value float64) float64 {
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 {
		return 0
	}
	if value > 1 {
		return 1
	}
	return value
}

func normalizeCandidate(candidate Candidate) Candidate {
	if candidate.SchemaVersion == 0 {
		candidate.SchemaVersion = SchemaVersion
	}
	candidate.ID = strings.TrimSpace(candidate.ID)
	candidate.Name = strings.TrimSpace(candidate.Name)
	candidate.Source.Repository = strings.TrimSpace(candidate.Source.Repository)
	candidate.Source.URL = strings.TrimSpace(candidate.Source.URL)
	candidate.Source.CommitSHA = strings.TrimSpace(candidate.Source.CommitSHA)
	candidate.Source.Version = strings.TrimSpace(candidate.Source.Version)
	candidate.Problem = strings.TrimSpace(candidate.Problem)
	candidate.Pattern = strings.TrimSpace(candidate.Pattern)
	candidate.LicenseSPDX = strings.TrimSpace(candidate.LicenseSPDX)
	candidate.Scores.Relevance = clamp(candidate.Scores.Relevance)
	candidate.Scores.Novelty = clamp(candidate.Scores.Novelty)
	candidate.Scores.Maturity = clamp(candidate.Scores.Maturity)
	candidate.Scores.TestQuality = clamp(candidate.Scores.TestQuality)
	candidate.Scores.SecurityFit = clamp(candidate.Scores.SecurityFit)
	candidate.Scores.TokenImpact = clamp(candidate.Scores.TokenImpact)
	candidate.Scores.QualityImpact = clamp(candidate.Scores.QualityImpact)
	candidate.Scores.UXImpact = clamp(candidate.Scores.UXImpact)
	return candidate
}

func candidateFingerprint(candidate Candidate) string {
	candidate = normalizeCandidate(candidate)
	// DiscoveredAt is intentionally excluded: provenance should remain stable
	// when the exact pinned source and analyst assessment are unchanged.
	candidate.Source.DiscoveredAt = time.Time{}
	raw, _ := json.Marshal(candidate)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

func weightedScore(scores Scores) float64 {
	// Quality/relevance/security carry the most weight. Novelty alone must never
	// promote a clever but unsafe experiment into the production core.
	return clamp(
		.20*scores.Relevance +
			.15*scores.Novelty +
			.12*scores.Maturity +
			.10*scores.TestQuality +
			.18*scores.SecurityFit +
			.08*scores.TokenImpact +
			.12*scores.QualityImpact +
			.05*scores.UXImpact,
	)
}

func severeRisk(r RiskSignals) bool {
	return r.SecretAccess || r.WritesOutsideSandbox || r.RequiresCorePatch
}

func executionRisk(r RiskSignals) bool {
	return r.InstallScripts || r.ArbitraryCode || r.UnboundedNetwork || r.UnpinnedDependencies || severeRisk(r)
}

func pinned(candidate Candidate) bool {
	switch candidate.Source.Kind {
	case SourceGitHubRepo:
		return candidate.Source.CommitSHA != ""
	case SourcePackage:
		return candidate.Source.Version != ""
	case SourcePaper, SourceManual:
		return candidate.Source.URL != "" || candidate.Source.Version != ""
	case SourceGitHubTopic:
		return false
	default:
		return candidate.Source.CommitSHA != "" || candidate.Source.Version != ""
	}
}

// Assess is deliberately conservative. It classifies an R&D candidate; it does
// not install, execute, import, or license third-party code. Production-ready
// means eligible for a human-reviewed implementation/promotion proposal, not
// permission to copy source into CodeLocal automatically.
func Assess(input Candidate) Assessment {
	candidate := normalizeCandidate(input)
	assessment := Assessment{
		CandidateID: candidate.ID,
		Fingerprint: candidateFingerprint(candidate),
		Score:       weightedScore(candidate.Scores),
		Disposition: DispositionWatch,
		Reasons:     []string{},
	}

	if candidate.ID == "" || candidate.Name == "" || candidate.Source.Kind == "" || candidate.Capability == "" {
		assessment.Disposition = DispositionReject
		assessment.Reasons = append(assessment.Reasons, "candidate metadata is incomplete")
		return assessment
	}

	if !pinned(candidate) {
		assessment.Reasons = append(assessment.Reasons, "source is not pinned to an immutable revision/version")
	}

	switch candidate.LicenseClass {
	case LicensePermissive:
		// Still preserve SPDX/provenance; permissive is not an attribution waiver.
	case LicenseReciprocal:
		assessment.RequiresLicenseReview = true
		assessment.Reasons = append(assessment.Reasons, "reciprocal license requires integration-boundary review")
	case LicenseProprietary:
		assessment.RequiresLicenseReview = true
		assessment.Reasons = append(assessment.Reasons, "proprietary source is research-only unless separately authorized")
	case LicenseUnknown, "":
		assessment.RequiresLicenseReview = true
		assessment.Reasons = append(assessment.Reasons, "license is unknown")
	default:
		assessment.RequiresLicenseReview = true
		assessment.Reasons = append(assessment.Reasons, "license classification is unrecognized")
	}

	if executionRisk(candidate.Risks) {
		assessment.RequiresSecurityReview = true
		assessment.Reasons = append(assessment.Reasons, "candidate carries executable or boundary-crossing risk")
	}
	if severeRisk(candidate.Risks) {
		assessment.Reasons = append(assessment.Reasons, "candidate requires high-risk privileges or core modification")
	}

	assessment.ReadyForLab = pinned(candidate) && candidate.LicenseClass != LicenseProprietary
	if severeRisk(candidate.Risks) || candidate.LicenseClass == LicenseUnknown || candidate.LicenseClass == "" {
		assessment.ReadyForLab = false
	}

	// Unknown/proprietary licensing and severe privilege assumptions never reach
	// an automatic production recommendation regardless of score.
	productionGate := pinned(candidate) &&
		candidate.LicenseClass == LicensePermissive &&
		!severeRisk(candidate.Risks) &&
		candidate.Evidence.TestsPresent &&
		candidate.Evidence.ReproductionPassed &&
		candidate.Scores.SecurityFit >= .75

	if candidate.LicenseClass == LicenseProprietary {
		assessment.Disposition = DispositionReject
		return assessment
	}
	if !pinned(candidate) || candidate.LicenseClass == LicenseUnknown || candidate.LicenseClass == "" {
		assessment.Disposition = DispositionWatch
		return assessment
	}
	if severeRisk(candidate.Risks) {
		assessment.Disposition = DispositionLab
		return assessment
	}
	if assessment.Score < .45 {
		assessment.Disposition = DispositionReject
		return assessment
	}
	if !productionGate || assessment.Score < .72 || !candidate.Evidence.BenchmarkPresent {
		assessment.Disposition = DispositionLab
		return assessment
	}

	assessment.ReadyForProduction = true
	switch candidate.Capability {
	case CapabilityContext, CapabilityMemory, CapabilityTokenCost, CapabilityAgentRuntime,
		CapabilitySecurity, CapabilityDebug, CapabilityVerification, CapabilityRouting:
		assessment.Disposition = DispositionNativeCore
	case CapabilityDevice, CapabilityPluginInfra:
		assessment.Disposition = DispositionAdapter
	case CapabilityDeveloperUX, CapabilityOther:
		assessment.Disposition = DispositionSkill
	default:
		assessment.Disposition = DispositionLab
		assessment.ReadyForProduction = false
	}
	return assessment
}

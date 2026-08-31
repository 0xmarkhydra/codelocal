package ecosystem

import "time"

const SchemaVersion = 1

type SourceKind string

const (
	SourceGitHubRepo SourceKind = "github_repo"
	SourceGitHubTopic SourceKind = "github_topic"
	SourcePaper      SourceKind = "paper"
	SourcePackage    SourceKind = "package"
	SourceManual     SourceKind = "manual"
)

type CapabilityClass string

const (
	CapabilityContext       CapabilityClass = "context"
	CapabilityMemory        CapabilityClass = "memory"
	CapabilityTokenCost     CapabilityClass = "token_cost"
	CapabilityAgentRuntime  CapabilityClass = "agent_runtime"
	CapabilitySecurity      CapabilityClass = "security"
	CapabilityDebug         CapabilityClass = "debug"
	CapabilityVerification  CapabilityClass = "verification"
	CapabilityDevice        CapabilityClass = "device"
	CapabilityPluginInfra   CapabilityClass = "plugin_infra"
	CapabilityRouting       CapabilityClass = "routing"
	CapabilityDeveloperUX   CapabilityClass = "developer_ux"
	CapabilityOther         CapabilityClass = "other"
)

type LicenseClass string

const (
	LicensePermissive LicenseClass = "permissive"
	LicenseReciprocal LicenseClass = "reciprocal"
	LicenseProprietary LicenseClass = "proprietary"
	LicenseUnknown     LicenseClass = "unknown"
)

type Source struct {
	Kind        SourceKind `json:"kind"`
	Repository  string     `json:"repository,omitempty"`
	URL         string     `json:"url,omitempty"`
	CommitSHA   string     `json:"commitSha,omitempty"`
	Version     string     `json:"version,omitempty"`
	DiscoveredAt time.Time `json:"discoveredAt,omitempty"`
}

type RiskSignals struct {
	InstallScripts       bool `json:"installScripts,omitempty"`
	ArbitraryCode        bool `json:"arbitraryCode,omitempty"`
	SecretAccess         bool `json:"secretAccess,omitempty"`
	UnboundedNetwork     bool `json:"unboundedNetwork,omitempty"`
	RequiresCorePatch    bool `json:"requiresCorePatch,omitempty"`
	UnpinnedDependencies bool `json:"unpinnedDependencies,omitempty"`
	WritesOutsideSandbox bool `json:"writesOutsideSandbox,omitempty"`
}

type Evidence struct {
	TestsPresent        bool `json:"testsPresent,omitempty"`
	CIObserved          bool `json:"ciObserved,omitempty"`
	BenchmarkPresent    bool `json:"benchmarkPresent,omitempty"`
	SecurityReview      bool `json:"securityReview,omitempty"`
	ProductionClaims    bool `json:"productionClaims,omitempty"`
	ReproductionPassed bool `json:"reproductionPassed,omitempty"`
}

// Scores are normalized to [0,1]. They are analyst inputs, not claims derived
// from README prose. Untrusted repository text must never be treated as an
// instruction to CodeLocal or as sufficient evidence by itself.
type Scores struct {
	Relevance    float64 `json:"relevance"`
	Novelty      float64 `json:"novelty"`
	Maturity     float64 `json:"maturity"`
	TestQuality  float64 `json:"testQuality"`
	SecurityFit  float64 `json:"securityFit"`
	TokenImpact  float64 `json:"tokenImpact"`
	QualityImpact float64 `json:"qualityImpact"`
	UXImpact     float64 `json:"uxImpact"`
}

type Candidate struct {
	SchemaVersion int             `json:"schemaVersion"`
	ID            string          `json:"id"`
	Name          string          `json:"name"`
	Source        Source          `json:"source"`
	Capability    CapabilityClass `json:"capability"`
	Problem       string          `json:"problem,omitempty"`
	Pattern       string          `json:"pattern,omitempty"`
	LicenseSPDX   string          `json:"licenseSpdx,omitempty"`
	LicenseClass  LicenseClass    `json:"licenseClass"`
	Risks         RiskSignals     `json:"risks"`
	Evidence      Evidence        `json:"evidence"`
	Scores        Scores          `json:"scores"`
	Duplicates    []string        `json:"duplicates,omitempty"`
}

type Disposition string

const (
	DispositionNativeCore Disposition = "native_core"
	DispositionAdapter    Disposition = "adapter"
	DispositionSkill      Disposition = "skill"
	DispositionLab        Disposition = "lab_experiment"
	DispositionWatch      Disposition = "watch"
	DispositionReject     Disposition = "reject"
)

type Assessment struct {
	CandidateID        string      `json:"candidateId"`
	Fingerprint        string      `json:"fingerprint"`
	Score              float64     `json:"score"`
	Disposition        Disposition `json:"disposition"`
	ReadyForLab        bool        `json:"readyForLab"`
	ReadyForProduction bool        `json:"readyForProduction"`
	RequiresLicenseReview bool      `json:"requiresLicenseReview,omitempty"`
	RequiresSecurityReview bool     `json:"requiresSecurityReview,omitempty"`
	Reasons            []string    `json:"reasons"`
}

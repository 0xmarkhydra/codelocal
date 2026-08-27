package skills

import "context"

type PolicyDecision string

const (
	PolicyAllow           PolicyDecision = "allow"
	PolicyRequireApproval PolicyDecision = "require_approval"
	PolicyDeny            PolicyDecision = "deny"
)

type PolicyRequest struct {
	SkillID      string       `json:"skillId"`
	Capabilities []Capability `json:"capabilities"`
	Risk         int          `json:"risk"`
}

type PolicyResult struct {
	Decision PolicyDecision `json:"decision"`
	Reason   string         `json:"reason,omitempty"`
}

type PolicyResolver interface {
	ResolveSkillPolicy(context.Context, PolicyRequest) (PolicyResult, error)
}

func RiskLevel(capabilities []Capability) int {
	risk := 0
	for _, capability := range capabilities {
		switch capability {
		case CapabilityProjectRead:
			risk = maxInt(risk, 1)
		case CapabilityProjectWrite:
			risk = maxInt(risk, 2)
		case CapabilityShell, CapabilityBrowser:
			risk = maxInt(risk, 3)
		case CapabilityNetwork, CapabilityCredentials:
			risk = maxInt(risk, 4)
		case CapabilityDestructive:
			risk = maxInt(risk, 10)
		}
	}
	return risk
}

func PolicyRequestFor(manifest Manifest) PolicyRequest {
	return PolicyRequest{
		SkillID:      manifest.ID,
		Capabilities: append([]Capability(nil), manifest.Capabilities...),
		Risk:         RiskLevel(manifest.Capabilities),
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

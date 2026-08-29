package skills

import (
	"context"
	"errors"
)

type PolicyDecision string

const (
	PolicyAllow           PolicyDecision = "allow"
	PolicyRequireApproval PolicyDecision = "require_approval"
	PolicyDeny            PolicyDecision = "deny"
)

type PolicyRequest struct {
	SkillID      string       `json:"skillId"`
	SkillVersion string       `json:"skillVersion,omitempty"`
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

var ErrSkillAuthorizationUnavailable = errors.New("skill runtime authorization is unavailable")

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
		SkillVersion: manifest.Version,
		Capabilities: append([]Capability(nil), manifest.Capabilities...),
		Risk:         RiskLevel(manifest.Capabilities),
	}
}

// AuthorizeExecution is the only Skill-domain gate for executable capabilities.
// It never invents approval semantics: Cloud/Desktop adapters must delegate to
// CodeLocal's existing workspace/runtime authorization source of truth. A
// missing resolver therefore fails closed. Pure knowledge skills require no
// runtime authorization and remain safe for automatic use.
func AuthorizeExecution(ctx context.Context, manifest Manifest, resolver PolicyResolver) (PolicyResult, error) {
	if manifest.Kind == KindKnowledge && len(manifest.Capabilities) == 0 {
		return PolicyResult{Decision: PolicyAllow, Reason: "knowledge-only skill"}, nil
	}
	if resolver == nil {
		return PolicyResult{Decision: PolicyDeny, Reason: "runtime authorization adapter unavailable"}, ErrSkillAuthorizationUnavailable
	}
	result, err := resolver.ResolveSkillPolicy(ctx, PolicyRequestFor(manifest))
	if err != nil {
		return PolicyResult{Decision: PolicyDeny, Reason: "authorization resolver failed"}, err
	}
	switch result.Decision {
	case PolicyAllow, PolicyRequireApproval, PolicyDeny:
		return result, nil
	default:
		return PolicyResult{Decision: PolicyDeny, Reason: "invalid authorization decision"}, nil
	}
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

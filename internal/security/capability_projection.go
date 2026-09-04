package security

import (
	"errors"
	"sort"
	"strings"
)

type CapabilityVisibility string

const (
	CapabilityAvailable CapabilityVisibility = "available"
	CapabilityPromptable CapabilityVisibility = "promptable"
)

var ErrInvalidCapabilityProjection = errors.New("invalid capability projection")

type CapabilityDescriptor struct {
	ID            string     `json:"id"`
	Action        string     `json:"action"`
	Tool          string     `json:"tool,omitempty"`
	Path          string     `json:"path,omitempty"`
	NetworkTarget string     `json:"networkTarget,omitempty"`
	SecretMode    SecretMode `json:"secretMode,omitempty"`
	SecretNames   []string   `json:"secretNames,omitempty"`
	Mutates       bool       `json:"mutates,omitempty"`
}

type ProjectedCapability struct {
	CapabilityDescriptor
	Visibility  CapabilityVisibility `json:"visibility"`
	ApprovalKey string               `json:"approvalKey,omitempty"`
	RiskLevel   RiskLevel            `json:"riskLevel"`
}

type CapabilityProjectionRequest struct {
	ActorID           string                 `json:"actorId"`
	AgentID           string                 `json:"agentId,omitempty"`
	AgentRole         string                 `json:"agentRole,omitempty"`
	WorkspaceRoot     string                 `json:"workspaceRoot,omitempty"`
	CWD               string                 `json:"cwd,omitempty"`
	ApprovalAvailable bool                   `json:"approvalAvailable"`
	Capabilities      []CapabilityDescriptor `json:"capabilities"`
}

// ProjectCapabilities is the single policy seam shared by model tool schemas and
// the Tool Program SDK. A denied capability is absent. A prompt-only capability
// is visible only when the session can actually satisfy an approval. This avoids
// advertising impossible operations that would only produce model retries.
func ProjectCapabilities(kernel *PolicyKernel, input CapabilityProjectionRequest) ([]ProjectedCapability, error) {
	if kernel == nil || strings.TrimSpace(input.ActorID) == "" {
		return nil, ErrInvalidCapabilityProjection
	}
	seen := map[string]struct{}{}
	out := make([]ProjectedCapability, 0, len(input.Capabilities))
	for _, raw := range input.Capabilities {
		capability := normalizeCapabilityDescriptor(raw)
		if capability.ID == "" || capability.Action == "" {
			return nil, ErrInvalidCapabilityProjection
		}
		if _, exists := seen[capability.ID]; exists {
			return nil, ErrInvalidCapabilityProjection
		}
		seen[capability.ID] = struct{}{}
		decision := kernel.Evaluate(PolicyRequest{
			ActorID: input.ActorID, AgentID: input.AgentID, AgentRole: input.AgentRole,
			Action: capability.Action, Tool: capability.Tool, Path: capability.Path,
			NetworkTarget: capability.NetworkTarget, WorkspaceRoot: input.WorkspaceRoot, CWD: input.CWD,
			SecretMode: capability.SecretMode, SecretNames: capability.SecretNames,
			SecretOutput: SecretOutputRedacted,
		})
		switch decision.Effect {
		case EffectDeny:
			continue
		case EffectPrompt:
			if !input.ApprovalAvailable || decision.ApprovalKey == "" {
				continue
			}
			out = append(out, ProjectedCapability{CapabilityDescriptor: capability, Visibility: CapabilityPromptable, ApprovalKey: decision.ApprovalKey, RiskLevel: decision.RiskLevel})
		case EffectAllow:
			out = append(out, ProjectedCapability{CapabilityDescriptor: capability, Visibility: CapabilityAvailable, RiskLevel: decision.RiskLevel})
		default:
			return nil, ErrInvalidCapabilityProjection
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func normalizeCapabilityDescriptor(value CapabilityDescriptor) CapabilityDescriptor {
	value.ID = strings.TrimSpace(value.ID)
	value.Action = strings.ToLower(strings.TrimSpace(value.Action))
	value.Tool = strings.ToLower(strings.TrimSpace(value.Tool))
	value.Path = filepathSlash(value.Path)
	value.NetworkTarget = strings.TrimSpace(value.NetworkTarget)
	if value.SecretMode == "" {
		value.SecretMode = SecretNone
	}
	names := make([]string, 0, len(value.SecretNames))
	for _, name := range value.SecretNames {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	value.SecretNames = uniquePolicyStrings(names)
	return value
}

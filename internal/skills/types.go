package skills

import (
	"fmt"
	"strings"
)

type Scope string

type Kind string

type Capability string

const (
	ScopeSystem    Scope = "system"
	ScopePersonal  Scope = "personal"
	ScopeCommunity Scope = "community"

	KindKnowledge Kind = "knowledge"
	KindWorkflow  Kind = "workflow"
	KindRuntime   Kind = "runtime"
	KindHybrid    Kind = "hybrid"

	CapabilityProjectRead  Capability = "project_read"
	CapabilityProjectWrite Capability = "project_write"
	CapabilityShell        Capability = "shell"
	CapabilityNetwork      Capability = "network"
	CapabilityBrowser      Capability = "browser"
	CapabilityCredentials  Capability = "local_credentials"
	CapabilityDestructive  Capability = "destructive"
)

type Manifest struct {
	ID           string       `json:"id"`
	Name         string       `json:"name"`
	Version      string       `json:"version"`
	Publisher    string       `json:"publisher"`
	Scope        Scope        `json:"scope"`
	Kind         Kind         `json:"kind"`
	Intents      []string     `json:"intents,omitempty"`
	Tags         []string     `json:"tags,omitempty"`
	Stacks       []string     `json:"stacks,omitempty"`
	Capabilities []Capability `json:"capabilities,omitempty"`
	Quality      float64      `json:"quality"`
	Verified     bool         `json:"verified"`
	SourceURL    string       `json:"sourceUrl,omitempty"`
	License      string       `json:"license,omitempty"`
}

func (m Manifest) Validate() error {
	if strings.TrimSpace(m.ID) == "" || strings.TrimSpace(m.Name) == "" || strings.TrimSpace(m.Version) == "" {
		return fmt.Errorf("skill id, name and version are required")
	}
	switch m.Scope {
	case ScopeSystem, ScopePersonal, ScopeCommunity:
	default:
		return fmt.Errorf("unsupported skill scope %q", m.Scope)
	}
	switch m.Kind {
	case KindKnowledge, KindWorkflow, KindRuntime, KindHybrid:
	default:
		return fmt.Errorf("unsupported skill kind %q", m.Kind)
	}
	if m.Quality < 0 || m.Quality > 1 {
		return fmt.Errorf("skill quality must be between 0 and 1")
	}
	return nil
}

type TaskContext struct {
	Query         string             `json:"query"`
	Intents       []string           `json:"intents,omitempty"`
	Stack         []string           `json:"stack,omitempty"`
	Signals       []string           `json:"signals,omitempty"`
	Trivial       bool               `json:"trivial,omitempty"`
	Affinity      map[string]float64 `json:"affinity,omitempty"`
	MaxSelections int                `json:"maxSelections,omitempty"`
}

type Selection struct {
	Skill         Manifest `json:"skill"`
	Relevance     float64  `json:"relevance"`
	Compatibility float64  `json:"compatibility"`
	Benefit       float64  `json:"benefit"`
	Utility       float64  `json:"utility"`
	Reason        string   `json:"reason"`
}

type PlanStep struct {
	Order        int          `json:"order"`
	SkillID      string       `json:"skillId"`
	Phase        string       `json:"phase"`
	Reason       string       `json:"reason"`
	Capabilities []Capability `json:"capabilities,omitempty"`
}

type Plan struct {
	Selections []Selection      `json:"selections"`
	Steps      []PlanStep       `json:"steps"`
	Knowledge  []KnowledgeMatch `json:"knowledge,omitempty"`
}

package orchestration

import "errors"

type Specialist string

const (
	SpecialistQuick        Specialist = "quick"
	SpecialistInvestigator Specialist = "investigator"
	SpecialistImplementer  Specialist = "implementer"
	SpecialistTester       Specialist = "tester"
	SpecialistReviewer     Specialist = "reviewer"
	SpecialistSecurity     Specialist = "security"
	SpecialistDeep         Specialist = "deep"
)

var (
	ErrDelegationDepth       = errors.New("agent delegation depth exceeded")
	ErrDelegationConcurrency = errors.New("agent delegation concurrency exceeded")
	ErrUnknownSpecialist     = errors.New("unknown specialist")
)

type SpecialistPolicy struct {
	Role                  Specialist `json:"role"`
	EngineProfile         string     `json:"engineProfile"`
	ToolAllowlist         []string   `json:"toolAllowlist,omitempty"`
	MaxDepth              int        `json:"maxDepth"`
	MaxConcurrentChildren int        `json:"maxConcurrentChildren"`
	TokenBudget           int64      `json:"tokenBudget,omitempty"`
	ReadOnly              bool       `json:"readOnly,omitempty"`
}
type DelegationRequest struct {
	Role           Specialist `json:"role"`
	ParentDepth    int        `json:"parentDepth"`
	ActiveChildren int        `json:"activeChildren"`
}

func DefaultSpecialistPolicies() map[Specialist]SpecialistPolicy {
	return map[Specialist]SpecialistPolicy{
		SpecialistQuick:        {Role: SpecialistQuick, EngineProfile: "fast", ToolAllowlist: []string{"read", "search", "symbols"}, MaxDepth: 1, MaxConcurrentChildren: 2, TokenBudget: 12000, ReadOnly: true},
		SpecialistInvestigator: {Role: SpecialistInvestigator, EngineProfile: "balanced", ToolAllowlist: []string{"read", "search", "symbols", "git", "run"}, MaxDepth: 2, MaxConcurrentChildren: 3, TokenBudget: 24000, ReadOnly: true},
		SpecialistImplementer:  {Role: SpecialistImplementer, EngineProfile: "coding", ToolAllowlist: []string{"read", "search", "symbols", "edit", "git", "run"}, MaxDepth: 2, MaxConcurrentChildren: 2, TokenBudget: 48000},
		SpecialistTester:       {Role: SpecialistTester, EngineProfile: "fast", ToolAllowlist: []string{"read", "run", "browser"}, MaxDepth: 1, MaxConcurrentChildren: 1, TokenBudget: 16000, ReadOnly: true},
		SpecialistReviewer:     {Role: SpecialistReviewer, EngineProfile: "reasoning", ToolAllowlist: []string{"read", "search", "git", "run"}, MaxDepth: 1, MaxConcurrentChildren: 1, TokenBudget: 24000, ReadOnly: true},
		SpecialistSecurity:     {Role: SpecialistSecurity, EngineProfile: "reasoning", ToolAllowlist: []string{"read", "search", "git", "run"}, MaxDepth: 1, MaxConcurrentChildren: 1, TokenBudget: 32000, ReadOnly: true},
		SpecialistDeep:         {Role: SpecialistDeep, EngineProfile: "strong", ToolAllowlist: []string{"read", "search", "symbols", "edit", "git", "run", "browser"}, MaxDepth: 2, MaxConcurrentChildren: 2, TokenBudget: 64000},
	}
}

func ValidateDelegation(policies map[Specialist]SpecialistPolicy, request DelegationRequest) (SpecialistPolicy, error) {
	policy, ok := policies[request.Role]
	if !ok {
		return SpecialistPolicy{}, ErrUnknownSpecialist
	}
	if request.ParentDepth+1 > policy.MaxDepth {
		return SpecialistPolicy{}, ErrDelegationDepth
	}
	if request.ActiveChildren >= policy.MaxConcurrentChildren {
		return SpecialistPolicy{}, ErrDelegationConcurrency
	}
	return policy, nil
}

// RecommendSpecialist maps semantic work to a bounded role. Engine/model choice
// remains separate so provider names never leak into planning.
func RecommendSpecialist(taskKind string, complexity int, securitySensitive bool) Specialist {
	return RecommendSubagent(BuiltinSubagentDefinitions(), SubagentRoutingQuery{TaskKind: taskKind, Complexity: complexity, SecuritySensitive: securitySensitive}).Role
}

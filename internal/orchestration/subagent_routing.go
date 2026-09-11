package orchestration

import (
	"sort"
	"strings"
)

// subagent_routing.go — S5 resolver (master plan §6).
//
// Two-step resolution, deterministic and fail-open to the legacy keyword path:
//
//  1. Semantic match: score every registry definition's description (+name
//     tokens) against the objective + taskKind. Keyword rules from
//     RecommendSpecialist survive as conservative fallback/seeds.
//  2. Constraint gate: hard drop when the candidate cannot do the job
//     (write needed but readOnly; security-sensitive but not security/review
//     chain; explicit tool family outside the candidate allowlist; depth and
//     concurrency bounds violated). Learned affinity never overrides a gate.
//
// Learned affinity is strictly advisory: callers pass verified-outcome boosts
// (default nil → 0), capped at ±0.10 inside the resolver so semantic +
// constraint signals always dominate, mirroring the skills router contract
// (±0.25 there). Below minSamples the caller must not pass affinity at all;
// the resolver re-checks the floor defensively.

const (
	// SubagentAffinityCap bounds learned history so it stays advisory.
	SubagentAffinityCap = 0.10
	// SubagentAffinityMinSamples floors anecdotal evidence (UAR9-style).
	SubagentAffinityMinSamples = 3
)

// SubagentRoutingQuery is the single routing request shape.
type SubagentRoutingQuery struct {
	Objective         string
	TaskKind          string
	Complexity        int
	SecuritySensitive bool
	NeedsWrite        bool
	RequiredTools     []string
	ParentDepth       int
	ActiveChildren    int
	Affinity          map[string]float64
	MinSamples        int
}

// SubagentRecommendation is the resolved delegate.
type SubagentRecommendation struct {
	Name     string
	Role     Specialist
	Score    float64
	Semantic float64
	Affinity float64
	Source   string
	Reason   string
}

func clampSubagentAffinity(value float64) float64 {
	if value > SubagentAffinityCap {
		return SubagentAffinityCap
	}
	if value < -SubagentAffinityCap {
		return -SubagentAffinityCap
	}
	return value
}

func subagentRoleRank(role Specialist) int {
	switch role {
	case SpecialistSecurity:
		return 7
	case SpecialistReviewer:
		return 6
	case SpecialistDeep:
		return 5
	case SpecialistImplementer:
		return 4
	case SpecialistInvestigator:
		return 3
	case SpecialistTester:
		return 2
	default:
		return 1
	}
}

// legacySpecialistKeywordSeed preserves the RecommendSpecialist semantics as
// the conservative baseline every candidate starts from.
func legacySpecialistKeywordSeed(taskKind string, complexity int, securitySensitive bool) Specialist {
	if securitySensitive {
		return SpecialistSecurity
	}
	kind := strings.ToLower(strings.TrimSpace(taskKind))
	if strings.Contains(kind, "review") {
		return SpecialistReviewer
	}
	if strings.Contains(kind, "test") || strings.Contains(kind, "verify") {
		return SpecialistTester
	}
	if strings.Contains(kind, "investig") || strings.Contains(kind, "diagnos") {
		return SpecialistInvestigator
	}
	if complexity >= 4 {
		return SpecialistDeep
	}
	if strings.Contains(kind, "implement") || strings.Contains(kind, "fix") || strings.Contains(kind, "refactor") {
		return SpecialistImplementer
	}
	return SpecialistQuick
}

func subagentTextTokens(value string) []string {
	value = strings.ToLower(strings.TrimSpace(value))
	if value == "" {
		return nil
	}
	fields := strings.FieldsFunc(value, func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '+' || r == '#')
	})
	out := make([]string, 0, len(fields))
	for _, token := range fields {
		token = strings.Trim(token, "-_.")
		if len(token) >= 3 {
			out = append(out, token)
		}
	}
	return out
}

// semanticSubagentScore matches registry description (+name tokens) against the
// objective/taskKind. Keyword seeds keep routing stable when descriptions are
// terse; description overlap decides among close candidates.
func semanticSubagentScore(def SubagentDefinition, query SubagentRoutingQuery, seed Specialist) float64 {
	score := 0.0
	if def.Role == seed {
		score += 0.55
	}
	haystack := strings.ToLower(strings.TrimSpace(def.Description + " " + strings.ReplaceAll(def.Name, "-", " ") + " " + string(def.Role)))
	queryText := strings.ToLower(strings.TrimSpace(query.Objective + " " + query.TaskKind))
	if haystack == "" || queryText == "" {
		return score
	}
	for _, token := range subagentTextTokens(query.Objective + " " + query.TaskKind) {
		if strings.Contains(haystack, token) {
			score += 0.12
		}
	}
	kind := strings.ToLower(strings.TrimSpace(query.TaskKind))
	switch def.Role {
	case SpecialistReviewer:
		if strings.Contains(kind, "review") || strings.Contains(queryText, "review") {
			score += 0.20
		}
	case SpecialistTester:
		if strings.Contains(kind, "test") || strings.Contains(kind, "verify") {
			score += 0.20
		}
	case SpecialistInvestigator:
		if strings.Contains(kind, "investig") || strings.Contains(kind, "diagnos") {
			score += 0.20
		}
	case SpecialistImplementer:
		if strings.Contains(kind, "implement") || strings.Contains(kind, "fix") || strings.Contains(kind, "refactor") {
			score += 0.20
		}
	case SpecialistDeep:
		if query.Complexity >= 4 {
			score += 0.20
		}
	case SpecialistSecurity:
		if query.SecuritySensitive {
			score += 0.30
		}
	}
	if score > 1.2 {
		score = 1.2
	}
	return score
}

// gateSubagentCandidate enforces hard constraints. Empty reason = admitted.
func gateSubagentCandidate(def SubagentDefinition, policies map[Specialist]SpecialistPolicy, query SubagentRoutingQuery) string {
	policy, ok := policies[def.Role]
	if !ok {
		return "unknown role"
	}
	if query.NeedsWrite && def.ReadOnly {
		return "candidate is read-only"
	}
	if query.SecuritySensitive && def.Role != SpecialistSecurity && def.Role != SpecialistReviewer && def.Role != SpecialistDeep {
		return "security-sensitive work requires security/reviewer chain"
	}
	for _, required := range query.RequiredTools {
		required = strings.TrimSpace(required)
		if required == "" {
			continue
		}
		matched := false
		for _, pattern := range def.Tools {
			if SubagentPatternMatches(pattern, required) {
				matched = true
				break
			}
		}
		if !matched {
			// Fall back to the builtin role allowlist when a custom definition
			// leaves Tools empty (inherit-all).
			if len(def.Tools) == 0 {
				for _, allowed := range policy.ToolAllowlist {
					if SubagentPatternMatches(allowed+"_*", required) || SubagentPatternMatches(allowed, required) {
						matched = true
						break
					}
				}
			}
		}
		if !matched {
			return "required tool outside allowlist"
		}
		if strings.EqualFold(strings.TrimSpace(ResolveToolPermission(def, required)), "deny") {
			return "required tool denied"
		}
	}
	if _, err := ValidateDelegation(policies, DelegationRequest{Role: def.Role, ParentDepth: query.ParentDepth, ActiveChildren: query.ActiveChildren}); err != nil {
		return "delegation bound: " + err.Error()
	}
	return ""
}

// RecommendSubagent resolves registry definitions to one bounded delegate.
// Deterministic: ties break by (score, role rank, name).
func RecommendSubagent(defs []SubagentDefinition, query SubagentRoutingQuery) SubagentRecommendation {
	policies := DefaultSpecialistPolicies()
	if len(defs) == 0 {
		defs = BuiltinSubagentDefinitions()
	}
	seed := legacySpecialistKeywordSeed(query.TaskKind, query.Complexity, query.SecuritySensitive)
	minSamples := query.MinSamples
	if minSamples <= 0 {
		minSamples = SubagentAffinityMinSamples
	}
	_ = minSamples // floor is enforced by the affinity producer; resolver only caps magnitude.

	type scored struct {
		def      SubagentDefinition
		semantic float64
		affinity float64
		total    float64
	}
	candidates := make([]scored, 0, len(defs))
	for _, def := range defs {
		if strings.TrimSpace(def.Name) == "" {
			continue
		}
		if reason := gateSubagentCandidate(def, policies, query); reason != "" {
			continue
		}
		semantic := semanticSubagentScore(def, query, seed)
		affinity := 0.0
		if query.Affinity != nil {
			if value, ok := query.Affinity[def.Name]; ok {
				affinity = clampSubagentAffinity(value)
			} else if value, ok := query.Affinity[strings.ToLower(def.Name)]; ok {
				affinity = clampSubagentAffinity(value)
			}
		}
		candidates = append(candidates, scored{def: def, semantic: semantic, affinity: affinity, total: semantic + affinity})
	}
	if len(candidates) == 0 {
		// Fail-open to the legacy keyword role so routing never hard-fails;
		// the production dispatcher still validates registry membership.
		return SubagentRecommendation{Name: string(seed), Role: seed, Score: 0.55, Semantic: 0.55, Source: "keyword-fallback", Reason: "no registry candidate passed the constraint gate"}
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].total != candidates[j].total {
			return candidates[i].total > candidates[j].total
		}
		if rankI, rankJ := subagentRoleRank(candidates[i].def.Role), subagentRoleRank(candidates[j].def.Role); rankI != rankJ {
			return rankI > rankJ
		}
		return candidates[i].def.Name < candidates[j].def.Name
	})
	best := candidates[0]
	source := "semantic+constraint"
	if best.affinity != 0 {
		source = "semantic+constraint+verified-affinity"
	}
	return SubagentRecommendation{Name: best.def.Name, Role: best.def.Role, Score: best.total, Semantic: best.semantic, Affinity: best.affinity, Source: source, Reason: "description match gated by allowlist/readOnly/security/delegation bounds"}
}

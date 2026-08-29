package skills

import (
	"math"
	"sort"
	"strings"
)

const (
	defaultMaxSelections = 3
	minimumUtility       = 0.18
)

// CandidateSource is the scale boundary for routing. Registry implements the
// simple in-memory source today; Marketplace/Cloud can later prefilter by
// intent, stack, trust and visibility before the router does final utility
// scoring. This prevents the long-term design from requiring O(all skills)
// scans for every chat request.
type CandidateSource interface {
	Candidates(TaskContext) []Manifest
}

type RegistryCandidates struct {
	Registry *Registry
}

func (s RegistryCandidates) Candidates(TaskContext) []Manifest {
	if s.Registry == nil {
		return nil
	}
	return s.Registry.List()
}

type Router struct {
	candidates CandidateSource
}

func NewRouter(registry *Registry) *Router {
	return NewRouterWithCandidates(RegistryCandidates{Registry: registry})
}

func NewRouterWithCandidates(source CandidateSource) *Router {
	return &Router{candidates: source}
}

func (r *Router) Route(task TaskContext) []Selection {
	if r == nil || r.candidates == nil || task.Trivial {
		return nil
	}
	maxSelections := task.MaxSelections
	if maxSelections <= 0 || maxSelections > defaultMaxSelections {
		maxSelections = defaultMaxSelections
	}

	selections := make([]Selection, 0, maxSelections)
	for _, manifest := range r.candidates.Candidates(task) {
		relevance := relevanceScore(manifest, task)
		if relevance == 0 {
			continue
		}
		compatibility := compatibilityScore(manifest, task.Stack)
		benefit := expectedBenefit(task)
		affinity := clamp(task.Affinity[manifest.ID], -0.25, 0.25)
		contextCost := 0.04
		latencyCost := 0.02
		riskCost := capabilityRisk(manifest.Capabilities)
		utility := relevance*manifest.Quality*compatibility*benefit + affinity - contextCost - latencyCost - riskCost
		if utility < minimumUtility {
			continue
		}
		selections = append(selections, Selection{
			Skill:         manifest,
			Relevance:     round(relevance),
			Compatibility: round(compatibility),
			Benefit:       round(benefit),
			Utility:       round(utility),
			Reason:        selectionReason(manifest, task),
		})
	}

	sort.SliceStable(selections, func(i, j int) bool {
		if selections[i].Utility == selections[j].Utility {
			return selections[i].Skill.ID < selections[j].Skill.ID
		}
		return selections[i].Utility > selections[j].Utility
	})
	if len(selections) > maxSelections {
		selections = selections[:maxSelections]
	}
	return selections
}

func relevanceScore(manifest Manifest, task TaskContext) float64 {
	text := strings.ToLower(strings.Join(append([]string{task.Query}, task.Signals...), " "))
	if strings.TrimSpace(text) == "" && len(task.Intents) == 0 {
		return 0
	}
	score := 0.0
	for _, intent := range manifest.Intents {
		if containsFold(task.Intents, intent) || strings.Contains(text, normalizeToken(intent)) {
			score += 0.55
		}
	}
	for _, tag := range manifest.Tags {
		if strings.Contains(text, normalizeToken(tag)) {
			score += 0.15
		}
	}
	return clamp(score, 0, 1)
}

func compatibilityScore(manifest Manifest, stack []string) float64 {
	if len(stack) == 0 || len(manifest.Stacks) == 0 {
		return 0.85
	}
	for _, candidate := range stack {
		if containsFold(manifest.Stacks, candidate) {
			return 1
		}
	}
	return 0.72
}

func expectedBenefit(task TaskContext) float64 {
	if len(task.Signals) > 0 || len(task.Intents) > 0 {
		return 1
	}
	return 0.88
}

func capabilityRisk(capabilities []Capability) float64 {
	risk := 0.0
	for _, capability := range capabilities {
		switch capability {
		case CapabilityProjectRead:
			risk += 0.01
		case CapabilityProjectWrite:
			risk += 0.04
		case CapabilityShell, CapabilityBrowser:
			risk += 0.06
		case CapabilityNetwork, CapabilityCredentials:
			risk += 0.08
		case CapabilityDestructive:
			risk += 0.25
		}
	}
	return clamp(risk, 0, 0.45)
}

func selectionReason(manifest Manifest, task TaskContext) string {
	if len(task.Intents) > 0 {
		return "matched task intent and skill expertise"
	}
	return "matched task context and skill expertise"
}

func containsFold(values []string, target string) bool {
	for _, value := range values {
		if strings.EqualFold(strings.TrimSpace(value), strings.TrimSpace(target)) {
			return true
		}
	}
	return false
}

func normalizeToken(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "_", " ")
	value = strings.ReplaceAll(value, "-", " ")
	return value
}

func clamp(value, minValue, maxValue float64) float64 {
	return math.Max(minValue, math.Min(maxValue, value))
}

func round(value float64) float64 {
	return math.Round(value*1000) / 1000
}

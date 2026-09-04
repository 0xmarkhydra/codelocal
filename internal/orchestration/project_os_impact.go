package orchestration

import (
	"sort"
	"strings"
)

// ImpactTask is the minimal task surface needed for requirement impact analysis.
type ImpactTask struct {
	ID          string
	Title       string
	Description string
	Status      string
}

// ImpactHit is one task likely affected by a requirement change.
type ImpactHit struct {
	TaskID string  `json:"taskId"`
	Score  float64 `json:"score"`
	Reason string  `json:"reason"`
}

func impactTokens(text string) map[string]bool {
	out := map[string]bool{}
	for _, field := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool {
		return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '/')
	}) {
		field = strings.Trim(field, "_-/")
		if len([]rune(field)) < 3 {
			continue
		}
		out[field] = true
	}
	return out
}

// AnalyzeRequirementImpact scores tasks against changed refs (doc paths, keywords,
// requirement phrases). It is deterministic and dependency-free: callers decide the
// threshold and persist the outcome explicitly. Terminal tasks are never reported.
func AnalyzeRequirementImpact(changedRefs []string, tasks []ImpactTask) []ImpactHit {
	refTokens := map[string]bool{}
	for _, ref := range changedRefs {
		for tok := range impactTokens(ref) {
			refTokens[tok] = true
		}
	}
	if len(refTokens) == 0 {
		return nil
	}
	hits := []ImpactHit{}
	for _, task := range tasks {
		if task.ID == "" {
			continue
		}
		switch task.Status {
		case "DONE", "CANCELLED":
			continue
		}
		taskTokens := impactTokens(task.Title + " " + task.Description)
		if len(taskTokens) == 0 {
			continue
		}
		overlap := 0
		for tok := range taskTokens {
			if refTokens[tok] {
				overlap++
			}
		}
		if overlap == 0 {
			continue
		}
		score := float64(overlap) / float64(len(taskTokens))
		if overlap >= 2 {
			score += 0.2
		}
		if score > 1 {
			score = 1
		}
		hits = append(hits, ImpactHit{TaskID: task.ID, Score: score, Reason: "token-overlap"})
	}
	sort.Slice(hits, func(i, j int) bool {
		if hits[i].Score != hits[j].Score {
			return hits[i].Score > hits[j].Score
		}
		return hits[i].TaskID < hits[j].TaskID
	})
	return hits
}

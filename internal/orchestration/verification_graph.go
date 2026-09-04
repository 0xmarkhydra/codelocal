package orchestration

import (
	"sort"
	"strings"
)

// VerificationLane groups one lane (usually a requirement category such as
// unit, browser, computer or mobile) of a task verification graph. Optional
// checks are reported but never gate lane readiness.
type VerificationLane struct {
	Lane     string   `json:"lane"`
	Required []string `json:"required"`
	Passed   []string `json:"passed"`
	Failed   []string `json:"failed"`
	Pending  []string `json:"pending"`
	Ready    bool     `json:"ready"`
}

type VerificationGraph struct {
	Lanes []VerificationLane `json:"lanes"`
	Ready bool               `json:"ready"`
}

// SummarizeVerification folds requirements and evidence into a lane graph.
// The latest evidence per check wins. A lane is ready when every required
// check passed; the graph is ready when every lane is ready and at least one
// required check exists. Empty requirement sets are never ready.
func SummarizeVerification(requirements []VerificationRequirement, results []VerificationEvidence) VerificationGraph {
	required := map[string]VerificationRequirement{}
	for _, req := range requirements {
		id := strings.TrimSpace(req.ID)
		if id == "" {
			continue
		}
		required[id] = req
	}
	latest := map[string]VerificationStatus{}
	for _, result := range results {
		id := strings.TrimSpace(result.CheckID)
		if _, ok := required[id]; !ok {
			continue
		}
		latest[id] = result.Status
	}
	lanes := map[string]*VerificationLane{}
	laneOf := func(req VerificationRequirement) *VerificationLane {
		name := strings.TrimSpace(req.Category)
		if name == "" {
			name = "general"
		}
		lane, ok := lanes[name]
		if !ok {
			lane = &VerificationLane{Lane: name}
			lanes[name] = lane
		}
		return lane
	}
	graph := VerificationGraph{}
	for id, req := range required {
		lane := laneOf(req)
		if req.Required {
			lane.Required = append(lane.Required, id)
		}
		switch latest[id] {
		case VerificationPassed:
			lane.Passed = append(lane.Passed, id)
		case VerificationFailed:
			lane.Failed = append(lane.Failed, id)
		default:
			lane.Pending = append(lane.Pending, id)
		}
	}
	graph.Ready = len(required) > 0
	for _, lane := range lanes {
		sort.Strings(lane.Required)
		sort.Strings(lane.Passed)
		sort.Strings(lane.Failed)
		sort.Strings(lane.Pending)
		lane.Ready = true
		for _, id := range lane.Required {
			if latest[id] != VerificationPassed {
				lane.Ready = false
				break
			}
		}
		if !lane.Ready {
			graph.Ready = false
		}
		graph.Lanes = append(graph.Lanes, *lane)
	}
	sort.Slice(graph.Lanes, func(i, j int) bool { return graph.Lanes[i].Lane < graph.Lanes[j].Lane })
	if graph.Lanes == nil {
		graph.Lanes = []VerificationLane{}
	}
	return graph
}

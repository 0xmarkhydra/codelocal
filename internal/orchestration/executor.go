package orchestration

import "strings"

// ExecutionTraceStep is the compact, secret-free execution evidence returned by
// the bounded agent executor. Arguments and tool output are intentionally not
// persisted here; the runtime records only semantic operation identities.
type ExecutionTraceStep struct {
	Index        int    `json:"index"`
	Source       string `json:"source"`
	Tool         string `json:"tool"`
	Operation    string `json:"operation"`
	Lane         Lane   `json:"lane"`
	Status       string `json:"status"`
	DurationMS   int64  `json:"durationMs"`
	Replanned    bool   `json:"replanned,omitempty"`
	ReplanReason string `json:"replanReason,omitempty"`
	Verification bool   `json:"verification,omitempty"`
	Fallback     bool   `json:"fallback,omitempty"`
}

type EfficiencyEvaluation struct {
	Score                  int      `json:"score"`
	Grade                  string   `json:"grade"`
	StructuredStepRatio    float64  `json:"structuredStepRatio"`
	RepeatedOperations     int      `json:"repeatedOperations"`
	RouteSwitches          int      `json:"routeSwitches"`
	FallbackSteps          int      `json:"fallbackSteps"`
	ModelRoundTripsAvoided int      `json:"modelRoundTripsAvoided"`
	Signals                []string `json:"signals,omitempty"`
}

func LaneForOperation(operation string) Lane {
	op := strings.ToLower(strings.TrimSpace(operation))
	switch {
	case strings.HasPrefix(op, "browser."):
		return LaneBrowser
	case strings.HasPrefix(op, "computer."):
		return LaneComputer
	case strings.HasPrefix(op, "terminal."), strings.HasPrefix(op, "process."):
		return LaneShell
	case strings.HasPrefix(op, "context."), strings.HasPrefix(op, "project."), strings.HasPrefix(op, "read."),
		strings.HasPrefix(op, "search."), strings.HasPrefix(op, "dependency."), strings.HasPrefix(op, "lsp."),
		strings.HasPrefix(op, "edit."), strings.HasPrefix(op, "verify."), strings.HasPrefix(op, "git."):
		return LaneCode
	default:
		return LaneNone
	}
}

func EvaluateExecutionEfficiency(plan AgentPlan, trace []ExecutionTraceStep) EfficiencyEvaluation {
	if len(trace) == 0 {
		return EfficiencyEvaluation{Score: 0, Grade: "idle"}
	}

	score := 100
	seen := map[string]int{}
	repeated := 0
	fallbacks := 0
	structured := 0
	routeSwitches := 0
	meaningful := 0
	var previous Lane
	firstLane := LaneNone
	for _, step := range trace {
		if step.Status == "skipped" {
			continue
		}
		meaningful++
		seen[step.Operation]++
		if seen[step.Operation] > 1 && step.Operation != "verify.changes" && step.Operation != "process.poll" {
			repeated++
		}
		if step.Fallback {
			fallbacks++
		}
		if step.Lane != LaneShell && step.Lane != LaneNone {
			structured++
		}
		if step.Lane != LaneNone {
			if firstLane == LaneNone {
				firstLane = step.Lane
			}
			if previous != LaneNone && previous != step.Lane {
				routeSwitches++
			}
			previous = step.Lane
		}
	}

	score -= repeated * 7
	score -= fallbacks * 9
	if routeSwitches > 2 {
		score -= (routeSwitches - 2) * 3
	}
	if meaningful > 12 {
		score -= (meaningful - 12) * 2
	}
	if plan.Route.Primary != LaneNone && firstLane != LaneNone && firstLane != plan.Route.Primary && firstLane != LaneCode {
		score -= 5
	}
	if score < 0 {
		score = 0
	}
	if score > 100 {
		score = 100
	}

	ratio := 0.0
	if meaningful > 0 {
		ratio = float64(structured) / float64(meaningful)
	}
	grade := "excellent"
	switch {
	case score < 65:
		grade = "inefficient"
	case score < 80:
		grade = "acceptable"
	case score < 90:
		grade = "good"
	}
	signals := []string{}
	if repeated > 0 {
		signals = append(signals, "repeated semantic operations detected")
	}
	if fallbacks > 0 {
		signals = append(signals, "fallback execution was required")
	}
	if routeSwitches > 2 {
		signals = append(signals, "execution switched capability lanes frequently")
	}
	if ratio >= 0.8 {
		signals = append(signals, "execution stayed mostly on structured surfaces")
	}
	avoided := 0
	if meaningful > 1 {
		avoided = meaningful - 1
	}
	return EfficiencyEvaluation{
		Score:                  score,
		Grade:                  grade,
		StructuredStepRatio:    ratio,
		RepeatedOperations:     repeated,
		RouteSwitches:          routeSwitches,
		FallbackSteps:          fallbacks,
		ModelRoundTripsAvoided: avoided,
		Signals:                signals,
	}
}

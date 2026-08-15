package orchestration

import "strings"

type LoopState struct {
	Phase            string `json:"phase"`
	Iteration        int    `json:"iteration"`
	RecoveryAttempts int    `json:"recoveryAttempts"`
	LastOutcome      string `json:"lastOutcome,omitempty"`
	NextAction       string `json:"nextAction,omitempty"`
}

type LoopEvent struct {
	Operation string
	Success   bool
	Failure   string
	CheckKey  string
}

func recoveryNextAction(advice RecoveryAdvice) string {
	switch advice.Kind {
	case FailureStaleUI:
		return "reobserve the affected UI/window before retry"
	case FailureCompile, FailureType:
		return "collect fresh diagnostics and inspect the failing symbol before editing again"
	case FailureTest:
		return "inspect the failing test and causal diff before a bounded repair attempt"
	case FailureRuntime:
		return "inspect runtime evidence and causal code path before one bounded retry"
	case FailurePermission:
		return "request explicit approval or permission; do not retry automatically"
	case FailureEnvironment:
		return "surface the environment blocker instead of immediate retry"
	default:
		return "surface the failure and refresh context; do not blindly retry"
	}
}

// AdvanceLoop converts each tool outcome into a durable orchestration state.
// It does not execute follow-up tools; ChatGPT remains the reasoning agent and
// CodeLocal exposes the safest next action while preserving approval boundaries.
func AdvanceLoop(current LoopState, event LoopEvent) LoopState {
	next := current
	operation := strings.ToLower(strings.TrimSpace(event.Operation))
	if !event.Success {
		advice := ClassifyFailure(event.Failure)
		next.Phase = "recover"
		next.LastOutcome = "failed"
		next.NextAction = recoveryNextAction(advice)
		if advice.Retryable && AllowRecoveryAttempt(advice, current.RecoveryAttempts) {
			next.RecoveryAttempts++
		}
		return next
	}

	next.LastOutcome = "succeeded"
	switch {
	case operation == "context.task":
		if next.Phase == "" || next.Phase == "plan" || next.Phase == "recover" {
			next.Phase = "inspect"
		}
		next.NextAction = "inspect ranked context and exact code relationships"
	case strings.HasPrefix(operation, "read."), strings.HasPrefix(operation, "search."), strings.HasPrefix(operation, "lsp."), strings.HasPrefix(operation, "project."):
		if next.Phase == "" || next.Phase == "plan" {
			next.Phase = "inspect"
		}
		next.NextAction = "act on the smallest high-confidence change or gather one missing dependency"
	case strings.HasPrefix(operation, "edit."):
		next.Phase = "verify"
		next.Iteration++
		next.RecoveryAttempts = 0
		next.NextAction = "run verify.changes, then the smallest required verification checks"
	case operation == "verify.changes":
		next.Phase = "verify"
		next.RecoveryAttempts = 0
		next.NextAction = "run the missing context-aware checks, then evaluate quality gate"
	case strings.HasPrefix(operation, "terminal.") && event.CheckKey != "":
		next.Phase = "verify"
		next.RecoveryAttempts = 0
		next.NextAction = "refresh verify.changes evidence and evaluate quality gate"
	case strings.HasPrefix(operation, "browser."), strings.HasPrefix(operation, "computer."):
		next.Phase = "verify"
		next.NextAction = "observe fresh UI state and verify the intended effect before continuing"
	case strings.HasPrefix(operation, "git.commit"):
		next.Phase = "finalize"
		next.NextAction = "report completion or continue release/push only when requested"
	default:
		if next.Phase == "" {
			next.Phase = "inspect"
		}
		if next.NextAction == "" {
			next.NextAction = "continue from fresh task evidence"
		}
	}
	return next
}

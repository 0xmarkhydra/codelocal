package orchestration

import "strings"

type FailureKind string

const (
	FailureUnknown     FailureKind = "unknown"
	FailureCompile     FailureKind = "compile"
	FailureType        FailureKind = "type"
	FailureTest        FailureKind = "test"
	FailureRuntime     FailureKind = "runtime"
	FailureEnvironment FailureKind = "environment"
	FailurePermission  FailureKind = "permission"
	FailureStaleUI     FailureKind = "stale_ui"
)

type RecoveryAdvice struct {
	Kind       FailureKind `json:"kind"`
	Retryable  bool        `json:"retryable"`
	Reobserve  bool        `json:"reobserve,omitempty"`
	MaxRetries int         `json:"maxRetries"`
	Reason     string      `json:"reason"`
}

// ClassifyFailure is deliberately conservative. It recommends bounded recovery
// behavior but never executes another action, changes permissions, or bypasses
// the existing CodeLocal authorizer.
func ClassifyFailure(message string) RecoveryAdvice {
	text := strings.ToLower(strings.TrimSpace(message))
	if text == "" {
		return RecoveryAdvice{Kind: FailureUnknown, MaxRetries: 0, Reason: "empty failure has no safe automatic recovery"}
	}

	switch {
	case containsAny(text, "permission denied", "accessibility permission", "screen recording permission", "not authorized", "approval required"):
		return RecoveryAdvice{Kind: FailurePermission, MaxRetries: 0, Reason: "permission failures require explicit user or policy action"}
	case containsAny(text, "element path stale", "stale element", "detached from document", "target closed", "window not found"):
		return RecoveryAdvice{Kind: FailureStaleUI, Retryable: true, Reobserve: true, MaxRetries: 2, Reason: "UI identity changed; refresh structured observation before retrying"}
	case containsAny(text, "no space left", "disk quota", "out of memory", "cannot allocate memory", "network is unreachable", "connection refused", "dns"):
		return RecoveryAdvice{Kind: FailureEnvironment, MaxRetries: 0, Reason: "environment failure is unlikely to improve through immediate retry"}
	case containsAny(text, "undefined:", "syntax error", "expected ';'", "expected declaration", "cannot find package", "build failed"):
		return RecoveryAdvice{Kind: FailureCompile, Retryable: true, MaxRetries: 2, Reason: "compile failures may be repaired from diagnostics"}
	case containsAny(text, "type mismatch", "cannot use", "is not assignable", "type error", "ts2322", "ts2345"):
		return RecoveryAdvice{Kind: FailureType, Retryable: true, MaxRetries: 2, Reason: "type failures may be repaired from diagnostics"}
	case containsAny(text, "test failed", "--- fail:", "assertion", "expected", "received"):
		return RecoveryAdvice{Kind: FailureTest, Retryable: true, MaxRetries: 2, Reason: "test failures may be repaired when causally related to the current patch"}
	case containsAny(text, "panic:", "segmentation fault", "uncaught exception", "runtime error"):
		return RecoveryAdvice{Kind: FailureRuntime, Retryable: true, MaxRetries: 1, Reason: "runtime failures get one bounded repair attempt before escalation"}
	default:
		return RecoveryAdvice{Kind: FailureUnknown, MaxRetries: 0, Reason: "unclassified failures should be surfaced instead of blindly retried"}
	}
}

func AllowRecoveryAttempt(advice RecoveryAdvice, attempts int) bool {
	return advice.Retryable && attempts >= 0 && attempts < advice.MaxRetries
}

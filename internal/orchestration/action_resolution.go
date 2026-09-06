package orchestration

import (
	"errors"
	"fmt"
	"sort"
	"strings"
)

// ResolvedAction is the normalized discriminator produced before an action is
// allowed to reach a runtime executor.
type ResolvedAction struct {
	Raw        string
	Normalized string
}

// ResolveAction canonicalizes a planner or compact-tool action without
// deciding whether that action is supported by a particular executor.
func ResolveAction(raw string) ResolvedAction {
	return ResolvedAction{Raw: raw, Normalized: strings.TrimSpace(raw)}
}

// ReplanRequiredError means the planner produced no executable action. It is
// deliberately distinct from an unsupported non-empty action, which can still
// indicate a stale public tool schema.
type ReplanRequiredError struct {
	Reason string
}

func (e *ReplanRequiredError) Error() string {
	reason := strings.TrimSpace(e.Reason)
	if reason == "" {
		reason = "empty action from planner"
	}
	return "replan required: " + reason
}

// IsReplanRequired reports whether an action validation failure should return
// control to the planner instead of being dispatched or treated as a crash.
func IsReplanRequired(err error) bool {
	var target *ReplanRequiredError
	return errors.As(err, &target)
}

// ActionValidator owns the supported action set for one executor boundary.
// Empty actions request a replan; unsupported non-empty actions remain regular
// validation failures so callers can preserve their compatibility behavior.
type ActionValidator struct {
	supported map[string]struct{}
}

func NewActionValidator(actions []string) ActionValidator {
	supported := make(map[string]struct{}, len(actions))
	for _, action := range actions {
		if normalized := strings.TrimSpace(action); normalized != "" {
			supported[normalized] = struct{}{}
		}
	}
	return ActionValidator{supported: supported}
}

func (v ActionValidator) Validate(action ResolvedAction) error {
	if action.Normalized == "" {
		return &ReplanRequiredError{Reason: "empty action from planner"}
	}
	if _, ok := v.supported[action.Normalized]; ok {
		return nil
	}
	values := make([]string, 0, len(v.supported))
	for value := range v.supported {
		values = append(values, value)
	}
	sort.Strings(values)
	return fmt.Errorf("unsupported action %q; supported actions: %s", action.Normalized, strings.Join(values, ", "))
}

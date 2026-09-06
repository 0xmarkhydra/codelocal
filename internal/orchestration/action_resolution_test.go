package orchestration

import (
	"strings"
	"testing"
)

func TestActionValidatorRequestsReplanForEmptyAction(t *testing.T) {
	validator := NewActionValidator([]string{"inspect", "execute"})
	err := validator.Validate(ResolveAction(" \t "))
	if !IsReplanRequired(err) {
		t.Fatalf("empty action error=%v, want replan required", err)
	}
	if !strings.Contains(err.Error(), "empty action from planner") {
		t.Fatalf("empty action reason is not actionable: %v", err)
	}
}

func TestActionValidatorKeepsUnsupportedActionDistinct(t *testing.T) {
	validator := NewActionValidator([]string{"inspect", "execute"})
	err := validator.Validate(ResolveAction(" retired "))
	if err == nil || IsReplanRequired(err) {
		t.Fatalf("unsupported action error=%v", err)
	}
	if !strings.Contains(err.Error(), `unsupported action "retired"`) {
		t.Fatalf("unsupported action not normalized: %v", err)
	}
}

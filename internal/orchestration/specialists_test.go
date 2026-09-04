package orchestration

import (
	"errors"
	"testing"
)

func TestValidateDelegationBoundsDepthAndConcurrency(t *testing.T) {
	policies := DefaultSpecialistPolicies()
	if _, err := ValidateDelegation(policies, DelegationRequest{Role: SpecialistQuick, ParentDepth: 1}); !errors.Is(err, ErrDelegationDepth) {
		t.Fatalf("expected depth bound, got %v", err)
	}
	if _, err := ValidateDelegation(policies, DelegationRequest{Role: SpecialistInvestigator, ParentDepth: 0, ActiveChildren: 3}); !errors.Is(err, ErrDelegationConcurrency) {
		t.Fatalf("expected concurrency bound, got %v", err)
	}
}

func TestRecommendSpecialistIsSemanticNotProviderSpecific(t *testing.T) {
	if got := RecommendSpecialist("security review", 2, true); got != SpecialistSecurity {
		t.Fatalf("unexpected role %s", got)
	}
	if got := RecommendSpecialist("fix backend bug", 2, false); got != SpecialistImplementer {
		t.Fatalf("unexpected role %s", got)
	}
	if got := RecommendSpecialist("architecture", 4, false); got != SpecialistDeep {
		t.Fatalf("unexpected role %s", got)
	}
}

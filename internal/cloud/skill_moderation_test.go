package cloud

import "testing"

func TestSkillVersionTransitionPolicy(t *testing.T) {
	allowed := [][2]SkillVersionState{
		{SkillVersionCandidate, SkillVersionEvaluating},
		{SkillVersionCandidate, SkillVersionRejected},
		{SkillVersionEvaluating, SkillVersionCanary},
		{SkillVersionEvaluating, SkillVersionRejected},
		{SkillVersionCanary, SkillVersionPromoted},
		{SkillVersionPromoted, SkillVersionRolledBack},
		{SkillVersionRolledBack, SkillVersionPromoted},
		{SkillVersionPromoted, SkillVersionDeprecated},
	}
	for _, transition := range allowed {
		if !skillVersionTransitionAllowed(transition[0], transition[1]) {
			t.Fatalf("expected transition %s -> %s", transition[0], transition[1])
		}
	}

	for _, transition := range [][2]SkillVersionState{
		{SkillVersionCandidate, SkillVersionPromoted},
		{SkillVersionCandidate, SkillVersionCanary},
		{SkillVersionEvaluating, SkillVersionPromoted},
		{SkillVersionRejected, SkillVersionEvaluating},
		{SkillVersionBlocked, SkillVersionPromoted},
		{SkillVersionDeprecated, SkillVersionPromoted},
	} {
		if skillVersionTransitionAllowed(transition[0], transition[1]) {
			t.Fatalf("unsafe transition accepted %s -> %s", transition[0], transition[1])
		}
	}
}

func TestSkillEvaluationThresholdRequiresHighConfidence(t *testing.T) {
	if MinSkillEvaluationScore < 0.8 || MinSkillEvaluationScore > 1 {
		t.Fatalf("unexpected evaluation threshold %.2f", MinSkillEvaluationScore)
	}
}

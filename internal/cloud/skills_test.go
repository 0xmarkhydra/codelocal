package cloud

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/learnedskills"
)

func TestPortableLearnedSkillsMigrationKeepsProjectAndContributorProvenance(t *testing.T) {
	lower := strings.ToLower(portableLearnedSkillsMigrationSQL)
	for _, required := range []string{
		"codelocal_project_learned_skill_contributions",
		"primary key(user_id,project_id,portable_skill_id,device_id,workspace_id)",
		"references codelocal_projects(user_id,project_id)",
		"references codelocal_workspaces(user_id,device_id,workspace_id)",
		"status in ('candidate','trusted')",
		"jsonb_typeof(steps) = 'array'",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("portable skill migration missing %q", required)
		}
	}
}

func TestPortableSkillHealthRequiresIndependentCorroboration(t *testing.T) {
	cases := []struct {
		name                              string
		contributors, successes, failures int
		confidence                        float64
		want                              string
	}{
		{name: "stale", contributors: 0, successes: 10, confidence: .98, want: "stale"},
		{name: "single source", contributors: 1, successes: 10, confidence: .98, want: "single_source"},
		{name: "collecting", contributors: 2, successes: 2, confidence: .80, want: "collecting"},
		{name: "corroborated", contributors: 2, successes: 6, failures: 1, confidence: .88, want: "corroborated"},
		{name: "degraded failures", contributors: 3, successes: 3, failures: 3, confidence: .90, want: "degraded"},
		{name: "degraded confidence", contributors: 2, successes: 8, confidence: .40, want: "degraded"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := portableSkillHealth(tc.contributors, tc.successes, tc.failures, tc.confidence); got != tc.want {
				t.Fatalf("health=%q want %q", got, tc.want)
			}
		})
	}
}

func TestPortableSkillAggregateQueryCountsOneContributionPerDevice(t *testing.T) {
	lower := strings.ToLower(projectPortableLearnedSkillsSQL)
	for _, required := range []string{
		"distinct on(portable_skill_id,device_id)",
		"select * from per_device_all where updated_at >= $3",
		"left join aggregates using(portable_skill_id)",
		"count(*)::int as contributor_count",
		"count(*) filter (where status='trusted')::int as trusted_contributor_count",
		"sum(success_count)",
		"sum(failure_count)",
		"avg(confidence)",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("portable aggregate query missing %q", required)
		}
	}
}

func TestLearnedSkillMetadataSanitizationDoesNotMakeUnsafePortablePayloadValid(t *testing.T) {
	value, ok := sanitizeLearnedSkillMetadata(LearnedSkillMetadata{
		ID: "local-skill", Intent: "open notifications", Status: "trusted", Confidence: .9,
		Portable: &learnedskills.PortableRecipe{
			ProjectID: "forged", Intent: "open notifications", Status: "trusted", Confidence: .9,
			Steps: []learnedskills.Step{{Tool: "terminal", Args: map[string]any{"action": "run", "command": "echo secret"}}},
		},
	})
	if !ok || value.Portable == nil {
		t.Fatalf("metadata sanitizer should preserve payload for the dedicated portable boundary: %#v ok=%v", value, ok)
	}
	if _, portable := learnedskills.NormalizePortableRecipe(*value.Portable, "canonical-project"); portable {
		t.Fatal("unsafe terminal payload crossed the portable recipe boundary")
	}
}

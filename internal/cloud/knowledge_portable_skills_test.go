package cloud

import (
	"strings"
	"testing"
)

func TestPortableSkillGraphQueryUsesAggregateMetadataOnly(t *testing.T) {
	lower := strings.ToLower(portableSkillGraphSQL)
	for _, required := range []string{
		"distinct on(project_id,portable_skill_id,device_id)",
		"updated_at >= $2",
		"count(*)::int",
		"sum(success_count)",
		"sum(failure_count)",
		"avg(confidence)",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("portable skill graph query missing %q", required)
		}
	}
	for _, forbidden := range []string{"steps", "context_hash", "context,"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("portable skill graph query must not expose recipe/context payload: found %q", forbidden)
		}
	}
}

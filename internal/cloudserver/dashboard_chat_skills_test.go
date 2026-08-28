package cloudserver

import (
	"strings"
	"testing"
)

func TestDashboardSkillContextAutoSelectsUIUX(t *testing.T) {
	messages := []map[string]any{
		{"role": "system", "content": "base"},
		{"role": "user", "content": "Màn dashboard này nhìn xấu và khó chịu, làm đẹp hơn"},
	}
	plan := dashboardSkillPlan(messages)
	if len(plan.Selections) != 1 || plan.Selections[0].Skill.ID != "ui-ux-pro" {
		t.Fatalf("expected ui-ux-pro, got %#v", plan.Selections)
	}
	withSkills := dashboardWithSkillContext(messages)
	if len(withSkills) != len(messages)+1 {
		t.Fatalf("expected one bounded system context, got %d messages", len(withSkills))
	}
	content, _ := withSkills[len(withSkills)-1]["content"].(string)
	if !strings.Contains(content, dashboardSkillContextMarker) || !strings.Contains(content, "Project Brain") {
		t.Fatalf("unexpected skill context %q", content)
	}
}

func TestDashboardSkillContextSkipsBackendTask(t *testing.T) {
	messages := []map[string]any{{"role": "user", "content": "fix Redis reconnect and exponential backoff"}}
	if got := dashboardWithSkillContext(messages); len(got) != len(messages) {
		t.Fatalf("backend task should not receive UI skill context: %#v", got)
	}
}

func TestDashboardSkillContextIsNotDuplicated(t *testing.T) {
	messages := []map[string]any{{"role": "user", "content": "redesign dashboard responsive"}}
	first := dashboardWithSkillContext(messages)
	second := dashboardWithSkillContext(first)
	if len(first) != len(second) {
		t.Fatalf("skill context duplicated: first=%d second=%d", len(first), len(second))
	}
}

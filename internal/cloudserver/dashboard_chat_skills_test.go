package cloudserver

import (
	"encoding/json"
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

func TestDashboardSkillContextUnderstandsMultimodalUIMessage(t *testing.T) {
	messages := []map[string]any{{
		"role": "user",
		"content": []map[string]any{
			{"type": "text", "text": "Nhìn màn này khó chịu quá, làm đẹp hơn"},
			{"type": "image_url", "image_url": map[string]any{"url": "https://example.invalid/screenshot.png"}},
		},
	}}
	plan := dashboardSkillPlan(messages)
	if len(plan.Selections) != 1 || plan.Selections[0].Skill.ID != "ui-ux-pro" {
		t.Fatalf("expected ui-ux-pro for screenshot UI task, got %#v", plan.Selections)
	}
}

func TestDashboardImageAloneDoesNotForceUIUX(t *testing.T) {
	messages := []map[string]any{{
		"role": "user",
		"content": []map[string]any{
			{"type": "text", "text": "Phân tích hóa đơn trong ảnh này"},
			{"type": "image_url", "image_url": map[string]any{"url": "https://example.invalid/receipt.png"}},
		},
	}}
	if plan := dashboardSkillPlan(messages); len(plan.Selections) != 0 {
		t.Fatalf("non-UI image must not force UI skill: %#v", plan.Selections)
	}
}

func TestDashboardSkillHeaderUsesRealSelections(t *testing.T) {
	plan := dashboardSkillPlan([]map[string]any{{"role": "user", "content": "redesign dashboard responsive"}})
	raw := dashboardSkillHeaderValue(plan)
	if raw == "" {
		t.Fatal("expected skill metadata header")
	}
	var badges []dashboardSkillBadge
	if err := json.Unmarshal([]byte(raw), &badges); err != nil {
		t.Fatalf("invalid skill header: %v", err)
	}
	if len(badges) != 1 || badges[0].ID != "ui-ux-pro" || badges[0].Name != "UI/UX Pro" {
		t.Fatalf("unexpected badges %#v", badges)
	}
}

func TestDashboardSkillContextSkipsBackendTask(t *testing.T) {
	messages := []map[string]any{{"role": "user", "content": "fix Redis reconnect and exponential backoff"}}
	if got := dashboardWithSkillContext(messages); len(got) != len(messages) {
		t.Fatalf("backend task should not receive UI skill context: %#v", got)
	}
	if header := dashboardSkillHeaderValue(dashboardSkillPlan(messages)); header != "" {
		t.Fatalf("backend task must not advertise a UI skill: %s", header)
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

package cloudserver

import (
	"fmt"
	"strings"

	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
)

const dashboardSkillContextMarker = "[CodeLocal Skills]"

func dashboardLatestUserMessage(messages []map[string]any) string {
	for index := len(messages) - 1; index >= 0; index-- {
		role, _ := messages[index]["role"].(string)
		if role != "user" {
			continue
		}
		content, _ := messages[index]["content"].(string)
		if strings.TrimSpace(content) != "" {
			return strings.TrimSpace(content)
		}
	}
	return ""
}

func dashboardHasSkillContext(messages []map[string]any) bool {
	for _, message := range messages {
		role, _ := message["role"].(string)
		content, _ := message["content"].(string)
		if role == "system" && strings.Contains(content, dashboardSkillContextMarker) {
			return true
		}
	}
	return false
}

func dashboardSkillTask(message string) skillintel.TaskContext {
	lower := " " + strings.ToLower(strings.TrimSpace(message)) + " "
	signals := []string{}
	intents := []string{}
	ui := false
	for _, token := range []string{" ui ", "ux", "dashboard", "landing", "layout", "responsive", "accessibility", "design", "redesign", "visual", "screen", "page", "giao diện", "màn hình", "màn này", "trang này", "thiết kế", "bố cục", "đẹp", "xấu", "khó chịu", "dễ nhìn", "font", "màu"} {
		if strings.Contains(lower, token) {
			ui = true
			break
		}
	}
	if ui {
		signals = append(signals, "ui", "visual")
		intents = append(intents, "design_ui")
	}
	if strings.Contains(lower, "dashboard") {
		signals = append(signals, "dashboard")
	}
	if strings.Contains(lower, "responsive") || strings.Contains(lower, "mobile") || strings.Contains(lower, "điện thoại") {
		signals = append(signals, "responsive")
	}
	if strings.Contains(lower, "accessibility") || strings.Contains(lower, "a11y") || strings.Contains(lower, "aria") {
		signals = append(signals, "accessibility")
		intents = append(intents, "audit_ux")
	}
	if strings.Contains(lower, "redesign") || strings.Contains(lower, "refactor") || strings.Contains(lower, "đẹp hơn") || strings.Contains(lower, "xấu") || strings.Contains(lower, "khó chịu") || strings.Contains(lower, "làm lại") {
		intents = append(intents, "refactor_ui")
	}
	return skillintel.TaskContext{Query: message, Intents: intents, Signals: signals, MaxSelections: 3}
}

func dashboardSkillPlan(messages []map[string]any) skillintel.Plan {
	message := dashboardLatestUserMessage(messages)
	if message == "" {
		return skillintel.Plan{}
	}
	return skillintel.DefaultEngine().Plan(dashboardSkillTask(message))
}

func dashboardSkillSystemMessage(plan skillintel.Plan) string {
	if len(plan.Selections) == 0 {
		return ""
	}
	names := make([]string, 0, len(plan.Selections))
	for _, selection := range plan.Selections {
		names = append(names, selection.Skill.Name)
	}
	var builder strings.Builder
	builder.WriteString(dashboardSkillContextMarker)
	builder.WriteString(" Auto-selected reusable expertise: ")
	builder.WriteString(strings.Join(names, ", "))
	builder.WriteString(". User instruction, current project code and Project Brain remain authoritative over these reusable recommendations. Apply only the relevant guidance; do not mention internal routing unless useful to the user.\n")
	for _, match := range plan.Knowledge {
		fmt.Fprintf(&builder, "- %s: %s\n", match.Chunk.Title, match.Chunk.Content)
	}
	return strings.TrimSpace(builder.String())
}

func dashboardWithSkillContext(messages []map[string]any) []map[string]any {
	if len(messages) == 0 || dashboardHasSkillContext(messages) {
		return messages
	}
	context := dashboardSkillSystemMessage(dashboardSkillPlan(messages))
	if context == "" {
		return messages
	}
	out := make([]map[string]any, 0, len(messages)+1)
	out = append(out, messages...)
	out = append(out, map[string]any{"role": "system", "content": context})
	return out
}

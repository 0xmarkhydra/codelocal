package cloudserver

import (
	"encoding/json"
	"fmt"
	"strings"

	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
)

const (
	dashboardSkillContextMarker = "[CodeLocal Skills]"
	dashboardSkillHeader        = "X-CodeLocal-Skills"
)

type dashboardSkillBadge struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

func dashboardMessageTextAndImage(content any) (string, bool) {
	switch value := content.(type) {
	case string:
		return strings.TrimSpace(value), false
	case []map[string]any:
		return dashboardContentParts(value)
	case []any:
		parts := make([]map[string]any, 0, len(value))
		for _, item := range value {
			if part, ok := item.(map[string]any); ok {
				parts = append(parts, part)
			}
		}
		return dashboardContentParts(parts)
	default:
		return "", false
	}
}

func dashboardContentParts(parts []map[string]any) (string, bool) {
	texts := []string{}
	hasImage := false
	for _, part := range parts {
		kind, _ := part["type"].(string)
		switch kind {
		case "text":
			if text, _ := part["text"].(string); strings.TrimSpace(text) != "" {
				texts = append(texts, strings.TrimSpace(text))
			}
		case "image_url", "input_image", "image":
			hasImage = true
		}
	}
	return strings.TrimSpace(strings.Join(texts, " ")), hasImage
}

func dashboardLatestUserEvidence(messages []map[string]any) (string, bool) {
	for index := len(messages) - 1; index >= 0; index-- {
		role, _ := messages[index]["role"].(string)
		if role != "user" {
			continue
		}
		text, hasImage := dashboardMessageTextAndImage(messages[index]["content"])
		if text != "" || hasImage {
			return text, hasImage
		}
	}
	return "", false
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

func dashboardSkillPlan(messages []map[string]any) skillintel.Plan {
	message, hasImage := dashboardLatestUserEvidence(messages)
	if message == "" && !hasImage {
		return skillintel.Plan{}
	}
	task := skillintel.ClassifyTask(skillintel.TaskEvidence{Query: message, HasImage: hasImage})
	return skillintel.DefaultEngine().Plan(task)
}

func dashboardSkillBadges(plan skillintel.Plan) []dashboardSkillBadge {
	badges := make([]dashboardSkillBadge, 0, len(plan.Selections))
	for _, selection := range plan.Selections {
		badges = append(badges, dashboardSkillBadge{ID: selection.Skill.ID, Name: selection.Skill.Name})
	}
	return badges
}

func dashboardSkillHeaderValue(plan skillintel.Plan) string {
	badges := dashboardSkillBadges(plan)
	if len(badges) == 0 {
		return ""
	}
	raw, err := json.Marshal(badges)
	if err != nil {
		return ""
	}
	return string(raw)
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

func dashboardWithSkillPlan(messages []map[string]any, plan skillintel.Plan) []map[string]any {
	if len(messages) == 0 || dashboardHasSkillContext(messages) {
		return messages
	}
	context := dashboardSkillSystemMessage(plan)
	if context == "" {
		return messages
	}
	out := make([]map[string]any, 0, len(messages)+1)
	out = append(out, messages...)
	out = append(out, map[string]any{"role": "system", "content": context})
	return out
}

func dashboardWithSkillContext(messages []map[string]any) []map[string]any {
	return dashboardWithSkillPlan(messages, dashboardSkillPlan(messages))
}

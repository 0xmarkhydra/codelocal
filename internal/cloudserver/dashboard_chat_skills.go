package cloudserver

import (
	"context"
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
	ID      string `json:"id"`
	Name    string `json:"name"`
	Version string `json:"version"`
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

func dashboardSkillTask(messages []map[string]any, affinity map[string]float64) skillintel.TaskContext {
	message, hasImage := dashboardLatestUserEvidence(messages)
	if message == "" && !hasImage {
		return skillintel.TaskContext{}
	}
	return skillintel.ClassifyTask(skillintel.TaskEvidence{Query: message, HasImage: hasImage, Affinity: affinity})
}

func dashboardSkillPlanWithAffinity(messages []map[string]any, affinity map[string]float64) skillintel.Plan {
	task := dashboardSkillTask(messages, affinity)
	if strings.TrimSpace(task.Query) == "" && len(task.Intents) == 0 && len(task.Signals) == 0 {
		return skillintel.Plan{}
	}
	return skillintel.DefaultEngine().Plan(task)
}

func dashboardSkillPlan(messages []map[string]any) skillintel.Plan {
	return dashboardSkillPlanWithAffinity(messages, nil)
}

// dashboardSkillPlanForUser derives tenant-private preference from verified
// experience. Failure to read affinity is deliberately fail-open to neutral
// routing; chat must never fail because personalization is unavailable.
func dashboardSkillPlanForUser(ctx context.Context, s *Server, userID string, messages []map[string]any) skillintel.Plan {
	if s == nil || s.Store == nil || strings.TrimSpace(userID) == "" {
		return dashboardSkillPlan(messages)
	}
	catalog := skillintel.DefaultEngine().Catalog()
	ids := make([]string, 0, len(catalog))
	for _, manifest := range catalog {
		ids = append(ids, manifest.ID)
	}
	affinity, err := s.Store.SkillAffinity(ctx, userID, ids)
	if err != nil {
		return dashboardSkillPlan(messages)
	}
	return dashboardSkillPlanWithAffinity(messages, affinity)
}

func dashboardSkillBadges(plan skillintel.Plan) []dashboardSkillBadge {
	badges := make([]dashboardSkillBadge, 0, len(plan.Selections))
	for _, selection := range plan.Selections {
		badges = append(badges, dashboardSkillBadge{
			ID: selection.Skill.ID, Name: selection.Skill.Name, Version: selection.Skill.Version,
		})
	}
	return badges
}

func dashboardSkillMetadata(plan skillintel.Plan) json.RawMessage {
	badges := dashboardSkillBadges(plan)
	if len(badges) == 0 {
		return json.RawMessage(`[]`)
	}
	raw, err := json.Marshal(badges)
	if err != nil {
		return json.RawMessage(`[]`)
	}
	return json.RawMessage(raw)
}

func dashboardSkillHeaderValue(plan skillintel.Plan) string {
	raw := dashboardSkillMetadata(plan)
	if string(raw) == "[]" {
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

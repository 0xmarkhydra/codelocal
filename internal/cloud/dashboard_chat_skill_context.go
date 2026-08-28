package cloud

import (
	"context"
	"encoding/json"
)

type dashboardChatSkillsContextKey struct{}

// WithDashboardChatSkills carries immutable Skill identity/version metadata
// alongside one assistant execution. It is request-local and never contains
// project code or Skill knowledge content.
func WithDashboardChatSkills(ctx context.Context, skills json.RawMessage) context.Context {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(skills) == 0 {
		skills = json.RawMessage(`[]`)
	}
	copy := append(json.RawMessage(nil), skills...)
	return context.WithValue(ctx, dashboardChatSkillsContextKey{}, copy)
}

func dashboardChatSkillsFromContext(ctx context.Context) json.RawMessage {
	if ctx == nil {
		return json.RawMessage(`[]`)
	}
	raw, _ := ctx.Value(dashboardChatSkillsContextKey{}).(json.RawMessage)
	if len(raw) == 0 || !json.Valid(raw) {
		return json.RawMessage(`[]`)
	}
	return append(json.RawMessage(nil), raw...)
}

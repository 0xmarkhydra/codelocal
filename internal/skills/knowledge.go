package skills

import (
	"sort"
	"strings"
)

// KnowledgeChunk is reusable public expertise owned by a Skill. It must never
// contain project/user-private context; Project Brain is merged separately.
type KnowledgeChunk struct {
	ID       string   `json:"id"`
	SkillID  string   `json:"skillId"`
	Domain   string   `json:"domain"`
	Title    string   `json:"title"`
	Content  string   `json:"content"`
	Tags     []string `json:"tags,omitempty"`
	Priority int      `json:"priority,omitempty"`
	Source   string   `json:"source,omitempty"`
}

type KnowledgeMatch struct {
	Chunk KnowledgeChunk `json:"chunk"`
	Score float64        `json:"score"`
}

// BuiltinKnowledge is intentionally compact. It normalizes high-value rules
// from the upstream MIT skill into CodeLocal-owned retrieval units instead of
// copying a large prompt into every task.
func BuiltinKnowledge() []KnowledgeChunk {
	const source = "https://github.com/nextlevelbuilder/ui-ux-pro-max-skill"
	return []KnowledgeChunk{
		{ID: "uiux:a11y", SkillID: "ui-ux-pro", Domain: "accessibility", Title: "Accessibility baseline", Priority: 100, Source: source, Tags: []string{"accessibility", "a11y", "focus", "contrast", "keyboard", "aria"}, Content: "Keep normal text contrast at least 4.5:1; provide visible focus, semantic labels for icon-only controls, logical keyboard order, non-color-only status cues, reduced-motion support, and do not let sticky UI obscure focused controls."},
		{ID: "uiux:hierarchy", SkillID: "ui-ux-pro", Domain: "layout", Title: "Visual hierarchy and spacing", Priority: 95, Source: source, Tags: []string{"dashboard", "layout", "spacing", "hierarchy", "responsive"}, Content: "Create hierarchy with size, spacing and contrast rather than color alone. Use a consistent 4/8 spacing rhythm, predictable content widths, adaptive gutters, readable line lengths, and prevent fixed/sticky UI from covering content."},
		{ID: "uiux:icons", SkillID: "ui-ux-pro", Domain: "visual", Title: "Professional icon system", Priority: 90, Source: source, Tags: []string{"icons", "visual", "navigation", "polish"}, Content: "Use a consistent vector icon family and stroke style; do not use emoji as structural/navigation icons. Keep icon sizing tokenized, align icons with text, and give meaningful or interactive icons appropriate accessible names/state."},
		{ID: "uiux:responsive", SkillID: "ui-ux-pro", Domain: "responsive", Title: "Responsive layout", Priority: 90, Source: source, Tags: []string{"responsive", "mobile", "breakpoint", "viewport", "layout"}, Content: "Design mobile-first, keep systematic breakpoints and gutters, avoid horizontal scroll, prefer min-height dynamic viewport units on mobile, preserve landscape usability, and surface core content before secondary content on narrow screens."},
		{ID: "uiux:interaction", SkillID: "ui-ux-pro", Domain: "interaction", Title: "Interaction quality", Priority: 90, Source: source, Tags: []string{"interaction", "button", "feedback", "loading", "state"}, Content: "Every interactive element needs clear hover/press/focus/disabled/loading feedback without layout shift. Do not rely on hover or gesture-only interaction for essential actions; keep async actions from being double-submitted."},
		{ID: "uiux:motion", SkillID: "ui-ux-pro", Domain: "motion", Title: "Purposeful motion", Priority: 75, Source: source, Tags: []string{"animation", "motion", "performance", "reduced-motion"}, Content: "Use motion to explain state change, not decoration. Prefer transform/opacity, keep transitions interruptible, avoid layout reflow, limit simultaneous decorative motion, and respect reduced-motion preferences."},
		{ID: "uiux:forms", SkillID: "ui-ux-pro", Domain: "forms", Title: "Forms and feedback", Priority: 80, Source: source, Tags: []string{"form", "input", "error", "validation", "empty-state"}, Content: "Use visible labels, specific inline errors connected to fields, clear submit loading/success/error states, semantic input types, helpful empty states, and confirmation before destructive actions."},
		{ID: "uiux:performance", SkillID: "ui-ux-pro", Domain: "performance", Title: "Perceived UI performance", Priority: 70, Source: source, Tags: []string{"performance", "image", "font", "loading", "cls"}, Content: "Reserve image/media dimensions to prevent layout shift, lazy-load noncritical content, split heavy routes/features, minimize blocking third-party scripts, and choose feedback that matches expected wait time."},
	}
}

func knowledgeScore(chunk KnowledgeChunk, task TaskContext) float64 {
	text := strings.ToLower(strings.Join(append(append([]string{task.Query}, task.Intents...), task.Signals...), " "))
	if strings.TrimSpace(text) == "" {
		return 0
	}
	score := float64(chunk.Priority) / 500.0
	for _, tag := range chunk.Tags {
		if strings.Contains(text, strings.ToLower(tag)) {
			score += 0.28
		}
	}
	if strings.Contains(text, strings.ToLower(chunk.Domain)) {
		score += 0.25
	}
	return clamp(score, 0, 1)
}

func RetrieveKnowledge(task TaskContext, skillIDs []string, limit int) []KnowledgeMatch {
	if limit <= 0 || limit > 8 {
		limit = 4
	}
	allowed := map[string]struct{}{}
	for _, id := range skillIDs {
		allowed[strings.TrimSpace(id)] = struct{}{}
	}
	matches := []KnowledgeMatch{}
	for _, chunk := range BuiltinKnowledge() {
		if len(allowed) > 0 {
			if _, ok := allowed[chunk.SkillID]; !ok {
				continue
			}
		}
		score := knowledgeScore(chunk, task)
		if score < 0.18 {
			continue
		}
		matches = append(matches, KnowledgeMatch{Chunk: chunk, Score: round(score)})
	}
	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].Score == matches[j].Score {
			return matches[i].Chunk.Priority > matches[j].Chunk.Priority
		}
		return matches[i].Score > matches[j].Score
	})
	if len(matches) > limit {
		matches = matches[:limit]
	}
	return matches
}

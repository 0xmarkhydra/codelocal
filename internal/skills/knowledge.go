package skills

import (
	"sort"
	"strings"
)

// KnowledgeChunk is reusable expertise owned by one immutable Skill version.
// It must never contain project/user-private context; Project Brain is merged
// separately at orchestration time.
type KnowledgeChunk struct {
	ID           string   `json:"id"`
	SkillID      string   `json:"skillId"`
	SkillVersion string   `json:"skillVersion"`
	Domain       string   `json:"domain"`
	Title        string   `json:"title"`
	Content      string   `json:"content"`
	Tags         []string `json:"tags,omitempty"`
	Priority     int      `json:"priority,omitempty"`
	Source       string   `json:"source,omitempty"`
	SourceRef    string   `json:"sourceRef,omitempty"`
	SourceHash   string   `json:"sourceHash,omitempty"`
}

type KnowledgeMatch struct {
	Chunk KnowledgeChunk `json:"chunk"`
	Score float64        `json:"score"`
}

// KnowledgeStore makes retrieval independent from storage. The initial
// in-memory implementation keeps this PR self-contained; Cloud/desktop can
// later use database/object-store/BM25/vector adapters without changing Router
// or Planner contracts.
type KnowledgeStore interface {
	Search(task TaskContext, selections []Selection, limit int) []KnowledgeMatch
}

type MemoryKnowledgeStore struct {
	chunks []KnowledgeChunk
}

func NewMemoryKnowledgeStore(chunks []KnowledgeChunk) *MemoryKnowledgeStore {
	copyChunks := append([]KnowledgeChunk(nil), chunks...)
	return &MemoryKnowledgeStore{chunks: copyChunks}
}

func DefaultKnowledgeStore() KnowledgeStore {
	return NewMemoryKnowledgeStore(BuiltinKnowledge())
}

// BuiltinKnowledge is intentionally compact for the first vertical slice. The
// important boundary is that Planner talks to KnowledgeStore, not this literal
// slice, so upstream ingestion can replace it without touching execution code.
func BuiltinKnowledge() []KnowledgeChunk {
	const source = "https://github.com/nextlevelbuilder/ui-ux-pro-max-skill"
	const version = "1.0.0"
	return []KnowledgeChunk{
		{ID: "uiux:a11y", SkillID: "ui-ux-pro", SkillVersion: version, Domain: "accessibility", Title: "Accessibility baseline", Priority: 100, Source: source, Tags: []string{"accessibility", "a11y", "focus", "contrast", "keyboard", "aria"}, Content: "Keep normal text contrast at least 4.5:1; provide visible focus, semantic labels for icon-only controls, logical keyboard order, non-color-only status cues, reduced-motion support, and do not let sticky UI obscure focused controls."},
		{ID: "uiux:hierarchy", SkillID: "ui-ux-pro", SkillVersion: version, Domain: "layout", Title: "Visual hierarchy and spacing", Priority: 95, Source: source, Tags: []string{"dashboard", "layout", "spacing", "hierarchy", "responsive"}, Content: "Create hierarchy with size, spacing and contrast rather than color alone. Use a consistent 4/8 spacing rhythm, predictable content widths, adaptive gutters, readable line lengths, and prevent fixed/sticky UI from covering content."},
		{ID: "uiux:icons", SkillID: "ui-ux-pro", SkillVersion: version, Domain: "visual", Title: "Professional icon system", Priority: 90, Source: source, Tags: []string{"icons", "visual", "navigation", "polish"}, Content: "Use a consistent vector icon family and stroke style; do not use emoji as structural/navigation icons. Keep icon sizing tokenized, align icons with text, and give meaningful or interactive icons appropriate accessible names/state."},
		{ID: "uiux:responsive", SkillID: "ui-ux-pro", SkillVersion: version, Domain: "responsive", Title: "Responsive layout", Priority: 90, Source: source, Tags: []string{"responsive", "mobile", "breakpoint", "viewport", "layout"}, Content: "Design mobile-first, keep systematic breakpoints and gutters, avoid horizontal scroll, prefer min-height dynamic viewport units on mobile, preserve landscape usability, and surface core content before secondary content on narrow screens."},
		{ID: "uiux:interaction", SkillID: "ui-ux-pro", SkillVersion: version, Domain: "interaction", Title: "Interaction quality", Priority: 90, Source: source, Tags: []string{"interaction", "button", "feedback", "loading", "state"}, Content: "Every interactive element needs clear hover/press/focus/disabled/loading feedback without layout shift. Do not rely on hover or gesture-only interaction for essential actions; keep async actions from being double-submitted."},
		{ID: "uiux:motion", SkillID: "ui-ux-pro", SkillVersion: version, Domain: "motion", Title: "Purposeful motion", Priority: 75, Source: source, Tags: []string{"animation", "motion", "performance", "reduced-motion"}, Content: "Use motion to explain state change, not decoration. Prefer transform/opacity, keep transitions interruptible, avoid layout reflow, limit simultaneous decorative motion, and respect reduced-motion preferences."},
		{ID: "uiux:forms", SkillID: "ui-ux-pro", SkillVersion: version, Domain: "forms", Title: "Forms and feedback", Priority: 80, Source: source, Tags: []string{"form", "input", "error", "validation", "empty-state"}, Content: "Use visible labels, specific inline errors connected to fields, clear submit loading/success/error states, semantic input types, helpful empty states, and confirmation before destructive actions."},
		{ID: "uiux:performance", SkillID: "ui-ux-pro", SkillVersion: version, Domain: "performance", Title: "Perceived UI performance", Priority: 70, Source: source, Tags: []string{"performance", "image", "font", "loading", "cls"}, Content: "Reserve image/media dimensions to prevent layout shift, lazy-load noncritical content, split heavy routes/features, minimize blocking third-party scripts, and choose feedback that matches expected wait time."},
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

func (s *MemoryKnowledgeStore) Search(task TaskContext, selections []Selection, limit int) []KnowledgeMatch {
	if s == nil {
		return nil
	}
	if limit <= 0 || limit > 8 {
		limit = 4
	}
	allowed := map[string]string{}
	for _, selection := range selections {
		allowed[strings.TrimSpace(selection.Skill.ID)] = strings.TrimSpace(selection.Skill.Version)
	}
	if len(allowed) == 0 {
		return nil
	}
	matches := []KnowledgeMatch{}
	for _, chunk := range s.chunks {
		version, ok := allowed[chunk.SkillID]
		if !ok || (chunk.SkillVersion != "" && chunk.SkillVersion != version) {
			continue
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

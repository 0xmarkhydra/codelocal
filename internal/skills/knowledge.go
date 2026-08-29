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
	const (
		skillID    = "ui-ux-pro"
		version    = "1.0.0"
		source     = "https://github.com/nextlevelbuilder/ui-ux-pro-max-skill"
		sourceRef  = "8bd29e775453ebcae52b6e6514fbf134df0c5770"
		sourceHash = "49e14e670dee9c587d47cd9e78935eb6182a0466"
	)
	chunk := func(id, domain, title, content string, priority int, tags ...string) KnowledgeChunk {
		return KnowledgeChunk{
			ID: id, SkillID: skillID, SkillVersion: version, Domain: domain,
			Title: title, Content: content, Priority: priority, Tags: tags,
			Source: source, SourceRef: sourceRef, SourceHash: sourceHash,
		}
	}
	return []KnowledgeChunk{
		chunk("uiux:a11y", "accessibility", "Accessibility baseline", "Keep normal text contrast at least 4.5:1; provide visible focus, semantic labels for icon-only controls, logical keyboard order, non-color-only status cues, reduced-motion support, and do not let sticky UI obscure focused controls.", 100, "accessibility", "a11y", "focus", "contrast", "keyboard", "aria"),
		chunk("uiux:hierarchy", "layout", "Visual hierarchy and spacing", "Create hierarchy with size, spacing and contrast rather than color alone. Use a consistent 4/8 spacing rhythm, predictable content widths, adaptive gutters, readable line lengths, and prevent fixed/sticky UI from covering content.", 95, "dashboard", "layout", "spacing", "hierarchy", "responsive"),
		chunk("uiux:icons", "visual", "Professional icon system", "Use a consistent vector icon family and stroke style; do not use emoji as structural/navigation icons. Keep icon sizing tokenized, align icons with text, and give meaningful or interactive icons appropriate accessible names/state.", 90, "icons", "visual", "navigation", "polish"),
		chunk("uiux:responsive", "responsive", "Responsive layout", "Design mobile-first, keep systematic breakpoints and gutters, avoid horizontal scroll, prefer min-height dynamic viewport units on mobile, preserve landscape usability, and surface core content before secondary content on narrow screens.", 90, "responsive", "mobile", "breakpoint", "viewport", "layout"),
		chunk("uiux:interaction", "interaction", "Interaction quality", "Every interactive element needs clear hover/press/focus/disabled/loading feedback without layout shift. Do not rely on hover or gesture-only interaction for essential actions; keep async actions from being double-submitted.", 90, "interaction", "button", "feedback", "loading", "state"),
		chunk("uiux:motion", "motion", "Purposeful motion", "Use motion to explain state change, not decoration. Prefer transform/opacity, keep transitions interruptible, avoid layout reflow, limit simultaneous decorative motion, and respect reduced-motion preferences.", 75, "animation", "motion", "performance", "reduced-motion"),
		chunk("uiux:forms", "forms", "Forms and feedback", "Use visible labels, specific inline errors connected to fields, clear submit loading/success/error states, semantic input types, helpful empty states, and confirmation before destructive actions.", 80, "form", "input", "error", "validation", "empty-state"),
		chunk("uiux:performance", "performance", "Perceived UI performance", "Reserve image/media dimensions to prevent layout shift, lazy-load noncritical content, split heavy routes/features, minimize blocking third-party scripts, and choose feedback that matches expected wait time.", 70, "performance", "image", "font", "loading", "cls"),
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

package skills

import (
	"path/filepath"
	"strings"
)

// TaskEvidence is the cross-surface input used to classify when reusable Skill
// expertise is worth activating. Web chat, MCP orchestration and the future
// desktop app should all feed this same classifier instead of maintaining
// surface-specific keyword rules.
type TaskEvidence struct {
	Query        string
	Stack        []string
	TouchedFiles []string
	RecentErrors []string
	HasImage     bool
	Affinity     map[string]float64
}

func ClassifyTask(e TaskEvidence) TaskContext {
	stack := normalizeStack(e.Stack)
	text := taskEvidenceText(e)
	uiIntent := hasUIIntent(text) || e.HasImage
	frontendEvidence := e.HasImage || hasFrontendFile(e.TouchedFiles) || hasFrontendFramework(stack)

	intents := []string{}
	signals := []string{}
	if frontendEvidence {
		signals = append(signals, "frontend")
	}
	if e.HasImage {
		signals = append(signals, "screenshot")
	}
	if uiIntent {
		signals = append(signals, "ui", "visual")
		intents = append(intents, "design_ui")
	}
	if containsAny(text, "audit", "review", "accessibility", "a11y", "đánh giá", "rà soát") {
		intents = appendUnique(intents, "audit_ux")
	}
	if containsAny(text, "refactor", "redesign", "improve", "polish", "đẹp hơn", "xấu", "khó chịu", "làm lại", "sửa giao diện") {
		intents = appendUnique(intents, "refactor_ui")
	}
	if containsAny(text, "dashboard") {
		signals = appendUnique(signals, "dashboard")
	}
	if containsAny(text, "landing") {
		signals = appendUnique(signals, "landing")
	}
	if containsAny(text, "responsive", "mobile", "điện thoại") {
		signals = appendUnique(signals, "responsive")
	}
	if containsAny(text, "accessibility", "a11y", "keyboard", "focus", "aria") {
		signals = appendUnique(signals, "accessibility")
	}

	// Technology evidence alone must never activate a UI skill for a backend task.
	if !uiIntent {
		intents = nil
		signals = nil
	}

	return TaskContext{
		Query:         strings.TrimSpace(e.Query),
		Intents:       intents,
		Stack:         stack,
		Signals:       signals,
		Trivial:       trivialTask(e, text),
		Affinity:      e.Affinity,
		MaxSelections: 3,
	}
}

func normalizeStack(values []string) []string {
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		normalized = strings.ReplaceAll(normalized, ".", "")
		normalized = strings.ReplaceAll(normalized, " ", "")
		switch normalized {
		case "next", "nextjs":
			normalized = "nextjs"
		case "reactnative", "react-native":
			normalized = "react-native"
		}
		if normalized == "" {
			continue
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	return out
}

func taskEvidenceText(e TaskEvidence) string {
	parts := []string{e.Query}
	parts = append(parts, e.RecentErrors...)
	for _, path := range e.TouchedFiles {
		parts = append(parts, filepath.ToSlash(path))
	}
	return " " + strings.ToLower(strings.Join(parts, " ")) + " "
}

func hasFrontendFile(paths []string) bool {
	for _, path := range paths {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".tsx", ".jsx", ".vue", ".svelte", ".html", ".css", ".scss", ".swift", ".dart":
			return true
		}
	}
	return false
}

func hasFrontendFramework(stack []string) bool {
	for _, item := range stack {
		switch strings.ToLower(item) {
		case "react", "nextjs", "vue", "svelte", "flutter", "swiftui", "react-native", "html", "tailwind":
			return true
		}
	}
	return false
}

func hasUIIntent(text string) bool {
	return containsAny(text,
		" ui ", "ux", "interface", "dashboard", "landing", "layout", "responsive", "accessibility", "a11y", "typography", "visual", "design", "redesign", "sidebar", "navbar", "modal", "form", "button", "card", "screen", "page",
		"giao diện", "màn hình", "màn này", "trang này", "thiết kế", "bố cục", "đẹp", "xấu", "khó chịu", "dễ nhìn", "font", "màu", "khoảng cách",
	)
}

func trivialTask(e TaskEvidence, text string) bool {
	if e.HasImage || len(e.TouchedFiles) > 1 || len(e.RecentErrors) > 0 {
		return false
	}
	if hasUIIntent(text) && containsAny(text, "đẹp", "xấu", "design", "layout", "responsive", "accessibility", "audit", "review", "redesign") {
		return false
	}
	if len(strings.Fields(strings.TrimSpace(e.Query))) > 14 {
		return false
	}
	return containsAny(text, "rename ", "change text", "change label", "replace text", "đổi chữ", "đổi text", "sửa chữ", "đổi tên", "typo", "chính tả")
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(text, value) {
			return true
		}
	}
	return false
}

func appendUnique(values []string, value string) []string {
	for _, current := range values {
		if current == value {
			return values
		}
	}
	return append(values, value)
}

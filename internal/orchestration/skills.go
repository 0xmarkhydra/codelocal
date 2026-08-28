package orchestration

import (
	"path/filepath"
	"strings"

	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
)

// SkillPlan is projected inside the existing AgentPlan. Skills therefore stay
// invisible as MCP tools: users chat normally, while context_for_task selects
// reusable expertise server-side without expanding the public tool surface.
type SkillPlan = skillintel.Plan

func skillStack(project ProjectProfile) []string {
	values := append(append([]string(nil), project.Frameworks...), project.Languages...)
	out := make([]string, 0, len(values))
	seen := map[string]struct{}{}
	for _, value := range values {
		normalized := strings.ToLower(strings.TrimSpace(value))
		normalized = strings.ReplaceAll(normalized, ".", "")
		normalized = strings.ReplaceAll(normalized, " ", "")
		switch normalized {
		case "next", "nextjs", "next.js":
			normalized = "nextjs"
		case "reactnative", "react-native":
			normalized = "react-native"
		case "typescript", "javascript":
			// Language alone is useful context but should not imply UI intent.
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

func skillTaskText(input PlanInput) string {
	parts := []string{input.Task}
	parts = append(parts, input.RecentErrors...)
	for _, path := range input.TouchedFiles {
		parts = append(parts, filepath.ToSlash(path))
	}
	return strings.ToLower(strings.Join(parts, " "))
}

func hasFrontendFile(paths []string) bool {
	for _, path := range paths {
		ext := strings.ToLower(filepath.Ext(path))
		switch ext {
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

func skillUIIntent(text string) bool {
	return containsAny(text,
		" ui ", "ux", "interface", "dashboard", "landing", "layout", "responsive", "accessibility", "a11y", "typography", "visual", "design", "redesign", "sidebar", "navbar", "modal", "form", "button", "card", "screen", "page",
		"giao diện", "màn hình", "màn này", "trang này", "thiết kế", "bố cục", "đẹp", "xấu", "khó chịu", "dễ nhìn", "responsive", "font", "màu", "khoảng cách",
	)
}

func trivialSkillTask(input PlanInput, text string) bool {
	if len(input.TouchedFiles) > 1 || len(input.RecentErrors) > 0 {
		return false
	}
	if skillUIIntent(text) && containsAny(text, "đẹp", "xấu", "design", "layout", "responsive", "accessibility", "audit", "review", "redesign") {
		return false
	}
	words := strings.Fields(strings.TrimSpace(input.Task))
	if len(words) > 14 {
		return false
	}
	return containsAny(text,
		"rename ", "change text", "change label", "replace text", "đổi chữ", "đổi text", "sửa chữ", "đổi tên", "typo", "chính tả",
	)
}

func skillTaskContext(input PlanInput) skillintel.TaskContext {
	text := " " + skillTaskText(input) + " "
	stack := skillStack(input.Project)
	signals := []string{}
	intents := []string{}

	frontendEvidence := hasFrontendFile(input.TouchedFiles) || hasFrontendFramework(stack)
	uiIntent := skillUIIntent(text)
	if frontendEvidence {
		signals = append(signals, "frontend")
	}
	if uiIntent {
		signals = append(signals, "visual", "ui")
		intents = append(intents, "design_ui")
	}
	if containsAny(text, "audit", "review", "accessibility", "a11y", "đánh giá", "rà soát") {
		intents = append(intents, "audit_ux")
	}
	if containsAny(text, "refactor", "redesign", "improve", "polish", "đẹp hơn", "xấu", "khó chịu", "làm lại", "sửa giao diện") {
		intents = append(intents, "refactor_ui")
	}
	if containsAny(text, "dashboard") {
		signals = append(signals, "dashboard")
	}
	if containsAny(text, "landing") {
		signals = append(signals, "landing")
	}
	if containsAny(text, "responsive", "mobile", "điện thoại") {
		signals = append(signals, "responsive")
	}
	if containsAny(text, "accessibility", "a11y", "keyboard", "focus", "aria") {
		signals = append(signals, "accessibility")
	}

	// Framework evidence alone must never activate a UI skill for a backend task.
	// A concrete UI intent or frontend file plus visual/UI language is required.
	if !uiIntent {
		intents = nil
		signals = nil
	}

	return skillintel.TaskContext{
		Query:         input.Task,
		Intents:       intents,
		Stack:         stack,
		Signals:       signals,
		Trivial:       trivialSkillTask(input, text),
		MaxSelections: 3,
	}
}

func buildSkillPlan(input PlanInput) SkillPlan {
	return skillintel.DefaultEngine().Plan(skillTaskContext(input))
}

package automation

import (
	"context"
	"errors"
	"fmt"
	"strings"
)

// ComputerObservation is a compact, structured view of the current desktop
// state. It intentionally prefers native window/accessibility metadata over a
// screenshot so ChatGPT can reason about the UI without paying the latency and
// token cost of vision on every step.
type ComputerObservation struct {
	Windows     any    `json:"windows"`
	UITree      any    `json:"uiTree,omitempty"`
	UITreeError string `json:"uiTreeError,omitempty"`
	WindowID    string `json:"windowId,omitempty"`
}

// ObserveComputer always preserves a successful window observation. When a
// windowId is supplied but the accessibility tree is unavailable, CodeLocal
// returns uiTreeError instead of discarding the useful window state.
func ObserveComputer(ctx context.Context, computer *ComputerController, windowID string) (ComputerObservation, error) {
	if computer == nil {
		return ComputerObservation{}, errors.New("Computer Use helper unavailable")
	}
	windows, err := computer.Call(ctx, "list_windows", map[string]any{})
	if err != nil {
		return ComputerObservation{}, err
	}
	observation := ComputerObservation{Windows: windows, WindowID: strings.TrimSpace(windowID)}
	if observation.WindowID == "" {
		return observation, nil
	}
	uiTree, err := computer.Call(ctx, "ui_tree", map[string]any{"windowId": observation.WindowID})
	if err != nil {
		observation.UITreeError = err.Error()
		return observation, nil
	}
	observation.UITree = uiTree
	return observation, nil
}

type semanticCandidate struct {
	ElementID string
	Role      string
	Name      string
	Desc      string
	Value     string
	Bounds    any
	Score     int
}

func normalizeSemanticText(value string) string {
	return strings.Join(strings.Fields(strings.ToLower(strings.TrimSpace(value))), " ")
}

func semanticScore(target string, node map[string]any) int {
	target = normalizeSemanticText(target)
	if target == "" {
		return 0
	}
	name := normalizeSemanticText(fmt.Sprint(node["name"]))
	desc := normalizeSemanticText(fmt.Sprint(node["description"]))
	value := normalizeSemanticText(fmt.Sprint(node["value"]))
	role := normalizeSemanticText(fmt.Sprint(node["role"]))
	score := 0
	for _, candidate := range []struct {
		text   string
		exact  int
		inside int
	}{
		{name, 100, 70},
		{desc, 85, 55},
		{value, 65, 45},
		{role, 35, 20},
	} {
		if candidate.text == "" || candidate.text == "<nil>" {
			continue
		}
		if candidate.text == target {
			score = maxInt(score, candidate.exact)
			continue
		}
		if strings.Contains(candidate.text, target) || strings.Contains(target, candidate.text) {
			score = maxInt(score, candidate.inside)
		}
	}
	// Prefer interactive controls when text scores are otherwise similar.
	if score > 0 {
		if strings.Contains(role, "button") || strings.Contains(role, "link") || strings.Contains(role, "menu") || strings.Contains(role, "checkbox") || strings.Contains(role, "radio") {
			score += 10
		}
		if enabled, ok := node["enabled"].(bool); ok && !enabled {
			// A disabled exact label must not beat an enabled near-match.
			score -= 60
		}
	}
	return score
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func walkSemanticNodes(value any, target string, best *semanticCandidate) {
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			walkSemanticNodes(child, target, best)
		}
	case map[string]any:
		if elementID := strings.TrimSpace(fmt.Sprint(typed["elementId"])); elementID != "" && elementID != "<nil>" {
			score := semanticScore(target, typed)
			if score > best.Score {
				best.ElementID = elementID
				best.Role = strings.TrimSpace(fmt.Sprint(typed["role"]))
				best.Name = strings.TrimSpace(fmt.Sprint(typed["name"]))
				best.Desc = strings.TrimSpace(fmt.Sprint(typed["description"]))
				best.Value = strings.TrimSpace(fmt.Sprint(typed["value"]))
				best.Bounds = typed["bounds"]
				best.Score = score
			}
		}
		for _, key := range []string{"nodes", "node", "children"} {
			if child, ok := typed[key]; ok {
				walkSemanticNodes(child, target, best)
			}
		}
	}
}

// FindComputerElement resolves human wording to the best accessibility element
// in a fresh UI tree. This is a deterministic local fast-path; vision remains a
// fallback for interfaces that expose no useful accessibility metadata.
func FindComputerElement(ctx context.Context, computer *ComputerController, windowID, target string) (map[string]any, error) {
	windowID = strings.TrimSpace(windowID)
	target = strings.TrimSpace(target)
	if windowID == "" {
		return nil, errors.New("semantic target lookup requires windowId")
	}
	if target == "" {
		return nil, errors.New("semantic target lookup requires target")
	}
	uiTree, err := computer.Call(ctx, "ui_tree", map[string]any{"windowId": windowID})
	if err != nil {
		return nil, err
	}
	best := semanticCandidate{}
	walkSemanticNodes(uiTree, target, &best)
	if best.ElementID == "" || best.Score < 35 {
		return nil, fmt.Errorf("no accessible UI element matched %q", target)
	}
	result := map[string]any{
		"elementId":   best.ElementID,
		"role":        best.Role,
		"name":        best.Name,
		"description": best.Desc,
		"value":       best.Value,
		"score":       best.Score,
	}
	if best.Bounds != nil {
		result["bounds"] = best.Bounds
	}
	return result, nil
}

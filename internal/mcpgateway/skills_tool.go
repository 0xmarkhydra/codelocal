package mcpgateway

import (
	"context"
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

func compactSkillToolDefinitions() []compactToolDef {
	return []compactToolDef{{
		Name:        "skills",
		Title:       "Use CodeLocal Skills",
		Description: "Discover and plan centrally managed CodeLocal skills for the current task. Skills are auto-routed expertise, not project knowledge: Project Brain remains authoritative for project-specific facts and conventions. Normal users do not need to install skills; use route or plan when specialist knowledge could materially improve the task, and skip trivial work.",
		Schema: actionSchema([]string{"catalog", "route", "plan"}, map[string]any{
			"query":         str("Current user task. Required for route and plan."),
			"intents":       array(str("Normalized task intent."), "Optional normalized task intents."),
			"stack":         array(str("Project technology or framework."), "Known project stack signals."),
			"signals":       array(str("Sanitized task-context signal."), "Optional sanitized contextual signals such as frontend, screenshot, dashboard or failing test. Do not include secrets or private source code."),
			"trivial":       boolean("Set true when specialist knowledge is unlikely to improve a simple change."),
			"maxSelections": integer("Maximum skills to select. CodeLocal caps this at 3.", 1, 3),
			"affinity":      anyObject("Optional user-specific skill affinity scores from -0.25 to 0.25. This personalizes ranking without changing global skill knowledge."),
		}),
		Annotations: compactAnnotations("Use CodeLocal Skills", true, false, false),
		Execute:     executeSkillsTool,
	}}
}

func executeSkillsTool(_ context.Context, _ *Service, _ string, args map[string]any, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action, _ := args["action"].(string)
	action = strings.TrimSpace(action)
	engine := skills.DefaultEngine()
	if action == "catalog" {
		return textResult(map[string]any{"skills": engine.Catalog()}, false), nil
	}
	query, _ := args["query"].(string)
	if strings.TrimSpace(query) == "" {
		return errorResult(fmt.Errorf("%s requires query", action)), nil
	}
	task := skills.TaskContext{
		Query:         query,
		Intents:       stringSliceArg(args["intents"]),
		Stack:         stringSliceArg(args["stack"]),
		Signals:       stringSliceArg(args["signals"]),
		Trivial:       boolArg(args["trivial"]),
		MaxSelections: intArg(args["maxSelections"]),
		Affinity:      floatMapArg(args["affinity"]),
	}
	switch action {
	case "route":
		return textResult(map[string]any{"selections": engine.Route(task)}, false), nil
	case "plan":
		return textResult(engine.Plan(task), false), nil
	default:
		return errorResult(unsupportedActionSchemaError(action)), nil
	}
}

func stringSliceArg(value any) []string {
	values, ok := value.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if item, ok := value.(string); ok && strings.TrimSpace(item) != "" {
			out = append(out, item)
		}
	}
	return out
}

func boolArg(value any) bool {
	result, _ := value.(bool)
	return result
}

func intArg(value any) int {
	switch value := value.(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return 0
	}
}

func floatMapArg(value any) map[string]float64 {
	values, ok := value.(map[string]any)
	if !ok {
		return nil
	}
	out := make(map[string]float64, len(values))
	for key, value := range values {
		if score, ok := value.(float64); ok {
			out[key] = score
		}
	}
	return out
}

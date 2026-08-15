package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"net/url"
	"regexp"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/learnedskills"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const learnedSkillReplayThreshold = 0.64

func learnedSkillEligibleForReplay(recipe *learnedskills.Recipe) bool {
	return recipe != nil && recipe.Status == learnedskills.StatusTrusted && recipe.MatchScore >= learnedSkillReplayThreshold
}

func learnedString(value any) string {
	text, _ := value.(string)
	return text
}

func learnedBool(value any, fallback bool) bool {
	flag, ok := value.(bool)
	if !ok {
		return fallback
	}
	return flag
}

var safeSimctlLaunch = regexp.MustCompile(`^xcrun\s+simctl\s+launch\s+booted\s+[A-Za-z0-9.-]+$`)

func (s *Service) callLearnedSkillRuntime(ctx context.Context, userID, session, workspaceKey, tool string, args map[string]any, sideEffecting bool) (any, bool, error) {
	key := strings.TrimSpace(workspaceKey)
	if key == "" {
		key = s.route(userID, session)
	}
	if key == "" {
		return nil, false, nil
	}
	workspace, err := s.Workspaces.Activate(ctx, userID, key)
	if err != nil {
		return nil, false, err
	}
	if workspace == nil || !capabilityBool(workspace.Capabilities, "learnedSkills") {
		return nil, false, nil
	}
	result, callErr := s.Hub.Call(ctx, userID, key, session, tool, args, sideEffecting, cloud.RandomHex(16))
	if callErr != nil {
		return nil, true, callErr
	}
	if !result.OK {
		return nil, true, errors.New(firstNonEmpty(result.Error, result.ErrorCode, "learned skill runtime operation failed"))
	}
	return result.Result, true, nil
}

func decodeLearnedRecipe(value any) *learnedskills.Recipe {
	root, ok := value.(map[string]any)
	if !ok || root["match"] == nil {
		return nil
	}
	raw, err := json.Marshal(root["match"])
	if err != nil {
		return nil
	}
	var recipe learnedskills.Recipe
	if json.Unmarshal(raw, &recipe) != nil || recipe.ID == "" || len(recipe.Steps) == 0 {
		return nil
	}
	return &recipe
}

func (s *Service) matchLearnedSkill(ctx context.Context, userID, session, workspaceKey, intent, taskKind string) (*learnedskills.Recipe, bool, error) {
	value, supported, err := s.callLearnedSkillRuntime(ctx, userID, session, workspaceKey, "learned_skill_match", map[string]any{
		"intent": intent, "taskKind": taskKind,
	}, false)
	if err != nil || !supported {
		return nil, supported, err
	}
	return decodeLearnedRecipe(value), true, nil
}

func (s *Service) recordLearnedSkill(ctx context.Context, userID, session, workspaceKey, intent, taskKind string, steps []learnedskills.Step) (*learnedskills.Recipe, error) {
	if len(steps) == 0 {
		return nil, nil
	}
	value, supported, err := s.callLearnedSkillRuntime(ctx, userID, session, workspaceKey, "learned_skill_record", map[string]any{
		"intent": intent, "taskKind": taskKind, "steps": steps, "verified": true,
	}, true)
	if err != nil || !supported {
		return nil, err
	}
	root, _ := value.(map[string]any)
	raw, _ := json.Marshal(root["recipe"])
	var recipe learnedskills.Recipe
	if json.Unmarshal(raw, &recipe) != nil || recipe.ID == "" {
		return nil, nil
	}
	return &recipe, nil
}

func (s *Service) feedbackLearnedSkill(ctx context.Context, userID, session, workspaceKey, id string, success bool) {
	if strings.TrimSpace(id) == "" {
		return
	}
	_, _, _ = s.callLearnedSkillRuntime(ctx, userID, session, workspaceKey, "learned_skill_feedback", map[string]any{"id": id, "success": success}, true)
}

func sanitizedBrowserURL(value string) (string, bool) {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return "", false
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return parsed.String(), true
}

func safeLearnedAutomationCommand(command string) bool {
	command = strings.Join(strings.Fields(strings.TrimSpace(command)), " ")
	return safeSimctlLaunch.MatchString(command)
}

func cleanLearnedStep(step boundedAgentStep) (learnedskills.Step, bool, bool, bool) {
	args := cloneArgs(step.Args)
	delete(args, "approvalToken")
	switch step.Tool {
	case "terminal":
		if strings.ToLower(strings.TrimSpace(learnedString(args["action"]))) != "run" {
			return learnedskills.Step{}, false, false, false
		}
		command := strings.Join(strings.Fields(learnedString(args["command"])), " ")
		if !safeLearnedAutomationCommand(command) {
			return learnedskills.Step{}, false, false, false
		}
		clean := map[string]any{"action": "run", "command": command}
		if cwd := strings.TrimSpace(learnedString(args["cwd"])); cwd != "" && cwd != "." {
			clean["cwd"] = cwd
		}
		return learnedskills.Step{Tool: "terminal", Args: clean}, true, true, false
	case "browser":
		action := strings.ToLower(strings.TrimSpace(learnedString(args["action"])))
		clean := map[string]any{"action": action}
		switch action {
		case "status":
			return learnedskills.Step{Tool: "browser", Args: clean}, true, false, false
		case "open":
			target, ok := sanitizedBrowserURL(learnedString(args["url"]))
			if !ok {
				return learnedskills.Step{}, false, false, false
			}
			clean["url"] = target
			if headed, ok := args["headed"].(bool); ok {
				clean["headed"] = headed
			}
			return learnedskills.Step{Tool: "browser", Args: clean}, true, true, learnedBool(args["verify"], false)
		case "snapshot":
			return learnedskills.Step{Tool: "browser", Args: clean}, true, false, true
		case "find":
			query := strings.TrimSpace(learnedString(args["query"]))
			if query == "" {
				return learnedskills.Step{}, false, false, false
			}
			clean["query"] = query
			return learnedskills.Step{Tool: "browser", Args: clean}, true, false, true
		default:
			return learnedskills.Step{}, false, false, false
		}
	case "computer":
		action := strings.ToLower(strings.TrimSpace(learnedString(args["action"])))
		clean := map[string]any{"action": action}
		if windowID := strings.TrimSpace(learnedString(args["windowId"])); windowID != "" {
			clean["windowId"] = windowID
		}
		switch action {
		case "status", "list_windows", "ui_tree", "observe":
			return learnedskills.Step{Tool: "computer", Args: clean}, true, false, action == "observe" || action == "ui_tree"
		case "click":
			target := strings.TrimSpace(learnedString(args["target"]))
			if target == "" || args["x"] != nil || args["y"] != nil || strings.TrimSpace(learnedString(args["elementId"])) != "" {
				return learnedskills.Step{}, false, false, false
			}
			clean["target"] = target
			if verify, ok := args["verify"].(bool); ok {
				clean["verify"] = verify
			}
			return learnedskills.Step{Tool: "computer", Args: clean}, true, true, learnedBool(args["verify"], false)
		default:
			return learnedskills.Step{}, false, false, false
		}
	default:
		return learnedskills.Step{}, false, false, false
	}
}

func learnedRecipeFromAgentSteps(steps []boundedAgentStep) ([]learnedskills.Step, bool) {
	if len(steps) == 0 {
		return nil, false
	}
	out := make([]learnedskills.Step, 0, len(steps))
	mutated := false
	verifiedAfterMutation := false
	for _, step := range steps {
		clean, replayable, mutation, verification := cleanLearnedStep(step)
		if !replayable {
			return nil, false
		}
		out = append(out, clean)
		if mutation {
			mutated = true
			verifiedAfterMutation = verification
		} else if mutated && verification {
			verifiedAfterMutation = true
		}
	}
	return out, mutated && verifiedAfterMutation
}

func approvalRequiredResult(result any) bool {
	call, ok := result.(*mcp.CallToolResult)
	if !ok || call == nil {
		return false
	}
	root := resultRoot(call)
	status := strings.ToLower(strings.TrimSpace(learnedString(root["status"])))
	return status == "approval_required"
}

func learnedRecipeSteps(recipe *learnedskills.Recipe) []boundedAgentStep {
	if recipe == nil {
		return nil
	}
	steps := make([]boundedAgentStep, 0, len(recipe.Steps))
	for _, step := range recipe.Steps {
		steps = append(steps, boundedAgentStep{Tool: step.Tool, Args: cloneArgs(step.Args)})
	}
	return steps
}

func learnedSkillReplayPolicy(step boundedAgentStep, operation operationInvocation, args map[string]any) autonomousStepDecision {
	if strings.TrimSpace(learnedString(args["approvalToken"])) != "" || strings.TrimSpace(learnedString(step.Args["approvalToken"])) != "" {
		return autonomousStepDecision{Reason: "learned skills never persist or consume approval tokens"}
	}
	_, replayable, _, _ := cleanLearnedStep(step)
	if !replayable {
		return autonomousStepDecision{Reason: "learned skill step is no longer safe or replayable"}
	}
	if operation.OperationID == "" {
		return autonomousStepDecision{Reason: "learned skill step has no stable operation identity"}
	}
	return autonomousStepDecision{Allowed: true, Reason: "verified local learned skill; normal runtime approval policy remains authoritative"}
}

package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/learnedskills"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const learnedSkillReplayThreshold = 0.64

func normalizeLearnedTarget(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return r
		}
		return ' '
	}, value)
	return strings.Join(strings.Fields(value), " ")
}

func containsLearnedPhrase(value, phrase string) bool {
	value = normalizeLearnedTarget(value)
	phrase = normalizeLearnedTarget(phrase)
	if value == "" || phrase == "" {
		return false
	}
	return strings.Contains(" "+value+" ", " "+phrase+" ")
}

func riskyCandidateNavigationTarget(value string) bool {
	value = normalizeLearnedTarget(value)
	if value == "" {
		return true
	}
	for _, risky := range []string{
		"send", "submit", "post", "publish", "like", "follow", "accept", "approve", "confirm",
		"delete", "remove", "buy", "pay", "checkout", "transfer", "message", "comment", "save", "upload",
		"continue", "next", "done", "finish", "login", "log in", "logout", "log out", "sign in", "sign out",
		"create", "add", "edit", "update", "subscribe", "unsubscribe", "join", "leave", "install", "download",
		"gửi", "đăng", "thích", "theo dõi", "chấp nhận", "phê duyệt", "xác nhận", "xóa", "xoá",
		"mua", "thanh toán", "chuyển tiền", "nhắn", "bình luận", "lưu", "tải lên", "đồng ý",
		"tiếp tục", "kế tiếp", "hoàn tất", "đăng nhập", "đăng xuất", "tạo", "thêm", "sửa", "cập nhật", "tham gia", "rời", "cài đặt", "tải xuống",
	} {
		if containsLearnedPhrase(value, risky) {
			return true
		}
	}
	return false
}

func safeLearnedNavigationTarget(value string) bool {
	value = normalizeLearnedTarget(value)
	if riskyCandidateNavigationTarget(value) {
		return false
	}
	for _, safe := range []string{
		"notification", "notifications", "unread", "inbox", "menu", "home", "back", "search", "filter", "sort", "history", "activity", "recent",
		"thông báo", "chưa đọc", "hộp thư", "menu", "trang chủ", "quay lại", "tìm kiếm", "lọc", "sắp xếp", "lịch sử", "hoạt động", "gần đây",
	} {
		if containsLearnedPhrase(value, safe) {
			return true
		}
	}
	return false
}

func safeCandidateLearnedReplay(recipe *learnedskills.Recipe) bool {
	if recipe == nil || recipe.Status != learnedskills.StatusCandidate || recipe.SuccessCount < 1 || recipe.MatchScore < learnedSkillReplayThreshold || len(recipe.Steps) == 0 {
		return false
	}
	for _, stored := range recipe.Steps {
		if stored.Tool != "computer" {
			return false
		}
		clean, replayable, _, _ := cleanLearnedStep(boundedAgentStep{Tool: stored.Tool, Args: cloneArgs(stored.Args)})
		if !replayable {
			return false
		}
		action := strings.ToLower(strings.TrimSpace(learnedString(clean.Args["action"])))
		switch action {
		case "status", "list_windows", "ui_tree", "observe", "focus":
			continue
		case "click":
			if strings.TrimSpace(learnedString(clean.Args["windowHint"])) == "" || !safeLearnedNavigationTarget(learnedString(clean.Args["target"])) {
				return false
			}
		default:
			return false
		}
	}
	return true
}

func learnedSkillEligibleForReplay(recipe *learnedskills.Recipe) bool {
	if recipe == nil || recipe.MatchScore < learnedSkillReplayThreshold {
		return false
	}
	if recipe.Status == learnedskills.StatusTrusted {
		return true
	}
	return safeCandidateLearnedReplay(recipe)
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
var dynamicWindowBadge = regexp.MustCompile(`^\(\d+\)\s*`)

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
		windowHint := strings.TrimSpace(learnedString(args["windowHint"]))
		windowID := strings.TrimSpace(learnedString(args["windowId"]))
		if windowHint != "" {
			clean["windowHint"] = windowHint
		} else if windowID != "" {
			// Legacy recipes may still carry a window id. Newly captured desktop
			// skills should prefer windowHint because native ids are ephemeral.
			clean["windowId"] = windowID
		}
		switch action {
		case "status", "list_windows", "ui_tree", "observe":
			return learnedskills.Step{Tool: "computer", Args: clean}, true, false, action == "observe" || action == "ui_tree"
		case "focus":
			if windowHint == "" && windowID == "" {
				return learnedskills.Step{}, false, false, false
			}
			return learnedskills.Step{Tool: "computer", Args: clean}, true, true, false
		case "click":
			target := strings.TrimSpace(learnedString(args["target"]))
			if target == "" || args["x"] != nil || args["y"] != nil || strings.TrimSpace(learnedString(args["elementId"])) != "" {
				return learnedskills.Step{}, false, false, false
			}
			if windowHint == "" && windowID == "" {
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
	semanticNavigation := false
	verifiedReadOnly := false
	for _, step := range steps {
		clean, replayable, mutation, verification := cleanLearnedStep(step)
		if !replayable {
			return nil, false
		}
		out = append(out, clean)
		if clean.Tool == "browser" || clean.Tool == "computer" {
			action := strings.ToLower(strings.TrimSpace(learnedString(clean.Args["action"])))
			if action == "open" || action == "find" || action == "click" || action == "focus" {
				semanticNavigation = true
			}
		}
		if mutation {
			mutated = true
			verifiedAfterMutation = verification
		} else if verification {
			if mutated {
				verifiedAfterMutation = true
			} else if semanticNavigation {
				verifiedReadOnly = true
			}
		}
	}
	if mutated {
		return out, verifiedAfterMutation
	}
	return out, semanticNavigation && verifiedReadOnly
}

const (
	directLearnedTraceTTL       = 10 * time.Minute
	directLearnedTraceMaxSteps  = 12
	directLearnedTraceMaxActive = 512
)

type directLearnedTrace struct {
	Intent        string
	Steps         []boundedAgentStep
	WindowHints   map[string]string
	RecordedSteps int
	UpdatedAt     time.Time
}

var directLearnedTraces = struct {
	sync.Mutex
	Items map[string]*directLearnedTrace
}{Items: map[string]*directLearnedTrace{}}

func directLearnedTraceKey(userID, session, workspaceKey string) string {
	return strings.TrimSpace(userID) + "\x00" + strings.TrimSpace(session) + "\x00" + strings.TrimSpace(workspaceKey)
}

func pruneDirectLearnedTracesLocked(now time.Time) {
	for key, trace := range directLearnedTraces.Items {
		if trace == nil || (!trace.UpdatedAt.IsZero() && now.Sub(trace.UpdatedAt) > 2*directLearnedTraceTTL) {
			delete(directLearnedTraces.Items, key)
		}
	}
	for len(directLearnedTraces.Items) > directLearnedTraceMaxActive {
		oldestKey := ""
		oldestAt := now
		for key, trace := range directLearnedTraces.Items {
			if trace == nil {
				oldestKey = key
				break
			}
			if oldestKey == "" || trace.UpdatedAt.Before(oldestAt) {
				oldestKey = key
				oldestAt = trace.UpdatedAt
			}
		}
		if oldestKey == "" {
			break
		}
		delete(directLearnedTraces.Items, oldestKey)
	}
}

func directLearnedAction(operation operationInvocation) string {
	for _, prefix := range []string{"computer.", "browser."} {
		if strings.HasPrefix(operation.OperationID, prefix) {
			return strings.TrimPrefix(operation.OperationID, prefix)
		}
	}
	return ""
}

func successfulDirectLearnedResult(result *mcp.CallToolResult, err error) bool {
	if err != nil || result == nil || result.IsError {
		return false
	}
	root := resultRoot(result)
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(root["status"])))
	return status != "approval_required" && status != "blocked"
}

func stableWindowHint(app, title string) string {
	title = dynamicWindowBadge.ReplaceAllString(strings.TrimSpace(title), "")
	return strings.Join(strings.Fields(strings.TrimSpace(strings.TrimSpace(app)+" "+title)), " ")
}

func collectDirectWindowHints(value any, hints map[string]string) {
	if hints == nil {
		return
	}
	switch typed := value.(type) {
	case []any:
		for _, child := range typed {
			collectDirectWindowHints(child, hints)
		}
	case map[string]any:
		windowID := strings.TrimSpace(learnedString(typed["windowId"]))
		if windowID != "" && windowID != "screen:main" {
			app := strings.TrimSpace(learnedString(typed["app"]))
			title := strings.TrimSpace(learnedString(typed["title"]))
			if (app != "" || title != "") && stableWindowHint(app, title) != "" {
				hints[windowID] = stableWindowHint(app, title)
			}
		}
		for _, child := range typed {
			collectDirectWindowHints(child, hints)
		}
	}
}

func directLearnedHasObservation(result *mcp.CallToolResult) bool {
	if result == nil {
		return false
	}
	root := resultRoot(result)
	if root["observation"] == nil {
		return false
	}
	if raw, exists := root["verificationError"]; exists && raw != nil && strings.TrimSpace(fmt.Sprint(raw)) != "" {
		return false
	}
	return true
}

func directLearnedStep(publicTool string, operation operationInvocation, args map[string]any, result *mcp.CallToolResult, windowHints map[string]string) (boundedAgentStep, bool) {
	if publicTool != "computer" && publicTool != "browser" {
		return boundedAgentStep{}, false
	}
	action := directLearnedAction(operation)
	if action == "" {
		return boundedAgentStep{}, false
	}
	input := cloneArgs(args)
	input["action"] = action
	delete(input, "workspaceKey")
	delete(input, "approvalToken")

	if publicTool == "computer" {
		windowHint := strings.TrimSpace(learnedString(input["windowHint"]))
		if windowHint == "" {
			if windowID := strings.TrimSpace(learnedString(input["windowId"])); windowID != "" {
				windowHint = strings.TrimSpace(windowHints[windowID])
			}
		}
		if windowHint != "" {
			input["windowHint"] = windowHint
			delete(input, "windowId")
		}
		if action == "click" && learnedBool(input["verify"], false) && !directLearnedHasObservation(result) {
			input["verify"] = false
		}
	}

	clean, replayable, _, _ := cleanLearnedStep(boundedAgentStep{Tool: publicTool, Args: input})
	if !replayable {
		return boundedAgentStep{}, false
	}
	return boundedAgentStep{Tool: clean.Tool, Args: cloneArgs(clean.Args)}, true
}

func directLearnedVerificationEndpoint(step boundedAgentStep, result *mcp.CallToolResult) bool {
	action := strings.ToLower(strings.TrimSpace(learnedString(step.Args["action"])))
	switch step.Tool {
	case "computer":
		return action == "observe" || action == "ui_tree" || (action == "click" && directLearnedHasObservation(result))
	case "browser":
		return action == "snapshot"
	default:
		return false
	}
}

func directLearnedIntent(steps []boundedAgentStep) string {
	parts := []string{}
	seen := map[string]struct{}{}
	add := func(value string) {
		value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
		if value == "" {
			return
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			return
		}
		seen[key] = struct{}{}
		parts = append(parts, value)
	}
	for _, step := range steps {
		for _, key := range []string{"windowHint", "target", "query", "description"} {
			add(learnedString(step.Args[key]))
		}
		if rawURL := learnedString(step.Args["url"]); rawURL != "" {
			if parsed, err := url.Parse(rawURL); err == nil {
				add(parsed.Hostname())
			}
		}
	}
	return strings.Join(parts, " ")
}

// captureDirectLearnedSkill observes successful public browser/computer calls.
// It stores only replayable semantic steps in a short in-memory buffer; raw UI
// trees, screenshots, approval tokens, coordinates and ephemeral element ids
// never enter the durable learned-skill store.
func (s *Service) captureDirectLearnedSkill(ctx context.Context, userID, publicTool string, operation operationInvocation, args map[string]any, result *mcp.CallToolResult, callErr error, req *mcp.CallToolRequest) {
	if s == nil || !successfulDirectLearnedResult(result, callErr) {
		return
	}
	if publicTool != "context" && publicTool != "computer" && publicTool != "browser" {
		return
	}
	session := sessionID(req)
	workspaceKey := strings.TrimSpace(memoryWorkspaceKey(s, userID, session, args))
	if workspaceKey == "" {
		workspaceKey = strings.TrimSpace(s.route(userID, session))
	}
	if workspaceKey == "" {
		return
	}
	key := directLearnedTraceKey(userID, session, workspaceKey)
	now := time.Now()

	if publicTool == "context" {
		intent := sanitizeTaskMemoryText(learnedString(args["taskHint"]))
		if strings.TrimSpace(intent) == "" {
			return
		}
		directLearnedTraces.Lock()
		pruneDirectLearnedTracesLocked(now)
		directLearnedTraces.Items[key] = &directLearnedTrace{Intent: intent, WindowHints: map[string]string{}, UpdatedAt: now}
		directLearnedTraces.Unlock()
		return
	}

	directLearnedTraces.Lock()
	pruneDirectLearnedTracesLocked(now)
	trace := directLearnedTraces.Items[key]
	if trace == nil || now.Sub(trace.UpdatedAt) > directLearnedTraceTTL {
		trace = &directLearnedTrace{WindowHints: map[string]string{}}
		directLearnedTraces.Items[key] = trace
	}
	collectDirectWindowHints(resultRoot(result), trace.WindowHints)
	step, replayable := directLearnedStep(publicTool, operation, args, result, trace.WindowHints)
	if !replayable {
		if operation.MutatesState {
			delete(directLearnedTraces.Items, key)
		}
		directLearnedTraces.Unlock()
		return
	}
	trace.Steps = append(trace.Steps, step)
	if len(trace.Steps) > directLearnedTraceMaxSteps {
		trace.Steps = append([]boundedAgentStep(nil), trace.Steps[len(trace.Steps)-directLearnedTraceMaxSteps:]...)
		trace.RecordedSteps = 0
	}
	trace.UpdatedAt = now
	if !directLearnedVerificationEndpoint(step, result) {
		directLearnedTraces.Unlock()
		return
	}
	if len(trace.Steps) <= trace.RecordedSteps {
		directLearnedTraces.Unlock()
		return
	}
	recipeSteps, verified := learnedRecipeFromAgentSteps(trace.Steps)
	intent := strings.TrimSpace(trace.Intent)
	if intent == "" {
		intent = directLearnedIntent(trace.Steps)
	}
	if !verified || strings.TrimSpace(intent) == "" {
		directLearnedTraces.Unlock()
		return
	}
	trace.RecordedSteps = len(trace.Steps)
	directLearnedTraces.Unlock()

	_, _ = s.recordLearnedSkill(ctx, userID, session, workspaceKey, intent, "", recipeSteps)
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
	clean, replayable, _, _ := cleanLearnedStep(step)
	if !replayable {
		return autonomousStepDecision{Reason: "learned skill step is no longer safe or replayable"}
	}
	if clean.Tool == "computer" && strings.EqualFold(strings.TrimSpace(learnedString(clean.Args["action"])), "click") && !safeLearnedNavigationTarget(learnedString(clean.Args["target"])) {
		return autonomousStepDecision{Reason: "learned skills autonomously replay only read/navigation desktop clicks; state-changing or ambiguous clicks require a fresh model/user decision"}
	}
	if operation.OperationID == "" {
		return autonomousStepDecision{Reason: "learned skill step has no stable operation identity"}
	}
	return autonomousStepDecision{Allowed: true, Reason: "verified local learned skill; normal runtime approval policy remains authoritative"}
}

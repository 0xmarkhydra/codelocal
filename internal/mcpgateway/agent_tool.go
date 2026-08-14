package mcpgateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/taskstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxBoundedAgentUserSteps = 12
	maxBoundedAgentOps       = 20
)

type boundedAgentStep struct {
	Tool string
	Args map[string]any
}

type autonomousStepDecision struct {
	Allowed bool
	Reason  string
}

func parseBoundedAgentSteps(value any) ([]boundedAgentStep, error) {
	raw, ok := value.([]any)
	if !ok || len(raw) == 0 {
		return nil, errors.New("agent requires at least one step")
	}
	if len(raw) > maxBoundedAgentUserSteps {
		return nil, fmt.Errorf("agent supports at most %d model-authored steps", maxBoundedAgentUserSteps)
	}
	steps := make([]boundedAgentStep, 0, len(raw))
	for index, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("agent step %d must be an object", index+1)
		}
		tool, _ := entry["tool"].(string)
		tool = strings.TrimSpace(tool)
		if tool == "" || tool == "agent" {
			return nil, fmt.Errorf("agent step %d has invalid tool %q", index+1, tool)
		}
		args := map[string]any{}
		if provided, exists := entry["args"]; exists && provided != nil {
			mapped, ok := provided.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("agent step %d args must be an object", index+1)
			}
			args = cloneArgs(mapped)
		}
		steps = append(steps, boundedAgentStep{Tool: tool, Args: args})
	}
	return steps, nil
}

func compactDefinitionsByName() map[string]compactToolDef {
	out := map[string]compactToolDef{}
	for _, definition := range compactToolDefinitions() {
		out[definition.Name] = definition
	}
	return out
}

func resolveAgentWorkspaceKey(ctx context.Context, service *Service, userID, session string, args map[string]any) (string, error) {
	key, _ := args["workspaceKey"].(string)
	key = strings.TrimSpace(key)
	if key == "" {
		key = strings.TrimSpace(service.route(userID, session))
	}
	if key != "" {
		return key, nil
	}
	if service.Workspaces == nil {
		return "", workspaceRoutingError(false)
	}
	catalog, err := service.Workspaces.Catalog(ctx, userID)
	if err != nil {
		return "", err
	}
	active := []gateway.WorkspaceView{}
	for _, workspace := range catalog {
		if workspace.Status == "active" {
			active = append(active, workspace)
		}
	}
	if len(active) == 1 {
		return active[0].Key, nil
	}
	return "", workspaceRoutingError(len(active) > 1)
}

func hasApprovalToken(args map[string]any) bool {
	token, _ := args["approvalToken"].(string)
	return strings.TrimSpace(token) != ""
}

func hashSafeStructuredEdit(operation operationInvocation, args map[string]any) bool {
	switch operation.OperationID {
	case "edit.patch", "edit.format":
		return true
	case "edit.replace", "edit.write":
		hash, _ := args["expectedHash"].(string)
		return strings.TrimSpace(hash) != ""
	case "edit.apply":
		files, ok := args["files"].([]any)
		if !ok || len(files) == 0 {
			return false
		}
		for _, raw := range files {
			file, ok := raw.(map[string]any)
			if !ok {
				return false
			}
			hash, _ := file["expectedHash"].(string)
			if strings.TrimSpace(hash) == "" {
				return false
			}
		}
		return true
	default:
		return false
	}
}

func safeAutonomousVerificationCommand(command string) bool {
	command = strings.TrimSpace(command)
	if command == "" || orchestration.CheckKey(command) == "" {
		return false
	}
	if strings.ContainsAny(command, "\n\r;&|><`$") {
		return false
	}
	normalized := strings.ToLower(strings.Join(strings.Fields(command), " "))
	allowed := []string{
		"git diff --check",
		"go test", "go vet", "go build",
		"npm test", "npm run test", "npm run typecheck", "npm run build", "npm run lint",
		"pnpm test", "pnpm run test", "pnpm typecheck", "pnpm run typecheck", "pnpm build", "pnpm run build", "pnpm lint", "pnpm run lint",
		"yarn test", "yarn typecheck", "yarn build", "yarn lint",
		"bun test", "bun run test", "bun run typecheck", "bun run build", "bun run lint",
		"cargo test", "cargo build",
		"pytest", "python -m pytest",
		"flutter test", "flutter analyze",
	}
	for _, prefix := range allowed {
		if normalized == prefix || strings.HasPrefix(normalized, prefix+" ") {
			return true
		}
	}
	return false
}

func autonomousStepPolicy(operation operationInvocation, args map[string]any, plan orchestration.AgentPlan) autonomousStepDecision {
	if hasApprovalToken(args) {
		return autonomousStepDecision{Reason: "bounded agent never consumes approval tokens automatically"}
	}
	if operation.OperationID == "git.stage" || operation.OperationID == "git.unstage" || operation.OperationID == "git.commit" || operation.OperationID == "git.push" {
		return autonomousStepDecision{Reason: "Git write/release actions are never autonomous"}
	}
	if operation.OpenWorld && operation.OperationID != "terminal.run" {
		return autonomousStepDecision{Reason: "open-world operations require an explicit model/user decision outside bounded execution"}
	}

	switch operation.OperationID {
	case "project.info", "project.map", "project.instructions", "context.task",
		"read.info", "read.file", "read.range", "read.many", "search.files", "search.text",
		"dependency.inspect", "dependency.read", "dependency.search",
		"lsp.info", "lsp.workspace_symbols", "lsp.document_symbols", "lsp.definition", "lsp.references", "lsp.implementations", "lsp.hover", "lsp.diagnostics", "lsp.callers", "lsp.callees", "lsp.import_graph",
		"verify.snapshot", "verify.changes",
		"git.status", "git.diff", "git.log", "git.show", "git.blame", "git.file_history",
		"terminal.preflight", "terminal.history",
		"browser.status", "browser.snapshot", "browser.find", "browser.console", "browser.requests",
		"computer.status", "computer.observe", "computer.list_windows", "computer.ui_tree":
		return autonomousStepDecision{Allowed: true, Reason: "bounded structured/read-only operation"}
	case "edit.write", "edit.replace", "edit.patch", "edit.apply", "edit.format":
		if plan.Route.Confidence > 0 && plan.Route.Confidence < 0.60 {
			return autonomousStepDecision{Reason: "planner confidence is too low for autonomous mutation"}
		}
		if !hashSafeStructuredEdit(operation, args) {
			return autonomousStepDecision{Reason: "autonomous edits require patch context or stale-hash protection"}
		}
		return autonomousStepDecision{Allowed: true, Reason: "workspace-bounded hash-safe edit"}
	case "terminal.run":
		command, _ := args["command"].(string)
		if !safeAutonomousVerificationCommand(command) {
			return autonomousStepDecision{Reason: "terminal execution is limited to recognized verification commands"}
		}
		return autonomousStepDecision{Allowed: true, Reason: "recognized verification command; local execution policy remains authoritative"}
	default:
		return autonomousStepDecision{Reason: "operation is outside the bounded autonomous allowlist"}
	}
}

func resultNeedsAgentHalt(result *mcp.CallToolResult) (bool, string) {
	if result == nil {
		return true, "missing tool result"
	}
	if result.IsError {
		root := resultRoot(result)
		reason, _ := root["error"].(string)
		if strings.TrimSpace(reason) == "" {
			reason = "tool returned an error"
		}
		return true, reason
	}
	root := resultRoot(result)
	if running, _ := root["running"].(bool); running {
		return true, "verification process is still running; poll it before continuing the bounded plan"
	}
	if rawExit, exists := root["exitCode"]; exists && rawExit != nil {
		if exitCode := intValue(rawExit); exitCode != 0 {
			return true, fmt.Sprintf("verification process exited with code %d", exitCode)
		}
	}
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(root["status"])))
	if status == "approval_required" || status == "blocked" {
		reason := strings.TrimSpace(fmt.Sprint(root["reason"]))
		if reason == "" {
			reason = status
		}
		return true, reason
	}
	return false, ""
}

func agentResultStructured(result *mcp.CallToolResult) any {
	if result == nil {
		return nil
	}
	return result.StructuredContent
}

func argsFingerprint(args map[string]any) string {
	raw, _ := json.Marshal(args)
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:6])
}

func agentPlanFromResult(result *mcp.CallToolResult) (orchestration.AgentPlan, bool) {
	root := resultRoot(result)
	if root == nil {
		return orchestration.AgentPlan{}, false
	}
	if plan, ok := root["agentPlan"].(orchestration.AgentPlan); ok {
		return plan, true
	}
	raw, err := json.Marshal(root["agentPlan"])
	if err != nil {
		return orchestration.AgentPlan{}, false
	}
	var plan orchestration.AgentPlan
	if json.Unmarshal(raw, &plan) != nil || plan.Version == 0 {
		return orchestration.AgentPlan{}, false
	}
	return plan, true
}

func replanReason(before, after orchestration.AgentPlan) string {
	changes := []string{}
	if before.Phase != after.Phase {
		changes = append(changes, "phase "+before.Phase+"→"+after.Phase)
	}
	if before.Route.Primary != after.Route.Primary {
		changes = append(changes, "route "+string(before.Route.Primary)+"→"+string(after.Route.Primary))
	}
	if before.Quality.Status != after.Quality.Status {
		changes = append(changes, "quality "+before.Quality.Status+"→"+after.Quality.Status)
	}
	return strings.Join(changes, ", ")
}

func agentPlanFromState(state taskstate.State, caps orchestration.Capabilities, project orchestration.ProjectProfile) orchestration.AgentPlan {
	return orchestration.BuildPlan(planInputFromState(state, caps, project))
}

func (s *Service) awaitBoundedVerificationProcess(ctx context.Context, userID string, result *mcp.CallToolResult, req *mcp.CallToolRequest, workspaceKey string) *mcp.CallToolResult {
	root := resultRoot(result)
	if root == nil {
		return result
	}
	running, _ := root["running"].(bool)
	processID, _ := root["processId"].(string)
	if !running || strings.TrimSpace(processID) == "" {
		return result
	}
	pollOperation, err := operationForRuntimeTool("exec_poll")
	if err != nil {
		return result
	}
	deadline := time.Now().Add(15 * time.Second)
	for running && time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return result
		case <-time.After(150 * time.Millisecond):
		}
		pollArgs := map[string]any{"processId": processID, "workspaceKey": workspaceKey}
		polled, _ := s.callOperationRemembering(ctx, userID, "process", pollOperation, pollArgs, req)
		if polled == nil {
			return result
		}
		result = polled
		root = resultRoot(result)
		running, _ = root["running"].(bool)
	}
	return result
}

func currentAgentState(userID, session, workspaceKey string) taskstate.State {
	state, _ := workingMemory.Get(userID, session, workspaceKey)
	return state
}

func checkPassed(state taskstate.State, key string) bool {
	for _, passed := range state.PassedChecks {
		if passed == key {
			return true
		}
	}
	return false
}

func verificationCommands(plan orchestration.AgentPlan, state taskstate.State) []string {
	commands := []string{}
	seenKeys := map[string]struct{}{}
	for _, check := range plan.Verification.Checks {
		if !check.Required || strings.TrimSpace(check.Command) == "" || check.Key == "" || check.Key == "project-check" || checkPassed(state, check.Key) {
			continue
		}
		if _, exists := seenKeys[check.Key]; exists {
			continue
		}
		seenKeys[check.Key] = struct{}{}
		if safeAutonomousVerificationCommand(check.Command) {
			commands = append(commands, check.Command)
		}
	}
	return commands
}

func (s *Service) runBoundedAgent(ctx context.Context, userID string, args map[string]any, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	objective, _ := args["objective"].(string)
	objective = sanitizeTaskMemoryText(objective)
	if objective == "" {
		return errorResult(errors.New("agent objective is required")), nil
	}
	steps, err := parseBoundedAgentSteps(args["steps"])
	if err != nil {
		return errorResult(err), nil
	}
	autoVerify := true
	if value, ok := args["autoVerify"].(bool); ok {
		autoVerify = value
	}
	stopWhenReady := true
	if value, ok := args["stopWhenReady"].(bool); ok {
		stopWhenReady = value
	}

	session := sessionID(req)
	workspaceKey, err := resolveAgentWorkspaceKey(ctx, s, userID, session, args)
	if err != nil {
		return errorResult(err), nil
	}
	if s.Workspaces == nil {
		return errorResult(errors.New("workspace service unavailable")), nil
	}
	workspace, err := s.Workspaces.Activate(ctx, userID, workspaceKey)
	if err != nil {
		return errorResult(err), nil
	}
	caps := executionCapabilities(workspace)
	definitions := compactDefinitionsByName()
	contextDef := definitions["context"]
	contextOperation, contextArgs, err := contextDef.Resolve(map[string]any{"taskHint": objective, "workspaceKey": workspaceKey})
	if err != nil {
		return errorResult(err), nil
	}

	trace := []orchestration.ExecutionTraceStep{}
	ops := 0
	start := time.Now()
	contextResult, _ := s.callOperationRemembering(ctx, userID, "context", contextOperation, contextArgs, req)
	ops++
	trace = append(trace, orchestration.ExecutionTraceStep{Index: ops, Source: "auto-context", Tool: "context", Operation: contextOperation.OperationID, Lane: orchestration.LaneCode, Status: "succeeded", DurationMS: time.Since(start).Milliseconds()})
	if halt, reason := resultNeedsAgentHalt(contextResult); halt {
		trace[len(trace)-1].Status = "halted"
		return textResult(map[string]any{"status": "halted", "objective": objective, "haltReason": reason, "trace": trace, "lastResult": agentResultStructured(contextResult)}, false), nil
	}

	project := projectProfileFromResult(contextResult)
	plan, ok := agentPlanFromResult(contextResult)
	if !ok {
		plan = agentPlanFromState(currentAgentState(userID, session, workspaceKey), caps, project)
	}
	initialPlan := plan
	dirtySinceVerify := false
	haltReason := ""
	var lastResult *mcp.CallToolResult = contextResult
	seenFingerprints := map[string]struct{}{}
	replans := 0

	execute := func(source string, step boundedAgentStep) bool {
		if ops >= maxBoundedAgentOps {
			haltReason = "bounded agent operation budget exhausted"
			return false
		}
		definition, exists := definitions[step.Tool]
		if !exists || definition.Execute != nil || definition.Resolve == nil {
			haltReason = "unsupported bounded agent tool: " + step.Tool
			return false
		}
		forwardInput := cloneArgs(step.Args)
		forwardInput["workspaceKey"] = workspaceKey
		operation, forward, resolveErr := definition.Resolve(forwardInput)
		if resolveErr != nil {
			haltReason = resolveErr.Error()
			return false
		}
		decision := autonomousStepPolicy(operation, forward, plan)
		lane := orchestration.LaneForOperation(operation.OperationID)
		verification := operation.OperationID == "verify.changes" || (operation.OperationID == "terminal.run" && orchestration.CheckKey(fmt.Sprint(forward["command"])) != "")
		fallback := lane != orchestration.LaneNone && lane != plan.Route.Primary && !(verification && lane == orchestration.LaneShell)
		ops++
		item := orchestration.ExecutionTraceStep{Index: ops, Source: source, Tool: step.Tool, Operation: operation.OperationID, Lane: lane, Status: "pending", Fallback: fallback, Verification: verification}
		if !decision.Allowed {
			item.Status = "halted"
			trace = append(trace, item)
			haltReason = decision.Reason
			return false
		}
		fingerprint := operation.OperationID + ":" + argsFingerprint(forward)
		_, repeated := seenFingerprints[fingerprint]
		seenFingerprints[fingerprint] = struct{}{}
		if repeated {
			item.ReplanReason = "duplicate operation+arguments observed"
		}

		before := plan
		started := time.Now()
		result, _ := s.callOperationRemembering(ctx, userID, step.Tool, operation, forward, req)
		if operation.OperationID == "terminal.run" {
			result = s.awaitBoundedVerificationProcess(ctx, userID, result, req, workspaceKey)
		}
		item.DurationMS = time.Since(started).Milliseconds()
		lastResult = result
		if halt, reason := resultNeedsAgentHalt(result); halt {
			item.Status = "halted"
			trace = append(trace, item)
			haltReason = reason
			state := currentAgentState(userID, session, workspaceKey)
			plan = agentPlanFromState(state, caps, project)
			return false
		}
		item.Status = "succeeded"
		state := currentAgentState(userID, session, workspaceKey)
		plan = agentPlanFromState(state, caps, project)
		if reason := replanReason(before, plan); reason != "" {
			item.Replanned = true
			item.ReplanReason = strings.TrimSpace(strings.TrimSpace(item.ReplanReason + "; " + reason))
			replans++
		}
		trace = append(trace, item)
		if strings.HasPrefix(operation.OperationID, "edit.") {
			dirtySinceVerify = true
		}
		if operation.OperationID == "verify.changes" {
			dirtySinceVerify = false
		}
		return true
	}

	for _, step := range steps {
		if !execute("model", step) {
			break
		}
		state := currentAgentState(userID, session, workspaceKey)
		if stopWhenReady && state.AgentPhase == "finalize" && state.QualityStatus == "ready" {
			break
		}
	}

	if haltReason == "" && autoVerify && dirtySinceVerify && ops < maxBoundedAgentOps {
		state := currentAgentState(userID, session, workspaceKey)
		verifyStep := boundedAgentStep{Tool: "verify", Args: map[string]any{"action": "changes", "paths": append([]string(nil), state.TouchedFiles...)}}
		if execute("auto-verify", verifyStep) {
			state = currentAgentState(userID, session, workspaceKey)
			plan = agentPlanFromState(state, caps, project)
			for _, command := range verificationCommands(plan, state) {
				if ops >= maxBoundedAgentOps || haltReason != "" {
					break
				}
				terminalStep := boundedAgentStep{Tool: "terminal", Args: map[string]any{"action": "run", "command": command, "yieldMs": 10000, "timeoutMs": 120000}}
				if !execute("auto-check", terminalStep) {
					break
				}
				state = currentAgentState(userID, session, workspaceKey)
				plan = agentPlanFromState(state, caps, project)
				if stopWhenReady && state.AgentPhase == "finalize" && state.QualityStatus == "ready" {
					break
				}
			}
		}
	}

	state := currentAgentState(userID, session, workspaceKey)
	plan = agentPlanFromState(state, caps, project)
	status := "completed"
	if haltReason != "" {
		status = "halted"
	} else if state.AgentPhase == "finalize" && state.QualityStatus == "ready" {
		status = "ready"
	}
	efficiency := orchestration.EvaluateExecutionEfficiency(initialPlan, trace)
	payload := map[string]any{
		"status":       status,
		"objective":    objective,
		"workspaceKey": workspaceKey,
		"plan":         plan,
		"agentLoop": map[string]any{
			"phase":             state.AgentPhase,
			"iteration":         state.AgentIteration,
			"recoveryAttempts":  state.RecoveryAttempts,
			"nextAction":        state.NextAction,
			"completionAllowed": state.AgentPhase == "finalize" && state.QualityStatus == "ready",
			"quality":           map[string]any{"score": state.QualityScore, "status": state.QualityStatus},
		},
		"replans":    replans,
		"trace":      trace,
		"efficiency": efficiency,
	}
	if haltReason != "" {
		payload["haltReason"] = haltReason
		payload["lastResult"] = agentResultStructured(lastResult)
	}
	return textResult(payload, false), nil
}

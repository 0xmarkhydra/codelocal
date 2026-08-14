package mcpgateway

import (
	"context"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/taskstate"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type longTermMemoryStore interface {
	Ingest(context.Context, longmemory.IngestInput) (longmemory.Record, error)
	Recall(context.Context, longmemory.RecallInput) ([]longmemory.Record, error)
}

var (
	workingMemory      = taskstate.New(512)
	memorySecretAssign = regexp.MustCompile(`(?i)\b(api[_-]?key|access[_-]?token|token|secret|password|passwd)\b\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	memoryBearer       = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
)

func sanitizeTaskMemoryText(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return ""
	}
	value = memorySecretAssign.ReplaceAllString(value, "$1=[REDACTED]")
	value = memoryBearer.ReplaceAllString(value, "Bearer [REDACTED]")
	runes := []rune(value)
	if len(runes) > 360 {
		value = string(runes[:360]) + "…"
	}
	return value
}

func stringSliceArg(args map[string]any, key string) []string {
	value, ok := args[key]
	if !ok {
		return nil
	}
	switch typed := value.(type) {
	case []string:
		return typed
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func stringSliceValue(value any) []string {
	switch typed := value.(type) {
	case []string:
		return append([]string(nil), typed...)
	case []any:
		out := make([]string, 0, len(typed))
		for _, item := range typed {
			if text := strings.TrimSpace(fmt.Sprint(item)); text != "" {
				out = append(out, text)
			}
		}
		return out
	default:
		return nil
	}
}

func intValue(value any) int {
	switch typed := value.(type) {
	case int:
		return typed
	case int64:
		return int(typed)
	case float64:
		return int(typed)
	default:
		return 0
	}
}

func resultRoot(result *mcp.CallToolResult) map[string]any {
	if result == nil {
		return nil
	}
	root, _ := result.StructuredContent.(map[string]any)
	return root
}

func projectProfileFromResult(result *mcp.CallToolResult) orchestration.ProjectProfile {
	root := resultRoot(result)
	project := nestedMap(root["project"])
	return orchestration.ProjectProfile{
		Languages:         stringSliceValue(project["languages"]),
		Frameworks:        stringSliceValue(project["frameworks"]),
		BuildCommands:     stringSliceValue(project["buildCommands"]),
		TestCommands:      stringSliceValue(project["testCommands"]),
		TypecheckCommands: stringSliceValue(project["typecheckCommands"]),
		LintCommands:      stringSliceValue(project["lintCommands"]),
	}
}

func boolPointer(value bool) *bool { return &value }
func intPointer(value int) *int    { return &value }

func requiredCheckKeys(plan orchestration.VerificationPlan) []string {
	seen := map[string]struct{}{}
	keys := []string{}
	for _, check := range plan.Checks {
		if !check.Required || strings.TrimSpace(check.Key) == "" || check.Key == "project-check" {
			continue
		}
		if _, ok := seen[check.Key]; ok {
			continue
		}
		seen[check.Key] = struct{}{}
		keys = append(keys, check.Key)
	}
	return keys
}

func requiredCheckKeysFromResult(result *mcp.CallToolResult) []string {
	root := resultRoot(result)
	if root == nil {
		return nil
	}
	if plan, ok := root["verificationPlan"].(orchestration.VerificationPlan); ok {
		return requiredCheckKeys(plan)
	}
	plan := nestedMap(root["verificationPlan"])
	items, _ := plan["checks"].([]any)
	seen := map[string]struct{}{}
	keys := []string{}
	for _, item := range items {
		entry := nestedMap(item)
		required, _ := entry["required"].(bool)
		key, _ := entry["key"].(string)
		key = strings.TrimSpace(key)
		if !required || key == "" || key == "project-check" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		keys = append(keys, key)
	}
	return keys
}

func storedVerificationPlan(state taskstate.State) orchestration.VerificationPlan {
	checks := make([]orchestration.VerificationCheck, 0, len(state.RequiredChecks))
	for _, key := range state.RequiredChecks {
		if key = strings.TrimSpace(key); key != "" {
			checks = append(checks, orchestration.VerificationCheck{Key: key, Required: true, Scope: "stored-agent-plan", Reason: "required by the latest capability-aware plan"})
		}
	}
	return orchestration.VerificationPlan{Mode: "stored-agent-plan", Checks: checks}
}

func qualityPatchForState(state taskstate.State) taskstate.Patch {
	quality := orchestration.EvaluateQuality(planInputFromState(state, orchestration.Capabilities{}, orchestration.ProjectProfile{}), storedVerificationPlan(state))
	if len(state.TouchedFiles) > 0 && len(state.RequiredChecks) == 0 {
		quality.Status = "verifying"
		quality.MissingChecks = append(quality.MissingChecks, "verification-plan")
		if quality.Score > 75 {
			quality.Score = 75
		}
	}
	phase := state.AgentPhase
	nextAction := state.NextAction
	switch {
	case state.DiagnosticRegression > 0 || len(state.RecentErrors) > 0:
		phase = "recover"
		nextAction = "inspect fresh diagnostics and causal diff, repair the regression, then re-verify"
	case quality.Status == "ready" && phase == "verify":
		phase = "finalize"
		nextAction = "finalize result; perform Git/release action only when requested"
	case phase == "verify":
		nextAction = "run missing required verification checks and refresh verify.changes evidence"
	}
	return taskstate.Patch{
		AgentPhase:    phase,
		NextAction:    nextAction,
		QualityScore:  intPointer(quality.Score),
		QualityStatus: quality.Status,
	}
}

func structuredFilePaths(args map[string]any, key string) []string {
	items, ok := args[key].([]any)
	if !ok {
		return nil
	}
	paths := make([]string, 0, len(items))
	for _, item := range items {
		entry, ok := item.(map[string]any)
		if !ok {
			continue
		}
		path, _ := entry["path"].(string)
		if path = strings.TrimSpace(path); path != "" {
			paths = append(paths, path)
		}
	}
	return paths
}

func taskPatchForOperation(publicTool string, operation operationInvocation, args map[string]any, result *mcp.CallToolResult) taskstate.Patch {
	patch := taskstate.Patch{LastAction: operation.OperationID}
	if publicTool == "context" {
		if task, _ := args["taskHint"].(string); task != "" {
			patch.Task = sanitizeTaskMemoryText(task)
		}
	}
	if publicTool == "edit" {
		if path, _ := args["path"].(string); strings.TrimSpace(path) != "" {
			patch.TouchedFiles = append(patch.TouchedFiles, path)
		}
		patch.TouchedFiles = append(patch.TouchedFiles, stringSliceArg(args, "paths")...)
		patch.TouchedFiles = append(patch.TouchedFiles, structuredFilePaths(args, "files")...)
	}
	if publicTool == "git" {
		if branch, _ := args["branch"].(string); strings.TrimSpace(branch) != "" {
			patch.Branch = branch
		}
	}
	if publicTool == "verify" || publicTool == "terminal" {
		// Never persist raw terminal commands: they may contain credentials,
		// tokens, private paths or other sensitive arguments. The stable
		// operation identity is enough for continuation/recovery hints.
		patch.RecentChecks = []string{operation.OperationID}
	}
	if publicTool == "verify" && result != nil && !result.IsError {
		patch.ReplaceErrors = true
	}
	if result != nil && result.IsError {
		patch.RecentErrors = []string{operation.OperationID + " failed"}
	}
	return patch
}

func agentPatchForOperation(operation operationInvocation, args map[string]any, result *mcp.CallToolResult, state taskstate.State) taskstate.Patch {
	checkKey := ""
	if operation.OperationID == "terminal.run" {
		if command, _ := args["command"].(string); command != "" {
			checkKey = orchestration.CheckKey(command)
		}
	}
	failure := ""
	if root := resultRoot(result); root != nil {
		failure, _ = root["error"].(string)
	}
	success := result != nil && !result.IsError
	loop := orchestration.AdvanceLoop(orchestration.LoopState{
		Phase:            state.AgentPhase,
		Iteration:        state.AgentIteration,
		RecoveryAttempts: state.RecoveryAttempts,
		LastOutcome:      state.LastOutcome,
		NextAction:       state.NextAction,
	}, orchestration.LoopEvent{Operation: operation.OperationID, Success: success, Failure: failure, CheckKey: checkKey})
	patch := taskstate.Patch{
		AgentPhase:       loop.Phase,
		AgentIteration:   intPointer(loop.Iteration),
		RecoveryAttempts: intPointer(loop.RecoveryAttempts),
		LastOutcome:      loop.LastOutcome,
		NextAction:       loop.NextAction,
	}
	if success && checkKey != "" {
		patch.PassedChecks = []string{checkKey}
	}
	if success && operation.OperationID == "verify.changes" {
		root := resultRoot(result)
		regression := intValue(root["diagnosticRegression"])
		diffObserved := strings.TrimSpace(fmt.Sprint(root["gitDiff"])) != "" || len(stringSliceValue(root["verificationScope"])) > 0
		patch.VerificationSeen = boolPointer(true)
		patch.DiagnosticRegression = intPointer(regression)
		patch.DiffObserved = boolPointer(diffObserved)
		if required := requiredCheckKeysFromResult(result); len(required) > 0 {
			patch.RequiredChecks = required
			patch.ReplaceRequiredChecks = true
		}
	}
	return patch
}

func planInputFromState(state taskstate.State, caps orchestration.Capabilities, project orchestration.ProjectProfile) orchestration.PlanInput {
	return orchestration.PlanInput{
		Task:             state.Task,
		Capabilities:     caps,
		Project:          project,
		TouchedFiles:     append([]string(nil), state.TouchedFiles...),
		LastAction:       state.LastAction,
		RecentErrors:     append([]string(nil), state.RecentErrors...),
		RecentChecks:     append([]string(nil), state.RecentChecks...),
		AgentPhase:       state.AgentPhase,
		Iteration:        state.AgentIteration,
		RecoveryAttempts: state.RecoveryAttempts,
		PassedChecks:     append([]string(nil), state.PassedChecks...),
		VerificationSeen: state.VerificationSeen,
		DiagnosticDelta:  state.DiagnosticRegression,
		DiffObserved:     state.DiffObserved,
	}
}

func carryTaskStatePatch(state taskstate.State) taskstate.Patch {
	return taskstate.Patch{
		Task:                  state.Task,
		Branch:                state.Branch,
		TouchedFiles:          append([]string(nil), state.TouchedFiles...),
		RecentChecks:          append([]string(nil), state.RecentChecks...),
		RecentErrors:          append([]string(nil), state.RecentErrors...),
		LastAction:            state.LastAction,
		AgentPhase:            state.AgentPhase,
		AgentIteration:        intPointer(state.AgentIteration),
		RecoveryAttempts:      intPointer(state.RecoveryAttempts),
		LastOutcome:           state.LastOutcome,
		NextAction:            state.NextAction,
		PassedChecks:          append([]string(nil), state.PassedChecks...),
		ReplacePassedChecks:   true,
		RequiredChecks:        append([]string(nil), state.RequiredChecks...),
		ReplaceRequiredChecks: true,
		VerificationSeen:      boolPointer(state.VerificationSeen),
		DiagnosticRegression:  intPointer(state.DiagnosticRegression),
		DiffObserved:          boolPointer(state.DiffObserved),
		QualityScore:          intPointer(state.QualityScore),
		QualityStatus:         state.QualityStatus,
	}
}

func memoryWorkspaceKey(s *Service, userID, session string, args map[string]any) string {
	if explicit, _ := args["workspaceKey"].(string); strings.TrimSpace(explicit) != "" {
		return strings.TrimSpace(explicit)
	}
	return strings.TrimSpace(s.route(userID, session))
}

func executionCapabilities(workspace *gateway.WorkspaceView) orchestration.Capabilities {
	if workspace == nil {
		return orchestration.Capabilities{}
	}
	filesystem := workspace.ProtocolVersion <= 1 || capabilityBool(workspace.Capabilities, "filesystem")
	automation := nestedMap(workspace.Capabilities["automation"])
	browser := nestedMap(automation["browser"])
	computer := nestedMap(automation["computer"])
	return orchestration.Capabilities{
		Filesystem: filesystem,
		LSP:        filesystem,
		Shell:      capabilityBool(workspace.Capabilities, "shell"),
		Browser:    capabilityFlag(browser, "available"),
		Computer:   capabilityFlag(computer, "available"),
	}
}

func attachTaskContext(result *mcp.CallToolResult, state taskstate.State, plan orchestration.AgentPlan) {
	if result == nil {
		return
	}
	root, ok := result.StructuredContent.(map[string]any)
	if !ok || root == nil {
		return
	}
	// StructuredContent is intentionally the only projection path. Duplicating
	// the same memory into TextContent would increase model tokens on every
	// context call while modern MCP clients already consume structured content.
	root["taskMemory"] = map[string]any{
		"task":                 state.Task,
		"branch":               state.Branch,
		"touchedFiles":         state.TouchedFiles,
		"recentChecks":         state.RecentChecks,
		"recentErrors":         state.RecentErrors,
		"lastAction":           state.LastAction,
		"agentPhase":           state.AgentPhase,
		"agentIteration":       state.AgentIteration,
		"recoveryAttempts":     state.RecoveryAttempts,
		"lastOutcome":          state.LastOutcome,
		"nextAction":           state.NextAction,
		"passedChecks":         state.PassedChecks,
		"requiredChecks":       state.RequiredChecks,
		"verificationSeen":     state.VerificationSeen,
		"diagnosticRegression": state.DiagnosticRegression,
		"diffObserved":         state.DiffObserved,
		"qualityScore":         state.QualityScore,
		"qualityStatus":        state.QualityStatus,
	}
	root["routeHint"] = plan.Route
	root["agentPlan"] = plan
	result.StructuredContent = root
}

func attachAgentLoop(result *mcp.CallToolResult, state taskstate.State) {
	if strings.TrimSpace(state.Task) == "" {
		return
	}
	root := resultRoot(result)
	if root == nil {
		return
	}
	remainingRepairs := max(0, 2-state.RecoveryAttempts)
	root["agentLoop"] = map[string]any{
		"phase":             state.AgentPhase,
		"iteration":         state.AgentIteration,
		"recoveryAttempts":  state.RecoveryAttempts,
		"repairBudgetLeft":  remainingRepairs,
		"lastOutcome":       state.LastOutcome,
		"nextAction":        state.NextAction,
		"passedChecks":      state.PassedChecks,
		"requiredChecks":    state.RequiredChecks,
		"completionAllowed": state.AgentPhase == "finalize" && state.QualityStatus == "ready",
		"quality": map[string]any{
			"score":  state.QualityScore,
			"status": state.QualityStatus,
		},
	}
	result.StructuredContent = root
}

func attachRecoveryHint(result *mcp.CallToolResult) {
	if result == nil || !result.IsError {
		return
	}
	root, ok := result.StructuredContent.(map[string]any)
	if !ok || root == nil {
		return
	}
	message, _ := root["error"].(string)
	if strings.TrimSpace(message) == "" {
		return
	}
	root["recovery"] = orchestration.ClassifyFailure(message)
	result.StructuredContent = root
}

func attachLongTermMemory(result *mcp.CallToolResult, records []longmemory.Record) {
	if result == nil || len(records) == 0 {
		return
	}
	root, ok := result.StructuredContent.(map[string]any)
	if !ok || root == nil {
		return
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, map[string]any{
			"id":        record.ID,
			"level":     record.Level,
			"summary":   record.Summary,
			"branch":    record.Branch,
			"files":     record.Files,
			"score":     record.Score,
			"createdAt": record.CreatedAt,
		})
	}
	root["longTermMemory"] = items
	result.StructuredContent = root
}

func longTermMemoryInput(userID, session, workspaceID, publicTool string, operation operationInvocation, state taskstate.State, result *mcp.CallToolResult) *longmemory.IngestInput {
	if strings.TrimSpace(state.Task) == "" || strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	level := longmemory.LevelEvent
	importance := 0.55
	var summary string
	switch {
	case publicTool == "verify" && result != nil && !result.IsError:
		level = longmemory.LevelScenario
		importance = 0.85
		summary = "Task verified successfully: " + state.Task
		if len(state.RecentChecks) > 0 {
			summary += ". Checks: " + strings.Join(state.RecentChecks, ", ")
		}
	case publicTool == "edit" && result != nil && !result.IsError:
		summary = "Files edited for task: " + state.Task
	case result != nil && result.IsError:
		summary = operation.OperationID + " failed while working on task: " + state.Task
	default:
		return nil
	}
	return &longmemory.IngestInput{
		UserID:         userID,
		WorkspaceID:    workspaceID,
		TaskID:         session,
		Level:          level,
		Summary:        summary,
		Branch:         state.Branch,
		Files:          append([]string(nil), state.TouchedFiles...),
		Confidence:     0.8,
		Importance:     importance,
		IdempotencyKey: longmemory.IdempotencyKey(session, workspaceID, operation.OperationID, summary),
	}
}

func (s *Service) recallLongTermMemory(ctx context.Context, userID, workspaceID string, state taskstate.State) []longmemory.Record {
	if s.Memory == nil || strings.TrimSpace(state.Task) == "" || strings.TrimSpace(workspaceID) == "" {
		return nil
	}
	recallCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	records, err := s.Memory.Recall(recallCtx, longmemory.RecallInput{
		UserID:      userID,
		WorkspaceID: workspaceID,
		Query:       state.Task,
		Limit:       6,
		Files:       append([]string(nil), state.TouchedFiles...),
	})
	if err != nil {
		slog.Warn("long-term memory recall failed; continuing without cloud memory", "error", err)
		return nil
	}
	return records
}

func (s *Service) ingestLongTermMemoryAsync(userID, session, workspaceID, publicTool string, operation operationInvocation, state taskstate.State, result *mcp.CallToolResult) {
	if s.Memory == nil {
		return
	}
	input := longTermMemoryInput(userID, session, workspaceID, publicTool, operation, state, result)
	if input == nil {
		return
	}
	go func(value longmemory.IngestInput) {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := s.Memory.Ingest(ctx, value); err != nil {
			slog.Warn("long-term memory ingest failed; tool result remains valid", "level", value.Level, "error", err)
		}
	}(*input)
}

func (s *Service) callOperationRemembering(ctx context.Context, userID, publicTool string, operation operationInvocation, args map[string]any, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session := sessionID(req)
	workspaceKey := memoryWorkspaceKey(s, userID, session, args)
	result, err := s.callOperation(ctx, userID, publicTool, operation, args, req)
	attachRecoveryHint(result)
	if workspaceKey == "" {
		workspaceKey = strings.TrimSpace(s.route(userID, session))
	}
	if workspaceKey == "" {
		return result, err
	}

	state := workingMemory.Update(userID, session, workspaceKey, taskPatchForOperation(publicTool, operation, args, result))
	if strings.TrimSpace(state.Task) == "" {
		if latest, ok := workingMemory.LatestTask(userID, workspaceKey, 30*time.Minute); ok {
			state = workingMemory.Update(userID, session, workspaceKey, carryTaskStatePatch(latest))
		}
	}
	state = workingMemory.Update(userID, session, workspaceKey, agentPatchForOperation(operation, args, result, state))
	if operation.OperationID == "verify.changes" || (operation.OperationID == "terminal.run" && orchestration.CheckKey(fmt.Sprint(args["command"])) != "") {
		state = workingMemory.Update(userID, session, workspaceKey, qualityPatchForState(state))
	}

	logicalWorkspaceID := workspaceKey
	var workspace *gateway.WorkspaceView
	if s.Workspaces != nil && (publicTool == "context" || s.Memory != nil) {
		if activated, activateErr := s.Workspaces.Activate(ctx, userID, workspaceKey); activateErr == nil {
			workspace = activated
			if strings.TrimSpace(activated.WorkspaceID) != "" {
				logicalWorkspaceID = strings.TrimSpace(activated.WorkspaceID)
			}
		}
	}
	if publicTool == "context" && err == nil && result != nil && !result.IsError {
		caps := orchestration.Capabilities{}
		if workspace != nil {
			caps = executionCapabilities(workspace)
		}
		plan := orchestration.BuildPlan(planInputFromState(state, caps, projectProfileFromResult(result)))
		state = workingMemory.Update(userID, session, workspaceKey, taskstate.Patch{
			AgentPhase:            plan.Phase,
			NextAction:            plan.NextAction,
			RequiredChecks:        requiredCheckKeys(plan.Verification),
			ReplaceRequiredChecks: true,
			QualityScore:          intPointer(plan.Quality.Score),
			QualityStatus:         plan.Quality.Status,
		})
		attachTaskContext(result, state, plan)
		attachLongTermMemory(result, s.recallLongTermMemory(ctx, userID, logicalWorkspaceID, state))
	}
	attachAgentLoop(result, state)
	s.ingestLongTermMemoryAsync(userID, session, logicalWorkspaceID, publicTool, operation, state, result)
	return result, err
}

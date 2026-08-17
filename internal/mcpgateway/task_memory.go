package mcpgateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"log/slog"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
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

type graphMemoryStore interface {
	RecallGraphContext(context.Context, longmemory.RecallInput, []longmemory.Record) (longmemory.GraphContext, error)
}

type repositoryMemoryStore interface {
	RepositoryIDsForFiles(context.Context, string, string, string, string, []string) ([]string, error)
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

func repositoryProfilesFromValue(value any) []orchestration.RepositoryProfile {
	out := []orchestration.RepositoryProfile{}
	appendProfile := func(item map[string]any) {
		if len(item) == 0 {
			return
		}
		out = append(out, orchestration.RepositoryProfile{
			ID: strings.TrimSpace(fmt.Sprint(item["id"])), Path: strings.TrimSpace(fmt.Sprint(item["path"])),
			BuildCommands: stringSliceValue(item["buildCommands"]), TestCommands: stringSliceValue(item["testCommands"]),
			TypecheckCommands: stringSliceValue(item["typecheckCommands"]), LintCommands: stringSliceValue(item["lintCommands"]),
		})
	}
	switch typed := value.(type) {
	case []map[string]any:
		for _, item := range typed {
			appendProfile(item)
		}
	case []any:
		for _, raw := range typed {
			appendProfile(nestedMap(raw))
		}
	}
	return out
}

func projectProfileFromResult(result *mcp.CallToolResult) orchestration.ProjectProfile {
	root := resultRoot(result)
	project := nestedMap(root["project"])
	commands := nestedMap(project["commands"])
	profile := orchestration.ProjectProfile{
		Languages: projectStringSlice(project, commands, "languages"), Frameworks: projectStringSlice(project, commands, "frameworks"),
		BuildCommands: projectStringSlice(project, commands, "buildCommands", "build"), TestCommands: projectStringSlice(project, commands, "testCommands", "test"),
		TypecheckCommands: projectStringSlice(project, commands, "typecheckCommands", "typecheck"), LintCommands: projectStringSlice(project, commands, "lintCommands", "lint"),
		Repositories: repositoryProfilesFromValue(project["repositories"]),
	}
	return profile
}

func projectStringSlice(project, commands map[string]any, keys ...string) []string {
	if len(keys) == 0 {
		return nil
	}
	if values := stringSliceValue(project[keys[0]]); len(values) > 0 {
		return values
	}
	for _, key := range keys[1:] {
		if values := stringSliceValue(commands[key]); len(values) > 0 {
			return values
		}
	}
	return nil
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

func patchFilePaths(value string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "+++ ") && !strings.HasPrefix(line, "--- ") {
			continue
		}
		path := strings.TrimSpace(strings.TrimPrefix(strings.TrimPrefix(line, "+++ "), "--- "))
		if tab := strings.IndexByte(path, '\t'); tab >= 0 {
			path = path[:tab]
		}
		path = strings.TrimPrefix(path, "a/")
		path = strings.TrimPrefix(path, "b/")
		path = strings.TrimSpace(path)
		if path == "" || path == "/dev/null" || path == "dev/null" || strings.HasPrefix(path, "/") || path == ".." || strings.HasPrefix(path, "../") {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func taskPatchForOperation(publicTool string, operation operationInvocation, args map[string]any, result *mcp.CallToolResult) taskstate.Patch {
	patch := taskstate.Patch{LastAction: operation.OperationID}
	if publicTool == "context" {
		if task, _ := args["taskHint"].(string); task != "" {
			patch.Task = sanitizeTaskMemoryText(task)
		}
		if root := resultRoot(result); root != nil {
			brain := nestedMap(root["projectBrain"])
			patch.RulesHash = sanitizeTaskMemoryText(fmt.Sprint(brain["ruleFingerprint"]))
			patch.ContextHash = sanitizeTaskMemoryText(fmt.Sprint(brain["fingerprint"]))
			patch.Branch = sanitizeTaskMemoryText(fmt.Sprint(root["gitBranch"]))
			blocked, _ := brain["mandatoryOverflow"].(bool)
			patch.RuleMutationBlocked = &blocked
			patch.ReplaceOmittedRuleIDs = true
			patch.OmittedRequiredRuleIDs = stringSliceArg(brain, "omittedRequiredRuleIds")
		}
	}
	if publicTool == "edit" {
		if path, _ := args["path"].(string); strings.TrimSpace(path) != "" {
			patch.TouchedFiles = append(patch.TouchedFiles, path)
		}
		patch.TouchedFiles = append(patch.TouchedFiles, stringSliceArg(args, "paths")...)
		patch.TouchedFiles = append(patch.TouchedFiles, structuredFilePaths(args, "files")...)
		if rawPatch, _ := args["patch"].(string); strings.TrimSpace(rawPatch) != "" {
			patch.TouchedFiles = append(patch.TouchedFiles, patchFilePaths(rawPatch)...)
		}
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

func qualityPolicyBlockingCount(result *mcp.CallToolResult) int {
	root := resultRoot(result)
	quality, ok := root["qualityPolicy"].(map[string]any)
	if !ok {
		return 0
	}
	count := intValue(quality["blockingCount"])
	if count < 0 {
		return 0
	}
	return count
}

func verificationCheckOutcome(operation operationInvocation, args map[string]any, result *mcp.CallToolResult) (checkKey string, completed bool, success bool, failure string) {
	if result == nil {
		return "", true, false, "missing tool result"
	}
	root := resultRoot(result)
	status := strings.ToLower(strings.TrimSpace(fmt.Sprint(root["status"])))
	if status == "approval_required" || status == "blocked" {
		reason := strings.TrimSpace(fmt.Sprint(root["reason"]))
		if status == "approval_required" {
			if reason != "" {
				reason = "approval required: " + reason
			} else {
				reason = "approval required"
			}
		} else if reason == "" {
			reason = status
		}
		return "", true, false, reason
	}
	if result.IsError {
		failure, _ = root["error"].(string)
		if strings.TrimSpace(failure) == "" {
			failure = "tool failed"
		}
		return "", true, false, failure
	}
	if operation.OperationID == "verify.changes" {
		if count := qualityPolicyBlockingCount(result); count > 0 {
			return "", true, false, fmt.Sprintf("quality policy has %d blocking violation(s)", count)
		}
		return "", true, true, ""
	}

	command := ""
	switch operation.OperationID {
	case "terminal.run":
		command, _ = args["command"].(string)
	case "process.poll":
		command, _ = root["command"].(string)
	default:
		return "", true, true, ""
	}
	cwd := ""
	if operation.OperationID == "terminal.run" {
		cwd, _ = args["cwd"].(string)
	} else if operation.OperationID == "process.poll" {
		cwd, _ = root["cwd"].(string)
	}
	checkKey = orchestration.ScopedCheckID(command, cwd)
	if checkKey == "" {
		return "", true, true, ""
	}
	if running, _ := root["running"].(bool); running {
		return checkKey, false, false, ""
	}
	if rawExit, exists := root["exitCode"]; exists && rawExit != nil {
		exitCode := intValue(rawExit)
		if exitCode != 0 {
			return checkKey, true, false, fmt.Sprintf("%s exited with code %d", checkKey, exitCode)
		}
	}
	return checkKey, true, true, ""
}

func agentPatchForOperation(operation operationInvocation, args map[string]any, result *mcp.CallToolResult, state taskstate.State) taskstate.Patch {
	checkKey, completed, success, failure := verificationCheckOutcome(operation, args, result)
	if !completed {
		return taskstate.Patch{
			AgentPhase:       "verify",
			AgentIteration:   intPointer(state.AgentIteration),
			RecoveryAttempts: intPointer(state.RecoveryAttempts),
			LastOutcome:      "running",
			NextAction:       "poll the running verification process before evaluating the quality gate",
		}
	}
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
	if success && (checkKey != "" || operation.OperationID == "verify.changes") {
		// Fresh successful verification evidence supersedes transient execution
		// errors from an earlier attempt. RecentErrors is a quality-gate blocker,
		// so retaining already-recovered failures would keep the task in recover
		// forever even after every required check passes.
		patch.ReplaceErrors = true
		patch.RecentErrors = nil
	}
	if success && checkKey != "" {
		patch.PassedChecks = []string{checkKey}
	}
	if !success {
		if operation.OperationID == "verify.changes" && qualityPolicyBlockingCount(result) > 0 {
			patch.ReplaceErrors = true
			patch.RecentErrors = []string{failure}
		} else {
			patch.RecentErrors = []string{operation.OperationID + " failed"}
		}
	}
	if operation.OperationID == "verify.changes" && result != nil && !result.IsError {
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

func shouldRefreshQuality(operation operationInvocation, args map[string]any, result *mcp.CallToolResult) bool {
	if operation.OperationID == "verify.changes" {
		return result != nil && !result.IsError
	}
	checkKey, completed, _, _ := verificationCheckOutcome(operation, args, result)
	return completed && checkKey != ""
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
		TaskID:                state.TaskID,
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
		RulesHash:             state.RulesHash,
		ContextHash:           state.ContextHash,
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
		"rulesHash":            state.RulesHash,
		"contextHash":          state.ContextHash,
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

const (
	longTermMemoryMaxItems       = 4
	longTermMemoryCharBudget     = 2600
	longTermMemorySummaryMaxChar = 560
	graphMemoryMaxNodes          = 8
	graphMemoryMaxEdges          = 10
	graphMemorySummaryMaxChar    = 240
)

func runeLen(value string) int { return len([]rune(value)) }

// compactLongTermMemory keeps the highest-ranked durable memories inside a
// small character budget before they are projected into MCP structured
// content. This reduces prompt growth without weakening recall itself: the
// store can still rank a broader candidate set, while the model receives only
// the compact evidence needed for the current task.
func compactLongTermMemory(records []longmemory.Record) []longmemory.Record {
	if len(records) == 0 {
		return nil
	}
	remaining := longTermMemoryCharBudget
	seen := map[string]struct{}{}
	out := make([]longmemory.Record, 0, min(len(records), longTermMemoryMaxItems))
	for _, record := range records {
		if len(out) >= longTermMemoryMaxItems || remaining <= 0 {
			break
		}
		summaryLimit := min(longTermMemorySummaryMaxChar, remaining)
		if summaryLimit > 1 {
			summaryLimit-- // reserve one rune for SanitizeText's truncation ellipsis
		}
		summary := longmemory.SanitizeText(record.Summary, summaryLimit)
		identity := strings.ToLower(strings.Join(strings.Fields(summary), " "))
		if identity == "" {
			continue
		}
		if _, exists := seen[identity]; exists {
			continue
		}
		seen[identity] = struct{}{}
		files := longmemory.SanitizeList(record.Files, 4)
		cost := runeLen(summary) + runeLen(record.Branch) + 32
		for _, file := range files {
			cost += runeLen(file) + 4
		}
		if len(out) > 0 && cost > remaining {
			continue
		}
		record.Summary = summary
		record.Files = files
		out = append(out, record)
		remaining -= min(cost, remaining)
	}
	return out
}

func attachLongTermMemory(result *mcp.CallToolResult, records []longmemory.Record) {
	if result == nil || len(records) == 0 {
		return
	}
	root, ok := result.StructuredContent.(map[string]any)
	if !ok || root == nil {
		return
	}
	records = compactLongTermMemory(records)
	if len(records) == 0 {
		return
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, map[string]any{
			"id":         record.ID,
			"scope":      record.Scope,
			"level":      record.Level,
			"kind":       record.Kind,
			"sourceType": record.SourceType,
			"summary":    record.Summary,
			"branch":     record.Branch,
			"files":      record.Files,
			"score":      record.Score,
			"createdAt":  record.CreatedAt,
			"updatedAt":  record.UpdatedAt,
		})
	}
	root["longTermMemory"] = items
	result.StructuredContent = root
}

func attachGraphMemoryContext(result *mcp.CallToolResult, graph longmemory.GraphContext) {
	if result == nil || (len(graph.Nodes) == 0 && len(graph.Edges) == 0) {
		return
	}
	root, ok := result.StructuredContent.(map[string]any)
	if !ok || root == nil {
		return
	}
	seedIDs := make(map[string]struct{}, len(graph.SeedMemoryIDs))
	for _, id := range graph.SeedMemoryIDs {
		seedIDs[id] = struct{}{}
	}
	nodes := make([]map[string]any, 0, min(len(graph.Nodes), graphMemoryMaxNodes))
	for _, node := range graph.Nodes {
		if len(nodes) >= graphMemoryMaxNodes {
			break
		}
		item := map[string]any{
			"id":         node.ID,
			"scope":      node.Scope,
			"kind":       node.Kind,
			"name":       node.CanonicalName,
			"confidence": node.Confidence,
			"importance": node.Importance,
			"lastSeenAt": node.LastSeenAt,
		}
		if node.SourceMemoryID != "" {
			item["sourceMemoryId"] = node.SourceMemoryID
		}
		if _, isSeed := seedIDs[node.SourceMemoryID]; !isSeed {
			if summary := longmemory.SanitizeText(node.Summary, graphMemorySummaryMaxChar-1); summary != "" {
				item["summary"] = summary
			}
		}
		nodes = append(nodes, item)
	}
	keptNodes := make(map[string]struct{}, len(nodes))
	for _, node := range nodes {
		if id, _ := node["id"].(string); id != "" {
			keptNodes[id] = struct{}{}
		}
	}
	edges := make([]map[string]any, 0, min(len(graph.Edges), graphMemoryMaxEdges))
	for _, edge := range graph.Edges {
		if len(edges) >= graphMemoryMaxEdges {
			break
		}
		if _, ok := keptNodes[edge.FromNodeID]; !ok {
			continue
		}
		if _, ok := keptNodes[edge.ToNodeID]; !ok {
			continue
		}
		edges = append(edges, map[string]any{
			"from":       edge.FromNodeID,
			"to":         edge.ToNodeID,
			"relation":   edge.Relation,
			"scope":      edge.Scope,
			"confidence": edge.Confidence,
			"importance": edge.Importance,
		})
	}
	root["memoryContext"] = map[string]any{
		"mode":          "budgeted-vector+graph",
		"seedMemoryIds": graph.SeedMemoryIDs,
		"nodes":         nodes,
		"edges":         edges,
	}
	result.StructuredContent = root
}

const agentCheckpointMarker = "codelocal-agent:v1"

func agentTaskFingerprint(task string) string {
	text := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(task)), " "))
	if text == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:6])
}

func agentCheckpointSymbols(state taskstate.State) []string {
	fingerprint := agentTaskFingerprint(state.Task)
	if fingerprint == "" {
		return nil
	}
	phase := strings.ToLower(strings.TrimSpace(state.AgentPhase))
	if phase == "" {
		phase = "plan"
	}
	out := []string{
		agentCheckpointMarker,
		"task:" + fingerprint,
		"phase:" + phase,
		"iteration:" + strconv.Itoa(max(0, state.AgentIteration)),
		"recovery:" + strconv.Itoa(max(0, min(2, state.RecoveryAttempts))),
	}
	if outcome := strings.ToLower(strings.TrimSpace(state.LastOutcome)); outcome == "succeeded" || outcome == "failed" {
		out = append(out, "outcome:"+outcome)
	}
	return out
}

func agentCheckpointPatch(state taskstate.State, records []longmemory.Record, now time.Time) (taskstate.Patch, bool) {
	if strings.TrimSpace(state.Task) == "" || state.AgentIteration > 0 || state.RecoveryAttempts > 0 || state.VerificationSeen || len(state.PassedChecks) > 0 || len(state.TouchedFiles) > 0 {
		return taskstate.Patch{}, false
	}
	fingerprint := agentTaskFingerprint(state.Task)
	if fingerprint == "" {
		return taskstate.Patch{}, false
	}
	for _, record := range records {
		if record.CreatedAt <= 0 || now.Sub(time.UnixMilli(record.CreatedAt)) > 7*24*time.Hour {
			continue
		}
		markers := map[string]string{}
		hasVersion := false
		matchesTask := false
		for _, symbol := range record.Symbols {
			symbol = strings.TrimSpace(symbol)
			if symbol == agentCheckpointMarker {
				hasVersion = true
				continue
			}
			parts := strings.SplitN(symbol, ":", 2)
			if len(parts) == 2 {
				markers[parts[0]] = parts[1]
				if parts[0] == "task" && parts[1] == fingerprint {
					matchesTask = true
				}
			}
		}
		if !hasVersion || !matchesTask {
			continue
		}
		iteration, _ := strconv.Atoi(markers["iteration"])
		recovery, _ := strconv.Atoi(markers["recovery"])
		iteration = max(0, min(1000, iteration))
		recovery = max(0, min(2, recovery))
		phase := strings.ToLower(strings.TrimSpace(markers["phase"]))
		nextAction := "refresh task context after restored runtime checkpoint"
		switch phase {
		case "finalize", "verify":
			phase = "verify"
			nextAction = "refresh verify.changes and required checks after restored runtime checkpoint"
		case "recover":
			nextAction = "refresh diagnostics and causal evidence before any retry after restored runtime checkpoint"
		case "plan":
			nextAction = "rebuild capability-aware plan from fresh repository evidence"
		default:
			phase = "inspect"
			nextAction = "refresh ranked context and repository relationships before continuing"
		}
		outcome := strings.ToLower(strings.TrimSpace(markers["outcome"]))
		if outcome != "succeeded" && outcome != "failed" {
			outcome = ""
		}
		// Deliberately do not restore passed checks, verification evidence or a
		// previous ready quality score. Files may have changed while CodeLocal was
		// offline; a restored checkpoint must earn fresh completion evidence.
		return taskstate.Patch{
			Branch:                record.Branch,
			TouchedFiles:          append([]string(nil), record.Files...),
			RecentErrors:          nil,
			ReplaceErrors:         true,
			AgentPhase:            phase,
			AgentIteration:        intPointer(iteration),
			RecoveryAttempts:      intPointer(recovery),
			LastOutcome:           outcome,
			NextAction:            nextAction,
			PassedChecks:          nil,
			ReplacePassedChecks:   true,
			RequiredChecks:        nil,
			ReplaceRequiredChecks: true,
			VerificationSeen:      boolPointer(false),
			DiagnosticRegression:  intPointer(0),
			DiffObserved:          boolPointer(false),
			QualityScore:          intPointer(0),
			QualityStatus:         "verifying",
		}, true
	}
	return taskstate.Patch{}, false
}

func (s *Service) projectIDForWorkspace(ctx context.Context, userID, workspaceID string) string {
	if s == nil || s.Workspaces == nil || strings.TrimSpace(workspaceID) == "" {
		return ""
	}
	items, err := s.Workspaces.Catalog(ctx, userID)
	if err != nil {
		return ""
	}
	projectID := ""
	for _, item := range items {
		if item.WorkspaceID != workspaceID || strings.TrimSpace(item.ProjectID) == "" {
			continue
		}
		if projectID != "" && projectID != item.ProjectID {
			// A bare workspace id may theoretically collide across devices. Never
			// broaden memory recall across projects when the binding is ambiguous.
			return ""
		}
		projectID = item.ProjectID
	}
	return projectID
}

func (s *Service) repositoryIDsForWorkspaceFiles(ctx context.Context, userID, projectID, deviceID, workspaceID string, files []string) []string {
	store, ok := s.Memory.(repositoryMemoryStore)
	if !ok || strings.TrimSpace(projectID) == "" || strings.TrimSpace(deviceID) == "" || strings.TrimSpace(workspaceID) == "" || len(files) == 0 {
		return nil
	}
	ids, err := store.RepositoryIDsForFiles(ctx, userID, projectID, deviceID, workspaceID, files)
	if err != nil {
		slog.Warn("repository memory routing failed; falling back to project/workspace memory", "error", err)
		return nil
	}
	return ids
}

func (s *Service) recallLongTermMemory(ctx context.Context, userID, deviceID, workspaceID string, state taskstate.State) ([]longmemory.Record, longmemory.GraphContext) {
	if s.Memory == nil || strings.TrimSpace(state.Task) == "" || strings.TrimSpace(workspaceID) == "" {
		return nil, longmemory.GraphContext{}
	}
	recallCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	projectID := s.projectIDForWorkspace(recallCtx, userID, workspaceID)
	input := longmemory.RecallInput{
		UserID:        userID,
		WorkspaceID:   workspaceID,
		ProjectID:     projectID,
		RepositoryIDs: s.repositoryIDsForWorkspaceFiles(recallCtx, userID, projectID, deviceID, workspaceID, state.TouchedFiles),
		Query:         state.Task,
		Branch:        state.Branch,
		Limit:         6,
		Files:         append([]string(nil), state.TouchedFiles...),
	}
	records, err := s.Memory.Recall(recallCtx, input)
	if err != nil {
		slog.Warn("long-term memory recall failed; continuing without cloud memory", "error", err)
		return nil, longmemory.GraphContext{}
	}
	graph := longmemory.GraphContext{}
	if graphStore, ok := s.Memory.(graphMemoryStore); ok && len(records) > 0 {
		if value, graphErr := graphStore.RecallGraphContext(recallCtx, input, records); graphErr != nil {
			slog.Warn("memory graph recall failed; continuing with vector memory", "error", graphErr)
		} else {
			graph = value
		}
	}
	canonicalInput := cloud.CanonicalKnowledgeRecallInput{
		UserID: userID, ProjectID: projectID, RepositoryIDs: append([]string(nil), input.RepositoryIDs...), Branch: state.Branch, Limit: 6,
	}
	// Shadow remains the default and is fully asynchronous. Guarded hybrid is
	// explicit-only and fail-soft: readiness/health/read failures preserve the
	// legacy ranked output, while a ready project may receive at most two
	// high-confidence canonical supplements. Legacy graph context remains the
	// graph source until Knowledge V2 graph projection has its own rollout gate.
	records = s.maybeApplyHybridCanonicalRecall(recallCtx, canonicalInput, state.Task, records)
	s.maybeShadowCanonicalRecall(canonicalInput, len(records))
	s.maybeRunCanonicalSemanticShadow(canonicalInput, state.Task)
	return records, graph
}

func semanticExperienceTaskKind(task, publicTool string) string {
	lower := strings.ToLower(strings.Join(strings.Fields(task), " "))
	containsAny := func(values ...string) bool {
		for _, value := range values {
			if strings.Contains(lower, value) {
				return true
			}
		}
		return false
	}
	switch {
	case containsAny("review", "audit", "đánh giá", "kiểm tra code"):
		return "review"
	case containsAny("refactor", "tái cấu trúc"):
		return "refactor"
	case containsAny("migration", "migrate", "chuyển đổi schema"):
		return "migration"
	case containsAny("deploy", "deployment", "triển khai", "release"):
		return "deployment"
	case containsAny("bug", "fix", "sửa lỗi", "lỗi "):
		return "bugfix"
	case containsAny("feature", "tính năng", "implement", "thêm "):
		return "feature"
	case containsAny("test", "kiểm thử"):
		return "test"
	default:
		if strings.TrimSpace(publicTool) == "" {
			return "coding"
		}
		return "coding"
	}
}

func (s *Service) enqueueVerifiedExperience(ctx context.Context, userID, session, deviceID, workspaceID, publicTool string, state taskstate.State, result *mcp.CallToolResult) {
	if s == nil || s.Store == nil || result == nil || strings.TrimSpace(state.Task) == "" || !state.VerificationSeen {
		return
	}
	succeeded := !result.IsError && state.QualityStatus == "ready" && state.DiagnosticRegression <= 0
	failed := state.DiagnosticRegression > 0 || (strings.EqualFold(state.LastOutcome, "failed") && len(state.RecentErrors) > 0)
	if !succeeded && !failed {
		return
	}
	outcome := "succeeded"
	rootCause := ""
	verificationSummary := fmt.Sprintf("Verified ready quality gate; quality=%d; passed=%d; required=%d; diffObserved=%t; diagnosticRegression=%d", state.QualityScore, len(state.PassedChecks), len(state.RequiredChecks), state.DiffObserved, state.DiagnosticRegression)
	idempotencyState := "verified-ready"
	if failed {
		outcome = "failed"
		idempotencyState = "verified-failure"
		rootCause = strings.Join(state.RecentErrors, "; ")
		if rootCause == "" && state.DiagnosticRegression > 0 {
			rootCause = fmt.Sprintf("diagnostic regression: +%d", state.DiagnosticRegression)
		}
		verificationSummary = fmt.Sprintf("Verified failure evidence; quality=%d; status=%s; passed=%d; required=%d; diagnosticRegression=%d", state.QualityScore, state.QualityStatus, len(state.PassedChecks), len(state.RequiredChecks), state.DiagnosticRegression)
	}
	input := cloud.ExperienceInput{
		UserID: userID, WorkspaceID: workspaceID, DeviceID: deviceID,
		TaskID: session, TaskKind: semanticExperienceTaskKind(state.Task, publicTool), Objective: state.Task, Branch: state.Branch,
		Files: append([]string(nil), state.TouchedFiles...), Symbols: agentCheckpointSymbols(state), Checks: append([]string(nil), state.PassedChecks...),
		Outcome: outcome, RootCause: rootCause, RulesHash: state.RulesHash, ContextHash: state.ContextHash, VerificationSummary: verificationSummary, Verified: true,
		IdempotencyKey: longmemory.IdempotencyKey(userID, session, workspaceID, deviceID, state.Task, state.RulesHash, state.ContextHash, idempotencyState),
		Metadata:       map[string]any{"qualityScore": state.QualityScore, "diffObserved": state.DiffObserved, "source": "codelocal-verification", "executionTool": publicTool},
	}
	enqueueCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	if _, err := s.Store.EnqueueExperience(enqueueCtx, input); err != nil {
		slog.Warn("verified experience durable enqueue failed; tool result remains valid", "error", err)
	}
}

func (s *Service) recordAutomaticLearning(ctx context.Context, userID, session, deviceID, workspaceID, publicTool string, state taskstate.State, result *mcp.CallToolResult) {
	// Operational task activity belongs to task/audit/history, never canonical
	// long-term knowledge. Verified Experience crosses a durable outbox boundary;
	// later Knowledge V2 promotion consumes only processed Experience.
	s.enqueueVerifiedExperience(ctx, userID, session, deviceID, workspaceID, publicTool, state, result)
}

func durableMemoryKind(value string) (string, bool) {
	kind := strings.ToLower(strings.TrimSpace(value))
	switch kind {
	case "goal", "preference", "decision", "user_fact", "project_fact", "idea", "constraint", "milestone", "problem", "person", "company":
		return kind, true
	default:
		return "", false
	}
}

func durableMemoryScore(value any, fallback float64) float64 {
	switch typed := value.(type) {
	case float64:
		if typed >= 0 && typed <= 1 {
			return typed
		}
	case float32:
		if typed >= 0 && typed <= 1 {
			return float64(typed)
		}
	case int:
		if typed >= 0 && typed <= 1 {
			return float64(typed)
		}
	}
	return fallback
}

func (s *Service) rememberConversationMemory(ctx context.Context, userID, session string, args map[string]any) (*mcp.CallToolResult, error) {
	if s.Memory == nil {
		return errorResult(fmt.Errorf("CodeLocal server memory is disabled")), nil
	}
	rawItems, ok := args["memories"].([]any)
	if !ok || len(rawItems) == 0 {
		return errorResult(fmt.Errorf("remember requires at least one memory")), nil
	}
	if len(rawItems) > 12 {
		rawItems = rawItems[:12]
	}

	type candidate struct {
		kind       string
		key        string
		summary    string
		scope      longmemory.Scope
		importance float64
		confidence float64
	}
	candidates := make([]candidate, 0, len(rawItems))
	needsWorkspace := false
	for index, raw := range rawItems {
		item, ok := raw.(map[string]any)
		if !ok {
			return errorResult(fmt.Errorf("memory %d must be an object", index+1)), nil
		}
		kind, valid := durableMemoryKind(fmt.Sprint(item["kind"]))
		if !valid {
			return errorResult(fmt.Errorf("memory %d has unsupported kind", index+1)), nil
		}
		memoryKey, _ := item["key"].(string)
		memoryKey = strings.ToLower(strings.Join(strings.Fields(longmemory.SanitizeText(memoryKey, 160)), " "))
		summary := longmemory.SanitizeText(fmt.Sprint(item["summary"]), 900)
		if strings.TrimSpace(summary) == "" {
			return errorResult(fmt.Errorf("memory %d requires a non-empty summary", index+1)), nil
		}
		scope := longmemory.Scope(strings.ToLower(strings.TrimSpace(fmt.Sprint(item["scope"]))))
		if scope != longmemory.ScopeGlobal && scope != longmemory.ScopeProject && scope != longmemory.ScopeWorkspace {
			return errorResult(fmt.Errorf("memory %d scope must be global, project or workspace", index+1)), nil
		}
		if scope == longmemory.ScopeProject || scope == longmemory.ScopeWorkspace {
			needsWorkspace = true
		}
		candidates = append(candidates, candidate{
			kind:       kind,
			key:        memoryKey,
			summary:    summary,
			scope:      scope,
			importance: durableMemoryScore(item["importance"], .75),
			confidence: durableMemoryScore(item["confidence"], .9),
		})
	}

	logicalWorkspaceID := ""
	logicalProjectID := ""
	workspaceKey, _ := args["workspaceKey"].(string)
	workspaceKey = strings.TrimSpace(workspaceKey)
	if workspaceKey == "" {
		workspaceKey = strings.TrimSpace(s.route(userID, session))
	}
	if needsWorkspace {
		if workspaceKey == "" {
			return errorResult(fmt.Errorf("project/workspace-scoped memory requires a selected workspace or workspaceKey")), nil
		}
		if s.Workspaces == nil {
			return errorResult(fmt.Errorf("workspace service is unavailable")), nil
		}
		workspace, err := s.Workspaces.Activate(ctx, userID, workspaceKey)
		if err != nil {
			return errorResult(err), nil
		}
		logicalWorkspaceID = strings.TrimSpace(workspace.WorkspaceID)
		logicalProjectID = strings.TrimSpace(workspace.ProjectID)
		if logicalWorkspaceID == "" {
			return errorResult(fmt.Errorf("selected workspace has no stable workspace id")), nil
		}
		if logicalProjectID == "" {
			logicalProjectID = s.projectIDForWorkspace(ctx, userID, logicalWorkspaceID)
		}
	}

	remembered := make([]map[string]any, 0, len(candidates))
	for _, item := range candidates {
		workspaceID := ""
		projectID := ""
		switch item.scope {
		case longmemory.ScopeWorkspace:
			workspaceID = logicalWorkspaceID
		case longmemory.ScopeProject:
			projectID = logicalProjectID
			if projectID == "" {
				return errorResult(fmt.Errorf("selected workspace is not bound to a logical project yet")), nil
			}
		}
		memoryIdentity := strings.ToLower(item.summary)
		memorySymbols := []string(nil)
		if item.key != "" {
			memoryIdentity = "key:" + item.key
			memorySymbols = []string{"memory-key:" + item.key}
		}
		record, err := s.Memory.Ingest(ctx, longmemory.IngestInput{
			UserID:         userID,
			WorkspaceID:    workspaceID,
			ProjectID:      projectID,
			Scope:          item.scope,
			TaskID:         session,
			Level:          longmemory.LevelWorkspace,
			Kind:           item.kind,
			SourceType:     "conversation",
			Summary:        item.summary,
			Symbols:        memorySymbols,
			Confidence:     item.confidence,
			Importance:     item.importance,
			IdempotencyKey: longmemory.IdempotencyKey(userID, string(item.scope), workspaceID, projectID, item.kind, memoryIdentity),
		})
		if err != nil {
			return errorResult(err), nil
		}
		if item.scope == longmemory.ScopeProject && item.key != "" && s.Store != nil {
			if promotionInput, ok := cloud.ExplicitMemoryPromotionInputForRecord(userID, projectID, record, item.key); ok {
				enqueueCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
				if _, enqueueErr := s.Store.EnqueueExplicitMemoryPromotion(enqueueCtx, promotionInput); enqueueErr != nil {
					slog.Warn("explicit memory promotion enqueue failed; durable legacy memory remains valid", "error", enqueueErr)
				}
				cancel()
			}
		}
		remembered = append(remembered, map[string]any{
			"id": record.ID, "scope": record.Scope, "kind": record.Kind,
		})
	}
	return textResult(map[string]any{"remembered": len(remembered), "memories": remembered}, false), nil
}

func (s *Service) recallConversationMemory(ctx context.Context, userID, session string, args map[string]any) (*mcp.CallToolResult, error) {
	if s.Memory == nil {
		return errorResult(fmt.Errorf("CodeLocal server memory is disabled")), nil
	}
	query := longmemory.SanitizeText(fmt.Sprint(args["query"]), 1200)
	if strings.TrimSpace(query) == "" {
		return errorResult(fmt.Errorf("recall requires a non-empty query")), nil
	}
	limit := intValue(args["limit"])
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}

	logicalWorkspaceID := ""
	logicalProjectID := ""
	workspaceKey, _ := args["workspaceKey"].(string)
	workspaceKey = strings.TrimSpace(workspaceKey)
	if workspaceKey == "" {
		workspaceKey = strings.TrimSpace(s.route(userID, session))
	}
	if workspaceKey != "" && s.Workspaces != nil {
		workspace, err := s.Workspaces.Activate(ctx, userID, workspaceKey)
		if err != nil {
			return errorResult(err), nil
		}
		logicalWorkspaceID = strings.TrimSpace(workspace.WorkspaceID)
		logicalProjectID = strings.TrimSpace(workspace.ProjectID)
		if logicalProjectID == "" && logicalWorkspaceID != "" {
			logicalProjectID = s.projectIDForWorkspace(ctx, userID, logicalWorkspaceID)
		}
	}

	recallCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	input := longmemory.RecallInput{UserID: userID, WorkspaceID: logicalWorkspaceID, ProjectID: logicalProjectID, Query: query, Limit: limit}
	records, err := s.Memory.Recall(recallCtx, input)
	if err != nil {
		return errorResult(err), nil
	}
	items := make([]map[string]any, 0, len(records))
	for _, record := range records {
		items = append(items, map[string]any{
			"id": record.ID, "scope": record.Scope, "level": record.Level, "kind": record.Kind,
			"sourceType": record.SourceType, "summary": record.Summary, "branch": record.Branch,
			"files": record.Files, "score": record.Score, "createdAt": record.CreatedAt, "updatedAt": record.UpdatedAt,
		})
	}
	result := textResult(map[string]any{
		"query": query, "workspaceId": logicalWorkspaceID, "memories": items,
	}, false)
	if graphStore, ok := s.Memory.(graphMemoryStore); ok && len(records) > 0 {
		graph, graphErr := graphStore.RecallGraphContext(recallCtx, input, records)
		if graphErr == nil {
			attachGraphMemoryContext(result, graph)
		}
	}
	return result, nil
}

func ruleGovernedMutation(operation operationInvocation) bool {
	if !operation.MutatesState {
		return false
	}
	id := strings.TrimSpace(operation.OperationID)
	return strings.HasPrefix(id, "edit.") || strings.HasPrefix(id, "git.") || strings.HasPrefix(id, "terminal.") || strings.HasPrefix(id, "process.")
}

func mutationBlockedState(userID, session, workspaceKey string) (taskstate.State, bool) {
	state, ok := workingMemory.Get(userID, session, workspaceKey)
	return state, ok && state.RuleMutationBlocked
}

func mutationTargetPaths(publicTool string, operation operationInvocation, args map[string]any) []string {
	patch := taskPatchForOperation(publicTool, operation, args, nil)
	seen := map[string]struct{}{}
	out := make([]string, 0, len(patch.TouchedFiles))
	for _, path := range patch.TouchedFiles {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if _, exists := seen[path]; exists {
			continue
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	return out
}

func contextRefreshRequired(result *mcp.CallToolResult, targets []string) *mcp.CallToolResult {
	if result == nil {
		return errorResult(fmt.Errorf("Project Brain context refresh failed before mutation"))
	}
	root := resultRoot(result)
	if root == nil {
		root = map[string]any{}
	}
	root["status"] = "context_refresh_required"
	root["error"] = "Project Brain resolved rules for the concrete mutation target. Review the refreshed projectBrain packet, then retry the mutation."
	root["targets"] = append([]string(nil), targets...)
	result.StructuredContent = root
	result.IsError = true
	return result
}

func (s *Service) refreshProjectBrainBeforeMutation(ctx context.Context, userID, session, workspaceKey, publicTool string, operation operationInvocation, args map[string]any, req *mcp.CallToolRequest) (*mcp.CallToolResult, bool) {
	state, ok := workingMemory.Get(userID, session, workspaceKey)
	if !ok || strings.TrimSpace(state.Task) == "" {
		return nil, false
	}
	targets := mutationTargetPaths(publicTool, operation, args)
	if len(targets) == 0 {
		return nil, false
	}
	contextOperation, err := operationForRuntimeTool("context_for_task")
	if err != nil {
		return errorResult(err), true
	}
	contextArgs := map[string]any{
		"taskHint": state.Task,
		"targets":  targets,
		"limit":    30,
	}
	if workspaceKey != "" {
		contextArgs["workspaceKey"] = workspaceKey
	}
	oldHash := state.ContextHash
	result, callErr := s.callOperation(ctx, userID, "context", contextOperation, contextArgs, req)
	if callErr != nil {
		return errorResult(callErr), true
	}
	if result == nil || result.IsError {
		if result == nil {
			return errorResult(fmt.Errorf("Project Brain context refresh returned no result before mutation")), true
		}
		return result, true
	}
	patch := taskPatchForOperation("context", contextOperation, contextArgs, result)
	state = workingMemory.Update(userID, session, workspaceKey, patch)
	if state.ContextHash != "" && state.ContextHash != oldHash {
		// Stop before the write. The caller receives the exact-target rule packet
		// and can retry only after it has observed the refreshed constraints.
		return contextRefreshRequired(result, targets), true
	}
	return nil, false
}

func taskExecutionRuntimeArgs(args map[string]any, state taskstate.State) map[string]any {
	if strings.TrimSpace(state.TaskID) == "" {
		return args
	}
	out := make(map[string]any, len(args)+2)
	for key, value := range args {
		out[key] = value
	}
	out["__codelocalTaskId"] = state.TaskID
	out["__codelocalTaskOwner"] = state.SessionID
	return out
}

func (s *Service) callOperationRemembering(ctx context.Context, userID, publicTool string, operation operationInvocation, args map[string]any, req *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	session := sessionID(req)
	workspaceKey := memoryWorkspaceKey(s, userID, session, args)
	if workspaceKey == "" {
		workspaceKey = strings.TrimSpace(s.route(userID, session))
	}
	if workspaceKey != "" && ruleGovernedMutation(operation) {
		if refreshed, stop := s.refreshProjectBrainBeforeMutation(ctx, userID, session, workspaceKey, publicTool, operation, args, req); stop {
			return refreshed, nil
		}
		if state, blocked := mutationBlockedState(userID, session, workspaceKey); blocked {
			missing := strings.Join(state.OmittedRequiredRuleIDs, ", ")
			if missing == "" {
				missing = "unknown"
			}
			return errorResult(fmt.Errorf("Project Brain mandatory rules exceed the current context budget; project mutation is blocked until context is re-resolved with concrete targets (omitted rule IDs: %s)", missing)), nil
		}
	}
	runtimeArgs := args
	if workspaceKey != "" {
		if current, ok := workingMemory.Get(userID, session, workspaceKey); ok {
			runtimeArgs = taskExecutionRuntimeArgs(args, current)
		}
	}
	result, err := s.callOperation(ctx, userID, publicTool, operation, runtimeArgs, req)
	attachRecoveryHint(result)
	if operation.OperationID == "memory.remember" || operation.OperationID == "memory.recall" {
		return result, err
	}
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
	if shouldRefreshQuality(operation, args, result) {
		state = workingMemory.Update(userID, session, workspaceKey, qualityPatchForState(state))
	}

	logicalWorkspaceID := workspaceKey
	logicalDeviceID := ""
	var workspace *gateway.WorkspaceView
	if s.Workspaces != nil && (publicTool == "context" || s.Memory != nil) {
		if activated, activateErr := s.Workspaces.Activate(ctx, userID, workspaceKey); activateErr == nil {
			workspace = activated
			logicalDeviceID = strings.TrimSpace(activated.DeviceID)
			if strings.TrimSpace(activated.WorkspaceID) != "" {
				logicalWorkspaceID = strings.TrimSpace(activated.WorkspaceID)
			}
		}
	}
	if publicTool == "context" && err == nil && result != nil && !result.IsError {
		recalled, graphContext := s.recallLongTermMemory(ctx, userID, logicalDeviceID, logicalWorkspaceID, state)
		if checkpoint, ok := agentCheckpointPatch(state, recalled, time.Now()); ok {
			state = workingMemory.Update(userID, session, workspaceKey, checkpoint)
		}
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
		attachLongTermMemory(result, recalled)
		attachGraphMemoryContext(result, graphContext)
	}
	attachAgentLoop(result, state)
	s.recordAutomaticLearning(ctx, userID, session, logicalDeviceID, logicalWorkspaceID, publicTool, state, result)
	return result, err
}

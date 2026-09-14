package localclient

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/approval"
	"github.com/0xmarkhydra/codelocal/internal/audit"
	"github.com/0xmarkhydra/codelocal/internal/codequality"
	"github.com/0xmarkhydra/codelocal/internal/editing"
	"github.com/0xmarkhydra/codelocal/internal/history"
	"github.com/0xmarkhydra/codelocal/internal/idempotency"
	"github.com/0xmarkhydra/codelocal/internal/learnedskills"
	"github.com/0xmarkhydra/codelocal/internal/localfs"
	"github.com/0xmarkhydra/codelocal/internal/mcphub"
	processmgr "github.com/0xmarkhydra/codelocal/internal/process"
	"github.com/0xmarkhydra/codelocal/internal/project"
	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
	"github.com/0xmarkhydra/codelocal/internal/protocol"
	"github.com/0xmarkhydra/codelocal/internal/repository"
	"github.com/0xmarkhydra/codelocal/internal/security"
	"github.com/0xmarkhydra/codelocal/internal/taskexecution"
	"github.com/0xmarkhydra/codelocal/internal/version"
)

type Engine struct {
	Root           string
	WorkspaceID    string
	WorkspaceName  string
	WorkspaceKey   string
	DeviceID       string
	ShellEnabled   bool
	ApprovalMode   string
	FS             *localfs.FS
	Project        *project.Engine
	Repositories   *repository.Registry
	TaskExecutions *taskexecution.Manager
	Editing        *editing.Engine
	Processes      *processmgr.Manager
	Approvals      *approval.Memory
	Broker         *approval.Broker
	History        *history.Terminal
	Journal        *idempotency.Journal
	MCP            *mcphub.Hub
	Skills         *learnedskills.Store
	mu             sync.Mutex
	baselines      map[string][]map[string]any
	runtimeEnv     map[string]string
	runtimeSecrets map[string]string
	runtimeRedact  []string
}

type HandleOptions struct{ RequestID, SessionID, IdempotencyKey string }

func New(root, workspaceID, workspaceName, workspaceKey, deviceID string) (*Engine, error) {
	fs, err := localfs.New(root)
	if err != nil {
		return nil, err
	}
	approvals := approval.New()
	broker := approval.NewBroker()
	terminalHistory := history.New()
	shellEnabled := os.Getenv("CODELOCAL_ALLOW_SHELL") != "0"
	approvalMode := string(approval.ResolveMode(workspaceID))
	repositories := repository.New(fs.Root, workspaceName)
	engine := &Engine{Root: fs.Root, WorkspaceID: workspaceID, WorkspaceName: workspaceName, WorkspaceKey: workspaceKey, DeviceID: deviceID, ShellEnabled: shellEnabled, ApprovalMode: approvalMode, FS: fs, Project: project.NewWithRepositories(fs, repositories), Repositories: repositories, TaskExecutions: taskexecution.NewManager(nil, nil), Editing: editing.New(fs), Approvals: approvals, Broker: broker, History: terminalHistory, Journal: idempotency.New(workspaceKey), Skills: learnedskills.New(), baselines: map[string][]map[string]any{}, runtimeEnv: map[string]string{}, runtimeSecrets: map[string]string{}}
	engine.Processes = processmgr.NewManager(fs.Root, workspaceKey, func(record *processmgr.Record, stream, value string) {}, func(record *processmgr.Record) {
		_, _ = terminalHistory.Finished(record)
		engine.Project.Invalidate()
		audit.Write(audit.Event{Event: "process.finished", RequestID: record.RequestID, MCPSessionID: record.OwnerSessionID, WorkspaceKey: workspaceKey, ProcessID: record.ProcessID, Status: string(record.Status), Detail: map[string]any{"exitCode": record.ExitCode, "signal": record.Signal}})
	})
	mcpHub, err := mcphub.New(fs.Root, func(config mcphub.ServerConfig) error {
		return fmt.Errorf("MCP_CONNECT_APPROVAL_REQUIRED: connecting %s requires explicit user approval in the current MCP client", config.Name)
	})
	if err != nil {
		return nil, err
	}
	mcpHub.SetSecretResolver(func(name string) (string, bool) {
		engine.mu.Lock()
		defer engine.mu.Unlock()
		value, ok := engine.runtimeSecrets[name]
		return value, ok
	})
	engine.MCP = mcpHub
	return engine, nil
}

func (e *Engine) SetRuntimeEnvironment(values, secrets map[string]string) {
	e.mu.Lock()
	e.runtimeEnv = make(map[string]string, len(values))
	for key, value := range values {
		e.runtimeEnv[key] = value
	}
	e.runtimeSecrets = make(map[string]string, len(secrets))
	for key, value := range secrets {
		e.runtimeSecrets[key] = value
	}
	e.rebuildRuntimeRedactLocked()
	_, penpotConfigured := e.runtimeSecrets[managedPenpotCredentialRef()]
	e.mu.Unlock()
	ref := ""
	if penpotConfigured {
		ref = managedPenpotCredentialRef()
	}
	_ = e.MCP.SetManagedPenpotCredentialRef(ref)
}

func (e *Engine) rebuildRuntimeRedactLocked() {
	secretNames := make([]string, 0, len(e.runtimeSecrets))
	for key := range e.runtimeSecrets {
		secretNames = append(secretNames, key)
	}
	sort.Strings(secretNames)
	e.runtimeRedact = e.runtimeRedact[:0]
	for _, key := range secretNames {
		if value := strings.TrimSpace(e.runtimeSecrets[key]); value != "" {
			e.runtimeRedact = append(e.runtimeRedact, value)
		}
	}
}

func (e *Engine) setRuntimeSecret(name, value string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.runtimeSecrets[name] = value
	e.rebuildRuntimeRedactLocked()
}

func (e *Engine) deleteRuntimeSecret(name string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	delete(e.runtimeSecrets, name)
	e.rebuildRuntimeRedactLocked()
}

func (e *Engine) runtimeEnvironment(requestedSecrets []string) (map[string]string, []string, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make(map[string]string, len(e.runtimeEnv)+len(requestedSecrets))
	for key, value := range e.runtimeEnv {
		out[key] = value
	}
	redact := append([]string(nil), e.runtimeRedact...)
	seenRedact := map[string]struct{}{}
	for _, value := range redact {
		seenRedact[value] = struct{}{}
	}
	for _, name := range requestedSecrets {
		if !processmgr.CanInjectEnvKey(name) {
			return nil, nil, fmt.Errorf("runtime secret %s cannot be injected into a child process", name)
		}
		value, ok := e.runtimeSecrets[name]
		if !ok {
			value, ok = os.LookupEnv(name)
		}
		if !ok {
			return nil, nil, fmt.Errorf("runtime secret %s is not configured", name)
		}
		out[name] = value
		if trimmed := strings.TrimSpace(value); trimmed != "" {
			if _, exists := seenRedact[trimmed]; !exists {
				redact = append(redact, trimmed)
				seenRedact[trimmed] = struct{}{}
			}
		}
	}
	return out, redact, nil
}

func (e *Engine) SemanticProviders() []string {
	providers := []string{"go-native-structure"}
	seen := map[string]struct{}{"go-native-structure": {}}
	// Registration must stay cheap. SemanticInfo() also builds the structural
	// code graph, which can walk a large multi-repository workspace and make a
	// sleeping workspace miss the Cloud activation timeout before /client is
	// even registered. Provider discovery alone is sufficient for capabilities.
	info := e.Project.SemanticProviderInfo()
	appendProvider := func(item map[string]any) {
		installed, _ := item["installed"].(bool)
		id := asString(item["id"])
		if !installed || id == "" {
			return
		}
		if _, ok := seen[id]; ok {
			return
		}
		seen[id] = struct{}{}
		providers = append(providers, id)
	}
	if items, ok := info["providers"].([]map[string]any); ok {
		for _, item := range items {
			appendProvider(item)
		}
	} else if items, ok := info["providers"].([]any); ok {
		for _, raw := range items {
			appendProvider(object(raw))
		}
	}
	return providers
}

func (e *Engine) Close() {
	e.Processes.StopAll("workspace runtime stopped")
	if e.Project != nil {
		e.Project.Close()
	}
	if e.MCP != nil {
		e.MCP.Close()
	}
}

func asString(v any) string {
	if v == nil {
		return ""
	}
	if s, ok := v.(string); ok {
		return s
	}
	return fmt.Sprint(v)
}
func asBool(v any, def bool) bool {
	if b, ok := v.(bool); ok {
		return b
	}
	return def
}
func asInt(v any, def int) int {
	switch n := v.(type) {
	case float64:
		return int(n)
	case int:
		return n
	case json.Number:
		if i, err := n.Int64(); err == nil {
			return int(i)
		}
	case string:
		if i, err := strconv.Atoi(n); err == nil {
			return i
		}
	}
	return def
}
func stringSlice(v any) []string {
	raw, ok := v.([]any)
	if !ok {
		if s, ok := v.([]string); ok {
			return s
		}
		return nil
	}
	out := []string{}
	for _, item := range raw {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
func object(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}
func networkPolicy() security.NetworkPolicy {
	switch strings.ToLower(os.Getenv("CODELOCAL_NETWORK")) {
	case "deny":
		return security.NetworkDeny
	case "allow":
		return security.NetworkAllow
	default:
		return security.NetworkApproval
	}
}

func (e *Engine) cwd(relative string) (string, error) {
	if strings.TrimSpace(relative) == "" {
		relative = "."
	}
	path, err := e.FS.Existing(relative)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		return "", errors.New("cwd must be a workspace directory")
	}
	return path, nil
}

func (e *Engine) authorize(command, cwd, provided, sessionID string) (bool, map[string]any, security.Decision, error) {
	decision := security.Classify(command, networkPolicy(), security.Context{WorkspaceRoot: e.Root, CWD: cwd})
	return e.authorizeDecision(command, cwd, provided, sessionID, decision)
}

func (e *Engine) authorizeDecision(command, cwd, provided, sessionID string, decision security.Decision) (bool, map[string]any, security.Decision, error) {
	return e.authorizeDecisionWithDisplay(command, cwd, e.FS.Rel(cwd), provided, sessionID, decision)
}

func (e *Engine) authorizeDecisionWithDisplay(command, executionCWD, displayCWD, provided, sessionID string, decision security.Decision) (bool, map[string]any, security.Decision, error) {
	if decision.Blocked {
		return false, map[string]any{"status": "blocked", "riskLevel": decision.RiskLevel, "reason": decision.Reason, "matchedRules": decision.MatchedRules, "command": decision.RedactedCommand, "approvalPolicy": decision.ApprovalPolicy}, decision, errors.New(decision.Reason)
	}
	if !decision.RequiresApproval {
		return true, nil, decision, nil
	}
	mode := approval.ResolveMode(e.WorkspaceID)
	e.ApprovalMode = string(mode)
	if approval.FullAllows(mode, decision) {
		return true, map[string]any{"fullAccessApproved": true, "approvalMode": approval.UserMode(mode), "approvalKey": decision.ApprovalKey}, decision, nil
	}
	if approval.AgentAllows(mode, decision) {
		return true, map[string]any{"agentApproved": true, "approvalMode": approval.UserMode(mode), "approvalKey": decision.ApprovalKey}, decision, nil
	}
	if approval.DeniesApproval(mode, decision) {
		reason := "local approval mode " + string(mode) + " does not permit this action"
		return false, map[string]any{"status": "blocked", "riskLevel": decision.RiskLevel, "reason": reason, "matchedRules": decision.MatchedRules, "command": decision.RedactedCommand, "approvalPolicy": decision.ApprovalPolicy, "approvalMode": string(mode)}, decision, nil
	}
	if decision.ApprovalPolicy == security.ApprovalRememberable && decision.ApprovalKey != "" {
		remembered, err := e.Approvals.Find(e.WorkspaceKey, sessionID, decision.ApprovalKey, decision.RiskLevel)
		if err != nil {
			return false, nil, decision, err
		}
		if remembered != nil {
			_, _ = e.Approvals.Touch(e.WorkspaceKey, sessionID, decision.ApprovalKey)
			return true, map[string]any{"remembered": true, "approvalId": remembered.ID}, decision, nil
		}
	}
	if provided != "" && e.Broker.ConsumeScoped(sessionID, provided, command, displayCWD, decision) {
		if decision.ApprovalPolicy == security.ApprovalRememberable {
			entry, err := e.Approvals.Remember(e.WorkspaceKey, sessionID, decision)
			if err != nil {
				return false, nil, decision, err
			}
			return true, map[string]any{"remembered": entry != nil, "approvalId": func() string {
				if entry != nil {
					return entry.ID
				}
				return ""
			}()}, decision, nil
		}
		return true, map[string]any{"approved": true}, decision, nil
	}
	preflight := e.Broker.PreflightScoped(sessionID, command, displayCWD, decision)
	return false, map[string]any{"status": preflight.Status, "riskLevel": preflight.RiskLevel, "reason": preflight.Reason, "matchedRules": preflight.MatchedRules, "command": preflight.Command, "approvalPolicy": preflight.ApprovalPolicy, "approvalKey": preflight.ApprovalKey, "approvalLabel": preflight.ApprovalLabel, "approvalToken": preflight.ApprovalToken, "expiresAt": preflight.ExpiresAt, "workspaceAccess": approval.UserModePrompt(mode)}, decision, nil
}

func (e *Engine) preflightAt(command, securityRoot, executionCWD, displayCWD, sessionID string) (map[string]any, error) {
	decision := security.Classify(command, networkPolicy(), security.Context{WorkspaceRoot: securityRoot, CWD: executionCWD})
	_, decision, secretErr := e.prepareOpaqueSecretExecution(map[string]any{}, command, securityRoot, executionCWD, decision)
	if secretErr != nil {
		return nil, secretErr
	}
	mode := approval.ResolveMode(e.WorkspaceID)
	e.ApprovalMode = string(mode)
	if approval.FullAllows(mode, decision) {
		return map[string]any{"status": "safe", "riskLevel": decision.RiskLevel, "reason": decision.Reason, "matchedRules": decision.MatchedRules, "command": decision.RedactedCommand, "approvalPolicy": decision.ApprovalPolicy, "approvalKey": decision.ApprovalKey, "approvalLabel": decision.ApprovalLabel, "fullAccessApproved": true, "approvalMode": approval.UserMode(mode)}, nil
	}
	if approval.AgentAllows(mode, decision) {
		return map[string]any{"status": "safe", "riskLevel": decision.RiskLevel, "reason": decision.Reason, "matchedRules": decision.MatchedRules, "command": decision.RedactedCommand, "approvalPolicy": decision.ApprovalPolicy, "approvalKey": decision.ApprovalKey, "approvalLabel": decision.ApprovalLabel, "agentApproved": true, "approvalMode": approval.UserMode(mode)}, nil
	}
	if approval.DeniesApproval(mode, decision) {
		return map[string]any{"status": "blocked", "riskLevel": decision.RiskLevel, "reason": "local approval mode " + string(mode) + " does not permit this action", "matchedRules": decision.MatchedRules, "command": decision.RedactedCommand, "approvalPolicy": decision.ApprovalPolicy, "approvalMode": string(mode)}, nil
	}
	if decision.ApprovalPolicy == security.ApprovalRememberable && decision.ApprovalKey != "" {
		remembered, _ := e.Approvals.Find(e.WorkspaceKey, sessionID, decision.ApprovalKey, decision.RiskLevel)
		if remembered != nil {
			return map[string]any{"status": "safe", "riskLevel": decision.RiskLevel, "reason": decision.Reason, "matchedRules": decision.MatchedRules, "command": decision.RedactedCommand, "approvalPolicy": decision.ApprovalPolicy, "approvalKey": decision.ApprovalKey, "approvalLabel": decision.ApprovalLabel, "remembered": true, "approvalId": remembered.ID, "expiresAt": remembered.ExpiresAt}, nil
		}
	}
	pre := e.Broker.PreflightScoped(sessionID, command, displayCWD, decision)
	return map[string]any{"status": pre.Status, "riskLevel": pre.RiskLevel, "reason": pre.Reason, "matchedRules": pre.MatchedRules, "command": pre.Command, "approvalPolicy": pre.ApprovalPolicy, "approvalKey": pre.ApprovalKey, "approvalLabel": pre.ApprovalLabel, "approvalToken": pre.ApprovalToken, "expiresAt": pre.ExpiresAt, "workspaceAccess": approval.UserModePrompt(mode)}, nil
}

func (e *Engine) PreflightScoped(command, cwd, sessionID string) (map[string]any, error) {
	if !e.ShellEnabled {
		return map[string]any{"status": "blocked", "riskLevel": "BLOCKED", "reason": "Shell execution is disabled for this workspace.", "matchedRules": []string{"shell-disabled"}, "command": command, "approvalPolicy": "blocked"}, nil
	}
	absolute, err := e.cwd(cwd)
	if err != nil {
		return nil, err
	}
	return e.preflightAt(command, e.Root, absolute, e.FS.Rel(absolute), sessionID)
}

func (e *Engine) Preflight(command, cwd string) (map[string]any, error) {
	return e.PreflightScoped(command, cwd, "")
}

func (e *Engine) startProcess(command string, args map[string]any, opts HandleOptions, usePTY bool) (map[string]any, error) {
	if !e.ShellEnabled {
		return nil, errors.New("Shell execution is disabled. Set CODELOCAL_ALLOW_SHELL=1 and restart CodeLocal to enable terminal tools.")
	}
	logicalCWD := strings.TrimSpace(asString(args["cwd"]))
	if logicalCWD == "" {
		logicalCWD = "."
	}
	target, cwd, err := e.taskExecutionBindingForCWD(context.Background(), args, opts, logicalCWD, asString(args[privateTaskExecutionID]) != "")
	if err != nil {
		return nil, err
	}
	securityRoot := e.Root
	if target.Active && target.FS != nil {
		securityRoot = target.FS.Root
	}
	decision := security.Classify(command, networkPolicy(), security.Context{WorkspaceRoot: securityRoot, CWD: cwd})
	requestedSecrets, decision, secretErr := e.prepareOpaqueSecretExecution(args, command, securityRoot, cwd, decision)
	if secretErr != nil {
		return nil, secretErr
	}
	approved, approvalState, decision, authErr := e.authorizeDecisionWithDisplay(command, cwd, logicalCWD, asString(args["approvalToken"]), opts.SessionID, decision)
	if authErr != nil && decision.Blocked {
		return approvalState, nil
	}
	if authErr != nil {
		return nil, authErr
	}
	if !approved {
		return approvalState, nil
	}
	timeoutMs := asInt(args["timeoutMs"], 0)
	runtimeEnv, redactValues, secretErr := e.runtimeEnvironment(requestedSecrets)
	if secretErr != nil {
		return nil, secretErr
	}
	start, err := e.Processes.Start(command, processmgr.StartOptions{CWD: cwd, DisplayCWD: logicalCWD, Timeout: time.Duration(timeoutMs) * time.Millisecond, OwnerSessionID: opts.SessionID, RequestID: opts.RequestID, UsePTY: usePTY, Cols: asInt(args["cols"], 120), Rows: asInt(args["rows"], 36), Env: runtimeEnv, RedactValues: redactValues})
	if err != nil {
		return nil, err
	}
	approvalKind := "automatic"
	if approvalState != nil {
		if fullApproved, _ := approvalState["fullAccessApproved"].(bool); fullApproved {
			approvalKind = "full-access"
		} else if agentApproved, _ := approvalState["agentApproved"].(bool); agentApproved {
			approvalKind = "agent-mode"
		} else if remembered, _ := approvalState["remembered"].(bool); remembered {
			approvalKind = "remembered"
		} else {
			approvalKind = "chat-confirmed"
		}
	}
	_, _ = e.History.Started(history.StartInput{WorkspaceKey: e.WorkspaceKey, ProcessID: start.ProcessID, RequestID: opts.RequestID, SessionID: opts.SessionID, CWD: logicalCWD, Command: command, RiskLevel: string(decision.RiskLevel), MatchedRules: decision.MatchedRules, Approval: approvalKind, StartedAt: start.StartedAt, ExecutionMode: start.ExecutionMode})
	audit.Write(audit.Event{Event: "process.started", RequestID: opts.RequestID, MCPSessionID: opts.SessionID, WorkspaceKey: e.WorkspaceKey, ProcessID: start.ProcessID, RiskLevel: string(decision.RiskLevel), Detail: map[string]any{"command": decision.RedactedCommand, "cwd": logicalCWD, "approval": approvalKind, "taskExecution": taskExecutionMetadata(target)}})
	result := snapshotMap(start)
	if metadata := taskExecutionMetadata(target); metadata != nil {
		result["taskExecution"] = metadata
	}
	return result, nil
}

func snapshotMap(s processmgr.Snapshot) map[string]any {
	raw, _ := json.Marshal(s)
	out := map[string]any{}
	_ = json.Unmarshal(raw, &out)
	return out
}
func (e *Engine) runCommand(ctx context.Context, args map[string]any, opts HandleOptions) (map[string]any, error) {
	result, err := e.startProcess(asString(args["command"]), args, opts, false)
	if err != nil {
		return nil, err
	}
	if result["processId"] == nil {
		return result, nil
	}
	yield := asInt(args["yieldMs"], 1000)
	if yield < 0 {
		yield = 0
	}
	if yield > 10000 {
		yield = 10000
	}
	if yield > 0 {
		timer := time.NewTimer(time.Duration(yield) * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return result, nil
		case <-timer.C:
		}
	}
	id := asString(result["processId"])
	snapshot, err := e.Processes.Snapshot(id, nil, nil)
	if err == nil {
		return snapshotMap(snapshot), nil
	}
	return result, nil
}

func runGit(root string, args ...string) (map[string]any, error) {
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	cmd.Env = append(os.Environ(), "PAGER=cat", "GIT_PAGER=cat", "CI=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	code := 0
	if cmd.ProcessState != nil {
		code = cmd.ProcessState.ExitCode()
	}
	if err != nil {
		var exit *exec.ExitError
		if !errors.As(err, &exit) {
			return nil, err
		}
	}
	output := stdout.String() + stderr.String()
	return map[string]any{"command": "git " + strings.Join(args, " "), "stdout": stdout.String(), "stderr": stderr.String(), "exitCode": code, "output": output}, nil
}

func safeGitPath(path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return errors.New("Git path is required")
	}
	if filepath.IsAbs(path) {
		return errors.New("absolute Git paths are not allowed")
	}
	clean := filepath.Clean(path)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return errors.New("Git path escapes the authorized workspace")
	}
	if security.IsSensitivePath(filepath.ToSlash(clean)) {
		return fmt.Errorf("access blocked by sensitive-path policy: %s", path)
	}
	return nil
}

func (e *Engine) guardedGitAt(root string, args []string, approvalToken, sessionID string) (map[string]any, error) {
	command := "git " + strings.Join(args, " ")
	decision := security.Classify(command, networkPolicy(), security.Context{WorkspaceRoot: e.Root, CWD: root})
	if len(args) >= 3 && args[0] == "push" && decision.ApprovalPolicy == security.ApprovalRememberable && strings.HasPrefix(decision.ApprovalKey, "git.push:") {
		remote := args[1]
		resolved, resolveErr := runGit(root, "remote", "get-url", "--push", remote)
		url := ""
		if resolveErr == nil {
			url = strings.TrimSpace(asString(resolved["stdout"]))
		}
		if url == "" {
			decision.ApprovalPolicy = security.ApprovalAlways
			decision.ApprovalKey = ""
			decision.ApprovalLabel = ""
		} else {
			digest := sha256.Sum256([]byte(url))
			decision.ApprovalKey += fmt.Sprintf(":remote-%x", digest[:8])
		}
	}
	approved, state, _, err := e.authorizeDecision(command, root, approvalToken, sessionID, decision)
	if err != nil && state != nil {
		return state, nil
	}
	if err != nil {
		return nil, err
	}
	if !approved {
		return state, nil
	}
	return runGit(root, args...)
}

func (e *Engine) readInstructions(path string) (map[string]any, error) {
	target, err := e.FS.Existing(path)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(target)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		target = filepath.Dir(target)
	}
	files := []string{}
	contents := []map[string]any{}
	current := target
	for {
		candidate := filepath.Join(current, "AGENTS.md")
		if data, readErr := os.ReadFile(candidate); readErr == nil {
			rel := e.FS.Rel(candidate)
			files = append(files, rel)
			contents = append(contents, map[string]any{"path": rel, "content": string(data)})
		}
		if current == e.Root {
			break
		}
		parent := filepath.Dir(current)
		if parent == current || !strings.HasPrefix(parent, e.Root) {
			break
		}
		current = parent
	}
	sort.Strings(files)
	return map[string]any{"instructionFiles": files, "instructions": contents}, nil
}

func projectBrainTargets(result map[string]any, explicit []string) []string {
	out := []string{}
	seen := map[string]struct{}{}
	appendPath := func(value any) {
		path := strings.TrimSpace(fmt.Sprint(value))
		if path == "" || path == "." {
			return
		}
		if _, exists := seen[path]; exists {
			return
		}
		seen[path] = struct{}{}
		out = append(out, path)
	}
	for _, path := range explicit {
		appendPath(path)
		if len(out) >= 32 {
			return out
		}
	}
	if len(out) > 0 {
		return out
	}
	// Ranked files remain useful for initial grounding, but explicit targets
	// discovered during work always win and trigger a fresh rule resolution.
	switch ranked := result["rankedFiles"].(type) {
	case []map[string]any:
		for _, item := range ranked {
			appendPath(item["path"])
			if len(out) >= 32 || (len(explicit) == 0 && len(out) >= 8) {
				break
			}
		}
	case []any:
		for _, raw := range ranked {
			if item, ok := raw.(map[string]any); ok {
				appendPath(item["path"])
			}
			if len(out) >= 32 || (len(explicit) == 0 && len(out) >= 8) {
				break
			}
		}
	}
	return out
}

func boundedContextList(value any, limit int) []any {
	out := []any{}
	switch typed := value.(type) {
	case []any:
		out = append(out, typed...)
	case []string:
		for _, item := range typed {
			out = append(out, item)
		}
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

func compactRepositoryContext(value any, limit int) []map[string]any {
	items := []map[string]any{}
	appendItem := func(item map[string]any) {
		if item == nil || len(items) >= limit {
			return
		}
		items = append(items, map[string]any{
			"id": item["id"], "path": item["path"], "identitySource": item["identitySource"], "packageManager": item["packageManager"],
			"manifestCount": len(boundedContextList(item["manifests"], 256)), "moduleCount": len(boundedContextList(item["modules"], 1024)), "entrypointCount": len(boundedContextList(item["entrypoints"], 256)),
			"buildCommands": boundedContextList(item["buildCommands"], 4), "testCommands": boundedContextList(item["testCommands"], 4),
			"typecheckCommands": boundedContextList(item["typecheckCommands"], 4), "lintCommands": boundedContextList(item["lintCommands"], 4),
		})
	}
	switch typed := value.(type) {
	case []map[string]any:
		for _, item := range typed {
			appendItem(item)
		}
	case []any:
		for _, raw := range typed {
			if item, ok := raw.(map[string]any); ok {
				appendItem(item)
			}
		}
	}
	return items
}

func compactProjectContext(projectMap map[string]any) map[string]any {
	out := map[string]any{}
	for _, key := range []string{"languages", "frameworks", "workspaceRoots", "sourceRoots", "testRoots"} {
		out[key] = boundedContextList(projectMap[key], 16)
	}
	out["repositories"] = compactRepositoryContext(projectMap["repositories"], 24)
	out["entrypoints"] = boundedContextList(projectMap["entrypoints"], 12)
	out["packageManager"] = projectMap["packageManager"]
	out["commands"] = map[string]any{"build": boundedContextList(projectMap["buildCommands"], 8), "test": boundedContextList(projectMap["testCommands"], 8), "lint": boundedContextList(projectMap["lintCommands"], 8), "typecheck": boundedContextList(projectMap["typecheckCommands"], 8)}
	if intelligence, ok := projectMap["intelligence"].(map[string]any); ok {
		out["intelligence"] = map[string]any{"builtAt": intelligence["builtAt"], "dirty": intelligence["dirty"], "files": intelligence["files"], "sourceFiles": intelligence["sourceFiles"], "knowledgeSources": intelligence["knowledgeSources"], "repositories": intelligence["repositories"], "languages": boundedContextList(intelligence["languages"], 16)}
	}
	return out
}

func focusedRepositoryKeys(route any) map[string]struct{} {
	value, ok := route.(map[string]any)
	if !ok || asString(value["mode"]) != "focused" {
		return nil
	}
	keys := map[string]struct{}{}
	appendKey := func(item map[string]any) {
		if len(keys) >= 24 || item == nil {
			return
		}
		keys[asString(item["repositoryId"])+"\x00"+asString(item["repositoryPath"])] = struct{}{}
	}
	switch typed := value["selectedRepositories"].(type) {
	case []map[string]any:
		for _, item := range typed {
			appendKey(item)
		}
	case []any:
		for _, raw := range typed {
			if item, ok := raw.(map[string]any); ok {
				appendKey(item)
			}
		}
	}
	return keys
}

func repositoryOwnerPath(path string, repositoryPaths []string) string {
	path = strings.TrimPrefix(filepath.ToSlash(filepath.Clean(path)), "./")
	best, bestDepth := "", -1
	for _, root := range repositoryPaths {
		matches := root == "." || path == root || strings.HasPrefix(path, root+"/")
		if !matches {
			continue
		}
		depth := 0
		if root != "." {
			depth = strings.Count(root, "/") + 1
		}
		if depth > bestDepth {
			best, bestDepth = root, depth
		}
	}
	return best
}

func filterFocusedProjectPaths(value any, allRepositoryPaths []string, selectedPaths map[string]struct{}) []any {
	out := []any{}
	for _, raw := range boundedContextList(value, 64) {
		path := strings.TrimSpace(fmt.Sprint(raw))
		owner := repositoryOwnerPath(path, allRepositoryPaths)
		if owner == "" {
			out = append(out, raw)
			continue
		}
		if _, ok := selectedPaths[owner]; ok {
			out = append(out, raw)
		}
	}
	return out
}

func compactProjectContextForRoute(projectMap map[string]any, route any) map[string]any {
	out := compactProjectContext(projectMap)
	keys := focusedRepositoryKeys(route)
	if len(keys) == 0 {
		return out
	}
	repositories, _ := out["repositories"].([]map[string]any)
	focused, allPaths, selectedPaths := []map[string]any{}, []string{}, map[string]struct{}{}
	for _, repo := range repositories {
		path := asString(repo["path"])
		allPaths = append(allPaths, path)
		if _, ok := keys[asString(repo["id"])+"\x00"+path]; ok {
			focused = append(focused, repo)
			selectedPaths[path] = struct{}{}
		}
	}
	out["repositories"] = focused
	for _, key := range []string{"workspaceRoots", "sourceRoots", "testRoots", "entrypoints"} {
		out[key] = filterFocusedProjectPaths(out[key], allPaths, selectedPaths)
	}
	out["repositoryContextFocused"] = true
	return out
}

func (e *Engine) attachProjectBrainContext(result map[string]any, explicitTargets []string, taskHint string) {
	if e == nil || e.FS == nil || result == nil {
		return
	}
	projectMap, ok := result["project"].(map[string]any)
	if !ok || projectMap == nil {
		return
	}
	manifest := projectbrain.FromProjectMap(projectMap)
	identity := projectidentity.Discover(e.Root, e.WorkspaceName)
	resolved, err := projectbrain.ResolveRulesWithOptions(e.FS, manifest, projectbrain.ResolveOptions{
		Targets: projectBrainTargets(result, explicitTargets), TaskHint: taskHint, Repositories: identity.Repositories,
	})
	if err != nil {
		return
	}
	packet := projectbrain.CompileContext(resolved, projectbrain.DefaultRuleContextBudget)
	result["projectBrain"] = packet
	if len(explicitTargets) > 0 {
		result["projectBrainTargetSource"] = "explicit"
	} else {
		result["projectBrainTargetSource"] = "ranked"
	}
	branch, _, branches := e.repositoryBranchState()
	result["gitBranch"] = branch
	result["gitBranches"] = branches
	if budget, ok := result["contextBudget"].(map[string]any); ok {
		budget["projectBrainMaxChars"] = packet.Budget.MaxChars
		budget["projectBrainUsedChars"] = packet.Budget.UsedChars
		budget["projectBrainInputChars"] = packet.Budget.InputChars
		budget["projectBrainDeduplicatedChars"] = packet.Budget.DeduplicatedChars
		budget["projectBrainDuplicateRules"] = packet.Budget.DuplicateRules
		if packet.Budget.InputChars > 0 {
			budget["projectBrainDeduplicationRatio"] = float64(packet.Budget.DeduplicatedChars) / float64(packet.Budget.InputChars)
		} else {
			budget["projectBrainDeduplicationRatio"] = float64(0)
		}
	}
}

func (e *Engine) inspectDependency(name, ecosystem string) (map[string]any, error) {
	if ecosystem == "" || ecosystem == "auto" {
		if strings.HasPrefix(name, "@") || exists(filepath.Join(e.Root, "node_modules", name)) {
			ecosystem = "node"
		} else if exists(filepath.Join(e.Root, "go.mod")) {
			ecosystem = "go"
		} else if exists(filepath.Join(e.Root, "Cargo.toml")) {
			ecosystem = "rust"
		} else if exists(filepath.Join(e.Root, "pyproject.toml")) {
			ecosystem = "python"
		}
	}
	switch ecosystem {
	case "node":
		path := filepath.Join(e.Root, "node_modules", filepath.FromSlash(name), "package.json")
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		var pkg map[string]any
		if json.Unmarshal(data, &pkg) != nil {
			return nil, errors.New("invalid dependency package.json")
		}
		return map[string]any{"ecosystem": "node", "name": name, "root": e.FS.Rel(filepath.Dir(path)), "manifest": pkg}, nil
	case "go":
		cmd := exec.Command("go", "list", "-m", "-json", name)
		cmd.Dir = e.Root
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		if err := cmd.Run(); err != nil {
			return map[string]any{"ecosystem": "go", "name": name, "installed": false, "error": strings.TrimSpace(stderr.String())}, nil
		}
		module := map[string]any{}
		if err := json.Unmarshal(stdout.Bytes(), &module); err != nil {
			return nil, err
		}
		return map[string]any{"ecosystem": "go", "name": name, "installed": true, "module": module}, nil
	case "rust":
		cmd := exec.Command("cargo", "metadata", "--format-version", "1", "--no-deps")
		cmd.Dir = e.Root
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		if err := cmd.Run(); err != nil {
			return map[string]any{"ecosystem": "rust", "name": name, "installed": false}, nil
		}
		metadata := map[string]any{}
		if err := json.Unmarshal(stdout.Bytes(), &metadata); err != nil {
			return nil, err
		}
		var found any
		if packages, ok := metadata["packages"].([]any); ok {
			for _, raw := range packages {
				pkg := object(raw)
				if asString(pkg["name"]) == name {
					found = pkg
					break
				}
			}
		}
		return map[string]any{"ecosystem": "rust", "name": name, "installed": found != nil, "package": found}, nil
	case "python":
		python, err := exec.LookPath("python3")
		if err != nil {
			python, err = exec.LookPath("python")
		}
		if err != nil {
			return map[string]any{"ecosystem": "python", "name": name, "found": false, "reason": "python executable not found"}, nil
		}
		script := `import importlib.util,json,sys; s=importlib.util.find_spec(sys.argv[1]); print(json.dumps({"found": bool(s), "origin": getattr(s,"origin",None), "locations": list(getattr(s,"submodule_search_locations",[]) or [])}))`
		cmd := exec.Command(python, "-c", script, name)
		cmd.Dir = e.Root
		output, err := cmd.Output()
		if err != nil {
			return map[string]any{"ecosystem": "python", "name": name, "found": false}, nil
		}
		result := map[string]any{"ecosystem": "python", "name": name}
		_ = json.Unmarshal(output, &result)
		result["ecosystem"] = "python"
		result["name"] = name
		return result, nil
	default:
		return nil, fmt.Errorf("dependency ecosystem not available for %s", name)
	}
}
func exists(path string) bool { _, err := os.Stat(path); return err == nil }

func (e *Engine) readDependency(args map[string]any) (map[string]any, error) {
	name := asString(args["name"])
	path := asString(args["path"])
	if path == "" {
		path = "package.json"
	}
	root := filepath.Join(e.Root, "node_modules", filepath.FromSlash(name))
	candidate := filepath.Join(root, filepath.FromSlash(path))
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return nil, errors.New("dependency path escape")
	}
	return e.FS.Read(e.FS.Rel(candidate), asInt(args["startLine"], 0), asInt(args["endLine"], 0))
}
func (e *Engine) searchDependency(args map[string]any) (map[string]any, error) {
	name := asString(args["name"])
	root := filepath.Join(e.Root, "node_modules", filepath.FromSlash(name))
	return e.FS.Search(asString(args["query"]), e.FS.Rel(root), asInt(args["maxResults"], 100), asBool(args["fixedStrings"], false), true)
}

func fileEditArgs(args map[string]any) ([]editing.FileEdit, error) {
	raw, ok := args["files"].([]any)
	if !ok {
		return nil, errors.New("files must be an array")
	}
	out := []editing.FileEdit{}
	for _, item := range raw {
		m := object(item)
		file := editing.FileEdit{Path: asString(m["path"]), ExpectedHash: asString(m["expectedHash"])}
		editsRaw, _ := m["edits"].([]any)
		for _, editRaw := range editsRaw {
			em := object(editRaw)
			ed := editing.Edit{Replacement: asString(em["replacement"])}
			for key, target := range map[string]**int{"startOffset": &ed.StartOffset, "endOffset": &ed.EndOffset, "startLine": &ed.StartLine, "startColumn": &ed.StartColumn, "endLine": &ed.EndLine, "endColumn": &ed.EndColumn} {
				if value, exists := em[key]; exists {
					v := asInt(value, 0)
					*target = &v
				}
			}
			file.Edits = append(file.Edits, ed)
		}
		out = append(out, file)
	}
	return out, nil
}

func stableProjectID(snapshot projectidentity.Snapshot) string {
	return projectidentity.StableProjectID(snapshot)
}

func learnedSkillBranchPolicy(taskKind string) string {
	switch strings.ToLower(strings.TrimSpace(taskKind)) {
	case "code", "coding", "bugfix", "feature", "refactor", "review", "build", "test", "migration", "deployment", "deploy":
		return "exact"
	default:
		return "any"
	}
}

func (e *Engine) availableLearnedSkillCapabilities() []string {
	capabilities := []string{"filesystem", "git", "lsp", "mcp", "browser", "computer"}
	if e != nil && e.ShellEnabled {
		capabilities = append(capabilities, "shell")
	}
	sort.Strings(capabilities)
	return capabilities
}

func learnedSkillRequirements(steps []learnedskills.Step) []string {
	seen := map[string]struct{}{}
	for _, step := range steps {
		tool := strings.ToLower(strings.TrimSpace(step.Tool))
		capability := ""
		switch tool {
		case "terminal", "run_command", "exec_start", "pty_start", "shell":
			capability = "shell"
		case "git", "git_stage", "git_commit", "git_push", "git_diff", "git_status":
			capability = "git"
		case "browser":
			capability = "browser"
		case "computer":
			capability = "computer"
		case "mcp", "mcp_call":
			capability = "mcp"
		case "read", "edit", "write_file", "edit_file", "apply_patch", "apply_edits":
			capability = "filesystem"
		}
		if capability != "" {
			seen[capability] = struct{}{}
		}
	}
	out := make([]string, 0, len(seen))
	for capability := range seen {
		out = append(out, capability)
	}
	sort.Strings(out)
	return out
}

func (e *Engine) LearnedSkillContext(taskKind string) *learnedskills.ContextFingerprint {
	return e.currentLearnedSkillContext(taskKind)
}

func (e *Engine) currentLearnedSkillContext(taskKind string) *learnedskills.ContextFingerprint {
	if e == nil || e.FS == nil || e.Project == nil {
		return nil
	}
	projectMap, err := e.Project.Map(false)
	if err != nil {
		return nil
	}
	manifest := projectbrain.FromProjectMap(projectMap)
	identity := projectidentity.Discover(e.Root, e.WorkspaceName)
	repositoryIDs := make([]string, 0, len(identity.Repositories))
	for _, repository := range identity.Repositories {
		if id := strings.TrimSpace(repository.ID); id != "" {
			repositoryIDs = append(repositoryIDs, id)
		}
	}
	sort.Strings(repositoryIDs)
	workflowFiles := map[string]string{}
	for _, candidate := range []string{"package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.mod", "go.sum", "Cargo.toml", "Cargo.lock", "Makefile", "Dockerfile", "railway.toml"} {
		info, infoErr := e.FS.FileInfo(candidate)
		if infoErr != nil {
			continue
		}
		if hash := strings.TrimSpace(asString(info["hash"])); hash != "" {
			workflowFiles[candidate] = hash
		}
	}
	workflowKeys := make([]string, 0, len(workflowFiles))
	for key := range workflowFiles {
		workflowKeys = append(workflowKeys, key)
	}
	sort.Strings(workflowKeys)
	dependencyParts := make([]string, 0, len(workflowKeys)*2)
	for _, key := range workflowKeys {
		dependencyParts = append(dependencyParts, key, workflowFiles[key])
	}
	dependencyHash := ""
	if len(dependencyParts) > 0 {
		sum := sha256.Sum256([]byte(strings.Join(dependencyParts, "\x00")))
		dependencyHash = fmt.Sprintf("%x", sum[:])
	}
	_, branchFingerprint, _ := e.repositoryBranchState()
	return &learnedskills.ContextFingerprint{
		ProjectID: stableProjectID(identity), RepositoryIDs: repositoryIDs, RulesHash: manifest.RootHash,
		DependencyHash: dependencyHash, WorkflowFiles: workflowFiles, BranchPolicy: learnedSkillBranchPolicy(taskKind), Branch: branchFingerprint,
		RequiredCapabilities: e.availableLearnedSkillCapabilities(),
	}
}

func learnedSkillSteps(value any) ([]learnedskills.Step, error) {
	raw, ok := value.([]any)
	if !ok {
		if typed, ok := value.([]learnedskills.Step); ok {
			return typed, nil
		}
		return nil, errors.New("learned skill steps must be an array")
	}
	steps := make([]learnedskills.Step, 0, len(raw))
	for index, item := range raw {
		entry := object(item)
		tool := strings.TrimSpace(asString(entry["tool"]))
		if tool == "" {
			return nil, fmt.Errorf("learned skill step %d requires tool", index+1)
		}
		args := object(entry["args"])
		steps = append(steps, learnedskills.Step{Tool: tool, Args: args})
	}
	return steps, nil
}

func (e *Engine) Handle(ctx context.Context, tool string, args map[string]any, opts HandleOptions) (any, error) {
	key := strings.TrimSpace(opts.IdempotencyKey)
	if !protocol.SideEffecting(tool) || key == "" {
		return e.handle(ctx, tool, args, opts)
	}
	existing, err := e.Journal.Get(key)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		switch existing.Status {
		case idempotency.Completed:
			return existing.Result, nil
		case idempotency.Started:
			return nil, fmt.Errorf("duplicate side-effecting request is already or ambiguously started: %s", key)
		}
	}
	if _, err := e.Journal.Start(key, tool); err != nil {
		return nil, err
	}
	result, handleErr := e.handle(ctx, tool, args, opts)
	if handleErr != nil {
		_ = e.Journal.Fail(key, handleErr.Error())
		return nil, handleErr
	}
	if state, ok := result.(map[string]any); ok {
		status := asString(state["status"])
		if status == "approval_required" || status == "blocked" {
			// This request has not performed its side effect yet. In particular,
			// never persist the short-lived approval token in the journal.
			_ = e.Journal.Abandon(key)
			return result, nil
		}
	}
	if err := e.Journal.Complete(key, result); err != nil {
		return nil, err
	}
	return result, nil
}

func (e *Engine) handle(ctx context.Context, tool string, args map[string]any, opts HandleOptions) (any, error) {
	if args == nil {
		args = map[string]any{}
	}
	audit.Write(audit.Event{Event: "tool.call", RequestID: opts.RequestID, MCPSessionID: opts.SessionID, WorkspaceKey: e.WorkspaceKey, Tool: tool, Detail: args})
	if result, handled, err := e.handleCodeIntelligence(ctx, tool, args); handled {
		return result, err
	}
	switch tool {
	case "learned_skill_list":
		recipes, err := e.Skills.List(e.WorkspaceKey, asInt(args["limit"], 20))
		if err != nil {
			return nil, err
		}
		items := make([]map[string]any, 0, len(recipes))
		for _, recipe := range recipes {
			items = append(items, map[string]any{
				"id": recipe.ID, "intent": recipe.Intent, "taskKind": recipe.TaskKind,
				"status": recipe.Status, "confidence": recipe.Confidence,
				"successCount": recipe.SuccessCount, "failureCount": recipe.FailureCount,
				"createdAt": recipe.CreatedAt, "updatedAt": recipe.UpdatedAt, "lastUsedAt": recipe.LastUsedAt,
				"contextHash": recipe.ContextHash, "staleReason": recipe.StaleReason,
				"stepCount": len(recipe.Steps), "source": "local",
			})
		}
		return map[string]any{"skills": items, "source": "local"}, nil
	case "learned_skill_match":
		taskKind := asString(args["taskKind"])
		recipe, err := e.Skills.MatchWithContext(e.WorkspaceKey, asString(args["intent"]), taskKind, e.currentLearnedSkillContext(taskKind))
		return map[string]any{"match": recipe}, err
	case "learned_skill_record":
		steps, err := learnedSkillSteps(args["steps"])
		if err != nil {
			return nil, err
		}
		taskKind := asString(args["taskKind"])
		contextFingerprint := e.currentLearnedSkillContext(taskKind)
		if contextFingerprint != nil {
			// Persist requirements, not the full set of currently available
			// capabilities. Matching interprets this field as requirements ⊆
			// current capabilities, so gaining a capability never stales a skill.
			contextFingerprint.RequiredCapabilities = learnedSkillRequirements(steps)
		}
		recipe, err := e.Skills.RecordWithContext(e.WorkspaceKey, asString(args["intent"]), taskKind, steps, asBool(args["verified"], false), contextFingerprint)
		return map[string]any{"recipe": recipe}, err
	case "learned_skill_feedback":
		recipe, err := e.Skills.Feedback(e.WorkspaceKey, asString(args["id"]), asBool(args["success"], false))
		return map[string]any{"recipe": recipe}, err
	case "project_info":
		projectMap, _ := e.Project.Map(false)
		instructions, _ := e.readInstructions(".")
		branch, _, branches := e.repositoryBranchState()
		return map[string]any{"protocolVersion": protocol.Version, "projectRoot": e.Root, "projectName": e.WorkspaceName, "deviceId": e.DeviceID, "workspaceId": e.WorkspaceID, "workspaceKey": e.WorkspaceKey, "project": compactProjectContext(projectMap), "instructions": instructions["instructionFiles"], "semantic": e.Project.SemanticInfo(), "executionSecurity": map[string]any{"platform": security.Platform(), "backend": "host-policy", "mode": "policy-only", "available": true, "networkMode": networkPolicy(), "notes": []string{"commands execute on the host after deterministic local policy checks", "Smart mode auto-approves only deterministic rememberable actions for this locally enabled workspace", "Full access auto-approves all non-blocked workspace actions while deterministic hard blocks remain enforced", "workspace access mode is stored locally by workspace and survives MCP/session reconnects; remembered prompt approvals remain MCP-session and TTL scoped", "prompt and smart modes still require fresh confirmation for critical actions", "explicit paths outside the authorized workspace and credential retrieval are blocked"}}, "shellEnabled": e.ShellEnabled, "approvalMode": approval.UserMode(approval.ResolveMode(e.WorkspaceID)), "terminalApproval": "chat-mediated", "approvalMemory": "local-workspace-session-ttl-scoped", "networkPolicy": networkPolicy(), "gitBranch": branch, "gitBranches": branches, "version": version.Version, "recommendedWorkflow": map[string]any{"codingTask": []string{"Call context_for_task with the user's concrete task before broad repository scans.", "Use ranked files, semantic/LSP symbols, graph neighbors and symbol-centered snippets as the initial context packet.", "Follow with exact definitions/references/callers/callees or targeted line reads only when the packet is insufficient.", "Use search_code primarily for literal strings, config keys, logs and unknown text.", "After edits, run verify_changes and the smallest relevant checks."}, "rationale": "Semantic-first retrieval reduces irrelevant context and preserves code relationships before ChatGPT reads larger source ranges."}, "capabilities": []string{fmt.Sprintf("protocol-v%d", protocol.Version), "gitignore-aware-retrieval", "sensitive-path-policy", "polyglot-semantic-router", "lsp", "context-engine", "transactional-edits", "process-manager-v2", "cancellation", "idempotency", "host-policy-execution", "structured-command-policy", "approval-memory", "git-write-approval", "terminal-chat-approval", "terminal-history", "mcp-hub", "learned-skills", "audit"}}, nil
	case "approval_mode":
		requested := strings.TrimSpace(asString(args["mode"]))
		if requested != "" {
			mode, ok := approval.ParseUserMode(requested)
			if !ok {
				return nil, errors.New("mode must be one of: prompt, smart, full")
			}
			if err := approval.SetWorkspaceMode(e.WorkspaceID, mode); err != nil {
				return nil, err
			}
		}
		mode := approval.ResolveMode(e.WorkspaceID)
		e.ApprovalMode = string(mode)
		return map[string]any{
			"mode": approval.UserMode(mode), "label": approval.UserModeLabel(mode), "scope": "workspace",
			"persistsAcrossSessions": true,
			"choices":                approval.UserModeChoices(),
		}, nil
	case "project_map":
		return e.Project.Map(asBool(args["force"], false))
	case "context_for_task":
		taskHint := asString(args["taskHint"])
		result, err := e.Project.ContextForTask(ctx, taskHint, asInt(args["limit"], 30))
		if err != nil {
			return nil, err
		}
		e.attachProjectBrainContext(result, stringSlice(args["targets"]), taskHint)
		if projectMap, ok := result["project"].(map[string]any); ok {
			result["project"] = compactProjectContextForRoute(projectMap, result["repositoryRoute"])
		}
		return result, nil
	case "read_instructions":
		return e.readInstructions(defaultString(asString(args["path"]), "."))
	case "list_files":
		return e.FS.List(defaultString(asString(args["path"]), "."), asInt(args["maxDepth"], 4), asBool(args["includeIgnored"], false))
	case "file_info":
		return e.FS.FileInfo(asString(args["path"]))
	case "read_file":
		return e.taskReadFile(ctx, args, opts, asString(args["path"]), 0, 0)
	case "read_file_range":
		return e.taskReadFile(ctx, args, opts, asString(args["path"]), asInt(args["startLine"], 1), asInt(args["endLine"], 1))
	case "read_files":
		return e.taskReadMany(ctx, args, opts, stringSlice(args["paths"]))
	case "search_code":
		return e.FS.Search(asString(args["query"]), defaultString(asString(args["path"]), "."), asInt(args["maxResults"], 200), asBool(args["fixedStrings"], false), asBool(args["includeIgnored"], false))
	case "inspect_dependency":
		return e.inspectDependency(asString(args["name"]), asString(args["ecosystem"]))
	case "read_dependency":
		return e.readDependency(args)
	case "search_dependency":
		return e.searchDependency(args)
	case "write_file":
		result, err := e.taskWriteFile(ctx, args, opts, asString(args["path"]), asString(args["content"]), asString(args["expectedHash"]))
		if err == nil && asString(args[privateTaskExecutionID]) == "" {
			e.Project.Invalidate()
		}
		return result, err
	case "edit_file":
		result, err := e.taskExactEdit(ctx, args, opts, asString(args["path"]), asString(args["oldText"]), asString(args["newText"]), asBool(args["replaceAll"], false), asString(args["expectedHash"]))
		if err == nil && asString(args[privateTaskExecutionID]) == "" {
			e.Project.Invalidate()
		}
		return result, err
	case "apply_patch":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskApplyPatch(ctx, args, opts, asString(args["patch"]))
		}
		result, err := e.Editing.ApplyPatch(asString(args["patch"]))
		if err == nil {
			e.Project.Invalidate()
		}
		return result, err
	case "apply_edits":
		files, err := fileEditArgs(args)
		if err != nil {
			return nil, err
		}
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskApplyEdits(ctx, args, opts, files)
		}
		result, err := e.Editing.Apply(files)
		if err == nil {
			e.Project.Invalidate()
		}
		return result, err
	case "format_changed_files":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskFormatFiles(ctx, args, opts, stringSlice(args["paths"]))
		}
		result, err := e.formatFiles(ctx, stringSlice(args["paths"]))
		if err == nil {
			e.Project.Invalidate()
		}
		return result, err
	case "snapshot_diagnostics":
		return e.snapshotDiagnostics(ctx, stringSlice(args["paths"]))
	case "verify_changes":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskVerifyChanges(ctx, args, opts, stringSlice(args["paths"]), asString(args["baselineId"]))
		}
		return e.verifyChanges(ctx, stringSlice(args["paths"]), asString(args["baselineId"]))
	case "git_status":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitStatus(args)
		}
		return e.gitStatus(asString(args["repository"]))
	case "git_diff":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitDiff(ctx, args, opts)
		}
		return e.gitDiff(asString(args["repository"]), asString(args["path"]), asBool(args["cached"], false))
	case "git_log":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitRead(ctx, "log", args, opts)
		}
		n := asInt(args["limit"], 20)
		if n < 1 {
			n = 1
		}
		if n > 100 {
			n = 100
		}
		path := asString(args["path"])
		repo, repoPath, err := e.gitTarget(asString(args["repository"]), path)
		if err != nil {
			return nil, err
		}
		gitArgs := []string{"log", "-" + strconv.Itoa(n), "--date=iso", "--pretty=format:%h%x09%ad%x09%an%x09%s"}
		if path != "" {
			gitArgs = append(gitArgs, "--", repoPath)
		}
		result, err := runGit(repo.Root, gitArgs...)
		return withRepository(result, repo), err
	case "git_show":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitRead(ctx, "show", args, opts)
		}
		repo, _, err := e.gitTarget(asString(args["repository"]), "")
		if err != nil {
			return nil, err
		}
		result, err := runGit(repo.Root, "show", "--stat", "--oneline", "--decorate", defaultString(asString(args["ref"]), "HEAD"))
		return withRepository(result, repo), err
	case "git_blame":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitRead(ctx, "blame", args, opts)
		}
		path := asString(args["path"])
		repo, repoPath, err := e.gitTarget(asString(args["repository"]), path)
		if err != nil {
			return nil, err
		}
		gitArgs := []string{"blame", "--line-porcelain"}
		if start := asInt(args["startLine"], 0); start > 0 {
			end := asInt(args["endLine"], start)
			gitArgs = append(gitArgs, "-L", fmt.Sprintf("%d,%d", start, end))
		}
		gitArgs = append(gitArgs, "--", repoPath)
		result, err := runGit(repo.Root, gitArgs...)
		return withRepository(result, repo), err
	case "git_file_history":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitRead(ctx, "file_history", args, opts)
		}
		path := asString(args["path"])
		repo, repoPath, err := e.gitTarget(asString(args["repository"]), path)
		if err != nil {
			return nil, err
		}
		n := asInt(args["limit"], 30)
		if n < 1 {
			n = 1
		}
		if n > 100 {
			n = 100
		}
		result, err := runGit(repo.Root, "log", "--follow", "-"+strconv.Itoa(n), "--date=iso", "--pretty=format:%h%x09%ad%x09%an%x09%s", "--", repoPath)
		return withRepository(result, repo), err
	case "git_stage":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitStage(ctx, args, opts, false)
		}
		paths := stringSlice(args["paths"])
		if len(paths) == 0 {
			return nil, errors.New("git_stage requires at least one path")
		}
		repo, repoPaths, err := e.gitPathsTarget(asString(args["repository"]), paths)
		if err != nil {
			return nil, err
		}
		gitArgs := append([]string{"add", "--"}, repoPaths...)
		result, err := e.guardedGitAt(repo.Root, gitArgs, asString(args["approvalToken"]), opts.SessionID)
		return withRepository(result, repo), err
	case "git_unstage":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitStage(ctx, args, opts, true)
		}
		paths := stringSlice(args["paths"])
		if len(paths) == 0 {
			return nil, errors.New("git_unstage requires at least one path")
		}
		repo, repoPaths, err := e.gitPathsTarget(asString(args["repository"]), paths)
		if err != nil {
			return nil, err
		}
		gitArgs := append([]string{"restore", "--staged", "--"}, repoPaths...)
		result, err := e.guardedGitAt(repo.Root, gitArgs, asString(args["approvalToken"]), opts.SessionID)
		return withRepository(result, repo), err
	case "git_commit":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitCommit(ctx, args, opts)
		}
		repo, _, err := e.gitTarget(asString(args["repository"]), "")
		if err != nil {
			return nil, err
		}
		staged, err := runGit(repo.Root, "diff", "--cached", "--name-only")
		if err != nil {
			return nil, err
		}
		stagedPaths := strings.Fields(asString(staged["stdout"]))
		if len(stagedPaths) == 0 {
			return nil, errors.New("no staged changes to commit in selected repository")
		}
		expected := stringSlice(args["expectedPaths"])
		if len(expected) > 0 {
			allowed := map[string]struct{}{}
			for _, path := range expected {
				expectedRepo, repoPath, resolveErr := e.gitTarget(asString(args["repository"]), path)
				if resolveErr != nil {
					return nil, resolveErr
				}
				if expectedRepo.ID != repo.ID || expectedRepo.RelativePath != repo.RelativePath {
					return nil, errors.New("expectedPaths span repositories outside the selected commit target")
				}
				allowed[filepath.ToSlash(repoPath)] = struct{}{}
			}
			unexpected := []string{}
			for _, path := range stagedPaths {
				path = filepath.ToSlash(path)
				if _, ok := allowed[path]; !ok {
					unexpected = append(unexpected, path)
				}
			}
			if len(unexpected) > 0 {
				return nil, fmt.Errorf("unexpected staged changes: %s", strings.Join(unexpected, ", "))
			}
		}
		result, err := e.guardedGitAt(repo.Root, []string{"commit", "-m", asString(args["message"])}, asString(args["approvalToken"]), opts.SessionID)
		return withRepository(result, repo), err
	case "git_push":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskGitPush(ctx, args, opts)
		}
		if asBool(args["force"], false) {
			return map[string]any{"status": "blocked", "riskLevel": "BLOCKED", "reason": "force push is blocked by CodeLocal"}, nil
		}
		repo, _, err := e.gitTarget(asString(args["repository"]), "")
		if err != nil {
			return nil, err
		}
		gitArgs := []string{"push"}
		if remote := asString(args["remote"]); remote != "" {
			gitArgs = append(gitArgs, remote)
		}
		if branch := asString(args["branch"]); branch != "" {
			gitArgs = append(gitArgs, branch)
		}
		result, err := e.guardedGitAt(repo.Root, gitArgs, asString(args["approvalToken"]), opts.SessionID)
		return withRepository(result, repo), err
	case "sandbox_info":
		return map[string]any{"platform": security.Platform(), "backend": "host-policy", "mode": "policy-only", "available": true, "networkMode": networkPolicy()}, nil
	case "sandbox_smoke_test":
		return map[string]any{"ok": true, "backend": "host-policy", "mode": "policy-only"}, nil
	case "terminal_preflight":
		if asString(args[privateTaskExecutionID]) != "" {
			return e.taskPreflight(args, opts)
		}
		return e.PreflightScoped(asString(args["command"]), defaultString(asString(args["cwd"]), "."), opts.SessionID)
	case "terminal_history":
		return e.History.Query(e.WorkspaceKey, asString(args["query"]), defaultString(asString(args["event"]), "started"), asInt(args["limit"], 50))
	case "run_command":
		return e.runCommand(ctx, args, opts)
	case "artifact_publish":
		return e.publishArtifactMarker(asString(args["path"]))
	case "exec_start":
		return e.startProcess(asString(args["command"]), args, opts, false)
	case "pty_start":
		return e.startProcess(asString(args["command"]), args, opts, true)
	case "exec_poll", "pty_poll":
		stdout := int64(asInt(args["stdoutCursor"], 0))
		stderr := int64(asInt(args["stderrCursor"], 0))
		snap, err := e.Processes.Snapshot(asString(args["processId"]), &stdout, &stderr)
		return snapshotMap(snap), err
	case "process_poll":
		cursor := int64(asInt(args["cursor"], 0))
		snap, err := e.Processes.Snapshot(asString(args["processId"]), &cursor, &cursor)
		return snapshotMap(snap), err
	case "exec_write", "pty_write", "process_write":
		return e.Processes.Write(asString(args["processId"]), asString(args["input"]))
	case "pty_resize":
		return e.Processes.Resize(asString(args["processId"]), asInt(args["cols"], 120), asInt(args["rows"], 36))
	case "exec_signal", "pty_signal", "exec_kill", "pty_kill", "process_kill":
		return e.Processes.Signal(asString(args["processId"]), defaultString(asString(args["signal"]), "SIGTERM"))
	case "exec_cancel":
		return e.Processes.Cancel(asString(args["processId"]), defaultString(asString(args["reason"]), "cancelled by ChatGPT"))
	case "process_list":
		return map[string]any{"processes": e.Processes.List()}, nil
	case "approval_list":
		items, err := e.Approvals.List(e.WorkspaceKey)
		return map[string]any{"approvals": items}, err
	case "approval_revoke":
		id := first(asString(args["id"]), asString(args["actionKey"]))
		if id == "" {
			return nil, errors.New("approval_revoke requires id or actionKey")
		}
		count, err := e.Approvals.Revoke(id, e.WorkspaceKey)
		return map[string]any{"removed": count}, err
	case "approval_reset":
		count, err := e.Approvals.Reset(e.WorkspaceKey)
		return map[string]any{"removed": count}, err
	case "mcp_list":
		servers, err := e.MCP.List()
		return map[string]any{"servers": servers}, err
	case "mcp_configure":
		return e.configureMCP(ctx, args)
	case "mcp_remove":
		return e.removeMCP(args)
	case "plugin_mcp_configure":
		return e.configurePluginMCP(ctx, args)
	case "plugin_mcp_remove":
		return e.removePluginMCP(args)
	case "mcp_search_tools":
		return e.MCP.Search(ctx, asString(args["query"]), asString(args["server"]), asInt(args["limit"], 8), asBool(args["refresh"], false))
	case "mcp_tool_info":
		return e.MCP.ToolInfo(ctx, asString(args["server"]), asString(args["tool"]), false)
	case "mcp_call":
		return e.callMCP(ctx, args, opts.SessionID)
	default:
		return nil, fmt.Errorf("unsupported CodeLocal tool: %s", tool)
	}
}

func defaultString(value, def string) string {
	if strings.TrimSpace(value) == "" {
		return def
	}
	return value
}
func first(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
func symbolAt(e *Engine, path string, line, column int) string {
	read, err := e.FS.Read(path, line, line)
	if err != nil {
		return filepath.Base(path)
	}
	text := asString(read["content"])
	if len(text) == 0 {
		return ""
	}
	index := column - 1
	if index < 0 {
		index = 0
	}
	if index >= len(text) {
		index = len(text) - 1
	}
	start := index
	for start > 0 && symbolByte(text[start-1]) {
		start--
	}
	end := index
	for end < len(text) && symbolByte(text[end]) {
		end++
	}
	return text[start:end]
}

func symbolByte(value byte) bool {
	return value == '_' || value == '$' || value >= 'A' && value <= 'Z' || value >= 'a' && value <= 'z' || value >= '0' && value <= '9'
}

func (e *Engine) callMCP(ctx context.Context, args map[string]any, sessionID string) (any, error) {
	server := strings.TrimSpace(asString(args["server"]))
	tool := strings.TrimSpace(asString(args["tool"]))
	if server == "" || tool == "" {
		return nil, errors.New("mcp_call requires server and tool")
	}

	// External MCP annotations such as readOnlyHint are advisory. Starting an
	// MCP server can itself execute code, so one fresh ChatGPT approval covers
	// both connection (if needed) and this exact tool invocation.
	command := "mcp " + server + "." + tool
	argumentJSON, err := json.Marshal(object(args["arguments"]))
	if err != nil || len(argumentJSON) > 64<<10 {
		return nil, errors.New("MCP arguments are invalid or oversized")
	}
	argumentHash := sha256.Sum256(argumentJSON)
	decision := security.Decision{
		RiskLevel:        security.RiskReview,
		MatchedRules:     []string{"mcp:" + server + ":" + tool, fmt.Sprintf("arguments:%x", argumentHash)},
		RequiresApproval: true,
		Blocked:          false,
		RedactedCommand:  command,
		Reason:           "external MCP call may execute local or remote side effects; read-only annotations are advisory",
		ApprovalPolicy:   security.ApprovalAlways,
	}
	approved, state, _, authErr := e.authorizeDecision(command, e.Root, asString(args["approvalToken"]), sessionID, decision)
	if authErr != nil {
		return state, nil
	}
	if !approved {
		return state, nil
	}

	info, err := e.MCP.ToolInfo(ctx, server, tool, true)
	if err != nil {
		return nil, err
	}
	audit.Write(audit.Event{Event: "mcp.chat_approved", WorkspaceKey: e.WorkspaceKey, Tool: server + "." + tool, RiskLevel: string(decision.RiskLevel), Status: "approved"})
	result, err := e.MCP.Call(ctx, server, info.Name, object(args["arguments"]), true)
	if err != nil {
		audit.Write(audit.Event{Event: "mcp.call", WorkspaceKey: e.WorkspaceKey, Tool: server + "." + tool, Status: "failed", Detail: err.Error()})
		return nil, err
	}
	audit.Write(audit.Event{Event: "mcp.call", WorkspaceKey: e.WorkspaceKey, Tool: server + "." + tool, Status: "ok"})
	return result, nil
}

func (e *Engine) formatFiles(ctx context.Context, paths []string) (map[string]any, error) {
	formatted := []string{}
	skipped := []string{}
	seen := map[string]struct{}{}
	for _, requested := range paths {
		if _, ok := seen[requested]; ok || strings.TrimSpace(requested) == "" {
			continue
		}
		seen[requested] = struct{}{}
		absolute, err := e.FS.Existing(requested)
		if err != nil {
			skipped = append(skipped, requested)
			continue
		}
		relative := e.FS.Rel(absolute)
		_, executionRoot, executionPath, _ := e.repositoryExecutionPath(relative)
		ext := strings.ToLower(filepath.Ext(relative))
		command := ""
		args := []string{}
		switch ext {
		case ".go":
			command, args = "gofmt", []string{"-w", executionPath}
		case ".rs":
			command, args = "rustfmt", []string{executionPath}
		case ".dart":
			command, args = "dart", []string{"format", executionPath}
		case ".py":
			command, args = "ruff", []string{"format", executionPath}
		case ".c", ".cc", ".cpp", ".cxx", ".h", ".hpp":
			command, args = "clang-format", []string{"-i", executionPath}
		case ".js", ".jsx", ".ts", ".tsx", ".json", ".css", ".scss", ".md", ".yaml", ".yml":
			prettier := filepath.Join(executionRoot, "node_modules", ".bin", executableName("prettier"))
			biome := filepath.Join(executionRoot, "node_modules", ".bin", executableName("biome"))
			if isExecutableFile(prettier) {
				command, args = prettier, []string{"--write", executionPath}
			} else if isExecutableFile(biome) {
				command, args = biome, []string{"format", "--write", executionPath}
			}
		}
		if command == "" {
			skipped = append(skipped, relative)
			continue
		}
		if !filepath.IsAbs(command) {
			if _, err := exec.LookPath(command); err != nil {
				skipped = append(skipped, relative)
				continue
			}
		}
		cmd := exec.CommandContext(ctx, command, args...)
		cmd.Dir = executionRoot
		cmd.Env = append(os.Environ(), "CI=1")
		if err := cmd.Run(); err != nil {
			skipped = append(skipped, relative)
			continue
		}
		formatted = append(formatted, relative)
	}
	return map[string]any{"formatted": formatted, "skipped": skipped}, nil
}

func executableName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".cmd"
	}
	return name
}

func isExecutableFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}
func (e *Engine) snapshotDiagnostics(ctx context.Context, paths []string) (map[string]any, error) {
	result, err := e.Project.Diagnostics(ctx, "", 500)
	if err != nil {
		return nil, err
	}
	raw, _ := result["diagnostics"].([]map[string]any)
	id := fmt.Sprintf("%d", time.Now().UnixNano())
	e.mu.Lock()
	e.baselines[id] = raw
	e.mu.Unlock()
	return map[string]any{"baselineId": id, "diagnostics": raw, "paths": paths}, nil
}
func (e *Engine) verifyChanges(ctx context.Context, paths []string, baselineID string) (map[string]any, error) {
	result, err := e.Project.Diagnostics(ctx, "", 500)
	if err != nil {
		return nil, err
	}
	current, _ := result["diagnostics"].([]map[string]any)
	e.mu.Lock()
	baseline := e.baselines[baselineID]
	e.mu.Unlock()

	effectivePaths := append([]string(nil), paths...)
	if len(effectivePaths) == 0 {
		effectivePaths = e.changedPathsFromGitStatus()
	}
	diff, err := e.verificationGitDiff(effectivePaths)
	if err != nil {
		return nil, err
	}

	projectMap, _ := e.Project.Map(false)
	verificationPlan := verificationPlanForChanges(projectMap, effectivePaths)
	checks := recommendedChecksForChanges(projectMap, effectivePaths)
	checkRuns := recommendedCheckRunsForChanges(projectMap, effectivePaths)
	if len(checks) > 8 {
		checks = checks[:8]
	}
	quality := codequality.Analyze(e.Root, effectivePaths)
	return map[string]any{
		"baselineId":           baselineID,
		"beforeDiagnostics":    baseline,
		"diagnostics":          current,
		"diagnosticRegression": len(current) - len(baseline),
		"verificationScope":    effectivePaths,
		"verificationPlan":     verificationPlan,
		"recommendedChecks":    checks,
		"recommendedCheckRuns": checkRuns,
		"gitDiff":              diff,
		"qualityPolicy":        quality,
	}, nil
}

var nonWord = regexp.MustCompile(`[^A-Za-z0-9_$]+`)

func words(text string) []string { return nonWord.Split(text, -1) }

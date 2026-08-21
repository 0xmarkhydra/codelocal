package localclient

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/approval"
	"github.com/0xmarkhydra/codelocal/internal/learnedskills"
	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
	"github.com/0xmarkhydra/codelocal/internal/protocol"
	"github.com/0xmarkhydra/codelocal/internal/security"
)

func newTestEngine(t *testing.T) *Engine {
	t.Helper()
	root := filepath.Join(t.TempDir(), "workspace")
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("CODELOCAL_STATE_DIR", filepath.Join(t.TempDir(), "state"))
	engine, err := New(root, "workspace-test", "Workspace Test", "device::workspace-test", "device")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(engine.Close)
	return engine
}

func TestShellDisabledBlocksTerminalTools(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_SHELL", "0")
	engine := newTestEngine(t)
	if engine.ShellEnabled {
		t.Fatal("shell should be disabled")
	}
	preflight, err := engine.Preflight("echo hello", ".")
	if err != nil {
		t.Fatal(err)
	}
	if preflight["status"] != "blocked" {
		t.Fatalf("preflight=%#v", preflight)
	}
	if _, err := engine.Handle(context.Background(), "exec_start", map[string]any{"command": "echo hello"}, HandleOptions{RequestID: "r1"}); err == nil {
		t.Fatal("exec_start should fail when shell is disabled")
	}
}

func TestAgentModeAutoApprovesRememberableButNotCriticalRuntimeActions(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_SHELL", "1")
	t.Setenv("CODELOCAL_APPROVAL_MODE", "")
	engine := newTestEngine(t)
	if err := approval.SetWorkspaceMode(engine.WorkspaceID, approval.ModeAgent); err != nil {
		t.Fatal(err)
	}

	routine := security.Decision{
		RiskLevel: security.RiskReview, RequiresApproval: true,
		ApprovalPolicy: security.ApprovalRememberable, ApprovalKey: "workspace-exec:test", ApprovalLabel: "test",
		MatchedRules: []string{"workspace code execution"}, RedactedCommand: "go test ./...", Reason: "workspace code execution",
	}
	approved, state, _, err := engine.authorizeDecision("go test ./...", engine.Root, "", "session-a", routine)
	if err != nil || !approved || state["agentApproved"] != true {
		t.Fatalf("routine action should be approved by agent mode: approved=%v state=%#v err=%v", approved, state, err)
	}

	critical := routine
	critical.RiskLevel = security.RiskCritical
	critical.ApprovalPolicy = security.ApprovalAlways
	critical.ApprovalKey = ""
	critical.Reason = "destructive action"
	approved, state, _, err = engine.authorizeDecision("dangerous action", engine.Root, "", "session-a", critical)
	if err != nil || approved || state["status"] != "approval_required" || state["approvalPolicy"] != security.ApprovalAlways {
		t.Fatalf("critical action must still require fresh approval: approved=%v state=%#v err=%v", approved, state, err)
	}
	access, ok := state["workspaceAccess"].(map[string]any)
	choices, choicesOK := access["choices"].([]approval.UserModeChoice)
	if !ok || !choicesOK || access["currentMode"] != "smart" || len(choices) != 3 {
		t.Fatalf("approval-required response must offer the three chat access choices: %#v", state["workspaceAccess"])
	}
}

func TestWorkspaceAccessModeCanBeSetInChatAndSurvivesSessionChanges(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_SHELL", "1")
	t.Setenv("CODELOCAL_APPROVAL_MODE", "")
	engine := newTestEngine(t)

	result, err := engine.Handle(context.Background(), "approval_mode", map[string]any{"mode": "full"}, HandleOptions{RequestID: "access-full", SessionID: "session-a"})
	if err != nil {
		t.Fatal(err)
	}
	state, ok := result.(map[string]any)
	if !ok || state["mode"] != "full" || state["label"] != "Toàn quyền truy cập" || state["persistsAcrossSessions"] != true {
		t.Fatalf("unexpected access mode state: %#v", result)
	}

	critical := security.Decision{
		RiskLevel: security.RiskCritical, RequiresApproval: true, ApprovalPolicy: security.ApprovalAlways,
		MatchedRules: []string{"critical test"}, RedactedCommand: "critical test", Reason: "critical test",
	}
	approved, approvalState, _, err := engine.authorizeDecision("critical test", engine.Root, "", "session-b", critical)
	if err != nil || !approved || approvalState["fullAccessApproved"] != true || approvalState["approvalMode"] != "full" {
		t.Fatalf("full access should survive MCP session changes: approved=%v state=%#v err=%v", approved, approvalState, err)
	}

	blocked := critical
	blocked.RiskLevel = security.RiskBlocked
	blocked.Blocked = true
	blocked.RequiresApproval = false
	blocked.ApprovalPolicy = security.ApprovalBlocked
	approved, blockedState, _, err := engine.authorizeDecision("blocked test", engine.Root, "", "session-c", blocked)
	if err == nil || approved || blockedState["status"] != "blocked" {
		t.Fatalf("full access must preserve hard security blocks: approved=%v state=%#v err=%v", approved, blockedState, err)
	}
}

func TestMCPCallAlwaysRequiresFreshChatApproval(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_SHELL", "1")
	engine := newTestEngine(t)
	options := HandleOptions{RequestID: "mcp-1", SessionID: "s1", IdempotencyKey: "mcp-idem"}
	result, err := engine.Handle(context.Background(), "mcp_call", map[string]any{"server": "external", "tool": "do_thing", "arguments": map[string]any{}}, options)
	if err != nil {
		t.Fatal(err)
	}
	state, ok := result.(map[string]any)
	if !ok || asString(state["status"]) != "approval_required" || asString(state["approvalPolicy"]) != "always" {
		t.Fatalf("unexpected MCP approval state: %#v", result)
	}
	firstToken, _ := state["approvalToken"].(string)
	if firstToken == "" {
		t.Fatal("approval token missing")
	}
	options.RequestID = "mcp-2"
	retry, err := engine.Handle(context.Background(), "mcp_call", map[string]any{"server": "external", "tool": "do_thing", "arguments": map[string]any{}}, options)
	if err != nil {
		t.Fatal(err)
	}
	retryState, ok := retry.(map[string]any)
	if !ok || retryState["status"] != "approval_required" {
		t.Fatalf("approval-required request was incorrectly journaled: %#v", retry)
	}
	if retryState["approvalToken"] != firstToken {
		t.Fatal("same-session retry rotated the pending approval token before it was consumed")
	}

	otherSession := options
	otherSession.RequestID = "mcp-3"
	otherSession.SessionID = "s2"
	otherSession.IdempotencyKey = "mcp-idem-s2"
	other, err := engine.Handle(context.Background(), "mcp_call", map[string]any{"server": "external", "tool": "do_thing", "arguments": map[string]any{}}, otherSession)
	if err != nil {
		t.Fatal(err)
	}
	otherState, ok := other.(map[string]any)
	if !ok || otherState["approvalToken"] == firstToken {
		t.Fatal("pending approval token leaked across ChatGPT sessions")
	}
}

func TestSideEffectingToolIdempotencyReusesCompletedResult(t *testing.T) {
	t.Setenv("CODELOCAL_ALLOW_SHELL", "1")
	engine := newTestEngine(t)
	path := filepath.Join(engine.Root, "example.txt")
	if err := os.WriteFile(path, []byte("first"), 0o644); err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	options := HandleOptions{RequestID: "r1", SessionID: "s1", IdempotencyKey: "idem-1"}
	if _, err := engine.Handle(ctx, "edit_file", map[string]any{"path": "example.txt", "oldText": "first", "newText": "second"}, options); err != nil {
		t.Fatal(err)
	}
	options.RequestID = "r2"
	if _, err := engine.Handle(ctx, "edit_file", map[string]any{"path": "example.txt", "oldText": "second", "newText": "third"}, options); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "second" {
		t.Fatalf("idempotent retry changed file: %q", data)
	}
}

func TestLearnedSkillPrivateRuntimeOperations(t *testing.T) {
	engine := newTestEngine(t)
	steps := []any{
		map[string]any{"tool": "terminal", "args": map[string]any{"action": "run", "command": "xcrun simctl launch booted vn.infivision.biddi.dev"}},
		map[string]any{"tool": "computer", "args": map[string]any{"action": "observe"}},
	}
	recorded, err := engine.Handle(context.Background(), "learned_skill_record", map[string]any{
		"intent": "mở BIDDI Beta", "taskKind": "desktop", "steps": steps, "verified": true,
	}, HandleOptions{RequestID: "skill-record"})
	if err != nil {
		t.Fatal(err)
	}
	recordState, ok := recorded.(map[string]any)
	if !ok || recordState["recipe"] == nil {
		t.Fatalf("unexpected learned skill record result: %#v", recorded)
	}

	matched, err := engine.Handle(context.Background(), "learned_skill_match", map[string]any{
		"intent": "mở app BIDDI Beta", "taskKind": "desktop",
	}, HandleOptions{RequestID: "skill-match"})
	if err != nil {
		t.Fatal(err)
	}
	matchState, ok := matched.(map[string]any)
	if !ok || matchState["match"] == nil {
		t.Fatalf("expected private learned skill match, got %#v", matched)
	}

	listed, err := engine.Handle(context.Background(), "learned_skill_list", map[string]any{"limit": 20}, HandleOptions{RequestID: "skill-list"})
	if err != nil {
		t.Fatal(err)
	}
	listState, ok := listed.(map[string]any)
	if !ok || listState["source"] != "local" {
		t.Fatalf("unexpected learned skill list result: %#v", listed)
	}
	items, ok := listState["skills"].([]map[string]any)
	if !ok || len(items) != 1 {
		t.Fatalf("expected one learned skill metadata item, got %#v", listState["skills"])
	}
	if items[0]["intent"] != "mở BIDDI Beta" || items[0]["stepCount"] != 2 || items[0]["source"] != "local" {
		t.Fatalf("unexpected learned skill metadata: %#v", items[0])
	}
	if items[0]["steps"] != nil {
		t.Fatalf("workspace learned-skill inspection must not expose replay step bodies: %#v", items[0])
	}
}

func TestContextForTaskIncludesBoundedProjectBrainRules(t *testing.T) {
	engine := newTestEngine(t)
	if err := os.MkdirAll(filepath.Join(engine.Root, "backend"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(engine.Root, "AGENTS.md"), []byte("Use gofmt before reporting completion.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(engine.Root, "CLAUDE.md"), []byte("use   gofmt before reporting completion;\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(engine.Root, "backend", "AGENTS.md"), []byte("Use repository interfaces in backend services.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(engine.Root, "backend", "service.go"), []byte("package backend\n\nfunc PaymentService() string { return \"ok\" }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	result, err := engine.Handle(context.Background(), "context_for_task", map[string]any{"taskHint": "fix PaymentService backend behavior", "limit": 10}, HandleOptions{RequestID: "context-project-brain"})
	if err != nil {
		t.Fatal(err)
	}
	packetMap, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("context_for_task returned %T", result)
	}
	projectSummary, ok := packetMap["project"].(map[string]any)
	if !ok {
		t.Fatalf("compact project summary missing: %#v", packetMap["project"])
	}
	for _, forbidden := range []string{"manifests", "modules", "knowledgeSources", "lockfiles", "instructionFiles"} {
		if _, exists := projectSummary[forbidden]; exists {
			t.Fatalf("context_for_task leaked bulky project field %q: %#v", forbidden, projectSummary[forbidden])
		}
	}
	brain, ok := packetMap["projectBrain"].(projectbrain.ContextPacket)
	if !ok {
		t.Fatalf("projectBrain packet = %T %#v", packetMap["projectBrain"], packetMap["projectBrain"])
	}
	joined := ""
	for _, rule := range brain.EffectiveRules {
		joined += rule.Text + "\n"
		if !strings.Contains(rule.Trust, "cannot grant execution permission") {
			t.Fatalf("rule trust boundary missing: %#v", rule)
		}
	}
	if !strings.Contains(joined, "Use gofmt") || !strings.Contains(joined, "repository interfaces") {
		t.Fatalf("expected root+nested rules in compiled context, got %q", joined)
	}
	if strings.Count(strings.ToLower(joined), "gofmt before reporting completion") != 1 || brain.Budget.DuplicateRules != 1 || brain.Budget.DeduplicatedChars <= 0 {
		t.Fatalf("duplicate cross-provider rule was not compacted: brain=%#v joined=%q", brain.Budget, joined)
	}
	contextBudget, ok := packetMap["contextBudget"].(map[string]any)
	duplicates, duplicateOK := contextBudget["projectBrainDuplicateRules"].(int)
	deduplicated, deduplicatedOK := contextBudget["projectBrainDeduplicatedChars"].(int)
	if !ok || !duplicateOK || !deduplicatedOK || duplicates != 1 || deduplicated <= 0 {
		t.Fatalf("project brain savings were not projected into context budget: %#v", packetMap["contextBudget"])
	}
	if brain.Budget.UsedChars > brain.Budget.MaxChars || brain.Budget.MaxChars != 8000 || len(brain.Fingerprint) != 64 {
		t.Fatalf("unexpected Project Brain budget/fingerprint: %#v", brain)
	}
}

func TestCompactProjectContextForRouteHidesUnselectedRepositories(t *testing.T) {
	projectMap := map[string]any{
		"repositories": []any{
			map[string]any{"id": "web-id", "path": "web", "identitySource": "remote", "manifests": []any{"web/package.json"}, "modules": []any{"web/src"}},
			map[string]any{"id": "auth-id", "path": "backend/auth", "identitySource": "remote", "manifests": []any{"backend/auth/go.mod"}, "modules": []any{"backend/auth/internal"}},
			map[string]any{"id": "payment-id", "path": "backend/payment", "identitySource": "remote", "manifests": []any{"backend/payment/go.mod"}, "modules": []any{"backend/payment/internal"}},
		},
		"languages": []any{"go", "typescript"}, "frameworks": []any{}, "workspaceRoots": []any{"."}, "sourceRoots": []any{"web/src", "backend/auth/internal", "backend/payment/internal"}, "testRoots": []any{},
	}
	route := map[string]any{"mode": "focused", "selectedRepositories": []map[string]any{
		{"repositoryId": "web-id", "repositoryPath": "web"},
		{"repositoryId": "auth-id", "repositoryPath": "backend/auth"},
	}}

	compact := compactProjectContextForRoute(projectMap, route)
	repositories, ok := compact["repositories"].([]map[string]any)
	if !ok || len(repositories) != 2 {
		t.Fatalf("focused compact repositories = %#v", compact["repositories"])
	}
	for _, repo := range repositories {
		if repo["path"] == "backend/payment" {
			t.Fatalf("unselected repository leaked into model-facing project context: %#v", repositories)
		}
	}
	for _, raw := range compact["sourceRoots"].([]any) {
		if raw == "backend/payment/internal" {
			t.Fatalf("unselected repository source root leaked into compact context: %#v", compact["sourceRoots"])
		}
	}
	if compact["repositoryContextFocused"] != true {
		t.Fatalf("focused marker missing: %#v", compact)
	}
}

func TestContextForTaskExplicitTargetsRefreshNestedRules(t *testing.T) {
	engine := newTestEngine(t)
	if err := os.MkdirAll(filepath.Join(engine.Root, "backend", "payments"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(engine.Root, "backend", "payments", "AGENTS.md"), []byte("Always use payment transaction boundaries.\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(engine.Root, "backend", "payments", "service.go"), []byte("package payments\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	result, err := engine.Handle(context.Background(), "context_for_task", map[string]any{"taskHint": "inspect unrelated startup behavior", "targets": []any{"backend/payments/service.go"}, "limit": 3}, HandleOptions{RequestID: "explicit-target"})
	if err != nil {
		t.Fatal(err)
	}
	packet := result.(map[string]any)
	if packet["projectBrainTargetSource"] != "explicit" {
		t.Fatalf("target source=%#v", packet["projectBrainTargetSource"])
	}
	brain := packet["projectBrain"].(projectbrain.ContextPacket)
	joined := ""
	for _, rule := range brain.EffectiveRules {
		joined += rule.Text + "\n"
	}
	if !strings.Contains(joined, "payment transaction boundaries") {
		t.Fatalf("explicit target did not refresh nested rule: %q", joined)
	}
}

func TestLearnedSkillContextHelpersAreBranchAndCapabilityAware(t *testing.T) {
	if learnedSkillBranchPolicy("bugfix") != "exact" || learnedSkillBranchPolicy("browser") != "any" {
		t.Fatal("unexpected branch policy")
	}
	reqs := learnedSkillRequirements([]learnedskills.Step{{Tool: "terminal"}, {Tool: "computer"}, {Tool: "read"}})
	joined := strings.Join(reqs, ",")
	for _, want := range []string{"computer", "filesystem", "shell"} {
		if !strings.Contains(joined, want) {
			t.Fatalf("missing capability %s in %v", want, reqs)
		}
	}
}

func TestSymbolAtHandlesEmptyAndOutOfRangeColumns(t *testing.T) {
	engine := newTestEngine(t)
	if err := os.WriteFile(filepath.Join(engine.Root, "symbols.go"), []byte("package demo\n\nalphaBeta\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if got := symbolAt(engine, "symbols.go", 2, 6); got != "" {
		t.Fatalf("empty line symbol = %q, want empty", got)
	}
	if got := symbolAt(engine, "symbols.go", 3, 999); got != "alphaBeta" {
		t.Fatalf("out-of-range column symbol = %q, want alphaBeta", got)
	}
	if got := symbolAt(engine, "symbols.go", 3, 0); got != "alphaBeta" {
		t.Fatalf("zero column symbol = %q, want alphaBeta", got)
	}

	for _, tool := range []string{"find_definition", "find_references", "find_implementations"} {
		result, err := engine.Handle(context.Background(), tool, map[string]any{"path": "symbols.go", "line": 2, "column": 6, "limit": 20}, HandleOptions{RequestID: tool + "-empty-line"})
		if err != nil {
			t.Fatalf("%s on empty line returned error: %v", tool, err)
		}
		if result == nil {
			t.Fatalf("%s on empty line returned nil result", tool)
		}
	}
}

func TestProjectInfoReportsCurrentProtocolVersion(t *testing.T) {
	engine := newTestEngine(t)
	result, err := engine.Handle(context.Background(), "project_info", nil, HandleOptions{RequestID: "project-info"})
	if err != nil {
		t.Fatal(err)
	}
	info, ok := result.(map[string]any)
	if !ok {
		t.Fatalf("project_info returned %T, want map", result)
	}
	if got := asInt(info["protocolVersion"], 0); got != protocol.Version {
		t.Fatalf("project_info protocolVersion = %d, want %d", got, protocol.Version)
	}
	capabilities, ok := info["capabilities"].([]string)
	if !ok {
		t.Fatalf("project_info capabilities = %#v, want []string", info["capabilities"])
	}
	want := fmt.Sprintf("protocol-v%d", protocol.Version)
	for _, capability := range capabilities {
		if capability == want {
			return
		}
	}
	t.Fatalf("project_info capabilities = %#v, missing %q", capabilities, want)
}

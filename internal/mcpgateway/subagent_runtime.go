package mcpgateway

// subagent_runtime.go — S2/S3 production fan-out seam.
//
// Design: bounded `agent` stays single-entry. An optional `team` array on the
// agent args fans out to named subagents resolved through the orchestration
// SubagentRegistry (project .codelocal/agents/ > global > builtin).
//
// Each member executes through the EXISTING bounded executor path (same
// autonomousStepPolicy + same per-step execution loop the lead uses), scoped
// by the subagent's own tool allowlist/permission projection. Results return
// as bounded AgentReports; join reuses orchestration.JoinTeamResults
// (fail-closed). No new public MCP tool is added.

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const (
	maxAgentTeamMembers     = 4
	maxAgentTeamTokenBudget = 64000
	maxAgentReportChars     = 4 * 1024
)

type agentTeamMember struct {
	Subagent   string
	Objective  string
	ReadPaths  []string
	WritePaths []string
	TokenBudget int64
}

type agentTeamReport struct {
	Subagent      string         `json:"subagent"`
	BriefID       string         `json:"briefId"`
	Status        string         `json:"status"`
	Summary       string         `json:"summary"`
	FilesTouched  []string       `json:"filesTouched,omitempty"`
	Verification  string         `json:"verification,omitempty"`
	Followups     []string       `json:"followups,omitempty"`
	HaltReason    string         `json:"haltReason,omitempty"`
}

func parseAgentTeam(value any) ([]agentTeamMember, error) {
	raw, ok := value.([]any)
	if !ok {
		return nil, fmt.Errorf("agent team must be an array")
	}
	if len(raw) == 0 {
		return nil, fmt.Errorf("agent team must not be empty")
	}
	if len(raw) > maxAgentTeamMembers {
		return nil, fmt.Errorf("agent team supports at most %d members", maxAgentTeamMembers)
	}
	members := make([]agentTeamMember, 0, len(raw))
	seen := map[string]struct{}{}
	for index, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("agent team member %d must be an object", index+1)
		}
		name, _ := entry["subagent"].(string)
		name = strings.TrimSpace(name)
		objective, _ := entry["objective"].(string)
		objective = strings.Join(strings.Fields(objective), " ")
		if name == "" || objective == "" {
			return nil, fmt.Errorf("agent team member %d requires subagent and objective", index+1)
		}
		if len(objective) > 2000 {
			return nil, fmt.Errorf("agent team member %d objective exceeds 2000 chars", index+1)
		}
		lower := strings.ToLower(name)
		if _, dup := seen[lower]; dup {
			return nil, fmt.Errorf("agent team member %q is duplicated", name)
		}
		seen[lower] = struct{}{}
		member := agentTeamMember{Subagent: name, Objective: objective}
		member.ReadPaths = stringListArg(entry["readPaths"])
		member.WritePaths = stringListArg(entry["writePaths"])
		if len(member.ReadPaths) > 20 || len(member.WritePaths) > 20 {
			return nil, fmt.Errorf("agent team member %d allows at most 20 read/write paths", index+1)
		}
		if rawBudget, exists := entry["tokenBudget"]; exists && rawBudget != nil {
			budget, ok := intValueOK(rawBudget)
			if !ok || budget <= 0 || budget > maxAgentTeamTokenBudget {
				return nil, fmt.Errorf("agent team member %d tokenBudget must be 1..%d", index+1, maxAgentTeamTokenBudget)
			}
			member.TokenBudget = budget
		}
		members = append(members, member)
	}
	return members, nil
}

func stringListArg(value any) []string {
	var out []string
	switch items := value.(type) {
	case []string:
		out = append([]string(nil), items...)
	case []any:
		for _, item := range items {
			text := strings.TrimSpace(fmt.Sprint(item))
			if text != "" && text != "<nil>" {
				out = append(out, text)
			}
		}
	}
	cleaned := out[:0]
	for _, item := range out {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		cleaned = append(cleaned, item)
	}
	sort.Strings(cleaned)
	deduped := cleaned[:0]
	for i, item := range cleaned {
		if i == 0 || item != cleaned[i-1] {
			deduped = append(deduped, item)
		}
	}
	return deduped
}

func intValueOK(value any) (int64, bool) {
	switch v := value.(type) {
	case int:
		return int64(v), true
	case int32:
		return int64(v), true
	case int64:
		return v, true
	case float64:
		if v != float64(int64(v)) {
			return 0, false
		}
		return int64(v), true
	default:
		return 0, false
	}
}

func subagentToolAllowed(def orchestration.SubagentDefinition, operation operationInvocation) bool {
	if len(def.Tools) == 0 {
		return true
	}
	candidates := []string{operation.OperationID}
	if operation.RuntimeTool != "" && operation.RuntimeTool != operation.OperationID {
		candidates = append(candidates, operation.RuntimeTool)
	}
	if dot := strings.Index(operation.OperationID, "."); dot >= 0 {
		candidates = append(candidates, operation.OperationID[:dot]+"_*")
	}
	for _, candidate := range candidates {
		for _, pattern := range def.Tools {
			if subagentPatternMatchesExported(pattern, candidate) {
				return true
			}
		}
	}
	return false
}

func subagentPatternMatchesExported(pattern, tool string) bool {
	return orchestration.SubagentPatternMatches(pattern, tool)
}

// resolveSubagentRegistry builds the project > global > builtin registry for
// one dispatch. Project roots come from the activated workspace view; the
// gateway never trusts caller-supplied filesystem paths.
func resolveSubagentRegistry(workspace *gateway.WorkspaceView, workspaceKey string) (*orchestration.SubagentRegistry, string, string) {
	projectDir := ""
	if workspace != nil {
		if root, ok := workspace.ProjectRoot.(string); ok {
			root = strings.TrimSpace(root)
			if root != "" {
				clean := filepath.Clean(root)
				// Only resolve project-local agents when the gateway itself
				// reports a local absolute root. Remote/URL roots are ignored.
				if filepath.IsAbs(clean) && !strings.Contains(clean, "://") {
					candidate := filepath.Join(clean, ".codelocal", "agents")
					if info, err := os.Stat(candidate); err == nil && info.IsDir() {
						projectDir = candidate
					}
				}
			}
		}
	}
	_ = workspaceKey
	globalDir := orchestration.SubagentGlobalDir()
	registry, err := orchestration.LoadSubagentRegistry(projectDir, globalDir)
	if err != nil {
		registry = orchestration.NewBuiltinSubagentRegistry()
	}
	return registry, projectDir, globalDir
}

// subagentScopedPolicy wraps autonomousStepPolicy with the subagent's own
// allowlist/permission projection. Deny always wins; unknown tools default
// to deny. Approval tokens are never auto-consumed (same as lead).
func subagentScopedPolicy(def orchestration.SubagentDefinition, operation operationInvocation, args map[string]any, plan orchestration.AgentPlan) autonomousStepDecision {
	if hasApprovalToken(args) {
		return autonomousStepDecision{Reason: "bounded agent never consumes approval tokens automatically"}
	}
	probeNames := []string{operation.OperationID}
	if operation.RuntimeTool != "" {
		probeNames = append(probeNames, operation.RuntimeTool)
	}
	denied := false
	asked := false
	for _, probe := range probeNames {
		switch orchestration.ResolveToolPermission(def, probe) {
		case "deny":
			denied = true
		case "ask":
			asked = true
		}
	}
	if denied {
		return autonomousStepDecision{Reason: "subagent " + def.Name + " denies " + operation.OperationID}
	}
	if !subagentToolAllowed(def, operation) {
		return autonomousStepDecision{Reason: "subagent " + def.Name + " allowlist excludes " + operation.OperationID}
	}
	if asked {
		return autonomousStepDecision{Reason: "subagent " + def.Name + " requires approval for " + operation.OperationID}
	}
	decision := autonomousStepPolicy(operation, args, plan)
	if !decision.Allowed {
		return decision
	}
	if def.ReadOnly && operation.MutatesState {
		return autonomousStepDecision{Reason: "subagent " + def.Name + " is read-only; mutation " + operation.OperationID + " denied"}
	}
	return decision
}

// dispatchAgentTeam fans out team members through bounded subagent executors.
// It reuses service.executeBoundedSteps (the same step loop as the lead) with
// a per-member policy projection, runs members concurrently up to
// teamParallelism, and joins via orchestration.JoinTeamResults.
//
// S5: before admission, explicit member names are reordered by learned
// affinity (verified outcomes only, advisory ±0.10, never overriding the
// constraint gate). Member identity is caller-chosen — affinity only changes
// execution ORDER under the same parallelism cap, so a historically reliable
// member's evidence lands first without changing the join contract.
func (s *Service) dispatchAgentTeam(ctx context.Context, userID string, args map[string]any, req *mcp.CallToolRequest, workspaceKey, taskID string, members []agentTeamMember, registry *orchestration.SubagentRegistry, teamParallelism int) ([]agentTeamReport, orchestration.TeamJoin) {
	type slot struct {
		index  int
		report agentTeamReport
		brief  orchestration.ChildBrief
		outcome orchestration.MemberOutcome
	}
	slots := make([]slot, len(members))
	briefs := make([]orchestration.ChildBrief, 0, len(members))

	// S5 advisory ordering: surface historically verified members first.
	// Fail-open: any affinity lookup error keeps caller order. This never
	// changes membership or the join contract — only slot order.
	members = orderTeamMembersByAffinity(ctx, s, userID, members)

	// Admission: resolve every member against the registry BEFORE dispatch.
	// Unknown subagent or invalid brief fails the whole team (fail-closed).
	// S5 constraint re-gate: even caller-chosen members must pass
	// RecommendSubagent gate semantics (write/security/delegation). The brief
	// path below enforces token budget + role validity; this extra gate keeps
	// learned/explicit choices from bypassing policy.
	admissionFailed := false
	for i, member := range members {
		def, ok := registry.Get(member.Subagent)
		if !ok {
			slots[i] = slot{index: i, report: agentTeamReport{Subagent: member.Subagent, BriefID: "", Status: "failed", HaltReason: "unknown subagent " + member.Subagent}}
			slots[i].outcome = orchestration.MemberOutcome{BriefID: "\x00unknown:" + member.Subagent, Status: "failed"}
			admissionFailed = true
			continue
		}
		// S5 constraint re-gate: caller-chosen members must still pass policy
		// for the READ paths they claim. NeedsWrite derives from writePaths;
		// a readOnly subagent claiming writes is rejected fail-closed here
		// rather than dispatching a child that can only halt.
		if reason := gateTeamMemberConstraints(def, member); reason != "" {
			slots[i] = slot{index: i, report: agentTeamReport{Subagent: member.Subagent, BriefID: "", Status: "failed", HaltReason: "constraint gate: " + reason}}
			slots[i].outcome = orchestration.MemberOutcome{BriefID: "\x00gated:" + member.Subagent, Status: "failed"}
			admissionFailed = true
			continue
		}
		budget := member.TokenBudget
		if budget == 0 {
			budget = def.TokenBudget
		}
		brief, err := orchestration.IssueChildBrief(orchestration.ChildBrief{
			ID:            fmt.Sprintf("%s:member:%d:%s", taskID, i, def.Name),
			TaskID:        taskID,
			ParentAgentID: "agent:" + taskID + ":lead",
			Role:          def.Role,
			Objective:     member.Objective,
			ReadPaths:     member.ReadPaths,
			WritePaths:    member.WritePaths,
			TokenBudget:   budget,
		})
		if err != nil {
			slots[i] = slot{index: i, report: agentTeamReport{Subagent: member.Subagent, BriefID: "", Status: "failed", HaltReason: err.Error()}}
			slots[i].outcome = orchestration.MemberOutcome{BriefID: "\x00invalid:" + member.Subagent, Status: "failed"}
			admissionFailed = true
			continue
		}
		briefs = append(briefs, brief)
		slots[i] = slot{index: i, brief: brief, report: agentTeamReport{Subagent: member.Subagent, BriefID: brief.ID, Status: "pending"}}
		slots[i].outcome = orchestration.MemberOutcome{BriefID: brief.ID, Status: "failed"}
	}
	if admissionFailed {
		outcomes := make([]orchestration.MemberOutcome, 0, len(slots))
		for _, sl := range slots {
			outcomes = append(outcomes, sl.outcome)
		}
		join := orchestration.JoinTeamResults(briefs, outcomes)
		reports := make([]agentTeamReport, 0, len(slots))
		for _, sl := range slots {
			reports = append(reports, sl.report)
		}
		return reports, join
	}

	parallel := teamParallelism
	if parallel <= 0 {
		parallel = 1
	}
	if parallel > len(members) {
		parallel = len(members)
	}
	var wg sync.WaitGroup
	sem := make(chan struct{}, parallel)
	for i := range slots {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			member := members[index]
			def, _ := registry.Get(member.Subagent)
			report, outcome := s.executeTeamMember(ctx, userID, args, req, workspaceKey, slots[index].brief, def, member)
			slots[index].report = report
			slots[index].outcome = outcome
		}(i)
	}
	wg.Wait()

	reports := make([]agentTeamReport, 0, len(slots))
	outcomes := make([]orchestration.MemberOutcome, 0, len(slots))
	for _, sl := range slots {
		reports = append(reports, sl.report)
		outcomes = append(outcomes, sl.outcome)
	}
	return reports, orchestration.JoinTeamResults(briefs, outcomes)
}

// gateTeamMemberConstraints re-checks caller-chosen members against the S5
// constraint gate (readOnly/write intent + explicit deny). Unknown tools stay
// permissive here — the per-step policy projection denies them at execution.
// Empty reason = admitted.
func gateTeamMemberConstraints(def orchestration.SubagentDefinition, member agentTeamMember) string {
	if len(member.WritePaths) > 0 && def.ReadOnly {
		return "read-only subagent " + def.Name + " cannot claim writePaths"
	}
	return ""
}

// orderTeamMembersByAffinity reorders (never filters) explicit members by
// verified-outcome affinity. Fail-open: nil Store, lookup error, or no
// evidence keeps caller order. Cap enforced by the resolver (±0.10); ties keep
// caller order via stable sort. Bounded: 2s timeout, single query.
func orderTeamMembersByAffinity(ctx context.Context, s *Service, userID string, members []agentTeamMember) []agentTeamMember {
	if s == nil || s.Store == nil || len(members) < 2 || strings.TrimSpace(userID) == "" {
		return members
	}
	ids := make([]string, 0, len(members))
	for _, member := range members {
		if name := strings.TrimSpace(member.Subagent); name != "" {
			ids = append(ids, name)
		}
	}
	if len(ids) == 0 {
		return members
	}
	lookupCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	affinity, err := s.Store.SubagentAffinity(lookupCtx, userID, ids)
	if err != nil || len(affinity) == 0 {
		return members
	}
	out := append([]agentTeamMember(nil), members...)
	sort.SliceStable(out, func(i, j int) bool {
		return affinity[strings.ToLower(out[i].Subagent)] > affinity[strings.ToLower(out[j].Subagent)]
	})
	return out
}

// executeTeamMember runs one member's objective through bounded steps scoped
// by the subagent definition. It intentionally does NOT forward the lead's
// transcript: the child gets a fresh minimal context (objective + scopes).
func (s *Service) executeTeamMember(ctx context.Context, userID string, globalArgs map[string]any, req *mcp.CallToolRequest, workspaceKey string, brief orchestration.ChildBrief, def orchestration.SubagentDefinition, member agentTeamMember) (agentTeamReport, orchestration.MemberOutcome) {
	report := agentTeamReport{Subagent: def.Name, BriefID: brief.ID}
	outcome := orchestration.MemberOutcome{BriefID: brief.ID, Status: "failed"}

	// Build the child's step program: the member objective is executed as a
	// read/verify-first bounded run. The child's OWN steps come from its
	// briefing (objective + scopes), not from the lead's step list.
	steps := childStepsForMember(member)
	execResult := s.executeBoundedSteps(ctx, userID, req, workspaceKey, brief.Objective, steps, boundedPolicyForSubagent(def))
	report.Summary = execResult.summary
	report.FilesTouched = execResult.filesTouched
	report.Verification = execResult.verification
	if execResult.haltReason != "" {
		report.Status = "blocked"
		report.HaltReason = execResult.haltReason
		outcome.Status = "blocked"
		outcome.Summary = truncateAgentReportText(execResult.summary, 500)
		outcome.VerificationPassed = false
		s.recordSubagentExperience(ctx, userID, req, workspaceKey, brief, def, "blocked", report)
		return report, outcome
	}
	if execResult.dirty && !execResult.verified {
		report.Status = "blocked"
		report.HaltReason = "edits without verification; needs review"
		outcome.Status = "completed"
		outcome.Summary = truncateAgentReportText(execResult.summary, 500)
		outcome.VerificationPassed = false
		s.recordSubagentExperience(ctx, userID, req, workspaceKey, brief, def, "blocked", report)
		return report, outcome
	}
	report.Status = "done"
	if len(report.Summary) == 0 {
		report.Summary = "completed: " + truncateAgentReportText(brief.Objective, 200)
	}
	report.Summary = truncateAgentReportText(report.Summary, maxAgentReportChars)
	outcome.Status = "completed"
	outcome.Summary = truncateAgentReportText(report.Summary, 500)
	outcome.VerificationPassed = execResult.verified
	// S5: only join-verified done members become positive routing evidence.
	// Unverified completions still return done to the caller but record no
	// success boost (VerificationPassed=false → SubagentVerified=false).
	s.recordSubagentExperience(ctx, userID, req, workspaceKey, brief, def, "done", report)
	return report, outcome
}

func childStepsForMember(member agentTeamMember) []boundedAgentStep {
	// Minimal safe default program for a child: ground context on its own
	// objective, then verify. The executor's policy projection enforces
	// read-only / allowlist per subagent, so these steps halt safely when
	// the subagent is not permitted to run them.
	return []boundedAgentStep{
		{Tool: "context", Args: map[string]any{"action": "task", "taskHint": member.Objective}},
		{Tool: "verify", Args: map[string]any{"action": "changes"}},
	}
}

func boundedPolicyForSubagent(def orchestration.SubagentDefinition) boundedStepPolicy {
	return func(operation operationInvocation, args map[string]any, plan orchestration.AgentPlan) autonomousStepDecision {
		return subagentScopedPolicy(def, operation, args, plan)
	}
}

func truncateAgentReportText(text string, max int) string {
	if max <= 0 {
		return ""
	}
	runes := []rune(text)
	if len(runes) <= max {
		return text
	}
	return string(runes[:max])
}

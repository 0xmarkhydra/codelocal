package mcpgateway

import (
	"context"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
)

func testCtx() context.Context { return context.Background() }

func TestParseAgentTeamValidatesShape(t *testing.T) {
	members, err := parseAgentTeam([]any{
		map[string]any{"subagent": "explorer", "objective": "survey auth flow", "readPaths": []any{"internal/auth"}},
		map[string]any{"subagent": "reviewer", "objective": "review diff", "tokenBudget": float64(8000)},
	})
	if err != nil || len(members) != 2 {
		t.Fatalf("valid team rejected: members=%v err=%v", members, err)
	}
	if members[1].TokenBudget != 8000 {
		t.Fatalf("tokenBudget=%d", members[1].TokenBudget)
	}
	for _, bad := range []any{
		[]any{},
		[]any{map[string]any{"subagent": "", "objective": "x"}},
		[]any{map[string]any{"subagent": "explorer", "objective": ""}},
		[]any{map[string]any{"subagent": "a", "objective": "x"}, map[string]any{"subagent": "a", "objective": "y"}},
		[]any{map[string]any{"subagent": "explorer", "objective": "x", "tokenBudget": float64(999999)}},
		"not-an-array",
	} {
		if _, err := parseAgentTeam(bad); err == nil {
			t.Fatalf("bad team accepted: %#v", bad)
		}
	}
	var tooMany []any
	for i := 0; i < maxAgentTeamMembers+1; i++ {
		tooMany = append(tooMany, map[string]any{"subagent": "m", "objective": "x"})
	}
	_ = tooMany
	oversized := []any{}
	for i := 0; i < maxAgentTeamMembers+1; i++ {
		oversized = append(oversized, map[string]any{"subagent": string(rune('a' + i)), "objective": "x"})
	}
	if _, err := parseAgentTeam(oversized); err == nil {
		t.Fatalf("oversized team accepted")
	}
}

func TestSubagentScopedPolicyDeniesOutsideAllowlist(t *testing.T) {
	def, err := orchestration.ParseSubagentContent("explorer.md", `---
name: explorer
description: Read-only survey.
mode: subagent
role: investigator
tools:
  - read_*
  - search_*
permission:
  edit_*: deny
---
Prompt.
`, orchestration.SubagentSourceGlobal)
	if err != nil {
		t.Fatal(err)
	}
	plan := orchestration.AgentPlan{}
	readOp := mustRuntimeOperation(t, "read_file")
	if decision := subagentScopedPolicy(def, readOp, map[string]any{"path": "x.go"}, plan); !decision.Allowed {
		t.Fatalf("read should be allowed: %#v", decision)
	}
	editOp := mustRuntimeOperation(t, "edit_file")
	if decision := subagentScopedPolicy(def, editOp, map[string]any{"path": "x.go", "oldText": "a", "newText": "b", "expectedHash": "h"}, plan); decision.Allowed {
		t.Fatalf("edit must be denied for explorer: %#v", decision)
	}
	if decision := subagentScopedPolicy(def, readOp, map[string]any{"path": "x.go", "approvalToken": "tok"}, plan); decision.Allowed {
		t.Fatalf("approval token must never be consumed: %#v", decision)
	}
}

func TestSubagentScopedPolicyEnforcesReadOnly(t *testing.T) {
	def, err := orchestration.ParseSubagentContent("writer.md", `---
name: writer
description: Narrow writer.
mode: subagent
role: implementer
tools:
  - read_*
  - edit_*
---
Prompt.
`, orchestration.SubagentSourceGlobal)
	if err != nil {
		t.Fatal(err)
	}
	def.ReadOnly = true // simulate narrowed read-only override at runtime
	plan := orchestration.AgentPlan{}
	editOp := mustRuntimeOperation(t, "edit_file")
	if decision := subagentScopedPolicy(def, editOp, map[string]any{"path": "x.go", "oldText": "a", "newText": "b", "expectedHash": "h"}, plan); decision.Allowed {
		t.Fatalf("read-only subagent must deny mutation: %#v", decision)
	}
}

func TestDispatchAgentTeamUnknownSubagentFailsClosed(t *testing.T) {
	registry := orchestration.NewBuiltinSubagentRegistry()
	service := &Service{}
	members := []agentTeamMember{{Subagent: "ghost", Objective: "do things"}}
	_, join := service.dispatchAgentTeam(testCtx(), "u", map[string]any{}, nil, "ws", "task-team-test", members, registry, 2)
	if join.Verdict != orchestration.JoinFailed {
		t.Fatalf("unknown subagent must fail join: %+v", join)
	}
	if len(join.Unknown) == 0 && len(join.Failed) == 0 {
		t.Fatalf("unknown member must surface: %+v", join)
	}
}

func TestGateTeamMemberConstraintsRejectsReadOnlyWriteClaim(t *testing.T) {
	def, err := orchestration.ParseSubagentContent("explorer.md", `---
name: explorer
description: Read-only survey.
mode: subagent
role: investigator
tools:
  - read_*
  - search_*
permission:
  edit_*: deny
---
Prompt.
`, orchestration.SubagentSourceGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if reason := gateTeamMemberConstraints(def, agentTeamMember{Subagent: "explorer", Objective: "survey", WritePaths: []string{"a.go"}}); reason == "" {
		t.Fatal("read-only member claiming writes must be gated")
	}
	if reason := gateTeamMemberConstraints(def, agentTeamMember{Subagent: "explorer", Objective: "survey", ReadPaths: []string{"a.go"}}); reason != "" {
		t.Fatalf("read-only read work must pass the gate: %q", reason)
	}
}

func TestOrderTeamMembersByAffinityKeepsCallerOrderWithoutStore(t *testing.T) {
	service := &Service{}
	members := []agentTeamMember{{Subagent: "b", Objective: "y"}, {Subagent: "a", Objective: "x"}}
	ordered := orderTeamMembersByAffinity(testCtx(), service, "u", members)
	if len(ordered) != 2 || ordered[0].Subagent != "b" || ordered[1].Subagent != "a" {
		t.Fatalf("fail-open ordering must keep caller order: %+v", ordered)
	}
}

func TestValidateAgentReportContract(t *testing.T) {
	valid := orchestration.AgentReport{BriefID: "b1", Status: "done", Summary: "ok"}
	if err := orchestration.ValidateAgentReport(valid); err != nil {
		t.Fatal(err)
	}
	if err := orchestration.ValidateAgentReport(orchestration.AgentReport{BriefID: "b1", Status: "maybe"}); err == nil {
		t.Fatal("bad status accepted")
	}
	big := orchestration.AgentReport{BriefID: "b1", Status: "done", Summary: strings.Repeat("w ", 501)}
	if err := orchestration.ValidateAgentReport(big); err == nil {
		t.Fatal("oversize summary accepted")
	}
}

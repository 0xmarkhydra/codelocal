package agentruntime

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/usage"
)

func TestRankEnginesRejectsUnsafeMutationAndUsesHistory(t *testing.T) {
	probes := []ProbeResult{
		{EngineID: "unsafe", Installed: true, Compatible: true, Auth: AuthAuthenticated, Capabilities: Capabilities{FileEditing: true, StructuredOutput: true, Isolation: IsolationUnknown}},
		{EngineID: "fast", Installed: true, Compatible: true, Auth: AuthAuthenticated, Capabilities: Capabilities{FileEditing: true, ShellExecution: true, StructuredOutput: true, Isolation: IsolationMediated}},
		{EngineID: "strong", Installed: true, Compatible: true, Auth: AuthAuthenticated, Capabilities: Capabilities{FileEditing: true, ShellExecution: true, StructuredOutput: true, PlanMode: true, Isolation: IsolationSandboxed}},
	}
	ranked := RankEngines(probes, RouteRequest{Mode: ModeMutate, EngineProfile: "coding", History: []EngineHistory{
		{EngineID: "fast", Quality: .7, HistoricSuccess: .8, TokenEfficiency: .9, Reliability: .9, CostEfficiency: .9, LatencyScore: .9},
		{EngineID: "strong", Quality: .95, HistoricSuccess: .9, TokenEfficiency: .3, Reliability: .9, CostEfficiency: .2, LatencyScore: .3},
	}})
	if len(ranked) != 2 {
		t.Fatalf("unsafe mutation engine was not filtered: %+v", ranked)
	}
	if ranked[0].EngineID != "fast" {
		t.Fatalf("expected evidence-based fast engine first, got %+v", ranked)
	}
}

func TestRankEnginesBudgetPressureFavorsEfficiency(t *testing.T) {
	probes := []ProbeResult{
		{EngineID: "cheap", Installed: true, Compatible: true, Auth: AuthAuthenticated, Capabilities: Capabilities{NonInteractive: true, StructuredOutput: true, Isolation: IsolationMediated}},
		{EngineID: "expensive", Installed: true, Compatible: true, Auth: AuthAuthenticated, Capabilities: Capabilities{NonInteractive: true, StructuredOutput: true, Isolation: IsolationMediated}},
	}
	history := []EngineHistory{
		{EngineID: "cheap", Quality: .7, HistoricSuccess: .8, TokenEfficiency: .95, Reliability: .9, CostEfficiency: .95, LatencyScore: .8},
		{EngineID: "expensive", Quality: .95, HistoricSuccess: .9, TokenEfficiency: .15, Reliability: .9, CostEfficiency: .1, LatencyScore: .8},
	}
	ranked := RankEngines(probes, RouteRequest{Mode: ModeReview, EngineProfile: "fast", Budget: usage.BudgetDecision{State: usage.BudgetSoft}, History: history})
	if len(ranked) != 2 || ranked[0].EngineID != "cheap" {
		t.Fatalf("budget pressure did not favor efficient engine: %+v", ranked)
	}
}

func TestRankEnginesRequiresRequestedCapabilities(t *testing.T) {
	probes := []ProbeResult{
		{EngineID: "no-resume", Installed: true, Compatible: true, Auth: AuthAuthenticated, Capabilities: Capabilities{StructuredOutput: true, Isolation: IsolationMediated}},
		{EngineID: "resume", Installed: true, Compatible: true, Auth: AuthAuthenticated, Capabilities: Capabilities{StructuredOutput: true, Resume: true, MCP: true, Isolation: IsolationMediated}},
	}
	ranked := RankEngines(probes, RouteRequest{RequireResume: true, RequireMCP: true})
	if len(ranked) != 1 || ranked[0].EngineID != "resume" {
		t.Fatalf("required capabilities were not enforced: %+v", ranked)
	}
}

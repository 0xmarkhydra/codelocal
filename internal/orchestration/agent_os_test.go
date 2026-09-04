package orchestration

import (
	"context"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/agentruntime"
	"github.com/0xmarkhydra/codelocal/internal/usage"
)

type coordinatorAdapter struct {
	id   string
	caps agentruntime.Capabilities
}

func (a coordinatorAdapter) ID() string          { return a.id }
func (a coordinatorAdapter) DisplayName() string { return a.id }
func (a coordinatorAdapter) Probe(context.Context) agentruntime.ProbeResult {
	return agentruntime.ProbeResult{EngineID: a.id, Installed: true, Compatible: true, Auth: agentruntime.AuthAuthenticated, Capabilities: a.caps}
}
func (a coordinatorAdapter) Capabilities() agentruntime.Capabilities { return a.caps }
func (a coordinatorAdapter) Start(context.Context, agentruntime.Request) (agentruntime.Session, error) {
	return nil, nil
}

func coordinatorRegistry(t *testing.T) *agentruntime.Registry {
	registry := agentruntime.NewRegistry()
	err := registry.Register(coordinatorAdapter{id: "auto-test", caps: agentruntime.Capabilities{NonInteractive: true, StructuredOutput: true, Resume: true, FileEditing: true, ShellExecution: true, MCP: true, PlanMode: true, ReviewMode: true, Isolation: agentruntime.IsolationMediated}})
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestPrepareAgentOSKeepsFocusedTaskSingleAgent(t *testing.T) {
	prepared, err := PrepareAgentOS(context.Background(), nil, coordinatorRegistry(t), AgentOSRequest{WorkspaceKey: "ws", TaskID: "task", Objective: "fix one auth bug", Shape: TaskShape{TaskKind: "fix", FilesEstimated: 1}, Budget: usage.Budget{MaxTotalTokens: 100000}})
	if err != nil {
		t.Fatal(err)
	}
	if len(prepared.Agents) != 1 || len(prepared.Team.Members) != 1 || prepared.Team.Parallelism != 1 {
		t.Fatalf("focused task over-orchestrated: %+v", prepared.Team)
	}
	if prepared.Kernel.Snapshot().Stage != KernelUnderstand {
		t.Fatalf("kernel not initialized: %+v", prepared.Kernel.Snapshot())
	}
	if len(prepared.Runtime.DAG().Nodes()) != 1 {
		t.Fatalf("DAG not compiled: %+v", prepared.Runtime.DAG().Nodes())
	}
}

func TestPrepareAgentOSCompilesComplexTeamIntoDurableGraphDAG(t *testing.T) {
	prepared, err := PrepareAgentOS(context.Background(), nil, coordinatorRegistry(t), AgentOSRequest{WorkspaceKey: "ws", TaskID: "complex", Objective: "migrate auth architecture safely", Shape: TaskShape{TaskKind: "architecture migration", FilesEstimated: 15, Subsystems: 5, SecuritySensitive: true, NeedsProductVerification: true}, Budget: usage.Budget{MaxTotalTokens: 500000}})
	if err != nil {
		t.Fatal(err)
	}
	if prepared.Team.Tier != TaskTier4 || len(prepared.Agents) < 5 {
		t.Fatalf("complex plan missing team: %+v", prepared.Team)
	}
	if len(prepared.Runtime.Graph().Descendants(prepared.LeadAgentID)) != len(prepared.Agents)-1 {
		t.Fatalf("graph topology mismatch: agents=%d descendants=%d", len(prepared.Agents), len(prepared.Runtime.Graph().Descendants(prepared.LeadAgentID)))
	}
	if len(prepared.Runtime.DAG().Nodes()) != len(prepared.Agents) {
		t.Fatalf("DAG/member mismatch")
	}
	if prepared.Usage.Snapshot().OpenReservations != len(prepared.Team.Members) {
		t.Fatalf("budget admission missing: %+v", prepared.Usage.Snapshot())
	}
}

func TestPrepareAgentOSFailsBeforeDispatchWhenNoSafeEngine(t *testing.T) {
	registry := agentruntime.NewRegistry()
	_ = registry.Register(coordinatorAdapter{id: "unsafe", caps: agentruntime.Capabilities{FileEditing: true, Isolation: agentruntime.IsolationUnknown}})
	_, err := PrepareAgentOS(context.Background(), nil, registry, AgentOSRequest{WorkspaceKey: "ws", TaskID: "task", Objective: "change code", Shape: TaskShape{TaskKind: "implement", FilesEstimated: 2}, Budget: usage.Budget{MaxTotalTokens: 100000}})
	if err == nil {
		t.Fatal("unsafe engine should not be admitted")
	}
}

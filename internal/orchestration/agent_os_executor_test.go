package orchestration

import (
	"context"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/agentruntime"
	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
	"github.com/0xmarkhydra/codelocal/internal/usage"
)

type runnerAdapter struct{}

func (runnerAdapter) ID() string          { return "runner-fake" }
func (runnerAdapter) DisplayName() string { return "Runner Fake" }
func (runnerAdapter) Capabilities() agentruntime.Capabilities {
	return agentruntime.Capabilities{NonInteractive: true, StructuredOutput: true, FileEditing: true, ShellExecution: true, MCP: true, PlanMode: true, ReviewMode: true, Transport: agentruntime.TransportNativeStructured, Isolation: agentruntime.IsolationMediated}
}
func (a runnerAdapter) Probe(context.Context) agentruntime.ProbeResult {
	return agentruntime.ProbeResult{EngineID: a.ID(), DisplayName: a.DisplayName(), Installed: true, Compatible: true, Auth: agentruntime.AuthAuthenticated, Capabilities: a.Capabilities()}
}
func (a runnerAdapter) Start(context.Context, agentruntime.Request) (agentruntime.Session, error) {
	ch := make(chan agentruntime.Event)
	close(ch)
	return &runnerSession{events: ch}, nil
}

type runnerSession struct{ events chan agentruntime.Event }

func (*runnerSession) ID() string                          { return "session-1" }
func (*runnerSession) EngineID() string                    { return "runner-fake" }
func (*runnerSession) Status() agentruntime.SessionStatus  { return agentruntime.SessionCompleted }
func (s *runnerSession) Events() <-chan agentruntime.Event { return s.events }
func (*runnerSession) Wait(context.Context) (agentruntime.Result, error) {
	return agentruntime.Result{SessionID: "session-1", EngineID: "runner-fake", ProviderStatus: "completed"}, nil
}
func (*runnerSession) Cancel(context.Context) error { return nil }

func TestExecutePreparedAgentOSCompletesFocusedTask(t *testing.T) {
	registry := agentruntime.NewRegistry()
	if err := registry.Register(runnerAdapter{}); err != nil {
		t.Fatal(err)
	}
	events := runtimeevents.NewStore(t.TempDir())
	prepared, err := PrepareAgentOS(context.Background(), events, registry, AgentOSRequest{WorkspaceKey: "ws", TaskID: "task", Objective: "fix one bug", Shape: TaskShape{TaskKind: "fix", FilesEstimated: 1}, Budget: usage.Budget{MaxTotalTokens: 100000}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := ExecutePreparedAgentOS(context.Background(), prepared, agentruntime.NewRuntime(registry), AgentOSExecutionRequest{WorkspaceKey: "ws", TaskID: "task", Objective: "fix one bug"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Completed || len(report.Agents) != 1 {
		t.Fatalf("unexpected report: %+v", report)
	}
	if report.Usage.OpenReservations != 0 {
		t.Fatalf("reservations must settle: %+v", report.Usage)
	}
}

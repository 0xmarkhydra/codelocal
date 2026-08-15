package agentruntime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
)

type fakeAdapter struct {
	id           string
	capabilities Capabilities
	installed    bool
}

func (f *fakeAdapter) ID() string                 { return f.id }
func (f *fakeAdapter) DisplayName() string        { return "Fake " + f.id }
func (f *fakeAdapter) Capabilities() Capabilities { return f.capabilities }
func (f *fakeAdapter) Probe(context.Context) ProbeResult {
	return ProbeResult{EngineID: f.id, DisplayName: f.DisplayName(), Installed: f.installed, Compatible: f.installed, Auth: AuthUnknown, Capabilities: f.capabilities, CheckedAt: nowMillis()}
}
func (f *fakeAdapter) Start(_ context.Context, request Request) (Session, error) {
	session := newFakeSession("session-"+request.TaskID, f.id, request.TaskID)
	session.emit(Event{ID: "event-start", SessionID: session.id, TaskID: request.TaskID, EngineID: f.id, Kind: EventSessionStarted, Timestamp: nowMillis(), Summary: "started"})
	session.emit(Event{ID: "event-message", SessionID: session.id, TaskID: request.TaskID, EngineID: f.id, Kind: EventMessage, Timestamp: nowMillis(), Summary: "fake response"})
	session.complete(Result{SessionID: session.id, EngineID: f.id, ProviderStatus: "completed", Summary: "fake complete"}, nil)
	return session, nil
}

type fakeSession struct {
	mu        sync.RWMutex
	id        string
	engineID  string
	taskID    string
	status    SessionStatus
	events    chan Event
	done      chan struct{}
	result    Result
	err       error
	completed bool
}

func newFakeSession(id, engineID, taskID string) *fakeSession {
	return &fakeSession{id: id, engineID: engineID, taskID: taskID, status: SessionRunning, events: make(chan Event, 8), done: make(chan struct{})}
}
func (s *fakeSession) ID() string       { return s.id }
func (s *fakeSession) EngineID() string { return s.engineID }
func (s *fakeSession) Status() SessionStatus {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.status
}
func (s *fakeSession) Events() <-chan Event { return s.events }
func (s *fakeSession) emit(event Event)     { s.events <- event }
func (s *fakeSession) complete(result Result, err error) {
	s.mu.Lock()
	if s.completed {
		s.mu.Unlock()
		return
	}
	s.completed = true
	s.result = result
	s.err = err
	s.status = SessionCompleted
	close(s.events)
	close(s.done)
	s.mu.Unlock()
}
func (s *fakeSession) Wait(ctx context.Context) (Result, error) {
	select {
	case <-ctx.Done():
		return Result{}, ctx.Err()
	case <-s.done:
		s.mu.RLock()
		defer s.mu.RUnlock()
		return s.result, s.err
	}
}
func (s *fakeSession) Cancel(context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.completed {
		return nil
	}
	s.status = SessionCancelled
	s.completed = true
	s.result = Result{SessionID: s.id, EngineID: s.engineID, ProviderStatus: "cancelled"}
	close(s.events)
	close(s.done)
	return nil
}

func TestRegistryRejectsDuplicateAndListsDeterministically(t *testing.T) {
	registry := NewRegistry()
	for _, id := range []string{"zeta", "alpha"} {
		if err := registry.Register(&fakeAdapter{id: id, installed: true}); err != nil {
			t.Fatal(err)
		}
	}
	if err := registry.Register(&fakeAdapter{id: "alpha", installed: true}); !errors.Is(err, ErrEngineAlreadyExists) {
		t.Fatalf("duplicate registration err=%v", err)
	}
	ids := registry.IDs()
	if fmt.Sprint(ids) != fmt.Sprint([]string{"alpha", "zeta"}) {
		t.Fatalf("registry IDs not deterministic: %v", ids)
	}
}

func TestRuntimeReviewLifecycleAndExplicitEngineOnly(t *testing.T) {
	registry := NewRegistry()
	adapter := &fakeAdapter{id: "fake", installed: true, capabilities: Capabilities{NonInteractive: true, StructuredOutput: true, Streaming: true, ReviewMode: true, Transport: TransportStructuredCLI, Isolation: IsolationDegraded}}
	if err := registry.Register(adapter); err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime(registry)
	request := Request{TaskID: "task-1", WorkspaceKey: "device::workspace", Objective: "review payment module", Mode: ModeReview}
	session, err := runtime.Start(context.Background(), "fake", request)
	if err != nil {
		t.Fatal(err)
	}
	events := []Event{}
	for event := range session.Events() {
		events = append(events, event)
	}
	result, err := session.Wait(context.Background())
	if err != nil || result.ProviderStatus != "completed" || len(events) != 2 || events[0].Kind != EventSessionStarted {
		t.Fatalf("unexpected fake lifecycle result=%#v events=%#v err=%v", result, events, err)
	}
	if _, err := runtime.Start(context.Background(), "auto", request); !errors.Is(err, ErrAutoRoutingDisabled) {
		t.Fatalf("auto routing should be disabled: %v", err)
	}
	if _, err := runtime.Start(context.Background(), "missing", request); !errors.Is(err, ErrEngineNotFound) {
		t.Fatalf("missing engine err=%v", err)
	}
}

func TestRuntimeRejectsUnsafeMutationAndAllowsMediatedMutation(t *testing.T) {
	registry := NewRegistry()
	_ = registry.Register(&fakeAdapter{id: "degraded", installed: true, capabilities: Capabilities{FileEditing: true, Transport: TransportPTY, Isolation: IsolationDegraded}})
	_ = registry.Register(&fakeAdapter{id: "mediated", installed: true, capabilities: Capabilities{FileEditing: true, StructuredOutput: true, Transport: TransportNativeStructured, Isolation: IsolationMediated}})
	runtime := NewRuntime(registry)
	request := Request{TaskID: "task-mutate", WorkspaceKey: "device::workspace", Objective: "change code", Mode: ModeMutate}
	if _, err := runtime.Start(context.Background(), "degraded", request); !errors.Is(err, ErrUnsafeMutation) {
		t.Fatalf("degraded provider mutation must be rejected: %v", err)
	}
	if _, err := runtime.Start(context.Background(), "mediated", request); err != nil {
		t.Fatalf("mediated provider mutation should pass runtime safety gate: %v", err)
	}
}

func TestFakeSessionCancelIsIdempotentAndWaitable(t *testing.T) {
	session := newFakeSession("session-cancel", "fake", "task-cancel")
	if err := session.Cancel(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := session.Cancel(context.Background()); err != nil {
		t.Fatalf("second cancel must be idempotent: %v", err)
	}
	result, err := session.Wait(context.Background())
	if err != nil || result.ProviderStatus != "cancelled" || session.Status() != SessionCancelled {
		t.Fatalf("cancelled session result=%#v status=%s err=%v", result, session.Status(), err)
	}
}

func TestDiscoverExecutablesAndDoctorDoNotExecuteProviders(t *testing.T) {
	calls := []string{}
	lookup := func(binary string) (string, error) {
		calls = append(calls, binary)
		if binary == "codex" {
			return "/usr/local/bin/codex", nil
		}
		return "", errors.New("not found")
	}
	statuses := DiscoverExecutables(DefaultExecutableSpecs(), lookup)
	if len(statuses) != 3 || statuses[1].EngineID != "codex" || !statuses[1].Installed || statuses[1].Path != "/usr/local/bin/codex" {
		t.Fatalf("unexpected executable discovery: %#v", statuses)
	}
	if fmt.Sprint(calls) != fmt.Sprint([]string{"claude", "codex", "cos"}) {
		t.Fatalf("discovery did more than deterministic lookup: %v", calls)
	}
	report := Doctor(NewRegistry(), lookup)
	if len(report.Executables) != 3 || report.SafetyNote == "" {
		t.Fatalf("unexpected doctor report: %#v", report)
	}
}

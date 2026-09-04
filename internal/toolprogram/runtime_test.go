package toolprogram

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/contextsurface"
	"github.com/0xmarkhydra/codelocal/internal/security"
)

func TestProgramUsesOnlyProjectedBindingsAndReducesOutput(t *testing.T) {
	binding := Binding{Capability: security.ProjectedCapability{CapabilityDescriptor: security.CapabilityDescriptor{ID: "tests", Action: "run", Tool: "go-test"}, Visibility: security.CapabilityAvailable}, Handler: func(context.Context, map[string]any) (HandlerResult, error) {
		return HandlerResult{Kind: contextsurface.ObservationTests, Text: strings.Repeat("noise line\n", 1000) + "--- FAIL: TestAuth\nexpected 200 got 500\nauth/service_test.go:42\n"}, nil
	}}
	runtime, err := New([]Binding{binding}, Limits{MaxOperations: 4, MaxStepTokens: 64}, nil)
	if err != nil {
		t.Fatal(err)
	}
	report, err := runtime.Execute(context.Background(), Program{ID: "p", Steps: []Step{{ID: "s1", CapabilityID: "tests"}}})
	if err != nil {
		t.Fatal(err)
	}
	if !report.Completed || len(report.Steps) != 1 || report.ReducedTokens >= report.OriginalTokens || report.AvoidedTokens <= 0 {
		t.Fatalf("unexpected reduction report: %+v", report)
	}
	if !strings.Contains(strings.ToLower(report.Steps[0].Observation.Item.Text), "fail") {
		t.Fatalf("critical failure evidence lost: %q", report.Steps[0].Observation.Item.Text)
	}
}

func TestProgramRejectsUnprojectedCapability(t *testing.T) {
	runtime, _ := New(nil, Limits{}, nil)
	_, err := runtime.Execute(context.Background(), Program{ID: "p", Steps: []Step{{ID: "s", CapabilityID: "raw-fs"}}})
	if !errors.Is(err, ErrCapabilityUnavailable) {
		t.Fatalf("unprojected capability executed: %v", err)
	}
}

func TestProgramPromptableCapabilityRequiresAuthorizer(t *testing.T) {
	capability := security.ProjectedCapability{CapabilityDescriptor: security.CapabilityDescriptor{ID: "net", Action: "network", Tool: "http"}, Visibility: security.CapabilityPromptable, ApprovalKey: "policy:key"}
	binding := Binding{Capability: capability, Handler: func(context.Context, map[string]any) (HandlerResult, error) { return HandlerResult{Text: "ok"}, nil }}
	runtime, _ := New([]Binding{binding}, Limits{}, nil)
	if _, err := runtime.Execute(context.Background(), Program{ID: "p", Steps: []Step{{ID: "s", CapabilityID: "net"}}}); !errors.Is(err, ErrApprovalRequired) {
		t.Fatalf("promptable capability bypassed approval: %v", err)
	}
	calls := 0
	runtime, _ = New([]Binding{binding}, Limits{}, func(context.Context, security.ProjectedCapability) error { calls++; return nil })
	if report, err := runtime.Execute(context.Background(), Program{ID: "p2", Steps: []Step{{ID: "s", CapabilityID: "net"}}}); err != nil || !report.Completed || calls != 1 {
		t.Fatalf("authorized program failed: report=%+v err=%v calls=%d", report, err, calls)
	}
}

func TestProgramEnforcesOperationAndArgsLimits(t *testing.T) {
	binding := Binding{Capability: security.ProjectedCapability{CapabilityDescriptor: security.CapabilityDescriptor{ID: "read", Action: "read"}, Visibility: security.CapabilityAvailable}, Handler: func(context.Context, map[string]any) (HandlerResult, error) { return HandlerResult{Text: "ok"}, nil }}
	runtime, _ := New([]Binding{binding}, Limits{MaxOperations: 1, MaxArgsBytes: 16}, nil)
	if _, err := runtime.Execute(context.Background(), Program{ID: "p", Steps: []Step{{ID: "a", CapabilityID: "read"}, {ID: "b", CapabilityID: "read"}}}); !errors.Is(err, ErrInvalidProgram) {
		t.Fatalf("operation limit ignored: %v", err)
	}
	if _, err := runtime.Execute(context.Background(), Program{ID: "p2", Steps: []Step{{ID: "a", CapabilityID: "read", Args: map[string]any{"x": strings.Repeat("a", 100)}}}}); !errors.Is(err, ErrProgramLimit) {
		t.Fatalf("args limit ignored: %v", err)
	}
}

package mcpgateway

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/agentruntime"
	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
	"github.com/0xmarkhydra/codelocal/internal/taskstate"
)

func TestAgentOSV2SnapshotCapabilitiesAreMediated(t *testing.T) {
	got := agentOSV2SnapshotCapabilities(orchestration.Capabilities{Filesystem: true, Shell: true})
	if !got.FileEditing || !got.ShellExecution || !got.MCP || !got.PlanMode {
		t.Fatalf("unexpected snapshot capabilities: %+v", got)
	}
	if got.Isolation != agentruntime.IsolationMediated {
		t.Fatalf("isolation = %q, want mediated tools", got.Isolation)
	}
}

func TestAgentOSV2TaskShapeDetectsCrossSubsystemProductWork(t *testing.T) {
	plan := orchestration.AgentPlan{
		TaskKind: orchestration.TaskKindDebug,
		Route:    orchestration.Decision{Primary: orchestration.LaneCode, Fallbacks: []orchestration.Lane{orchestration.LaneBrowser}},
	}
	state := taskstate.State{TouchedFiles: []string{"internal/auth/login.go", "web/app/login.tsx", "web/app/login.test.tsx"}}
	shape := agentOSV2TaskShape("Fix authentication UI toàn bộ", orchestration.Capabilities{Filesystem: true, Browser: true}, plan, state)
	if shape.FilesEstimated != 3 || shape.Subsystems != 2 {
		t.Fatalf("unexpected file/subsystem shape: %+v", shape)
	}
	if !shape.Architectural || !shape.SecuritySensitive || !shape.NeedsProductVerification {
		t.Fatalf("expected architecture/security/product signals: %+v", shape)
	}
	if tier := orchestration.ClassifyTaskTier(shape); tier != orchestration.TaskTier4 {
		t.Fatalf("tier = %q, want T4 because the objective is architectural", tier)
	}
}

func TestPrepareAgentOSV2ShadowPersistsPredictionWithoutStartingAdapter(t *testing.T) {
	events := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	plan := orchestration.AgentPlan{
		TaskKind:     orchestration.TaskKindDebug,
		Route:        orchestration.Decision{Primary: orchestration.LaneCode},
		Verification: orchestration.VerificationPlan{Mode: "required"},
	}
	state := taskstate.State{TouchedFiles: []string{"internal/auth/a.go", "web/auth/b.ts", "tests/auth/c_test.go"}}
	started := time.Date(2026, 8, 31, 12, 0, 0, 0, time.UTC)
	shadow := prepareAgentOSV2ShadowWithStore(context.Background(), events, "user", "session", "workspace", "fix auth bug", orchestration.Capabilities{Filesystem: true, Shell: true}, plan, state, started)
	if shadow.PrepareFailed || shadow.Prepared == nil {
		t.Fatalf("shadow preparation failed: %+v", shadow)
	}
	if len(shadow.Prepared.Agents) < 2 {
		t.Fatalf("expected bounded multi-agent prediction, got %d agents", len(shadow.Prepared.Agents))
	}
	for _, planned := range shadow.Prepared.Agents {
		if planned.Route.EngineID != "codelocal-host-shadow" {
			t.Fatalf("unexpected shadow route: %+v", planned.Route)
		}
	}

	summary := finalizeAgentOSV2Shadow(shadow, "ready", "ready", 100, 7, 1, false)
	if summary["mode"] != "shadow" || summary["prepared"] != true {
		t.Fatalf("unexpected shadow summary: %+v", summary)
	}
	stored, err := events.List("workspace", shadow.TaskID, 0, 100)
	if err != nil {
		t.Fatalf("List() error = %v", err)
	}
	preparedSeen, observedSeen := false, false
	for _, event := range stored {
		preparedSeen = preparedSeen || event.Type == agentOSV2ShadowPreparedEvent
		observedSeen = observedSeen || event.Type == agentOSV2ShadowObservedEvent
	}
	if !preparedSeen || !observedSeen {
		t.Fatalf("missing shadow evidence events: %+v", stored)
	}

	adapter := agentruntime.NewSnapshotAdapter("codelocal-host-shadow", "", agentOSV2SnapshotCapabilities(orchestration.Capabilities{Filesystem: true}))
	if _, err := adapter.Start(context.Background(), agentruntime.Request{}); err == nil {
		t.Fatal("snapshot adapter unexpectedly allowed execution")
	}
}

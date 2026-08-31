package mcpgateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/agentruntime"
	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
	"github.com/0xmarkhydra/codelocal/internal/taskstate"
)

const (
	agentOSV2ShadowPreparedEvent = "agent_os_v2.shadow.prepared"
	agentOSV2ShadowObservedEvent = "agent_os_v2.shadow.observed"
)

var agentOSV2ShadowEvents = runtimeevents.NewStore("")

type agentOSV2Shadow struct {
	WorkspaceKey     string
	TaskID           string
	SessionID        string
	Prepared         *orchestration.PreparedAgentOS
	PrepareFailed    bool
	PersistenceError bool
}

func agentOSV2SnapshotCapabilities(caps orchestration.Capabilities) agentruntime.Capabilities {
	return agentruntime.Capabilities{
		NonInteractive:   true,
		StructuredOutput: true,
		FileEditing:      caps.Filesystem,
		ShellExecution:   caps.Shell,
		MCP:              true,
		PlanMode:         true,
		ReviewMode:       true,
		Transport:        agentruntime.TransportNativeStructured,
		Isolation:        agentruntime.IsolationMediated,
	}
}

func agentOSV2SubsystemCount(paths []string) int {
	seen := map[string]struct{}{}
	for _, raw := range paths {
		path := filepath.ToSlash(filepath.Clean(strings.TrimSpace(raw)))
		if path == "" || path == "." {
			continue
		}
		part := strings.Split(strings.TrimPrefix(path, "./"), "/")[0]
		if part != "" && part != "." {
			seen[part] = struct{}{}
		}
	}
	return len(seen)
}

func agentOSV2TaskShape(objective string, caps orchestration.Capabilities, plan orchestration.AgentPlan, state taskstate.State) orchestration.TaskShape {
	text := strings.ToLower(strings.Join(strings.Fields(objective), " "))
	architectural := strings.Contains(text, "architect") || strings.Contains(text, "kiến trúc") || strings.Contains(text, "migration") || strings.Contains(text, "migrate") || strings.Contains(text, "toàn bộ")
	securitySensitive := strings.Contains(text, "security") || strings.Contains(text, "auth") || strings.Contains(text, "secret") || strings.Contains(text, "token") || strings.Contains(text, "bảo mật") || strings.Contains(text, "đăng nhập")
	productIntent := strings.Contains(text, "browser") || strings.Contains(text, "website") || strings.Contains(text, "frontend") || strings.Contains(text, "ui") || strings.Contains(text, "mobile") || strings.Contains(text, "ios") || strings.Contains(text, "android") || strings.Contains(text, "trình duyệt") || strings.Contains(text, "giao diện")
	productRoute := plan.Route.Primary == orchestration.LaneBrowser || plan.Route.Primary == orchestration.LaneComputer
	for _, lane := range plan.Route.Fallbacks {
		productRoute = productRoute || lane == orchestration.LaneBrowser || lane == orchestration.LaneComputer
	}
	return orchestration.TaskShape{
		TaskKind:                 string(plan.TaskKind),
		FilesEstimated:           len(state.TouchedFiles),
		Subsystems:               agentOSV2SubsystemCount(state.TouchedFiles),
		Architectural:            architectural,
		SecuritySensitive:        securitySensitive,
		NeedsProductVerification: productRoute || (productIntent && (caps.Browser || caps.Computer)),
	}
}

func agentOSV2ShadowTaskID(userID, session, workspaceKey, objective string, started time.Time) string {
	h := sha256.New()
	for _, part := range []string{userID, session, workspaceKey, objective, started.UTC().Format(time.RFC3339Nano)} {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return "shadow_" + hex.EncodeToString(h.Sum(nil))[:20]
}

func prepareAgentOSV2Shadow(ctx context.Context, userID, session, workspaceKey, objective string, caps orchestration.Capabilities, plan orchestration.AgentPlan, state taskstate.State) *agentOSV2Shadow {
	return prepareAgentOSV2ShadowWithStore(ctx, agentOSV2ShadowEvents, userID, session, workspaceKey, objective, caps, plan, state, time.Now().UTC())
}

func prepareAgentOSV2ShadowWithStore(ctx context.Context, events *runtimeevents.Store, userID, session, workspaceKey, objective string, caps orchestration.Capabilities, plan orchestration.AgentPlan, state taskstate.State, started time.Time) *agentOSV2Shadow {
	shadow := &agentOSV2Shadow{
		WorkspaceKey: workspaceKey,
		TaskID:       agentOSV2ShadowTaskID(userID, session, workspaceKey, objective, started),
		SessionID:    session,
	}
	if events == nil {
		shadow.PrepareFailed = true
		return shadow
	}

	registry := agentruntime.NewRegistry()
	adapter := agentruntime.NewSnapshotAdapter("codelocal-host-shadow", "CodeLocal Host (shadow)", agentOSV2SnapshotCapabilities(caps))
	if err := registry.Register(adapter); err != nil {
		shadow.PrepareFailed = true
		return shadow
	}

	prepared, err := orchestration.PrepareAgentOS(ctx, events, registry, orchestration.AgentOSRequest{
		WorkspaceKey: workspaceKey,
		TaskID:       shadow.TaskID,
		Objective:    objective,
		Shape:        agentOSV2TaskShape(objective, caps, plan, state),
		Verification: plan.Verification,
	})
	if err != nil {
		shadow.PrepareFailed = true
		return shadow
	}
	shadow.Prepared = prepared
	_, _, err = events.Append(workspaceKey, shadow.TaskID, runtimeevents.Event{
		Type:           agentOSV2ShadowPreparedEvent,
		SessionID:      session,
		TraceID:        shadow.TaskID,
		IdempotencyKey: "shadow-prepared",
		Payload: map[string]any{
			"tier":        string(prepared.Team.Tier),
			"agentCount":  len(prepared.Agents),
			"parallelism": prepared.Team.Parallelism,
		},
	})
	if err != nil {
		shadow.PersistenceError = true
	}
	return shadow
}

func finalizeAgentOSV2Shadow(shadow *agentOSV2Shadow, status, qualityStatus string, qualityScore, operationCount, replans int, halted bool) map[string]any {
	if shadow == nil {
		return nil
	}
	if shadow.Prepared != nil {
		_, _, err := agentOSV2ShadowEvents.Append(shadow.WorkspaceKey, shadow.TaskID, runtimeevents.Event{
			Type:           agentOSV2ShadowObservedEvent,
			SessionID:      shadow.SessionID,
			TraceID:        shadow.TaskID,
			IdempotencyKey: "shadow-observed",
			Payload: map[string]any{
				"actualStatus":  strings.TrimSpace(status),
				"qualityStatus": strings.TrimSpace(qualityStatus),
				"qualityScore":  qualityScore,
				"operationCount": operationCount,
				"replans":       replans,
				"halted":        halted,
			},
		})
		if err != nil {
			shadow.PersistenceError = true
		}
	}

	result := map[string]any{
		"generation":       "v2",
		"mode":             "shadow",
		"taskId":           shadow.TaskID,
		"prepared":         shadow.Prepared != nil,
		"prepareFailed":    shadow.PrepareFailed,
		"persistenceError": shadow.PersistenceError,
	}
	if shadow.Prepared == nil {
		return result
	}
	routes := make([]map[string]any, 0, len(shadow.Prepared.Agents))
	for _, item := range shadow.Prepared.Agents {
		routes = append(routes, map[string]any{
			"role":     string(item.Member.Role),
			"engineId": item.Route.EngineID,
			"score":    item.Route.Score,
		})
	}
	result["tier"] = string(shadow.Prepared.Team.Tier)
	result["agentCount"] = len(shadow.Prepared.Agents)
	result["parallelism"] = shadow.Prepared.Team.Parallelism
	result["routes"] = routes
	result["actual"] = map[string]any{
		"status":         status,
		"qualityStatus":  qualityStatus,
		"qualityScore":   qualityScore,
		"operationCount": operationCount,
		"replans":        replans,
		"halted":         halted,
	}
	return result
}

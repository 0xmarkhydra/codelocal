package cloudserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func storedDashboardToolCalls(t *testing.T, calls ...dashboardToolCall) json.RawMessage {
	t.Helper()
	raw, err := json.Marshal(calls)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func TestDashboardPersistedHistoryRehydratesLatestToolTurnAndCurrentCheckpoint(t *testing.T) {
	stored := []cloud.DashboardChatMessage{
		{ID: "user-old", Role: "user", Content: "old task"},
		{ID: "assistant-old", Role: "assistant", Content: "old done", ToolCalls: storedDashboardToolCalls(t,
			dashboardToolCall{ID: "call-old", Name: "read_project_file", Arguments: `{"path":"old.go"}`, Result: `{"content":"old"}`, Status: "done"},
		)},
		{ID: "user-latest", Role: "user", Content: "fix the dashboard"},
		{ID: "assistant-latest", Role: "assistant", Content: "paused", ToolCalls: storedDashboardToolCalls(t,
			dashboardToolCall{ID: "call-latest", Name: "read_project_file", Arguments: `{"path":"dashboard.tsx"}`, Result: `{"content":"latest"}`, Status: "done"},
		)},
		{ID: "current-user", Role: "user", Content: "fix the dashboard"},
		{ID: "current-assistant", Role: "assistant", ToolCalls: storedDashboardToolCalls(t,
			dashboardToolCall{ID: "call-resume", Name: "run_project_command", Arguments: `{"command":"npm test"}`, Result: `{"exitCode":0}`, Status: "done"},
		)},
	}

	messages, resume := dashboardPersistedHistoryMessages(stored, "current-user", "current-assistant")
	if resume.Completed {
		t.Fatal("empty checkpoint reply must remain resumable")
	}
	if len(resume.Results) != 1 || resume.Results[0].ID != "call-resume" {
		t.Fatalf("resume results = %#v, want current checkpoint", resume.Results)
	}

	encoded, err := json.Marshal(messages)
	if err != nil {
		t.Fatal(err)
	}
	text := string(encoded)
	for _, wanted := range []string{"old done", "call-latest", `\"content\":\"latest\"`, "paused"} {
		if !strings.Contains(text, wanted) {
			t.Fatalf("rehydrated history missing %q: %s", wanted, text)
		}
	}
	for _, excluded := range []string{"call-old", "call-resume", "current-user", "current-assistant"} {
		if strings.Contains(text, excluded) {
			t.Fatalf("rehydrated history unexpectedly contains %q: %s", excluded, text)
		}
	}

	latestTool := -1
	latestSummary := -1
	for index, message := range messages {
		if message["role"] == "tool" && message["tool_call_id"] == "call-latest" {
			latestTool = index
		}
		if message["role"] == "assistant" && message["content"] == "paused" {
			latestSummary = index
		}
	}
	if latestTool < 0 || latestSummary <= latestTool {
		t.Fatalf("tool result must precede its assistant summary: %#v", messages)
	}
}

func TestDashboardPersistedHistoryRecognizesCompletedTransportRetry(t *testing.T) {
	stored := []cloud.DashboardChatMessage{
		{ID: "current-user", Role: "user", Content: "ship it"},
		{ID: "current-assistant", Role: "assistant", Content: "Đã sửa và kiểm tra.", ToolCalls: storedDashboardToolCalls(t,
			dashboardToolCall{ID: "call-1", Name: "edit_project_file", Arguments: `{}`, Result: `{"ok":true}`, Status: "done"},
		)},
	}

	messages, resume := dashboardPersistedHistoryMessages(stored, "current-user", "current-assistant")
	if len(messages) != 0 {
		t.Fatalf("current retry rows leaked into prior history: %#v", messages)
	}
	if !resume.Completed || resume.CompletedReply != "Đã sửa và kiểm tra." || len(resume.Results) != 1 {
		t.Fatalf("completed retry not recovered: %#v", resume)
	}
}

func TestDashboardReplayCompletedExecutionStreamsSavedResult(t *testing.T) {
	r := httptest.NewRequest(http.MethodPost, "/api/v1/dashboard/chat?stream=1", nil)
	r = dashboardWithChatThread(r, "thread-1")
	r = dashboardWithExecutionState(r, "user-1", "request-1", nil)
	dashboardSetExecutionResume(r, dashboardChatExecutionResume{
		Completed:      true,
		CompletedReply: "Đã hoàn tất.",
		Results: []dashboardToolCall{{
			ID: "call-1", Name: "verify_project_changes", Arguments: `{}`, Result: `{"ok":true}`, Status: "done",
		}},
	})
	recorder := httptest.NewRecorder()
	if !dashboardReplayCompletedExecution(recorder, r) {
		t.Fatal("completed execution was not replayed")
	}
	body := recorder.Body.String()
	for _, wanted := range []string{"event: tool_calls", "event: done", "Đã hoàn tất.", `"replayed":true`, "thread-1"} {
		if !strings.Contains(body, wanted) {
			t.Fatalf("replay stream missing %q: %s", wanted, body)
		}
	}
}

func TestDashboardStoredToolResultsRejectsTruncatedJSON(t *testing.T) {
	if got := dashboardStoredToolResults(json.RawMessage(`[{"id":"call-1"}`)); len(got) != 0 {
		t.Fatalf("invalid checkpoint must not be replayed: %#v", got)
	}
}

func TestDashboardPersistedHistoryKeepsRecentImages(t *testing.T) {
	stored := []cloud.DashboardChatMessage{
		{ID: "user-old", Role: "user", Content: "old picture", Image: "data:image/png;base64,OLDOLD"},
		{ID: "assistant-old", Role: "assistant", Content: "old seen"},
		{ID: "user-new", Role: "user", Content: "new picture", Image: "data:image/png;base64,NEWNEW"},
		{ID: "assistant-new", Role: "assistant", Content: "new seen"},
	}

	messages, _ := dashboardPersistedHistoryMessages(stored, "current-user", "current-assistant")
	images := 0
	for _, message := range messages {
		if message["role"] != "user" {
			continue
		}
		parts, ok := message["content"].([]map[string]any)
		if !ok {
			continue
		}
		for _, part := range parts {
			if part["type"] == "image_url" {
				images++
			}
		}
	}
	if images != 2 {
		t.Fatalf("history must keep both recent images inline: %#v", messages)
	}
}

func TestDashboardCompactEphemeralImageStaysBounded(t *testing.T) {
	prefix := "data:image/png;base64,"
	big := prefix + strings.Repeat("A", 2*1024*1024)
	compacted := dashboardChatCompactEphemeralImage(big)
	if compacted == "" || len(compacted) > 1500*1024 {
		t.Fatalf("compacted image must stay bounded, got %d", len(compacted))
	}
	if !strings.HasPrefix(compacted, prefix) {
		t.Fatal("compacted image must keep the data-url prefix")
	}
}

func TestDashboardContextModeNormalizationAndBudgets(t *testing.T) {
	cases := map[string]string{
		"":           dashboardContextModeSmart,
		" SMART ":    dashboardContextModeSmart,
		"off":        dashboardContextModeOff,
		"AGGRESSIVE": dashboardContextModeAggressive,
		"unknown":    dashboardContextModeSmart,
	}
	for input, want := range cases {
		if got := dashboardNormalizeContextMode(input); got != want {
			t.Fatalf("normalize context mode %q = %q, want %q", input, got, want)
		}
	}
	if dashboardContextBudgetForMode(dashboardContextModeAggressive).MaxChars >= dashboardContextBudgetForMode(dashboardContextModeSmart).MaxChars {
		t.Fatal("aggressive mode must use a smaller working-context budget than smart mode")
	}
	if dashboardContextBudgetForMode(dashboardContextModeSmart).MaxChars >= dashboardContextBudgetForMode(dashboardContextModeOff).MaxChars {
		t.Fatal("smart mode must use a smaller working-context budget than off mode")
	}
}

func TestDashboardContextModeSelectsWorkingHistoryWithoutDeletingStoredRows(t *testing.T) {
	stored := make([]cloud.DashboardChatMessage, 0, 80)
	for index := 0; index < 80; index++ {
		stored = append(stored, cloud.DashboardChatMessage{
			ID:      fmt.Sprintf("msg-%02d", index),
			Role:    "user",
			Content: strings.Repeat("x", 3000),
		})
	}

	aggressive := dashboardSelectStoredHistory(stored, dashboardContextModeAggressive)
	smart := dashboardSelectStoredHistory(stored, dashboardContextModeSmart)
	off := dashboardSelectStoredHistory(stored, dashboardContextModeOff)
	if len(aggressive) >= len(smart) || len(smart) >= len(off) {
		t.Fatalf("working-history sizes must grow aggressive < smart < off, got %d < %d < %d", len(aggressive), len(smart), len(off))
	}
	if len(stored) != 80 || stored[0].ID != "msg-00" {
		t.Fatal("context selection must not mutate or delete persisted thread history")
	}
	if aggressive[len(aggressive)-1].ID != "msg-79" || smart[len(smart)-1].ID != "msg-79" || off[len(off)-1].ID != "msg-79" {
		t.Fatal("all context modes must preserve the newest turn")
	}
}

func TestDashboardContextModeCompactsToolResultReversibly(t *testing.T) {
	raw := strings.Repeat("A", 16000) + "IMPORTANT_TAIL"
	results := []dashboardToolCall{{ID: "call-1", Name: "run_project_command", Arguments: `{}`, Result: raw, Status: "done"}}

	aggressive := dashboardToolTranscriptForMode(results, "history", dashboardContextModeAggressive)
	tool, ok := aggressive[1]["content"].(string)
	if !ok || len(tool) >= len(raw) || !strings.Contains(tool, "context optimized") || !strings.Contains(tool, "IMPORTANT_TAIL") {
		t.Fatalf("aggressive tool compaction did not preserve useful head/tail context: %#v", aggressive)
	}
	off := dashboardToolTranscriptForMode(results, "history", dashboardContextModeOff)
	if off[1]["content"] != raw {
		t.Fatal("off mode must keep the raw tool result unchanged")
	}
}

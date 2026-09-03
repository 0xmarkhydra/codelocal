package cloudserver

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

const dashboardChatModelHistoryLimit = 12

type dashboardChatExecutionResume struct {
	Results        []dashboardToolCall
	CompletedReply string
	Completed      bool
}

func dashboardRequestHistoryMessages(history []dashboardChatHistoryItem) []map[string]any {
	messages := make([]map[string]any, 0, len(history))
	for _, item := range history {
		role := strings.TrimSpace(item.Role)
		if role != "user" && role != "assistant" && role != "tool" {
			continue
		}
		message := map[string]any{"role": role, "content": item.Content}
		if item.ToolCallID != "" {
			message["tool_call_id"] = item.ToolCallID
		}
		if item.Name != "" {
			message["name"] = item.Name
		}
		messages = append(messages, message)
	}
	return messages
}

func dashboardStoredToolResults(raw json.RawMessage) []dashboardToolCall {
	if len(raw) == 0 || !json.Valid(raw) {
		return nil
	}
	var stored []dashboardToolCall
	if json.Unmarshal(raw, &stored) != nil {
		return nil
	}
	results := make([]dashboardToolCall, 0, len(stored))
	for _, result := range stored {
		result.Name = strings.TrimSpace(result.Name)
		if result.Name == "" || strings.TrimSpace(result.Result) == "" {
			continue
		}
		if strings.TrimSpace(result.Arguments) == "" {
			result.Arguments = "{}"
		}
		if result.Status == "" {
			result.Status = dashboardToolResultStatus(result.Result)
		}
		results = append(results, result)
	}
	return results
}

func dashboardToolTranscript(results []dashboardToolCall, idPrefix string) []map[string]any {
	messages := make([]map[string]any, 0, len(results)*2)
	for index, result := range results {
		callID := strings.TrimSpace(result.ID)
		if callID == "" {
			callID = fmt.Sprintf("%s_%d", idPrefix, index+1)
		}
		arguments := strings.TrimSpace(result.Arguments)
		if arguments == "" {
			arguments = "{}"
		}
		messages = append(messages,
			map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{{
				"id": callID, "type": "function", "function": map[string]any{"name": result.Name, "arguments": arguments},
			}}},
			map[string]any{"role": "tool", "content": result.Result, "tool_call_id": callID, "name": result.Name},
		)
	}
	return messages
}

func dashboardPersistedHistoryMessages(stored []cloud.DashboardChatMessage, currentUserID, currentAssistantID string) ([]map[string]any, dashboardChatExecutionResume) {
	filtered := make([]cloud.DashboardChatMessage, 0, len(stored))
	resume := dashboardChatExecutionResume{}
	for _, message := range stored {
		switch message.ID {
		case currentUserID:
			// A transport retry reuses the same request id. The current user
			// message is appended below exactly once, so exclude its saved copy.
			continue
		case currentAssistantID:
			resume.Results = dashboardStoredToolResults(message.ToolCalls)
			if strings.TrimSpace(message.Content) != "" {
				resume.CompletedReply = message.Content
				resume.Completed = true
			}
			continue
		default:
			filtered = append(filtered, message)
		}
	}
	if len(filtered) > dashboardChatModelHistoryLimit {
		filtered = filtered[len(filtered)-dashboardChatModelHistoryLimit:]
	}

	latestToolMessage := -1
	toolResults := make(map[int][]dashboardToolCall)
	for index, message := range filtered {
		if message.Role != "assistant" {
			continue
		}
		if results := dashboardStoredToolResults(message.ToolCalls); len(results) > 0 {
			latestToolMessage = index
			toolResults[index] = results
		}
	}

	messages := make([]map[string]any, 0, len(filtered)+len(toolResults[latestToolMessage])*2)
	for index, message := range filtered {
		role := strings.TrimSpace(message.Role)
		if role != "user" && role != "assistant" && role != "tool" {
			continue
		}
		if index == latestToolMessage {
			messages = append(messages, dashboardToolTranscript(toolResults[index], "history_"+message.ID)...)
			if strings.TrimSpace(message.Content) != "" {
				messages = append(messages, map[string]any{"role": "assistant", "content": message.Content})
			}
			continue
		}
		if role == "assistant" && strings.TrimSpace(message.Content) == "" {
			continue
		}
		messages = append(messages, map[string]any{"role": role, "content": message.Content})
	}
	return messages, resume
}

func (s *Server) dashboardExecutionHistory(r *http.Request, userID, threadID string, fallback []dashboardChatHistoryItem) []map[string]any {
	fallbackMessages := dashboardRequestHistoryMessages(fallback)
	stored, err := s.Store.ListDashboardChatHistoryForThread(r.Context(), userID, threadID, 50)
	if err != nil {
		slog.Warn("dashboard chat execution history unavailable", "error", err, "user", userID, "thread", threadID)
		return fallbackMessages
	}
	if len(stored) == 0 && len(fallbackMessages) > 0 {
		return fallbackMessages
	}
	currentUserID := dashboardChatMessageID(r, userID, "user")
	currentAssistantID := dashboardChatMessageID(r, userID, "assistant")
	messages, resume := dashboardPersistedHistoryMessages(stored, currentUserID, currentAssistantID)
	dashboardSetExecutionResume(r, resume)
	return messages
}

func dashboardSeedToolProgress(results []dashboardToolCall) map[string]int {
	seen := make(map[string]int, len(results))
	for _, result := range results {
		fingerprint := dashboardToolProgressFingerprint(llmToolCall{Name: result.Name, Arguments: result.Arguments}, result.Result)
		seen[fingerprint]++
	}
	return seen
}

func (s *Server) saveDashboardChatMessageDurably(r *http.Request, message cloud.DashboardChatMessage) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(r.Context()), 5*time.Second)
	defer cancel()
	return s.saveDashboardChatMessage(r.WithContext(ctx), message)
}

func (s *Server) saveDashboardChatToolCheckpoint(r *http.Request, userID string, results []dashboardToolCall) {
	if s == nil || s.Store == nil || len(results) == 0 {
		return
	}
	encoded, err := json.Marshal(results)
	if err != nil {
		slog.Warn("dashboard chat checkpoint encode failed", "error", err, "user", userID)
		return
	}
	message := cloud.DashboardChatMessage{
		ID:        dashboardChatMessageID(r, userID, "assistant"),
		UserID:    userID,
		Role:      "assistant",
		Content:   "",
		ToolCalls: encoded,
		CreatedAt: time.Now().UnixMilli(),
	}
	if err := s.saveDashboardChatMessageDurably(r, message); err != nil {
		slog.Warn("dashboard chat checkpoint save failed", "error", err, "user", userID, "thread", dashboardChatThreadID(r))
	}
}

func dashboardReplayCompletedExecution(w http.ResponseWriter, r *http.Request) bool {
	resume := dashboardExecutionResumeFromRequest(r)
	if !resume.Completed {
		return false
	}
	if r.URL.Query().Get("stream") != "1" && !strings.Contains(r.Header.Get("Accept"), "text/event-stream") {
		webutil.JSON(w, http.StatusOK, map[string]any{
			"reply": resume.CompletedReply, "model": dashboardPublicModelName, "tool_calls": resume.Results, "threadId": dashboardChatThreadID(r), "replayed": true,
		})
		return true
	}
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, _ := w.(http.Flusher)
	_, _ = fmt.Fprint(w, ": codelocal-replayed\n\n")
	if flusher != nil {
		flusher.Flush()
	}
	if len(resume.Results) > 0 {
		writeDashboardSSE(w, flusher, "tool_calls", map[string]any{"tool_calls": resume.Results})
	}
	writeDashboardSSE(w, flusher, "done", map[string]any{
		"done": true, "reply": resume.CompletedReply, "tool_calls": resume.Results, "model": dashboardPublicModelName, "threadId": dashboardChatThreadID(r), "replayed": true,
	})
	return true
}

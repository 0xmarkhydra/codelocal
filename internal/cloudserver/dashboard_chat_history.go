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

const (
	dashboardContextModeOff        = "off"
	dashboardContextModeSmart      = "smart"
	dashboardContextModeAggressive = "aggressive"
	dashboardChatHistoryFetchLimit = 120
)

type dashboardChatContextBudget struct {
	MaxMessages     int
	MaxChars        int
	ToolResultChars int
}

func dashboardNormalizeContextMode(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case dashboardContextModeOff:
		return dashboardContextModeOff
	case dashboardContextModeAggressive:
		return dashboardContextModeAggressive
	default:
		return dashboardContextModeSmart
	}
}

func dashboardContextBudgetForMode(mode string) dashboardChatContextBudget {
	switch dashboardNormalizeContextMode(mode) {
	case dashboardContextModeOff:
		// Off disables CodeLocal compaction, but keeps a generous hard safety cap
		// so a malformed thread cannot create an unbounded provider request.
		return dashboardChatContextBudget{MaxMessages: 120, MaxChars: 360000}
	case dashboardContextModeAggressive:
		return dashboardChatContextBudget{MaxMessages: 24, MaxChars: 56000, ToolResultChars: 5000}
	default:
		return dashboardChatContextBudget{MaxMessages: 48, MaxChars: 120000, ToolResultChars: 12000}
	}
}

func dashboardStoredMessageContextWeight(message cloud.DashboardChatMessage) int {
	weight := len(message.Content) + len(message.ToolCalls)
	if strings.TrimSpace(message.Image) != "" {
		// Images are independently limited below. Count only a small metadata
		// allowance here instead of their potentially huge data URL payload.
		weight += 4096
	}
	if weight < 1 {
		return 1
	}
	return weight
}

func dashboardSelectStoredHistory(stored []cloud.DashboardChatMessage, mode string) []cloud.DashboardChatMessage {
	budget := dashboardContextBudgetForMode(mode)
	if len(stored) == 0 {
		return stored
	}
	start := len(stored)
	used := 0
	for index := len(stored) - 1; index >= 0; index-- {
		if len(stored)-index > budget.MaxMessages {
			break
		}
		weight := dashboardStoredMessageContextWeight(stored[index])
		if start < len(stored) && used+weight > budget.MaxChars {
			break
		}
		start = index
		used += weight
	}
	return stored[start:]
}

func dashboardCompactContextText(value string, limit int) string {
	if limit <= 0 || len(value) <= limit {
		return value
	}
	head := limit * 2 / 3
	tail := limit - head
	marker := "\n\n[CodeLocal context optimized: middle omitted; full tool result is preserved in thread history.]\n\n"
	if head+tail+len(marker) >= len(value) {
		return value
	}
	return value[:head] + marker + value[len(value)-tail:]
}

// dashboardChatStoredImageIsMeta mirrors dashboardChatImageMetaFromStored in
// dashboard_chat_api.go without creating an import cycle between the history
// helper tests and the API file: history rows carrying {"sha256":...} meta
// JSON resolve only at turn 1, never inline in follow-up context.
func dashboardChatStoredImageIsMeta(value string) bool {
	value = strings.TrimSpace(value)
	return strings.HasPrefix(value, "{") && strings.Contains(value, `"sha256"`)
}

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
		if role == "user" && strings.TrimSpace(item.Image) != "" {
			messages = append(messages, map[string]any{
				"role": role,
				"content": []map[string]any{
					{"type": "text", "text": item.Content},
					{"type": "image_url", "image_url": map[string]any{"url": strings.TrimSpace(item.Image)}},
				},
			})
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
	return dashboardToolTranscriptForMode(results, idPrefix, dashboardContextModeOff)
}

func dashboardToolTranscriptForMode(results []dashboardToolCall, idPrefix, contextMode string) []map[string]any {
	budget := dashboardContextBudgetForMode(contextMode)
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
		resultText := dashboardCompactContextText(result.Result, budget.ToolResultChars)
		messages = append(messages,
			map[string]any{"role": "assistant", "content": "", "tool_calls": []map[string]any{{
				"id": callID, "type": "function", "function": map[string]any{"name": result.Name, "arguments": arguments},
			}}},
			map[string]any{"role": "tool", "content": resultText, "tool_call_id": callID, "name": result.Name},
		)
	}
	return messages
}

func dashboardPersistedHistoryMessages(stored []cloud.DashboardChatMessage, currentUserID, currentAssistantID string) ([]map[string]any, dashboardChatExecutionResume) {
	return dashboardPersistedHistoryMessagesForMode(stored, currentUserID, currentAssistantID, dashboardContextModeSmart)
}

func dashboardPersistedHistoryMessagesForMode(stored []cloud.DashboardChatMessage, currentUserID, currentAssistantID, contextMode string) ([]map[string]any, dashboardChatExecutionResume) {
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
	filtered = dashboardSelectStoredHistory(filtered, contextMode)

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
	// Task 2: keep at most the 2 most recent inline user images in LLM context
	// so a follow-up turn still sees the picture without blowing the token
	// budget. S3 meta JSON resolves at turn 1 via prepared URLs only.
	imageKept := 0
	for index := len(filtered) - 1; index >= 0; index-- {
		if strings.TrimSpace(filtered[index].Role) != "user" || strings.TrimSpace(filtered[index].Image) == "" {
			continue
		}
		if dashboardChatStoredImageIsMeta(filtered[index].Image) {
			filtered[index].Image = ""
			continue
		}
		if imageKept < 2 {
			imageKept++
			continue
		}
		filtered[index].Image = ""
	}
	for index, message := range filtered {
		role := strings.TrimSpace(message.Role)
		if role != "user" && role != "assistant" && role != "tool" {
			continue
		}
		if role == "user" && strings.TrimSpace(message.Image) != "" {
			messages = append(messages, map[string]any{
				"role": role,
				"content": []map[string]any{
					{"type": "text", "text": message.Content},
					{"type": "image_url", "image_url": map[string]any{"url": strings.TrimSpace(message.Image)}},
				},
			})
			continue
		}
		if index == latestToolMessage {
			messages = append(messages, dashboardToolTranscriptForMode(toolResults[index], "history_"+message.ID, contextMode)...)
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

func (s *Server) dashboardExecutionHistory(r *http.Request, userID, threadID string, fallback []dashboardChatHistoryItem, contextMode string) []map[string]any {
	fallbackMessages := dashboardRequestHistoryMessages(fallback)
	stored, err := s.Store.ListDashboardChatHistoryForThread(r.Context(), userID, threadID, dashboardChatHistoryFetchLimit)
	if err != nil {
		slog.Warn("dashboard chat execution history unavailable", "error", err, "user", userID, "thread", threadID)
		return fallbackMessages
	}
	if len(stored) == 0 && len(fallbackMessages) > 0 {
		return fallbackMessages
	}
	currentUserID := dashboardChatMessageID(r, userID, "user")
	currentAssistantID := dashboardChatMessageID(r, userID, "assistant")
	messages, resume := dashboardPersistedHistoryMessagesForMode(stored, currentUserID, currentAssistantID, contextMode)
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

package cloudserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"
)

type dashboardChatCompletionsStreamCallbacks struct {
	OnText      func(string)
	OnToolDelta func(index int, id, name, arguments string)
}

type dashboardChatCompletionsStreamRound struct {
	ToolCalls  []llmToolCall
	Content    string
	Progressed bool
	Usage      dashboardChatTokenUsage
}

type dashboardChatCompletionsToolState struct {
	id        string
	name      string
	arguments string
}

func callChatCompletionsStreamWithTools(ctx context.Context, baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any, callbacks dashboardChatCompletionsStreamCallbacks) (dashboardChatCompletionsStreamRound, error) {
	body := map[string]any{
		"model":          model,
		"messages":       messages,
		"temperature":    0.7,
		"stream":         true,
		"stream_options": map[string]any{"include_usage": true},
	}
	if len(tools) > 0 {
		body["tools"] = tools
		body["tool_choice"] = "auto"
	}
	encoded, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/chat/completions", bytes.NewReader(encoded))
	if err != nil {
		return dashboardChatCompletionsStreamRound{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := dashboardLLMHTTPClient(0).Do(req)
	if err != nil {
		return dashboardChatCompletionsStreamRound{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return dashboardChatCompletionsStreamRound{}, &httpError{Status: resp.StatusCode, Body: string(raw)}
	}
	return parseChatCompletionsStream(resp.Body, model, callbacks)
}

func parseChatCompletionsStream(body io.Reader, model string, callbacks dashboardChatCompletionsStreamCallbacks) (dashboardChatCompletionsStreamRound, error) {
	var result dashboardChatCompletionsStreamRound
	var content strings.Builder
	toolsByIndex := map[int]*dashboardChatCompletionsToolState{}
	completed := false

	process := func(raw string) {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			return
		}
		if raw == "[DONE]" {
			completed = true
			return
		}
		var chunk struct {
			Usage *struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			} `json:"usage"`
			Choices []struct {
				Delta struct {
					Content   *string `json:"content"`
					ToolCalls []struct {
						Index    int     `json:"index"`
						ID       *string `json:"id"`
						Function *struct {
							Name      *string `json:"name"`
							Arguments *string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason *string `json:"finish_reason"`
			} `json:"choices"`
		}
		if json.Unmarshal([]byte(raw), &chunk) != nil {
			return
		}
		if chunk.Usage != nil {
			result.Usage = dashboardChatTokenUsage{InputTokens: chunk.Usage.PromptTokens, OutputTokens: chunk.Usage.CompletionTokens, TotalTokens: chunk.Usage.TotalTokens}
			result.Usage.normalize()
		}
		if len(chunk.Choices) == 0 {
			return
		}
		if chunk.Choices[0].FinishReason != nil && strings.TrimSpace(*chunk.Choices[0].FinishReason) != "" {
			completed = true
		}
		delta := chunk.Choices[0].Delta
		if delta.Content != nil && *delta.Content != "" {
			result.Progressed = true
			content.WriteString(*delta.Content)
			if callbacks.OnText != nil {
				callbacks.OnText(*delta.Content)
			}
		}
		for _, tc := range delta.ToolCalls {
			state := toolsByIndex[tc.Index]
			if state == nil {
				state = &dashboardChatCompletionsToolState{}
				toolsByIndex[tc.Index] = state
			}
			if tc.ID != nil && *tc.ID != "" {
				state.id = *tc.ID
			}
			if tc.Function != nil {
				if tc.Function.Name != nil && *tc.Function.Name != "" {
					state.name = *tc.Function.Name
				}
				if tc.Function.Arguments != nil {
					state.arguments += *tc.Function.Arguments
				}
			}
			result.Progressed = true
			if callbacks.OnToolDelta != nil {
				callbacks.OnToolDelta(tc.Index, state.id, state.name, state.arguments)
			}
		}
	}

	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	dataLines := make([]string, 0, 2)
	flushFrame := func() {
		if len(dataLines) == 0 {
			return
		}
		process(strings.Join(dataLines, "\n"))
		dataLines = dataLines[:0]
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			flushFrame()
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	flushFrame()
	result.Content = content.String()
	scanErr := scanner.Err()

	indexes := make([]int, 0, len(toolsByIndex))
	for index := range toolsByIndex {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	for _, index := range indexes {
		state := toolsByIndex[index]
		if state == nil || strings.TrimSpace(state.name) == "" {
			continue
		}
		id := strings.TrimSpace(state.id)
		if id == "" {
			id = "call_" + strings.TrimSpace(model) + "_" + time.Now().Format("150405.000000000")
		}
		result.ToolCalls = append(result.ToolCalls, llmToolCall{ID: id, Name: state.name, Arguments: state.arguments})
	}
	if scanErr != nil {
		return result, scanErr
	}
	if !completed {
		return result, io.ErrUnexpectedEOF
	}
	return result, nil
}

func dashboardStreamChatCompletionsRoundWithRetry(ctx context.Context, baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any, callbacks dashboardChatCompletionsStreamCallbacks) (dashboardChatCompletionsStreamRound, error) {
	var last dashboardChatCompletionsStreamRound
	var lastErr error
	delay := 250 * time.Millisecond
	for attempt := 0; attempt < dashboardLLMRetryAttempts; attempt++ {
		round, err := callChatCompletionsStreamWithTools(ctx, baseURL, apiKey, model, messages, tools, callbacks)
		last = round
		if err == nil {
			return round, nil
		}
		lastErr = err
		if round.Progressed || !dashboardIsTransientLLMError(err) || attempt == dashboardLLMRetryAttempts-1 {
			break
		}
		timer := time.NewTimer(delay)
		select {
		case <-ctx.Done():
			timer.Stop()
			return last, ctx.Err()
		case <-timer.C:
		}
		delay *= 2
	}
	return last, lastErr
}

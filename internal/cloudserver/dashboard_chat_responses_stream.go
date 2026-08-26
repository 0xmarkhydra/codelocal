package cloudserver

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

type dashboardResponsesStreamCallbacks struct {
	OnText      func(string)
	OnToolDelta func(index int, id, name, arguments string)
}

type dashboardResponsesStreamRound struct {
	ToolCalls  []llmToolCall
	Content    string
	Progressed bool
}

type dashboardResponsesStreamToolState struct {
	order       int
	outputIndex int
	itemID      string
	callID      string
	name        string
	arguments   string
}

type dashboardResponsesStreamEvent struct {
	Type        string `json:"type"`
	Delta       string `json:"delta"`
	Text        string `json:"text"`
	ItemID      string `json:"item_id"`
	OutputIndex int    `json:"output_index"`
	Name        string `json:"name"`
	Arguments   string `json:"arguments"`
	Message     string `json:"message"`
	Item        *struct {
		ID        string `json:"id"`
		Type      string `json:"type"`
		CallID    string `json:"call_id"`
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"item"`
	Response *struct {
		Error *struct {
			Message string `json:"message"`
		} `json:"error"`
		IncompleteDetails *struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
	} `json:"response"`
}

func callResponsesStreamWithTools(ctx context.Context, baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any, callbacks dashboardResponsesStreamCallbacks) (dashboardResponsesStreamRound, error) {
	body := map[string]any{"model": model, "input": responsesInput(messages), "stream": true}
	if converted := responsesTools(tools); len(converted) > 0 {
		body["tools"] = converted
		body["tool_choice"] = "auto"
	}
	encoded, _ := json.Marshal(body)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/responses", bytes.NewReader(encoded))
	if err != nil {
		return dashboardResponsesStreamRound{}, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "text/event-stream")
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := (&http.Client{Timeout: 0}).Do(req)
	if err != nil {
		return dashboardResponsesStreamRound{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
		return dashboardResponsesStreamRound{}, &httpError{Status: resp.StatusCode, Body: string(raw)}
	}

	var result dashboardResponsesStreamRound
	var content strings.Builder
	byItem := map[string]*dashboardResponsesStreamToolState{}
	byIndex := map[int]*dashboardResponsesStreamToolState{}
	ordered := make([]*dashboardResponsesStreamToolState, 0, 4)
	textDeltaSeen := false

	ensureTool := func(outputIndex int, itemID string) *dashboardResponsesStreamToolState {
		if itemID != "" {
			if state := byItem[itemID]; state != nil {
				return state
			}
		}
		if state := byIndex[outputIndex]; state != nil {
			if itemID != "" {
				state.itemID = itemID
				byItem[itemID] = state
			}
			return state
		}
		state := &dashboardResponsesStreamToolState{order: len(ordered), outputIndex: outputIndex, itemID: itemID}
		ordered = append(ordered, state)
		byIndex[outputIndex] = state
		if itemID != "" {
			byItem[itemID] = state
		}
		return state
	}

	emitTool := func(state *dashboardResponsesStreamToolState) {
		if callbacks.OnToolDelta == nil || state == nil {
			return
		}
		id := state.callID
		if id == "" {
			id = state.itemID
		}
		callbacks.OnToolDelta(state.order, id, state.name, state.arguments)
	}

	process := func(raw string) error {
		raw = strings.TrimSpace(raw)
		if raw == "" || raw == "[DONE]" {
			return nil
		}
		var event dashboardResponsesStreamEvent
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil
		}
		switch event.Type {
		case "response.output_text.delta":
			if event.Delta == "" {
				return nil
			}
			textDeltaSeen = true
			result.Progressed = true
			content.WriteString(event.Delta)
			if callbacks.OnText != nil {
				callbacks.OnText(event.Delta)
			}
		case "response.output_text.done":
			if !textDeltaSeen && event.Text != "" {
				result.Progressed = true
				content.WriteString(event.Text)
				if callbacks.OnText != nil {
					callbacks.OnText(event.Text)
				}
			}
		case "response.output_item.added", "response.output_item.done":
			if event.Item == nil || event.Item.Type != "function_call" {
				return nil
			}
			state := ensureTool(event.OutputIndex, event.Item.ID)
			if event.Item.CallID != "" {
				state.callID = event.Item.CallID
			}
			if event.Item.Name != "" {
				state.name = event.Item.Name
			}
			if event.Item.Arguments != "" {
				state.arguments = event.Item.Arguments
			}
			result.Progressed = true
			emitTool(state)
		case "response.function_call_arguments.delta":
			state := ensureTool(event.OutputIndex, event.ItemID)
			state.arguments += event.Delta
			result.Progressed = true
			emitTool(state)
		case "response.function_call_arguments.done":
			state := ensureTool(event.OutputIndex, event.ItemID)
			if event.Name != "" {
				state.name = event.Name
			}
			if event.Arguments != "" {
				state.arguments = event.Arguments
			}
			result.Progressed = true
			emitTool(state)
		case "error":
			if event.Message == "" {
				event.Message = "responses stream error"
			}
			return errors.New(event.Message)
		case "response.failed":
			message := "responses stream failed"
			if event.Response != nil && event.Response.Error != nil && event.Response.Error.Message != "" {
				message = event.Response.Error.Message
			}
			return errors.New(message)
		case "response.incomplete":
			message := "responses stream incomplete"
			if event.Response != nil && event.Response.IncompleteDetails != nil && event.Response.IncompleteDetails.Reason != "" {
				message += ": " + event.Response.IncompleteDetails.Reason
			}
			return errors.New(message)
		}
		return nil
	}

	scanner := bufio.NewScanner(resp.Body)
	scanner.Buffer(make([]byte, 64*1024), 4<<20)
	dataLines := make([]string, 0, 2)
	flushFrame := func() error {
		if len(dataLines) == 0 {
			return nil
		}
		raw := strings.Join(dataLines, "\n")
		dataLines = dataLines[:0]
		return process(raw)
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flushFrame(); err != nil {
				result.Content = content.String()
				return result, err
			}
			continue
		}
		if strings.HasPrefix(line, "data:") {
			dataLines = append(dataLines, strings.TrimSpace(strings.TrimPrefix(line, "data:")))
		}
	}
	if err := flushFrame(); err != nil {
		result.Content = content.String()
		return result, err
	}
	if err := scanner.Err(); err != nil {
		result.Content = content.String()
		return result, err
	}

	for _, state := range ordered {
		if strings.TrimSpace(state.name) == "" {
			continue
		}
		id := state.callID
		if id == "" {
			id = state.itemID
		}
		if id == "" {
			id = fmt.Sprintf("call_%d", state.order)
		}
		result.ToolCalls = append(result.ToolCalls, llmToolCall{ID: id, Name: state.name, Arguments: state.arguments})
	}
	result.Content = content.String()
	return result, nil
}

func dashboardStreamResponsesRoundWithRetry(ctx context.Context, baseURL, apiKey, model string, messages []map[string]any, tools []map[string]any, callbacks dashboardResponsesStreamCallbacks) (dashboardResponsesStreamRound, error) {
	var last dashboardResponsesStreamRound
	var lastErr error
	delay := 250 * time.Millisecond
	for attempt := 0; attempt < dashboardLLMRetryAttempts; attempt++ {
		round, err := callResponsesStreamWithTools(ctx, baseURL, apiKey, model, messages, tools, callbacks)
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

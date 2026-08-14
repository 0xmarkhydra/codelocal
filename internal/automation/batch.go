package automation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	maxComputerSequenceSteps = 12
	computerStepRetryWindow  = 2 * time.Second
	computerStepRetryDelay   = 50 * time.Millisecond
)

type computerSequenceStep struct {
	Operation string
	Target    string
	Text      string
}

func parseComputerSequence(args map[string]any) (string, []computerSequenceStep, error) {
	windowID := stringArg(args, "windowId")
	if windowID == "" || windowID == "screen:main" {
		return "", nil, errors.New("computer run requires one application windowId; full-screen batches are not allowed")
	}
	rawSteps, ok := args["steps"].([]any)
	if !ok || len(rawSteps) == 0 {
		return "", nil, errors.New("computer run requires at least one step")
	}
	if len(rawSteps) > maxComputerSequenceSteps {
		return "", nil, fmt.Errorf("computer run supports at most %d steps", maxComputerSequenceSteps)
	}
	steps := make([]computerSequenceStep, 0, len(rawSteps))
	for index, raw := range rawSteps {
		entry, ok := raw.(map[string]any)
		if !ok {
			return "", nil, fmt.Errorf("computer run step %d must be an object", index+1)
		}
		operation := strings.ToLower(strings.TrimSpace(firstNonEmpty(stringArg(entry, "action"), stringArg(entry, "operation"))))
		if operation != "click" && operation != "type" {
			return "", nil, fmt.Errorf("computer run step %d only supports semantic click/type", index+1)
		}
		if override := stringArg(entry, "windowId"); override != "" && override != windowID {
			return "", nil, fmt.Errorf("computer run step %d cannot switch windows", index+1)
		}
		target := firstNonEmpty(stringArg(entry, "target"), stringArg(entry, "description"))
		if target == "" {
			return "", nil, fmt.Errorf("computer run step %d requires a semantic target", index+1)
		}
		text := ""
		if operation == "type" {
			value, exists := entry["text"]
			if !exists {
				return "", nil, fmt.Errorf("computer run step %d type requires text", index+1)
			}
			var textOK bool
			text, textOK = value.(string)
			if !textOK {
				return "", nil, fmt.Errorf("computer run step %d text must be a string", index+1)
			}
		}
		steps = append(steps, computerSequenceStep{Operation: operation, Target: target, Text: text})
	}
	return windowID, steps, nil
}

func sequenceApprovalAction(windowID string, steps []computerSequenceStep) Action {
	targets := make([]string, 0, len(steps))
	texts := make([]string, 0, len(steps))
	for _, step := range steps {
		targets = append(targets, step.Operation+":"+step.Target)
		if step.Text != "" {
			texts = append(texts, step.Text)
		}
	}
	return Action{
		Domain:    "computer",
		Operation: "run",
		Origin:    windowID,
		Target:    strings.Join(targets, " -> "),
		Text:      strings.Join(texts, "\x00"),
	}
}

func retrySemanticAction(ctx context.Context, computer *ComputerController, step computerSequenceStep, windowID string) (any, error) {
	deadline := time.Now().Add(computerStepRetryWindow)
	var lastErr error
	for {
		result, err := computer.SemanticAction(ctx, step.Operation, windowID, step.Target, step.Text)
		if err == nil {
			return result, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(computerStepRetryDelay):
		}
	}
}

func (c *Controller) runComputerSequence(ctx context.Context, args map[string]any) (any, error) {
	if c == nil || c.Computer == nil {
		return nil, errors.New("Computer Use is enabled but a compatible native helper is not available on this CodeLocal build")
	}
	windowID, steps, err := parseComputerSequence(args)
	if err != nil {
		return nil, err
	}
	approved, state, err := c.authorize(sequenceApprovalAction(windowID, steps), args)
	if err != nil || !approved {
		return state, err
	}

	results := make([]any, 0, len(steps))
	started := time.Now()
	for index, step := range steps {
		result, stepErr := retrySemanticAction(ctx, c.Computer, step, windowID)
		if stepErr != nil {
			return map[string]any{
				"ok":            false,
				"background":    true,
				"physicalInput": false,
				"windowId":      windowID,
				"completed":     index,
				"failedStep":    index + 1,
				"error":         stepErr.Error(),
				"results":       results,
				"durationMs":    time.Since(started).Milliseconds(),
			}, nil
		}
		results = append(results, result)
	}

	envelope := map[string]any{
		"ok":            true,
		"background":    true,
		"physicalInput": false,
		"windowId":      windowID,
		"completed":     len(steps),
		"results":       results,
		"durationMs":    time.Since(started).Milliseconds(),
	}
	if boolArg(args, "verify", false) {
		observation, observeErr := ObserveComputer(ctx, c.Computer, windowID)
		if observeErr != nil {
			envelope["verificationError"] = observeErr.Error()
		} else {
			envelope["observation"] = observation
		}
	}
	return envelope, nil
}

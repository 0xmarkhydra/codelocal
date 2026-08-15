package automation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

const maxComputerSequenceSteps = 12

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

type computerSequenceAction func(context.Context, computerSequenceStep, string) (any, error)

func executeComputerSequence(ctx context.Context, windowID string, steps []computerSequenceStep, action computerSequenceAction) ([]any, error) {
	results := make([]any, 0, len(steps))
	for index, step := range steps {
		result, err := action(ctx, step, windowID)
		if err != nil {
			return results, fmt.Errorf("computer run step %d %s target %q failed after %d completed step(s): %w", index+1, step.Operation, step.Target, index, err)
		}
		results = append(results, result)
	}
	return results, nil
}

func (c *Controller) runComputerSequence(ctx context.Context, args map[string]any, sessionID string) (any, error) {
	if c == nil || c.Computer == nil {
		return nil, errors.New("Computer Use is enabled but a compatible native helper is not available on this CodeLocal build")
	}
	windowID, steps, err := parseComputerSequence(args)
	if err != nil {
		return nil, err
	}
	approved, state, err := c.authorize(sessionID, sequenceApprovalAction(windowID, steps), args)
	if err != nil || !approved {
		return state, err
	}

	started := time.Now()
	results, stepErr := executeComputerSequence(ctx, windowID, steps, func(ctx context.Context, step computerSequenceStep, windowID string) (any, error) {
		return c.Computer.SemanticAction(ctx, step.Operation, windowID, step.Target, step.Text)
	})
	if stepErr != nil {
		return nil, stepErr
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

package automation

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var physicalComputerInputMu sync.Mutex

type Controller struct {
	WorkspaceID  string
	WorkspaceKey string
	Root         string
	Browser      *BrowserController
	Computer     *ComputerController
	Authorizer   *Authorizer
}

func NewController(workspaceID, workspaceKey, root string) *Controller {
	controller := &Controller{WorkspaceID: workspaceID, WorkspaceKey: workspaceKey, Root: root, Authorizer: NewAuthorizer(workspaceKey)}
	if browser, err := NewBrowserController(workspaceID, workspaceKey, root); err == nil {
		controller.Browser = browser
	}
	if computer, err := NewComputerController(workspaceID, workspaceKey, root); err == nil {
		controller.Computer = computer
	}
	return controller
}

func IsTool(tool string) bool {
	return strings.HasPrefix(tool, "browser_") || strings.HasPrefix(tool, "computer_")
}

func (c *Controller) Capabilities() map[string]any {
	enabled, prepared := BrowserConfigured()
	browser := map[string]any{
		"available":       c != nil && c.Browser != nil,
		"enabled":         enabled,
		"prepared":        prepared,
		"isolatedProfile": true,
		"screenshots":     c != nil && c.Browser != nil,
		"devtools":        c != nil && c.Browser != nil,
		"attachExisting":  false,
	}
	computer := ComputerCapabilities()
	if c != nil && c.Computer != nil {
		computer["agentCursor"] = c.Computer.AgentCursorSupported()
	} else {
		computer["agentCursor"] = false
	}
	return map[string]any{"browser": browser, "computer": computer}
}

func stringArg(args map[string]any, name string) string {
	value, _ := args[name].(string)
	return strings.TrimSpace(value)
}

func boolArg(args map[string]any, name string, fallback bool) bool {
	value, ok := args[name].(bool)
	if !ok {
		return fallback
	}
	return value
}

func (c *Controller) authorize(action Action, args map[string]any) (bool, any, error) {
	approved, state, err := c.Authorizer.Authorize(action, stringArg(args, "approvalToken"))
	if err != nil || !approved {
		return approved, state, err
	}
	return true, state, nil
}

func (c *Controller) browserOrigin() string {
	if c == nil || c.Browser == nil {
		return ""
	}
	return c.Browser.CurrentOrigin()
}

func (c *Controller) browserActionResult(ctx context.Context, result any, verify bool) (any, error) {
	if !verify {
		return result, nil
	}
	observation, err := c.Browser.Snapshot(ctx)
	if err != nil {
		return map[string]any{"result": result, "verificationError": err.Error()}, nil
	}
	return map[string]any{"result": result, "observation": observation}, nil
}

func cursorVisible(value any) bool {
	entry, ok := value.(map[string]any)
	if !ok {
		return false
	}
	visible, _ := entry["visible"].(bool)
	return visible
}

func computerActionUsesPhysicalInput(operation string, args map[string]any, target string) bool {
	if strings.EqualFold(stringArg(args, "windowId"), "screen:main") {
		switch operation {
		case "click", "type", "key", "scroll", "drag":
			return true
		}
	}
	elementID := stringArg(args, "elementId")
	switch operation {
	case "click":
		return strings.HasPrefix(elementID, "vision:") || (elementID == "" && strings.TrimSpace(target) == "")
	case "type":
		return strings.TrimSpace(target) == ""
	case "key", "scroll", "drag":
		return true
	default:
		return false
	}
}

func (c *Controller) Handle(ctx context.Context, tool string, args map[string]any) (any, error) {
	if c == nil {
		return nil, errors.New("automation controller unavailable")
	}
	if args == nil {
		args = map[string]any{}
	}
	switch tool {
	case "browser_status":
		if c.Browser == nil {
			enabled, prepared := BrowserConfigured()
			return map[string]any{"available": false, "enabled": enabled, "prepared": prepared, "backend": "playwright-cli"}, nil
		}
		return c.Browser.Status(), nil
	case "browser_open":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		target := stringArg(args, "url")
		approved, state, err := c.authorize(Action{Domain: "browser", Operation: "open", Origin: target, Target: target}, args)
		if err != nil || !approved {
			return state, err
		}
		return c.Browser.Open(ctx, target, boolArg(args, "headed", true))
	case "browser_snapshot":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		return c.Browser.Snapshot(ctx)
	case "browser_find":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		return c.Browser.Find(ctx, stringArg(args, "query"))
	case "browser_click":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		ref := stringArg(args, "ref")
		approved, state, err := c.authorize(Action{Domain: "browser", Operation: "click", Origin: c.browserOrigin(), Target: firstNonEmpty(stringArg(args, "description"), ref)}, args)
		if err != nil || !approved {
			return state, err
		}
		result, err := c.Browser.Click(ctx, ref)
		if err != nil {
			return nil, err
		}
		return c.browserActionResult(ctx, result, boolArg(args, "verify", false))
	case "browser_fill":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		ref := stringArg(args, "ref")
		text := stringArg(args, "text")
		approved, state, err := c.authorize(Action{Domain: "browser", Operation: "fill", Origin: c.browserOrigin(), Target: firstNonEmpty(stringArg(args, "description"), ref), Text: text}, args)
		if err != nil || !approved {
			return state, err
		}
		result, err := c.Browser.Fill(ctx, ref, text)
		if err != nil {
			return nil, err
		}
		return c.browserActionResult(ctx, result, boolArg(args, "verify", false))
	case "browser_press":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		key := stringArg(args, "key")
		approved, state, err := c.authorize(Action{Domain: "browser", Operation: "press", Origin: c.browserOrigin(), Target: stringArg(args, "description"), Text: key}, args)
		if err != nil || !approved {
			return state, err
		}
		result, err := c.Browser.Press(ctx, key)
		if err != nil {
			return nil, err
		}
		return c.browserActionResult(ctx, result, boolArg(args, "verify", false))
	case "browser_console":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		return c.Browser.Console(ctx, stringArg(args, "level"))
	case "browser_requests":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		return c.Browser.Requests(ctx)
	case "browser_screenshot":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		approved, state, err := c.authorize(Action{Domain: "browser", Operation: "screenshot", Origin: c.browserOrigin(), Target: stringArg(args, "description")}, args)
		if err != nil || !approved {
			return state, err
		}
		return c.Browser.Screenshot(ctx, stringArg(args, "ref"))
	case "browser_close":
		if c.Browser == nil {
			return map[string]any{"closed": true, "alreadyStopped": true}, nil
		}
		approved, state, err := c.authorize(Action{Domain: "browser", Operation: "close", Origin: c.browserOrigin()}, args)
		if err != nil || !approved {
			return state, err
		}
		return c.Browser.Close(ctx)
	case "computer_status":
		status := ComputerCapabilities()
		if c.Computer != nil {
			status["agentCursor"] = c.Computer.AgentCursorSupported()
		} else {
			status["agentCursor"] = false
		}
		return status, nil
	case "computer_observe":
		if c.Computer == nil {
			return nil, errors.New("Computer Use is enabled but a compatible native helper is not available on this CodeLocal build")
		}
		approved, state, err := c.authorize(Action{Domain: "computer", Operation: "observe", Origin: stringArg(args, "windowId"), Target: stringArg(args, "description")}, args)
		if err != nil || !approved {
			return state, err
		}
		return ObserveComputer(ctx, c.Computer, stringArg(args, "windowId"))
	case "computer_run":
		return c.runComputerSequence(ctx, args)
	case "computer_list_windows", "computer_ui_tree", "computer_screenshot", "computer_focus", "computer_click", "computer_type", "computer_key", "computer_scroll", "computer_drag":
		if c.Computer == nil {
			return nil, errors.New("Computer Use is enabled but a compatible native helper is not available on this CodeLocal build")
		}
		op := strings.TrimPrefix(tool, "computer_")
		target := firstNonEmpty(stringArg(args, "target"), stringArg(args, "description"))
		action := Action{
			Domain: "computer", Operation: op, Origin: stringArg(args, "windowId"), Target: target, Text: stringArg(args, "text"),
			Physical: computerActionUsesPhysicalInput(op, args, target),
		}
		approved, state, err := c.authorize(action, args)
		if err != nil || !approved {
			return state, err
		}
		physicalInput := action.Physical

		verify := boolArg(args, "verify", false)
		windowID := stringArg(args, "windowId")

		// Fast path: resolve + interact inside the local native helper. AX-backed
		// actions do not move the user's physical cursor and avoid a separate
		// observe/resolve round-trip. Custom-rendered apps fall through to the
		// existing Vision/coordinate recovery path.
		if (op == "click" || op == "type") && stringArg(args, "elementId") == "" && target != "" && windowID != "" && windowID != "screen:main" {
			fastStarted := time.Now()
			fast, fastErr := c.Computer.SemanticAction(ctx, op, windowID, target, stringArg(args, "text"))
			if fastErr == nil {
				envelope := map[string]any{"result": fast, "background": true, "physicalInput": false, "durationMs": time.Since(fastStarted).Milliseconds()}
				if root, ok := fast.(map[string]any); ok {
					if resolvedTarget, ok := root["resolvedTarget"].(map[string]any); ok {
						envelope["resolvedTarget"] = resolvedTarget
						if op == "click" && c.Computer.AgentCursorSupported() {
							if elementID, _ := resolvedTarget["elementId"].(string); strings.TrimSpace(elementID) != "" {
								if cursor, cursorErr := c.Computer.AgentCursor(ctx, map[string]any{"elementId": elementID}); cursorErr == nil && cursorVisible(cursor) {
									envelope["agentCursor"] = cursor
								}
							}
						}
					}
				}
				if verify {
					observation, observeErr := ObserveComputer(ctx, c.Computer, windowID)
					if observeErr != nil {
						envelope["verificationError"] = observeErr.Error()
					} else {
						envelope["observation"] = observation
					}
				}
				return envelope, nil
			}
			if op == "type" {
				// A semantic type request promises background control. Falling back
				// to a global keystroke could type into whatever app the user is
				// actively using, so require an explicit physical action instead.
				return nil, fmt.Errorf("background semantic type failed for %q: %w", target, fastErr)
			}
		}

		resolved := map[string]any(nil)
		if op == "click" && stringArg(args, "elementId") == "" && target != "" {
			resolved, err = FindComputerElement(ctx, c.Computer, windowID, target)
			if err != nil {
				return nil, err
			}
			args["elementId"] = resolved["elementId"]
		}
		if op == "click" && strings.HasPrefix(stringArg(args, "elementId"), "vision:") && !action.Physical {
			physicalAction := action
			physicalAction.Physical = true
			approved, state, authErr := c.authorize(physicalAction, args)
			if authErr != nil || !approved {
				return state, authErr
			}
			physicalInput = true
		}

		var agentCursor any
		if op == "click" && c.Computer.AgentCursorSupported() {
			if elementID := stringArg(args, "elementId"); elementID != "" {
				// Visual feedback is best-effort. Cursor rendering must never turn a
				// valid approved input action into a failure.
				agentCursor, _ = c.Computer.AgentCursor(ctx, map[string]any{"elementId": elementID})
			}
		}

		if physicalInput {
			physicalComputerInputMu.Lock()
			defer physicalComputerInputMu.Unlock()
		}
		result, err := c.Computer.Call(ctx, op, args)
		if err != nil {
			return nil, err
		}
		if !verify {
			if resolved == nil && !cursorVisible(agentCursor) {
				return result, nil
			}
			envelope := map[string]any{"result": result}
			if resolved != nil {
				envelope["resolvedTarget"] = resolved
			}
			if cursorVisible(agentCursor) {
				envelope["agentCursor"] = agentCursor
			}
			return envelope, nil
		}
		observation, observeErr := ObserveComputer(ctx, c.Computer, stringArg(args, "windowId"))
		envelope := map[string]any{"result": result, "observation": observation}
		if resolved != nil {
			envelope["resolvedTarget"] = resolved
		}
		if cursorVisible(agentCursor) {
			envelope["agentCursor"] = agentCursor
		}
		if observeErr != nil {
			delete(envelope, "observation")
			envelope["verificationError"] = observeErr.Error()
		}
		return envelope, nil
	default:
		return nil, errors.New("unsupported automation tool: " + tool)
	}
}

func (c *Controller) Close() {
	if c == nil {
		return
	}
	if c.Browser != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
		_, _ = c.Browser.Close(ctx)
		cancel()
	}
	if c.Computer != nil {
		c.Computer.Close()
	}
}

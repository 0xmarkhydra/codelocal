package automation

import (
	"context"
	"errors"
	"strings"
	"time"
)

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
		return c.Browser.Click(ctx, ref)
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
		return c.Browser.Fill(ctx, ref, text)
	case "browser_press":
		if c.Browser == nil {
			return nil, errors.New("Browser Automation is not available on this CodeLocal runtime")
		}
		key := stringArg(args, "key")
		approved, state, err := c.authorize(Action{Domain: "browser", Operation: "press", Origin: c.browserOrigin(), Target: stringArg(args, "description"), Text: key}, args)
		if err != nil || !approved {
			return state, err
		}
		return c.Browser.Press(ctx, key)
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
		return ComputerCapabilities(), nil
	case "computer_list_windows", "computer_ui_tree", "computer_screenshot", "computer_focus", "computer_click", "computer_type", "computer_key", "computer_scroll", "computer_drag":
		if c.Computer == nil {
			return nil, errors.New("Computer Use is enabled but a compatible native helper is not available on this CodeLocal build")
		}
		op := strings.TrimPrefix(tool, "computer_")
		action := Action{Domain: "computer", Operation: op, Target: stringArg(args, "description"), Text: stringArg(args, "text")}
		approved, state, err := c.authorize(action, args)
		if err != nil || !approved {
			return state, err
		}
		return c.Computer.Call(ctx, op, args)
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

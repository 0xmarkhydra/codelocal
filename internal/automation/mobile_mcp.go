package automation

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/modelcontextprotocol/go-sdk/mcp"
)

const mobileMCPVersion = "1.0.2"

type MobileController struct {
	Root string

	mu      sync.Mutex
	session *mcp.ClientSession
}

func mobileMCPCommand() (string, []string, bool) {
	if configured := strings.TrimSpace(os.Getenv("CODELOCAL_MOBILE_MCP_CLI")); configured != "" {
		if info, err := os.Stat(configured); err == nil && !info.IsDir() {
			return configured, nil, true
		}
	}
	root := strings.TrimSpace(os.Getenv("CODELOCAL_PACKAGE_ROOT"))
	if root == "" {
		return "", nil, false
	}
	entry := filepath.Join(root, "node_modules", "@mobilenext", "mobile-mcp", "lib", "index.js")
	if info, err := os.Stat(entry); err != nil || info.IsDir() {
		return "", nil, false
	}
	node, err := exec.LookPath("node")
	if err != nil {
		return "", nil, false
	}
	return node, []string{entry}, true
}

func mobileMCPEnv() []string {
	env := os.Environ()
	filtered := env[:0]
	for _, entry := range env {
		if strings.HasPrefix(entry, "MOBILEMCP_DISABLE_TELEMETRY=") || strings.HasPrefix(entry, "MOBILEMCP_ALLOW_UNSAFE_URLS=") || strings.HasPrefix(entry, "MOBILEMCP_AUTH=") {
			continue
		}
		filtered = append(filtered, entry)
	}
	return append(filtered, "MOBILEMCP_DISABLE_TELEMETRY=1", "MOBILEMCP_ALLOW_UNSAFE_URLS=0")
}

func MobileCapabilities() map[string]any {
	settings, _ := Load()
	enabled := settings != nil && settings.Computer.Enabled
	_, _, ready := mobileMCPCommand()
	available := enabled && ready
	return map[string]any{
		"available":     available,
		"enabled":       enabled,
		"managed":       true,
		"backend":       "mobile-mcp",
		"version":       mobileMCPVersion,
		"ios":           available,
		"android":       available,
		"deviceList":    available,
		"screenCapture": available,
		"uiTree":        available,
		"pointer":       available,
		"keyboard":      available,
		"appLifecycle":  available,
		"openURL":       available,
		"orientation":   available,
		"recording":     available,
		"crashReports":  available,
	}
}

func NewMobileController(root string) (*MobileController, error) {
	settings, err := Load()
	if err != nil {
		return nil, err
	}
	if settings == nil || !settings.Computer.Enabled {
		return nil, errors.New("Computer Use is disabled")
	}
	if _, _, ready := mobileMCPCommand(); !ready {
		return nil, errors.New("managed Mobile MCP backend is not packaged on this CodeLocal install")
	}
	return &MobileController{Root: root}, nil
}

func (c *MobileController) connect(ctx context.Context) (*mcp.ClientSession, error) {
	if c == nil {
		return nil, errors.New("Mobile MCP controller unavailable")
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		return c.session, nil
	}
	command, args, ok := mobileMCPCommand()
	if !ok {
		return nil, errors.New("managed Mobile MCP backend is unavailable; update CodeLocal to a package that includes @mobilenext/mobile-mcp")
	}
	cmd := exec.Command(command, args...)
	cmd.Dir = c.Root
	cmd.Env = mobileMCPEnv()
	client := mcp.NewClient(&mcp.Implementation{Name: "codelocal-mobile-backend", Version: mobileMCPVersion}, nil)
	connectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	session, err := client.Connect(connectCtx, &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		return nil, fmt.Errorf("start managed Mobile MCP backend: %w", err)
	}
	c.session = session
	return session, nil
}

func mobileResultText(result *mcp.CallToolResult) string {
	if result == nil {
		return ""
	}
	parts := []string{}
	for _, item := range result.Content {
		if text, ok := item.(*mcp.TextContent); ok && strings.TrimSpace(text.Text) != "" {
			parts = append(parts, strings.TrimSpace(text.Text))
		}
	}
	return strings.Join(parts, "\n")
}

func normalizeMobileResult(result *mcp.CallToolResult) (any, error) {
	if result == nil {
		return nil, errors.New("Mobile MCP returned an empty result")
	}
	text := mobileResultText(result)
	if result.IsError {
		if text == "" {
			text = "Mobile MCP operation failed"
		}
		return nil, errors.New(text)
	}
	root := map[string]any{"backend": "mobile-mcp"}
	if text != "" {
		root["text"] = text
	}
	if result.StructuredContent != nil {
		root["structured"] = result.StructuredContent
	}
	for _, item := range result.Content {
		image, ok := item.(*mcp.ImageContent)
		if !ok {
			continue
		}
		root["__mcpImage"] = map[string]any{
			"mimeType": image.MIMEType,
			"data":     base64.StdEncoding.EncodeToString(image.Data),
		}
		break
	}
	return root, nil
}

func (c *MobileController) Call(ctx context.Context, tool string, args map[string]any) (any, error) {
	session, err := c.connect(ctx)
	if err != nil {
		return nil, err
	}
	result, err := session.CallTool(ctx, &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		return nil, fmt.Errorf("Mobile MCP %s failed: %w", tool, err)
	}
	return normalizeMobileResult(result)
}

func mobileText(value any) string {
	root, _ := value.(map[string]any)
	text, _ := root["text"].(string)
	return text
}

func mobileJSONArray(text string) ([]map[string]any, error) {
	start := strings.Index(text, "[")
	end := strings.LastIndex(text, "]")
	if start < 0 || end < start {
		return nil, errors.New("Mobile MCP response did not contain a JSON array")
	}
	var values []map[string]any
	if err := json.Unmarshal([]byte(text[start:end+1]), &values); err != nil {
		return nil, fmt.Errorf("decode Mobile MCP array: %w", err)
	}
	return values, nil
}

func normalizedMobileLabel(value any) string {
	return strings.ToLower(strings.TrimSpace(fmt.Sprint(value)))
}

func mobileElementScore(element map[string]any, target string) int {
	target = normalizedMobileLabel(target)
	if target == "" {
		return 0
	}
	best := 0
	for _, field := range []string{"label", "text", "name", "identifier", "value", "type"} {
		value := normalizedMobileLabel(element[field])
		if value == "" || value == "<nil>" {
			continue
		}
		score := 0
		switch {
		case value == target:
			score = 100
		case strings.Contains(value, target):
			score = 70
		case len(value) >= 3 && strings.Contains(target, value):
			score = 40
		}
		if field == "label" || field == "text" {
			score += 5
		}
		if score > best {
			best = score
		}
	}
	return best
}

func mobileNumber(value any) (float64, bool) {
	switch number := value.(type) {
	case float64:
		return number, true
	case float32:
		return float64(number), true
	case int:
		return float64(number), true
	case int64:
		return float64(number), true
	case json.Number:
		value, err := number.Float64()
		return value, err == nil
	default:
		return 0, false
	}
}

func mobileElementCenter(element map[string]any) (float64, float64, bool) {
	coordinates, _ := element["coordinates"].(map[string]any)
	if coordinates == nil {
		return 0, 0, false
	}
	x, xOK := mobileNumber(coordinates["x"])
	y, yOK := mobileNumber(coordinates["y"])
	width, widthOK := mobileNumber(coordinates["width"])
	height, heightOK := mobileNumber(coordinates["height"])
	if !xOK || !yOK {
		return 0, 0, false
	}
	if widthOK {
		x += width / 2
	}
	if heightOK {
		y += height / 2
	}
	return x, y, true
}

func (c *MobileController) FindElement(ctx context.Context, device, target string) (map[string]any, error) {
	value, err := c.Call(ctx, "mobile_list_elements_on_screen", map[string]any{"device": device})
	if err != nil {
		return nil, err
	}
	elements, err := mobileJSONArray(mobileText(value))
	if err != nil {
		return nil, err
	}
	bestScore := 0
	var best map[string]any
	for _, element := range elements {
		if score := mobileElementScore(element, target); score > bestScore {
			bestScore = score
			best = element
		}
	}
	if best == nil || bestScore == 0 {
		return nil, fmt.Errorf("mobile element %q was not found on device %s", target, device)
	}
	x, y, ok := mobileElementCenter(best)
	if !ok {
		return nil, fmt.Errorf("mobile element %q has no usable coordinates", target)
	}
	return map[string]any{"element": best, "x": x, "y": y, "score": bestScore}, nil
}

func (c *MobileController) SemanticClick(ctx context.Context, device, target string) (any, error) {
	resolved, err := c.FindElement(ctx, device, target)
	if err != nil {
		return nil, err
	}
	result, err := c.Call(ctx, "mobile_click_on_screen_at_coordinates", map[string]any{"device": device, "x": resolved["x"], "y": resolved["y"]})
	if err != nil {
		return nil, err
	}
	return map[string]any{"result": result, "resolvedTarget": resolved, "background": true, "physicalInput": false}, nil
}

func (c *MobileController) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.session != nil {
		_ = c.session.Close()
		c.session = nil
	}
}

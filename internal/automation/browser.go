package automation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/state"
)

var browserSessionUnsafe = regexp.MustCompile(`[^A-Za-z0-9_-]+`)

type BrowserController struct {
	WorkspaceID   string
	WorkspaceKey  string
	Root          string
	Session       string
	OutputDir     string
	CLI           string
	mu            sync.Mutex
	stateMu       sync.RWMutex
	currentOrigin string
}

func NewBrowserController(workspaceID, workspaceKey, root string) (*BrowserController, error) {
	settings, err := Load()
	if err != nil {
		return nil, err
	}
	if settings == nil || !settings.Browser.Enabled {
		return nil, errors.New("Browser Automation is disabled. Run codelocal interactively to enable it.")
	}
	if !settings.Browser.Prepared {
		return nil, errors.New("Browser Automation is enabled but its managed browser is not prepared yet. Restart codelocal to retry preparation.")
	}
	cli := BrowserCLIPath()
	if cli == "" {
		return nil, errors.New("bundled Playwright CLI is unavailable")
	}
	session := browserSessionUnsafe.ReplaceAllString(workspaceID, "-")
	session = strings.Trim(session, "-")
	if session == "" {
		session = "workspace"
	}
	if len(session) > 48 {
		session = session[:48]
	}
	outputDir := filepath.Join(state.Dir(), "browser", session, "output")
	if err := os.MkdirAll(outputDir, 0o700); err != nil {
		return nil, err
	}
	return &BrowserController{WorkspaceID: workspaceID, WorkspaceKey: workspaceKey, Root: root, Session: "codelocal-" + session, OutputDir: outputDir, CLI: cli}, nil
}

func BrowserConfigured() (enabled, prepared bool) {
	settings, err := Load()
	if err != nil || settings == nil {
		return false, false
	}
	return settings.Browser.Enabled, settings.Browser.Prepared
}

func (b *BrowserController) CurrentOrigin() string {
	if b == nil {
		return ""
	}
	b.stateMu.RLock()
	defer b.stateMu.RUnlock()
	return b.currentOrigin
}

func (b *BrowserController) setCurrentOrigin(origin string) {
	b.stateMu.Lock()
	b.currentOrigin = origin
	b.stateMu.Unlock()
}

func (b *BrowserController) Status() map[string]any {
	enabled, prepared := BrowserConfigured()
	return map[string]any{
		"available":       b != nil && b.CLI != "",
		"enabled":         enabled,
		"prepared":        prepared,
		"backend":         "playwright-cli",
		"session":         b.Session,
		"workspaceScoped": true,
		"isolatedProfile": true,
		"outputDir":       b.OutputDir,
		"currentOrigin":   b.CurrentOrigin(),
		"elevatedAttach":  false,
		"arbitraryJS":     false,
	}
}

func validateBrowserURL(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("url is required")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return "", errors.New("browser URL must be an absolute http(s) URL")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("unsupported browser URL scheme: %s", u.Scheme)
	}
	if u.User != nil {
		return "", errors.New("browser URLs containing embedded credentials are blocked")
	}
	return u.String(), nil
}

func BrowserOrigin(raw string) string {
	u, err := url.Parse(raw)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Scheme + "://" + u.Host
}

func BrowserLocalURL(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || host == "127.0.0.1" || host == "::1" || host == "0.0.0.0"
}

func (b *BrowserController) run(ctx context.Context, raw bool, args ...string) (string, error) {
	if b == nil || b.CLI == "" {
		return "", errors.New("Browser Automation is unavailable")
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	commandArgs := make([]string, 0, len(args)+2)
	commandArgs = append(commandArgs, "-s="+b.Session)
	if raw {
		commandArgs = append(commandArgs, "--raw")
	}
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, b.CLI, commandArgs...)
	cmd.Dir = b.Root
	cmd.Env = browserCommandEnv(
		"PLAYWRIGHT_CLI_SESSION="+b.Session,
		"PLAYWRIGHT_MCP_OUTPUT_DIR="+b.OutputDir,
		"CI=1",
	)
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("Playwright %s failed: %s", strings.Join(args, " "), text)
	}
	return text, nil
}

func (b *BrowserController) Open(ctx context.Context, rawURL string, headed bool) (map[string]any, error) {
	target, err := validateBrowserURL(rawURL)
	if err != nil {
		return nil, err
	}
	args := []string{"open", target}
	if headed {
		args = append(args, "--headed")
	}
	output, err := b.run(ctx, true, args...)
	if err == nil {
		b.setCurrentOrigin(BrowserOrigin(target))
	}
	return map[string]any{"url": target, "origin": BrowserOrigin(target), "headed": headed, "session": b.Session, "output": output}, err
}

func (b *BrowserController) Snapshot(ctx context.Context) (map[string]any, error) {
	output, err := b.run(ctx, true, "snapshot")
	return map[string]any{"session": b.Session, "snapshot": output}, err
}

func (b *BrowserController) Find(ctx context.Context, query string) (map[string]any, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	output, err := b.run(ctx, true, "find", query)
	return map[string]any{"session": b.Session, "query": query, "matches": output}, err
}

func (b *BrowserController) Click(ctx context.Context, ref string) (map[string]any, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("ref is required")
	}
	output, err := b.run(ctx, true, "click", ref)
	return map[string]any{"session": b.Session, "ref": ref, "output": output}, err
}

func (b *BrowserController) Fill(ctx context.Context, ref, text string) (map[string]any, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("ref is required")
	}
	output, err := b.run(ctx, true, "fill", ref, text)
	return map[string]any{"session": b.Session, "ref": ref, "characters": len([]rune(text)), "output": output}, err
}

func (b *BrowserController) Press(ctx context.Context, key string) (map[string]any, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 80 {
		return nil, errors.New("valid key is required")
	}
	output, err := b.run(ctx, true, "press", key)
	return map[string]any{"session": b.Session, "key": key, "output": output}, err
}

func (b *BrowserController) Console(ctx context.Context, level string) (map[string]any, error) {
	args := []string{"console"}
	if level = strings.TrimSpace(level); level != "" {
		args = append(args, level)
	}
	output, err := b.run(ctx, true, args...)
	return map[string]any{"session": b.Session, "console": output}, err
}

func (b *BrowserController) Requests(ctx context.Context) (map[string]any, error) {
	output, err := b.run(ctx, true, "requests")
	return map[string]any{"session": b.Session, "requests": output}, err
}

func (b *BrowserController) Screenshot(ctx context.Context, ref string) (map[string]any, error) {
	filename := "shot-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".png"
	path := filepath.Join(b.OutputDir, filename)
	args := []string{"screenshot"}
	if strings.TrimSpace(ref) != "" {
		args = append(args, strings.TrimSpace(ref))
	}
	args = append(args, "--filename="+path)
	output, err := b.run(ctx, true, args...)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Playwright screenshot: %w", err)
	}
	_ = os.Remove(path)
	return map[string]any{
		"session": b.Session,
		"output":  output,
		"__mcpImage": map[string]any{
			"mimeType": "image/png",
			"data":     base64.StdEncoding.EncodeToString(data),
		},
	}, nil
}

func (b *BrowserController) Close(ctx context.Context) (map[string]any, error) {
	output, err := b.run(ctx, true, "close")
	if err == nil {
		b.setCurrentOrigin("")
	}
	return map[string]any{"session": b.Session, "closed": err == nil, "output": output}, err
}

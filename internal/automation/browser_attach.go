package automation

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"time"
)

type browserAttachment struct {
	Endpoint string
}

var browserAttachments sync.Map // map[*BrowserController]browserAttachment

func (b *BrowserController) attachment() (browserAttachment, bool) {
	if b == nil {
		return browserAttachment{}, false
	}
	value, ok := browserAttachments.Load(b)
	if !ok {
		return browserAttachment{}, false
	}
	return value.(browserAttachment), true
}

func (b *BrowserController) AttachmentStatus() map[string]any {
	attachment, attached := b.attachment()
	return map[string]any{
		"attached": attached,
		"mode": func() string {
			if attached {
				return "existing-browser-cdp"
			}
			return "isolated"
		}(),
		"endpoint": func() string {
			if attached {
				return attachment.Endpoint
			}
			return ""
		}(),
	}
}

func validateLocalCDPEndpoint(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", errors.New("CDP endpoint is required")
	}
	parsed, err := url.Parse(raw)
	if err != nil || parsed.Host == "" {
		return "", errors.New("CDP endpoint must be an absolute http(s) URL")
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", errors.New("CDP endpoint must use http or https")
	}
	if parsed.User != nil {
		return "", errors.New("CDP endpoint credentials are not allowed")
	}
	host := strings.ToLower(parsed.Hostname())
	if host != "localhost" && host != "127.0.0.1" && host != "::1" {
		return "", errors.New("existing-browser attachment is restricted to a loopback CDP endpoint")
	}
	if parsed.Port() == "" {
		return "", errors.New("CDP endpoint must include an explicit local port")
	}
	return strings.TrimRight(parsed.String(), "/"), nil
}

func chromiumUserDataDirs() []string {
	home, _ := os.UserHomeDir()
	dirs := []string{}
	switch runtime.GOOS {
	case "darwin":
		base := filepath.Join(home, "Library", "Application Support")
		for _, relative := range []string{
			filepath.Join("Google", "Chrome"),
			filepath.Join("Google", "Chrome Canary"),
			filepath.Join("Microsoft Edge"),
			filepath.Join("BraveSoftware", "Brave-Browser"),
			"Chromium",
		} {
			dirs = append(dirs, filepath.Join(base, relative))
		}
	case "windows":
		base := strings.TrimSpace(os.Getenv("LOCALAPPDATA"))
		if base == "" {
			base = filepath.Join(home, "AppData", "Local")
		}
		for _, relative := range []string{
			filepath.Join("Google", "Chrome", "User Data"),
			filepath.Join("Google", "Chrome SxS", "User Data"),
			filepath.Join("Microsoft", "Edge", "User Data"),
			filepath.Join("BraveSoftware", "Brave-Browser", "User Data"),
			filepath.Join("Chromium", "User Data"),
		} {
			dirs = append(dirs, filepath.Join(base, relative))
		}
	case "linux":
		for _, relative := range []string{
			filepath.Join(".config", "google-chrome"),
			filepath.Join(".config", "google-chrome-beta"),
			filepath.Join(".config", "chromium"),
			filepath.Join(".config", "microsoft-edge"),
			filepath.Join(".config", "BraveSoftware", "Brave-Browser"),
		} {
			dirs = append(dirs, filepath.Join(home, relative))
		}
	}
	return dirs
}

func discoverLocalCDPEndpoint() (string, error) {
	for _, dir := range chromiumUserDataDirs() {
		raw, err := os.ReadFile(filepath.Join(dir, "DevToolsActivePort"))
		if err != nil {
			continue
		}
		lines := strings.Split(strings.TrimSpace(string(raw)), "\n")
		if len(lines) == 0 {
			continue
		}
		port, err := strconv.Atoi(strings.TrimSpace(lines[0]))
		if err != nil || port <= 0 || port > 65535 {
			continue
		}
		endpoint := "http://127.0.0.1:" + strconv.Itoa(port)
		connection, dialErr := net.DialTimeout("tcp", "127.0.0.1:"+strconv.Itoa(port), 250*time.Millisecond)
		if dialErr != nil {
			continue
		}
		_ = connection.Close()
		return endpoint, nil
	}
	return "", errors.New("no local Chromium DevToolsActivePort was found; enable remote debugging in the browser you want to attach")
}

func (b *BrowserController) runAttached(ctx context.Context, raw bool, endpoint string, args ...string) (string, error) {
	if b == nil || b.CLI == "" {
		return "", errors.New("Browser Automation is unavailable")
	}
	endpoint, err := validateLocalCDPEndpoint(endpoint)
	if err != nil {
		return "", err
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	commandArgs := make([]string, 0, len(args)+3)
	commandArgs = append(commandArgs, "-s="+b.Session, "--cdp-endpoint="+endpoint)
	if raw {
		commandArgs = append(commandArgs, "--raw")
	}
	commandArgs = append(commandArgs, args...)
	cmd := exec.CommandContext(ctx, b.CLI, commandArgs...)
	cmd.Dir = b.Root
	cmd.Env = append(os.Environ(), "PLAYWRIGHT_CLI_SESSION="+b.Session, "PLAYWRIGHT_MCP_OUTPUT_DIR="+b.OutputDir, "CI=1")
	output, err := cmd.CombinedOutput()
	text := strings.TrimSpace(string(output))
	if err != nil {
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
		if text == "" {
			text = err.Error()
		}
		return "", fmt.Errorf("Playwright CDP %s failed: %s", strings.Join(args, " "), text)
	}
	return text, nil
}

func (b *BrowserController) AttachExisting(ctx context.Context, endpoint string) (map[string]any, error) {
	if strings.TrimSpace(endpoint) == "" {
		var err error
		endpoint, err = discoverLocalCDPEndpoint()
		if err != nil {
			return nil, err
		}
	}
	validated, err := validateLocalCDPEndpoint(endpoint)
	if err != nil {
		return nil, err
	}
	// Snapshot proves that the pinned CLI can actually attach before we persist
	// elevated mode in this workspace controller.
	output, err := b.runAttached(ctx, true, validated, "snapshot")
	if err != nil {
		return nil, err
	}
	browserAttachments.Store(b, browserAttachment{Endpoint: validated})
	b.setCurrentOrigin("")
	return map[string]any{
		"attached": true,
		"mode":     "existing-browser-cdp",
		"endpoint": validated,
		"session":  b.Session,
		"snapshot": output,
		"warning":  "This workspace can now interact with pages in an existing signed-in Chromium session. Cookie/password extraction tools remain unavailable.",
	}, nil
}

func (b *BrowserController) DetachExisting(ctx context.Context) (map[string]any, error) {
	attachment, attached := b.attachment()
	if !attached {
		return map[string]any{"attached": false, "alreadyDetached": true, "mode": "isolated"}, nil
	}
	output, err := b.runAttached(ctx, true, attachment.Endpoint, "close")
	if err != nil {
		return nil, err
	}
	browserAttachments.Delete(b)
	b.setCurrentOrigin("")
	return map[string]any{"attached": false, "mode": "isolated", "output": output}, nil
}

func (b *BrowserController) effectiveRun(ctx context.Context, raw bool, args ...string) (string, error) {
	if attachment, attached := b.attachment(); attached {
		return b.runAttached(ctx, raw, attachment.Endpoint, args...)
	}
	return b.run(ctx, raw, args...)
}

func (b *BrowserController) EffectiveOpen(ctx context.Context, rawURL string, headed bool) (map[string]any, error) {
	target, err := validateBrowserURL(rawURL)
	if err != nil {
		return nil, err
	}
	args := []string{"open", target}
	if headed {
		args = append(args, "--headed")
	}
	output, err := b.effectiveRun(ctx, true, args...)
	if err == nil {
		b.setCurrentOrigin(BrowserOrigin(target))
	}
	return map[string]any{"url": target, "origin": BrowserOrigin(target), "headed": headed, "session": b.Session, "output": output, "attachment": b.AttachmentStatus()}, err
}

func (b *BrowserController) EffectiveSnapshot(ctx context.Context) (map[string]any, error) {
	output, err := b.effectiveRun(ctx, true, "snapshot")
	return map[string]any{"session": b.Session, "snapshot": output, "attachment": b.AttachmentStatus()}, err
}

func (b *BrowserController) EffectiveFind(ctx context.Context, query string) (map[string]any, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, errors.New("query is required")
	}
	output, err := b.effectiveRun(ctx, true, "find", query)
	return map[string]any{"session": b.Session, "query": query, "matches": output, "attachment": b.AttachmentStatus()}, err
}

func (b *BrowserController) EffectiveClick(ctx context.Context, ref string) (map[string]any, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("ref is required")
	}
	output, err := b.effectiveRun(ctx, true, "click", ref)
	return map[string]any{"session": b.Session, "ref": ref, "output": output, "attachment": b.AttachmentStatus()}, err
}

func (b *BrowserController) EffectiveFill(ctx context.Context, ref, text string) (map[string]any, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return nil, errors.New("ref is required")
	}
	output, err := b.effectiveRun(ctx, true, "fill", ref, text)
	return map[string]any{"session": b.Session, "ref": ref, "characters": len([]rune(text)), "output": output, "attachment": b.AttachmentStatus()}, err
}

func (b *BrowserController) EffectivePress(ctx context.Context, key string) (map[string]any, error) {
	key = strings.TrimSpace(key)
	if key == "" || len(key) > 80 {
		return nil, errors.New("valid key is required")
	}
	output, err := b.effectiveRun(ctx, true, "press", key)
	return map[string]any{"session": b.Session, "key": key, "output": output, "attachment": b.AttachmentStatus()}, err
}

func (b *BrowserController) EffectiveConsole(ctx context.Context, level string) (map[string]any, error) {
	args := []string{"console"}
	if level = strings.TrimSpace(level); level != "" {
		args = append(args, level)
	}
	output, err := b.effectiveRun(ctx, true, args...)
	return map[string]any{"session": b.Session, "console": output, "attachment": b.AttachmentStatus()}, err
}

func (b *BrowserController) EffectiveRequests(ctx context.Context) (map[string]any, error) {
	output, err := b.effectiveRun(ctx, true, "requests")
	return map[string]any{"session": b.Session, "requests": output, "attachment": b.AttachmentStatus()}, err
}

func (b *BrowserController) EffectiveScreenshot(ctx context.Context, ref string) (map[string]any, error) {
	filename := "shot-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".png"
	path := filepath.Join(b.OutputDir, filename)
	args := []string{"screenshot"}
	if strings.TrimSpace(ref) != "" {
		args = append(args, strings.TrimSpace(ref))
	}
	args = append(args, "--filename="+path)
	output, err := b.effectiveRun(ctx, true, args...)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read Playwright screenshot: %w", err)
	}
	_ = os.Remove(path)
	return map[string]any{
		"session":    b.Session,
		"output":     output,
		"attachment": b.AttachmentStatus(),
		"__mcpImage": map[string]any{"mimeType": "image/png", "data": base64.StdEncoding.EncodeToString(data)},
	}, nil
}

func (b *BrowserController) EffectiveClose(ctx context.Context) (map[string]any, error) {
	if attachment, attached := b.attachment(); attached {
		output, err := b.runAttached(ctx, true, attachment.Endpoint, "close")
		if err != nil {
			return nil, err
		}
		browserAttachments.Delete(b)
		b.setCurrentOrigin("")
		return map[string]any{"session": b.Session, "closed": true, "detached": true, "output": output}, nil
	}
	return b.Close(ctx)
}

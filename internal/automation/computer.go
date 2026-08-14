package automation

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
)

type ComputerController struct {
	WorkspaceID  string
	WorkspaceKey string
	Root         string
	Helper       string
	Backend      string
	Capabilities map[string]any
	mu           sync.Mutex
	cmd          *exec.Cmd
	stdin        io.WriteCloser
	stdout       *bufio.Reader
	stderr       bytes.Buffer
}

func computerHelperName() string {
	name := "computer-" + runtime.GOOS + "-" + runtime.GOARCH
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	return name
}

func ComputerHelperPath() string {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("CODELOCAL_COMPUTER_HELPER")); configured != "" {
		candidates = append(candidates, configured)
	}
	if root := strings.TrimSpace(os.Getenv("CODELOCAL_PACKAGE_ROOT")); root != "" {
		candidates = append(candidates,
			filepath.Join(root, "bin", "helpers", computerHelperName()),
			filepath.Join(root, "helpers", computerHelperName()),
		)
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	return ""
}

func helperCapabilities(helper string, environment Environment) map[string]any {
	base := map[string]any{
		"available":         false,
		"backend":           environment.ComputerBackend,
		"windowList":        false,
		"screenCapture":     false,
		"uiTree":            false,
		"pointer":           false,
		"keyboard":          false,
		"clipboard":         false,
		"backgroundControl": false,
		"secureDesktop":     false,
	}
	if helper == "" {
		return base
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, helper, "--capabilities")
	cmd.Env = append(os.Environ(), "CI=1")
	output, err := cmd.Output()
	if err != nil {
		return base
	}
	var reported map[string]any
	if json.Unmarshal(output, &reported) != nil {
		return base
	}
	for _, key := range []string{"available", "backend", "windowList", "screenCapture", "uiTree", "visionFallback", "pointer", "keyboard", "clipboard", "backgroundControl", "secureDesktop"} {
		if value, ok := reported[key]; ok {
			base[key] = value
		}
	}
	// Current macOS/Windows helpers expose a structured UI tree, so they can
	// necessarily enumerate target windows. The X11 helper has an explicit
	// EWMH/QueryTree implementation even though AT-SPI remains a separate flag.
	if _, explicitlyReported := reported["windowList"]; !explicitlyReported {
		uiTree, _ := base["uiTree"].(bool)
		backend, _ := base["backend"].(string)
		base["windowList"] = uiTree || strings.Contains(strings.ToLower(backend), "x11")
	}
	// CodeLocal never automates UAC / secure-desktop style surfaces even if a
	// helper accidentally claims otherwise.
	base["secureDesktop"] = false
	return base
}

func ComputerCapabilities() map[string]any {
	settings, _ := Load()
	enabled := settings != nil && settings.Computer.Enabled
	environment := Detect()
	helper := ComputerHelperPath()
	capabilities := helperCapabilities(helper, environment)
	available, _ := capabilities["available"].(bool)
	available = available && enabled && environment.ComputerSupported && helper != ""
	capabilities["available"] = available
	capabilities["enabled"] = enabled
	capabilities["helperReady"] = helper != ""
	capabilities["notes"] = environment.Notes
	if !available {
		for _, key := range []string{"windowList", "screenCapture", "uiTree", "pointer", "keyboard", "clipboard", "backgroundControl"} {
			capabilities[key] = false
		}
	}
	return capabilities
}

func NewComputerController(workspaceID, workspaceKey, root string) (*ComputerController, error) {
	settings, err := Load()
	if err != nil {
		return nil, err
	}
	if settings == nil || !settings.Computer.Enabled {
		return nil, errors.New("Computer Use is disabled")
	}
	environment := Detect()
	if !environment.ComputerSupported {
		return nil, errors.New("Computer Use is unavailable in this graphical session")
	}
	helper := ComputerHelperPath()
	if helper == "" {
		return nil, errors.New("native Computer Use helper is not packaged for this platform")
	}
	capabilities := ComputerCapabilities()
	available, _ := capabilities["available"].(bool)
	if !available {
		return nil, errors.New("native Computer Use helper is present but not ready in this graphical session")
	}
	backend, _ := capabilities["backend"].(string)
	return &ComputerController{WorkspaceID: workspaceID, WorkspaceKey: workspaceKey, Root: root, Helper: helper, Backend: backend, Capabilities: capabilities}, nil
}

func (c *ComputerController) resetProcessLocked() {
	if c.stdin != nil {
		_ = c.stdin.Close()
	}
	if c.cmd != nil && c.cmd.Process != nil {
		_ = c.cmd.Process.Kill()
		_, _ = c.cmd.Process.Wait()
	}
	c.cmd = nil
	c.stdin = nil
	c.stdout = nil
	c.stderr.Reset()
}

func (c *ComputerController) startProcessLocked() error {
	if c.cmd != nil && c.cmd.Process != nil && c.stdin != nil && c.stdout != nil {
		return nil
	}
	cmd := exec.Command(c.Helper, "--serve")
	cmd.Dir = c.Root
	cmd.Env = append(os.Environ(), "CI=1")
	stdin, err := cmd.StdinPipe()
	if err != nil {
		return err
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		_ = stdin.Close()
		return err
	}
	c.stderr.Reset()
	cmd.Stderr = &c.stderr
	if err := cmd.Start(); err != nil {
		_ = stdin.Close()
		return err
	}
	c.cmd = cmd
	c.stdin = stdin
	c.stdout = bufio.NewReaderSize(stdoutPipe, 64<<10)
	return nil
}

type computerHelperResponse struct {
	OK     bool   `json:"ok"`
	Result any    `json:"result"`
	Error  string `json:"error"`
}

func (c *ComputerController) callLocked(ctx context.Context, raw []byte) (computerHelperResponse, error) {
	var response computerHelperResponse
	if err := c.startProcessLocked(); err != nil {
		return response, err
	}
	if _, err := c.stdin.Write(append(raw, '\n')); err != nil {
		c.resetProcessLocked()
		return response, err
	}
	type readResult struct {
		line []byte
		err  error
	}
	readCh := make(chan readResult, 1)
	go func(reader *bufio.Reader) {
		line, err := reader.ReadBytes('\n')
		readCh <- readResult{line: line, err: err}
	}(c.stdout)
	select {
	case <-ctx.Done():
		c.resetProcessLocked()
		return response, ctx.Err()
	case read := <-readCh:
		if read.err != nil {
			message := strings.TrimSpace(c.stderr.String())
			c.resetProcessLocked()
			if message == "" {
				message = read.err.Error()
			}
			return response, errors.New(message)
		}
		if err := json.Unmarshal(read.line, &response); err != nil {
			c.resetProcessLocked()
			return response, fmt.Errorf("invalid Computer Use helper response: %w", err)
		}
		return response, nil
	}
}

func (c *ComputerController) Call(ctx context.Context, operation string, args map[string]any) (any, error) {
	if c == nil || c.Helper == "" {
		return nil, errors.New("Computer Use helper unavailable")
	}
	operation = strings.TrimSpace(operation)
	allowed := map[string]bool{
		"status": true, "list_windows": true, "ui_tree": true, "screenshot": true,
		"focus": true, "click": true, "type": true, "key": true, "scroll": true, "drag": true,
	}
	if !allowed[operation] {
		return nil, fmt.Errorf("unsupported Computer Use operation: %s", operation)
	}
	request := map[string]any{
		"version":       1,
		"operation":     operation,
		"workspaceId":   c.WorkspaceID,
		"workspaceRoot": c.Root,
		"arguments":     args,
	}
	raw, _ := json.Marshal(request)
	c.mu.Lock()
	defer c.mu.Unlock()
	response, err := c.callLocked(ctx, raw)
	if err != nil {
		return nil, fmt.Errorf("Computer Use helper failed: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Computer Use operation failed"
		}
		return nil, errors.New(response.Error)
	}
	return response.Result, nil
}

func (c *ComputerController) Close() {
	if c == nil {
		return
	}
	c.mu.Lock()
	c.resetProcessLocked()
	c.mu.Unlock()
}

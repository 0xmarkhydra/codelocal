package automation

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
)

type ComputerController struct {
	WorkspaceID  string
	WorkspaceKey string
	Root         string
	Helper       string
	Backend      string
	mu           sync.Mutex
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
	return &ComputerController{WorkspaceID: workspaceID, WorkspaceKey: workspaceKey, Root: root, Helper: helper, Backend: environment.ComputerBackend}, nil
}

func ComputerCapabilities() map[string]any {
	settings, _ := Load()
	enabled := settings != nil && settings.Computer.Enabled
	environment := Detect()
	helper := ComputerHelperPath()
	available := enabled && environment.ComputerSupported && helper != ""
	return map[string]any{
		"available":         available,
		"enabled":           enabled,
		"backend":           environment.ComputerBackend,
		"helperReady":       helper != "",
		"screenCapture":     available,
		"uiTree":            available,
		"pointer":           available,
		"keyboard":          available,
		"clipboard":         false,
		"backgroundControl": available && runtime.GOOS != "linux",
		"secureDesktop":     false,
		"notes":             environment.Notes,
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
		"version":      1,
		"operation":    operation,
		"workspaceId":  c.WorkspaceID,
		"workspaceRoot": c.Root,
		"arguments":    args,
	}
	raw, _ := json.Marshal(request)
	c.mu.Lock()
	defer c.mu.Unlock()
	cmd := exec.CommandContext(ctx, c.Helper, "--json")
	cmd.Dir = c.Root
	cmd.Env = append(os.Environ(), "CI=1")
	cmd.Stdin = bytes.NewReader(raw)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		message := strings.TrimSpace(stderr.String())
		if message == "" {
			message = err.Error()
		}
		return nil, fmt.Errorf("Computer Use helper failed: %s", message)
	}
	var response struct {
		OK     bool `json:"ok"`
		Result any  `json:"result"`
		Error  string `json:"error"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &response); err != nil {
		return nil, fmt.Errorf("invalid Computer Use helper response: %w", err)
	}
	if !response.OK {
		if response.Error == "" {
			response.Error = "Computer Use operation failed"
		}
		return nil, errors.New(response.Error)
	}
	return response.Result, nil
}

func (c *ComputerController) Close() {}

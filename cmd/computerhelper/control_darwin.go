//go:build darwin

package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	macControlActive  = "active"
	macControlPaused  = "paused"
	macControlStopped = "stopped"
)

func macComputerStateDir() string {
	if configured := strings.TrimSpace(os.Getenv("CODELOCAL_COMPUTER_STATE_DIR")); configured != "" {
		return configured
	}
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return filepath.Join(os.TempDir(), "codelocal-computer")
	}
	return filepath.Join(home, ".codelocal", "computer")
}

func macControlStateFile() string  { return filepath.Join(macComputerStateDir(), "control.state") }
func macActivityStateFile() string { return filepath.Join(macComputerStateDir(), "activity.json") }

func ensureMacComputerStateDir() error {
	return os.MkdirAll(macComputerStateDir(), 0o700)
}

func parseMacControlState(raw []byte) (string, error) {
	state := strings.ToLower(strings.TrimSpace(string(raw)))
	if state == "" {
		return macControlActive, nil
	}
	switch state {
	case macControlActive, macControlPaused, macControlStopped:
		return state, nil
	default:
		return "", fmt.Errorf("invalid local Computer Use control state %q", state)
	}
}

func macControlState() (string, error) {
	raw, err := os.ReadFile(macControlStateFile())
	if errors.Is(err, os.ErrNotExist) {
		return macControlActive, nil
	}
	if err != nil {
		return "", err
	}
	return parseMacControlState(raw)
}

func macOperationMutates(operation string) bool {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "focus", "semantic_click", "semantic_type", "semantic_batch", "click", "type", "key", "scroll", "drag":
		return true
	default:
		return false
	}
}

func guardMacComputerControl(operation string) error {
	operation = strings.ToLower(strings.TrimSpace(operation))
	if operation == "status" || operation == "user_activity" {
		return nil
	}
	state, err := macControlState()
	if err != nil {
		return fmt.Errorf("cannot safely read local Computer Use control state: %w", err)
	}
	if state == macControlStopped {
		return errors.New("Computer Use is stopped from the CodeLocal menu bar; resume it there before continuing")
	}
	if state == macControlPaused && macOperationMutates(operation) {
		return errors.New("Computer Use control is paused from the CodeLocal menu bar; resume it there before sending input")
	}
	return nil
}

func macActivityMode(operation string, args map[string]any) string {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "screenshot":
		return "viewing"
	case "focus", "key", "scroll", "drag":
		return "foreground"
	case "click":
		if stringValue(args, "elementId") == "" || strings.HasPrefix(stringValue(args, "elementId"), "vision:") {
			return "foreground"
		}
		return "background"
	case "type":
		if stringValue(args, "elementId") == "" {
			return "foreground"
		}
		return "background"
	case "semantic_click", "semantic_type", "semantic_batch", "ui_tree", "element_read":
		return "background"
	default:
		return ""
	}
}

func macActivityDetail(operation string) string {
	switch strings.ToLower(strings.TrimSpace(operation)) {
	case "screenshot":
		return "Viewing screen"
	case "focus", "key", "scroll", "drag":
		return "Foreground control"
	case "click", "type", "semantic_click", "semantic_type", "semantic_batch":
		return "Controlling in background"
	case "ui_tree", "element_read", "list_windows", "scene_events":
		return "Inspecting interface"
	default:
		return ""
	}
}

func setMacActivityStateBestEffort(mode, windowID, detail string) {
	if strings.TrimSpace(mode) == "" {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()
	_ = macNativeActivity(ctx, mode, windowID, detail)
}

func setMacActivityBestEffort(mode, operation string, args map[string]any) {
	setMacActivityStateBestEffort(mode, stringValue(args, "windowId"), macActivityDetail(operation))
}

func clearMacActivityBestEffort() {
	setMacActivityStateBestEffort("idle", "", "")
}

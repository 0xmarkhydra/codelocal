//go:build linux

package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"
)

func linuxMode() string {
	if strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" {
		return "wayland"
	}
	if strings.TrimSpace(os.Getenv("DISPLAY")) != "" {
		return "x11"
	}
	return "headless"
}

func linuxCapabilitiesWithATSPITree(base map[string]any) map[string]any {
	if base == nil {
		base = map[string]any{}
	}
	if available, _ := base["available"].(bool); available && atspiAvailable() {
		base["uiTree"] = true
		notes, _ := base["notes"].([]string)
		base["notes"] = append(notes, "AT-SPI semantic UI tree and Action interface are available; CodeLocal prefers semantic actions before coordinate input.")
	}
	return base
}

func platformCapabilities() map[string]any {
	switch linuxMode() {
	case "x11":
		return linuxCapabilitiesWithATSPITree(x11Capabilities())
	case "wayland":
		return linuxCapabilitiesWithATSPITree(waylandCapabilities())
	default:
		return map[string]any{
			"available": false,
			"backend": "linux-headless",
			"windowList": false,
			"screenCapture": false,
			"uiTree": false,
			"pointer": false,
			"keyboard": false,
			"clipboard": false,
			"backgroundControl": false,
			"secureDesktop": false,
			"notes": []string{"No graphical Linux session was detected."},
		}
	}
}

func platformHandle(ctx context.Context, input request) (any, error) {
	if input.Operation == "status" {
		return platformCapabilities(), nil
	}
	if linuxMode() == "headless" {
		return nil, errors.New("no graphical Linux session is available")
	}
	if input.Operation == "ui_tree" {
		result, err := atspiTree(ctx, 500)
		return result, atspiError("UI tree", err)
	}
	if input.Operation == "click" {
		if elementID := stringValue(input.Arguments, "elementId"); strings.HasPrefix(elementID, "atspi:") {
			result, err := atspiDoAction(ctx, elementID)
			return result, atspiError("action", err)
		}
	}
	switch linuxMode() {
	case "wayland":
		return waylandHandle(ctx, input)
	case "x11":
		return x11Handle(ctx, input)
	default:
		return nil, errors.New("no graphical Linux session is available")
	}
}

func waitContext(ctx context.Context, duration time.Duration) error {
	timer := time.NewTimer(duration)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-timer.C:
		return nil
	}
}

func absFloat(value float64) float64 {
	if value < 0 {
		return -value
	}
	return value
}

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

func platformCapabilities() map[string]any {
	switch linuxMode() {
	case "x11":
		return x11Capabilities()
	case "wayland":
		return waylandCapabilities()
	default:
		return map[string]any{
			"available": false,
			"backend": "linux-headless",
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

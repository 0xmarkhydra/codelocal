//go:build darwin

package main

import (
	"strings"
	"testing"
)

func TestMacTreeDegradedForWindowOnly(t *testing.T) {
	tree := map[string]any{"nodes": []any{map[string]any{"role": "AXWindow", "name": "Telegram"}}}
	if !macTreeDegraded(tree) {
		t.Fatal("window-only accessibility tree should trigger Vision fallback")
	}
}

func TestMacTreeKeepsUsefulAccessibilityControls(t *testing.T) {
	tree := map[string]any{"nodes": []any{map[string]any{"role": "AXWindow", "children": []any{
		map[string]any{"role": "AXButton", "name": "Save"},
	}}}}
	if macTreeDegraded(tree) {
		t.Fatal("actionable accessibility tree should remain on native fast path")
	}
}

func TestMacVisionPoint(t *testing.T) {
	x, y, ok := macVisionPoint("vision:123.50:456.25")
	if !ok || x != 123.5 || y != 456.25 {
		t.Fatalf("unexpected Vision point: %v %v %v", x, y, ok)
	}
	if _, _, ok := macVisionPoint("vision:screen:main:1:2"); ok {
		t.Fatal("malformed Vision element ID should be rejected")
	}
}

func TestMacPointerScriptDoesNotManuallyReleaseJXAEvents(t *testing.T) {
	if strings.Contains(macPointerScript, "CFRelease") {
		t.Fatal("JXA-managed CGEvent objects must not be manually CFReleased")
	}
}

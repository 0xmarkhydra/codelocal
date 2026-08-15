//go:build darwin

package main

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
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

func TestMacAXWindowRef(t *testing.T) {
	pid, index, ok := macAXWindowRef("ax:1234:2")
	if !ok || pid != 1234 || index != 2 {
		t.Fatalf("unexpected AX window ref: pid=%d index=%d ok=%v", pid, index, ok)
	}
	for _, value := range []string{"ax:0:1", "ax:12:-1", "123", "ax:bad:1"} {
		if _, _, ok := macAXWindowRef(value); ok {
			t.Fatalf("invalid AX window ref accepted: %q", value)
		}
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

func TestMacSemanticFastPathDoesNotSynthesizePhysicalInput(t *testing.T) {
	for _, script := range []string{macSemanticActionScript, macPersistentWorkerScript} {
		if strings.Contains(script, "CGEventPost") || strings.Contains(script, "CGEventCreateMouseEvent") {
			t.Fatal("semantic accessibility fast path must not synthesize physical mouse events")
		}
	}
	if strings.Contains(macPersistentWorkerScript, "CGWindowListCreateImage") {
		t.Fatal("persistent worker must not use deprecated CoreGraphics screen-capture APIs")
	}
}

func TestMacSemanticFastPathRejectsAmbiguousTargets(t *testing.T) {
	for _, script := range []string{macSemanticActionScript, macPersistentWorkerScript} {
		if !strings.Contains(script, "runnerUpScore") || !strings.Contains(script, "ambiguous accessible UI target") {
			t.Fatal("semantic accessibility fast path must reject near-tied targets instead of clicking traversal-order winners")
		}
	}
}

func TestMacV2CapabilitiesAdvertisePersistentEngine(t *testing.T) {
	caps := platformCapabilities()
	if value, _ := caps["engine"].(string); value != "computer-v2" {
		t.Fatalf("unexpected Computer Engine identity: %q", value)
	}
	if enabled, _ := caps["persistentEngine"].(bool); !enabled {
		t.Fatal("macOS v2 should advertise a persistent engine")
	}
	if streaming, _ := caps["screenCaptureStreaming"].(bool); streaming {
		t.Fatal("ScreenCaptureKit streaming must not be advertised before it is implemented")
	}
}

func TestMacPersistentWindowEnumerationAvoidsWhoseVisibleFilter(t *testing.T) {
	if strings.Contains(macPersistentWorkerScript, "applicationProcesses.whose({visible:true})") {
		t.Fatal("persistent window enumeration must avoid the fragile JXA whose visible filter")
	}
	if !strings.Contains(macPersistentWorkerScript, "applicationProcesses()") || !strings.Contains(macPersistentWorkerScript, "p.visible()") {
		t.Fatal("persistent window enumeration should enumerate processes then filter visibility explicitly")
	}
	if macPersistentWindowBudget < 1500*time.Millisecond {
		t.Fatalf("persistent window cold-start budget too small: %s", macPersistentWindowBudget)
	}
}

func TestMacPersistentWorkerLive(t *testing.T) {
	if os.Getenv("CODELOCAL_LIVE_COMPUTER_TEST") != "1" {
		t.Skip("set CODELOCAL_LIVE_COMPUTER_TEST=1 for local persistent worker smoke test")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if !macPersistentWorkerReady(ctx) {
		t.Fatal("persistent JXA worker did not answer ping")
	}
}

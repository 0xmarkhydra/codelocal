//go:build darwin

package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestMacNativeDaemonCandidatesPreferExplicitPath(t *testing.T) {
	candidates := macNativeDaemonCandidates("/explicit/native", "/pkg", "/pkg/bin/helpers/computer-darwin-arm64", "arm64")
	if len(candidates) < 4 {
		t.Fatalf("candidates=%#v", candidates)
	}
	if candidates[0] != "/explicit/native" {
		t.Fatalf("explicit candidate should win, got %#v", candidates)
	}
	if got := filepath.Base(candidates[1]); got != "computer-native-darwin-arm64" {
		t.Fatalf("unexpected packaged daemon name: %s", got)
	}
}

func writeFakeMacNativeDaemon(t *testing.T) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "computer-native-darwin-test")
	script := `#!/bin/sh
case "$1" in
  --capabilities)
    printf '%s\n' '{"ready":true,"engine":"native-ax-test","nativeAXBackend":true,"sceneEvents":true,"accessibilityTrusted":true}'
    ;;
  --serve)
    while IFS= read -r line; do
      case "$line" in
        *'"op":"ping"'*) printf '%s\n' '{"ok":true,"result":{"ready":true,"engine":"native-ax-test"}}' ;;
        *'"op":"events"'*) printf '%s\n' '{"ok":true,"result":{"sequence":1,"events":[{"kind":"AXValueChanged","pid":7}]}}' ;;
        *) printf '%s\n' '{"ok":false,"error":"unsupported fake op"}' ;;
      esac
    done
    ;;
  *) exit 2 ;;
esac
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return path
}

func resetSharedMacNativeWorkerForTest() {
	sharedMacNativeWorker.mu.Lock()
	sharedMacNativeWorker.resetLocked()
	sharedMacNativeWorker.mu.Unlock()
}

func TestMacNativeCaptureLive(t *testing.T) {
	if os.Getenv("CODELOCAL_LIVE_COMPUTER_TEST") != "1" {
		t.Skip("set CODELOCAL_LIVE_COMPUTER_TEST=1 for local native capture smoke test")
	}
	if macNativeDaemonPath() == "" {
		t.Skip("native macOS daemon is not configured")
	}
	resetSharedMacNativeWorkerForTest()
	t.Cleanup(resetSharedMacNativeWorkerForTest)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	windows, err := macNativeWindows(ctx)
	if err != nil || len(windows) == 0 {
		t.Fatalf("native windows unavailable: count=%d err=%v", len(windows), err)
	}
	var pid, windowIndex int
	for _, raw := range windows {
		item, _ := raw.(map[string]any)
		windowID, _ := item["windowId"].(string)
		if parsedPID, parsedIndex, ok := macAXWindowRef(windowID); ok {
			pid, windowIndex = parsedPID, parsedIndex
			break
		}
	}
	if pid == 0 {
		t.Skip("no native AX application window available")
	}
	capture, err := macNativeCapture(ctx, pid, windowIndex, 960)
	if err != nil {
		t.Fatalf("native ScreenCaptureKit capture failed: %v", err)
	}
	encoded, _ := capture["data"].(string)
	if encoded == "" || capture["mimeType"] != "image/png" || capture["engine"] != "screencapturekit" {
		t.Fatalf("unexpected native capture metadata: mime=%v engine=%v dataBytes=%d", capture["mimeType"], capture["engine"], len(encoded))
	}
}

func TestMacNativeVisionLive(t *testing.T) {
	if os.Getenv("CODELOCAL_LIVE_COMPUTER_TEST") != "1" {
		t.Skip("set CODELOCAL_LIVE_COMPUTER_TEST=1 for local native Vision smoke test")
	}
	if macNativeDaemonPath() == "" {
		t.Skip("native macOS daemon is not configured")
	}
	resetSharedMacNativeWorkerForTest()
	t.Cleanup(resetSharedMacNativeWorkerForTest)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	windows, err := macNativeWindows(ctx)
	if err != nil || len(windows) == 0 {
		t.Fatalf("native windows unavailable: count=%d err=%v", len(windows), err)
	}
	var lastErr error
	attempted := 0
	for _, raw := range windows {
		item, _ := raw.(map[string]any)
		windowID, _ := item["windowId"].(string)
		pid, windowIndex, ok := macAXWindowRef(windowID)
		if !ok {
			continue
		}
		attempted++
		nodes, visionErr := macNativeVision(ctx, pid, windowIndex, 960)
		if visionErr != nil {
			lastErr = visionErr
			if attempted < 8 {
				continue
			}
			break
		}
		for _, node := range nodes {
			entry, _ := node.(map[string]any)
			if entry["source"] != "vision" || entry["elementId"] == nil {
				t.Fatalf("unexpected native Vision node: %#v", entry)
			}
		}
		return
	}
	if attempted == 0 {
		t.Skip("no native AX application window available")
	}
	t.Skipf("no currently captureable native AX window for Vision smoke test after %d candidate(s): %v", attempted, lastErr)
}

func TestMacNativeDaemonProbeAndPersistentHandshake(t *testing.T) {
	path := writeFakeMacNativeDaemon(t)
	t.Setenv("CODELOCAL_COMPUTER_NATIVE_DAEMON", path)
	resetSharedMacNativeWorkerForTest()
	t.Cleanup(resetSharedMacNativeWorkerForTest)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	capabilities, ready := macNativeDaemonProbe(ctx)
	if !ready || capabilities["nativeAXBackend"] != true || capabilities["sceneEvents"] != true {
		t.Fatalf("unexpected probe: ready=%v capabilities=%#v", ready, capabilities)
	}
	if !sharedMacNativeWorker.ensureReady(ctx) {
		t.Fatal("persistent native worker handshake failed")
	}
	events, err := macNativeSceneEvents(ctx)
	if err != nil || len(events) != 1 {
		t.Fatalf("events=%#v err=%v", events, err)
	}
}

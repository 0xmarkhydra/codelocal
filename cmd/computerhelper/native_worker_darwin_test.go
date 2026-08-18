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

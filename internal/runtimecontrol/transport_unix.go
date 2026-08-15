//go:build !windows

package runtimecontrol

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net"
	"os"
	"path/filepath"
)

func Endpoint(dir string) string {
	candidate := filepath.Join(dir, "runtime.sock")
	// Unix-domain socket paths are short on macOS (roughly 104 bytes). Tests,
	// temporary directories and deeply nested state paths can exceed that even
	// though the runtime itself is perfectly valid. Keep the normal private
	// state path when it is comfortably short; otherwise use a deterministic
	// per-state-dir socket name under the OS temp directory.
	if len([]byte(candidate)) <= 90 {
		return candidate
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		abs = dir
	}
	digest := sha256.Sum256([]byte(abs))
	name := "codelocal-" + hex.EncodeToString(digest[:8]) + ".sock"
	return filepath.Join(os.TempDir(), name)
}

func dialControl(ctx context.Context, endpoint string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "unix", endpoint)
}

func listenControl(endpoint string) (net.Listener, error) { return net.Listen("unix", endpoint) }
func removeEndpoint(endpoint string) error                { return os.Remove(endpoint) }
func endpointExists(endpoint string) bool {
	_, err := os.Stat(endpoint)
	return err == nil
}
func secureEndpoint(endpoint string) error { return os.Chmod(endpoint, 0o600) }

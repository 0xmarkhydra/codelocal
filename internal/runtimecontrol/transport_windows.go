//go:build windows

package runtimecontrol

import (
	"context"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"net"
)

// Windows uses a deterministic loopback-only control port. The runtime lease also
// carries an instance ID, so another local process cannot be mistaken for CodeLocal.
func Endpoint(dir string) string {
	sum := sha256.Sum256([]byte(dir))
	port := 42000 + int(binary.BigEndian.Uint16(sum[:2])%12000)
	return fmt.Sprintf("127.0.0.1:%d", port)
}

func dialControl(ctx context.Context, endpoint string) (net.Conn, error) {
	return (&net.Dialer{}).DialContext(ctx, "tcp", endpoint)
}

func listenControl(endpoint string) (net.Listener, error) { return net.Listen("tcp", endpoint) }
func removeEndpoint(string) error                         { return nil }
func endpointExists(endpoint string) bool {
	conn, err := net.DialTimeout("tcp", endpoint, 150000000)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}
func secureEndpoint(string) error { return nil }

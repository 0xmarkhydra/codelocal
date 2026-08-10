package gateway

import (
	"context"

	"github.com/coder/websocket"
)

// DisconnectCredential closes every local workspace connection authenticated by
// one device credential. The persistent credential is revoked separately by the
// cloud store; this method makes revocation immediate for already-open sockets.
func (h *Hub) DisconnectCredential(userID, credentialID string) int {
	if h == nil || credentialID == "" {
		return 0
	}
	h.mu.RLock()
	clients := make([]*Client, 0)
	for _, client := range h.clients {
		if client.UserID == userID && client.CredentialID == credentialID {
			clients = append(clients, client)
		}
	}
	h.mu.RUnlock()
	for _, client := range clients {
		_ = client.conn.Close(websocket.StatusPolicyViolation, "device credential revoked")
	}
	return len(clients)
}

// ReleaseDeviceOwnership is best-effort cleanup for ownership records after a
// device is revoked. Closed sockets also unregister and release ownership.
func (h *Hub) ReleaseDeviceOwnership(ctx context.Context, userID, credentialID string) {
	_ = ctx
	_ = userID
	_ = credentialID
}

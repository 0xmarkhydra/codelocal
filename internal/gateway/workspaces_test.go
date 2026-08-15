package gateway

import (
	"context"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/protocol"
)

func TestActivateUsesConnectedLocalWorkspaceFastPath(t *testing.T) {
	const userID = "user"
	key := ClientKey(userID, "device", "workspace")
	hub := NewHub(nil, "gateway-test")
	client := &Client{
		Key:             key,
		UserID:          userID,
		DeviceID:        "device",
		DeviceName:      "Laptop",
		WorkspaceID:     "workspace",
		WorkspaceName:   "CodeLocal",
		ProjectRoot:     "/project",
		ProtocolVersion: 3,
		ClientVersion:   "1.5.6",
		Capabilities: protocol.Capabilities{
			Filesystem:           true,
			Git:                  true,
			Shell:                true,
			PTY:                  true,
			Idempotency:          true,
			Cancellation:         true,
			ApprovalMemory:       true,
			TerminalChatApproval: true,
			Automation: protocol.AutomationCapabilities{
				Browser:  protocol.BrowserCapabilities{Available: true, IsolatedProfile: true, Screenshots: true},
				Computer: protocol.ComputerCapabilities{Available: true, Backend: "test", WindowList: true, Pointer: true},
			},
		},
		closed: make(chan struct{}),
	}
	client.lastSeenAt.Store(1234)
	hub.clients[key] = client

	service := &WorkspaceService{Hub: hub}
	workspace, err := service.Activate(context.Background(), userID, key)
	if err != nil {
		t.Fatal(err)
	}
	if workspace.Key != key || workspace.Status != "active" || workspace.Authorized != true {
		t.Fatalf("unexpected local workspace view: %#v", workspace)
	}
	if workspace.ProtocolVersion != 3 || workspace.ClientVersion != "1.5.6" || workspace.LastSeenAt != 1234 {
		t.Fatalf("local workspace metadata was not preserved: %#v", workspace)
	}
	if workspace.Capabilities["filesystem"] != true || workspace.Capabilities["pty"] != true || workspace.Capabilities["approvalMemory"] != true {
		t.Fatalf("local workspace capabilities were not preserved: %#v", workspace.Capabilities)
	}
	automation, _ := workspace.Capabilities["automation"].(map[string]any)
	browser, _ := automation["browser"].(map[string]any)
	computer, _ := automation["computer"].(map[string]any)
	if browser["available"] != true || computer["backend"] != "test" || computer["pointer"] != true {
		t.Fatalf("automation capabilities were not preserved: %#v", automation)
	}
}

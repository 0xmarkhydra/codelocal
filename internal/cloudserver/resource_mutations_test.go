package cloudserver

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
)

func TestMutationPublicIDRejectsEmptyOversizedAndControlData(t *testing.T) {
	if got := mutationPublicID(" device-1 ", 20); got != "device-1" {
		t.Fatalf("mutationPublicID normalized = %q", got)
	}
	for _, value := range []string{"", "bad\nvalue", "bad\x00value", "this-value-is-definitely-too-long"} {
		if got := mutationPublicID(value, 20); got != "" {
			t.Fatalf("unsafe public id %q was accepted as %q", value, got)
		}
	}
}

func TestDeviceMutationResolvesPublicIDServerSide(t *testing.T) {
	devices := []cloud.Device{
		{DeviceID: "device-public", CredentialID: "credential-private", DeviceName: "Mac"},
		{DeviceID: "device-revoked", CredentialID: "credential-revoked", RevokedAt: 123},
	}
	device := deviceByPublicID(devices, "device-public")
	if device == nil || device.CredentialID != "credential-private" {
		t.Fatalf("public device did not resolve to server-side credential: %#v", device)
	}
	if deviceByPublicID(devices, "credential-private") != nil {
		t.Fatal("credential id must never act as the public mutation identifier")
	}
	if deviceByPublicID(devices, "device-revoked") != nil {
		t.Fatal("already-revoked devices must not resolve as mutable targets")
	}
	if got := deviceByCredentialID(devices, "credential-private"); got == nil || got.DeviceID != "device-public" {
		t.Fatalf("legacy credential route could not resolve through shared device helper: %#v", got)
	}
}

func TestWorkspaceMutationRequiresExactPublicDeviceAndWorkspacePair(t *testing.T) {
	workspaces := []gateway.WorkspaceView{
		{DeviceID: "device-a", WorkspaceID: "workspace-1", WorkspaceName: "Alpha", RuntimeOnline: true},
		{DeviceID: "device-b", WorkspaceID: "workspace-1", WorkspaceName: "Beta", RuntimeOnline: true},
	}
	workspace := workspaceByPublicID(workspaces, "device-a", "workspace-1")
	if workspace == nil || workspace.WorkspaceName != "Alpha" {
		t.Fatalf("unexpected workspace resolution: %#v", workspace)
	}
	if workspaceByPublicID(workspaces, "device-a", "missing") != nil || workspaceByPublicID(workspaces, "missing", "workspace-1") != nil {
		t.Fatal("workspace mutation target must match both public ids")
	}
}

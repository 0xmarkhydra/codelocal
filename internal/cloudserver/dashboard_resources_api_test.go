package cloudserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
)

func TestDevicesResourceDTOExcludesCredentialMaterial(t *testing.T) {
	payload := buildDevicesResourceDTO([]cloud.Device{
		{CredentialID: "credential-private", DeviceID: "device-1", DeviceName: "Mac", PublicKey: "public-private", SecretHash: "hash-private", CreatedAt: 10, LastSeenAt: 20},
		{CredentialID: "credential-offline", DeviceID: "device-2", DeviceName: "Linux", PublicKey: "public-offline", SecretHash: "hash-offline", CreatedAt: 11, LastSeenAt: 21},
		{CredentialID: "credential-revoked", DeviceID: "device-3", DeviceName: "Old", PublicKey: "public-revoked", SecretHash: "hash-revoked", CreatedAt: 12, LastSeenAt: 22, RevokedAt: 30},
	}, map[string]bool{"device-1": true})

	if payload.Summary.Paired != 2 || payload.Summary.Online != 1 || payload.Summary.Revoked != 1 {
		t.Fatalf("unexpected device summary: %#v", payload.Summary)
	}
	if len(payload.Items) != 3 || payload.Items[0].Status != "online" || payload.Items[1].Status != "offline" || payload.Items[2].Status != "revoked" {
		t.Fatalf("unexpected device items: %#v", payload.Items)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{"credential-private", "credential-offline", "credential-revoked", "public-private", "public-offline", "public-revoked", "hash-private", "hash-offline", "hash-revoked", `"credentialId"`, `"publicKey"`, `"secretHash"`, `"userId"`} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("devices API leaked internal field/value %q: %s", forbidden, serialized)
		}
	}
}

func TestWorkspacesResourceDTOExcludesRuntimeInternals(t *testing.T) {
	payload := buildWorkspacesResourceDTO([]gateway.WorkspaceView{
		{Key: "route-private", DeviceID: "device-1", DeviceName: "Mac", WorkspaceID: "workspace-1", WorkspaceName: "Alpha", ProjectID: "project-private", ProjectName: "Project Alpha", ProjectRoot: "/Users/private/alpha", Capabilities: map[string]any{"shell": true}, Authorized: true, ClientVersion: "private-version", ProtocolVersion: 3, Status: "active", RuntimeOnline: true, LastSeenAt: 100},
		{Key: "route-sleeping", DeviceID: "device-1", DeviceName: "Mac", WorkspaceID: "workspace-2", WorkspaceName: "Beta", ProjectRoot: "/Users/private/beta", Status: "sleeping", RuntimeOnline: true, LastSeenAt: 90},
		{Key: "route-offline", DeviceID: "device-2", DeviceName: "Linux", WorkspaceID: "workspace-3", WorkspaceName: "Gamma", Status: "device_offline", LastSeenAt: 80},
	})

	if payload.Summary.Total != 3 || payload.Summary.Active != 1 || payload.Summary.Sleeping != 1 || payload.Summary.Offline != 1 {
		t.Fatalf("unexpected workspace summary: %#v", payload.Summary)
	}
	if len(payload.Items) != 3 || payload.Items[2].Status != "offline" {
		t.Fatalf("unexpected workspace items: %#v", payload.Items)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{"route-private", "route-sleeping", "route-offline", "project-private", "Project Alpha", "/Users/private", "private-version", `"key"`, `"projectId"`, `"projectName"`, `"projectRoot"`, `"capabilities"`, `"authorized"`, `"protocolVersion"`, `"clientVersion"`} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("workspaces API leaked internal field/value %q: %s", forbidden, serialized)
		}
	}
}

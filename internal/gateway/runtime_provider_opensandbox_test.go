package gateway

import "testing"

func TestManagedRuntimeDeviceID(t *testing.T) {
	valid := "cloud-0123456789abcdef01234567"
	if !managedRuntimeDeviceID(valid) {
		t.Fatalf("expected %q to be recognized as managed runtime device", valid)
	}
	for _, value := range []string{
		"device-local",
		"cloud-0123456789abcdef0123456",
		"cloud-0123456789abcdef012345678",
		"cloud-0123456789abcdef0123456z",
		" cloud-0123456789abcdef0123456z ",
	} {
		if managedRuntimeDeviceID(value) {
			t.Fatalf("unexpected managed runtime device match: %q", value)
		}
	}
}

func TestManagedSnapshotNameIsStableAndScoped(t *testing.T) {
	first := managedSnapshotName("user", "workspace-key", "general-small", "runtime:v1")
	second := managedSnapshotName("user", "workspace-key", "general-small", "runtime:v1")
	if first == "" || first != second {
		t.Fatalf("snapshot name is unstable: %q %q", first, second)
	}
	if first == managedSnapshotName("user", "workspace-key", "video-cpu", "runtime:v1") {
		t.Fatal("runtime profile must scope snapshot identity")
	}
	if first == managedSnapshotName("user", "workspace-key", "general-small", "runtime:v2") {
		t.Fatal("runtime image version must scope snapshot identity")
	}
	if first == managedSnapshotName("other-user", "workspace-key", "general-small", "runtime:v1") {
		t.Fatal("user must scope snapshot identity")
	}
}

func TestProjectCloudWorkspacePreservesProductIdentity(t *testing.T) {
	source := &WorkspaceView{
		Key:             "user::mac::workspace",
		DeviceID:        "mac",
		DeviceName:      "MacBook",
		WorkspaceID:     "workspace",
		WorkspaceName:   "Product",
		ProjectID:       "project",
		ProjectName:     "Product",
		ProjectRoot:     "/Users/test/Product",
		Status:          "device_offline",
		RuntimeOnline:   false,
		Authorized:      nil,
		ClientVersion:   "1.0.0",
		ProtocolVersion: 1,
		Capabilities:    map[string]any{"filesystem": false},
		LastSeenAt:      100,
	}
	runtimeWorkspace := &WorkspaceView{
		Key:             "user::cloud-0123456789abcdef01234567::workspace",
		DeviceID:        "cloud-0123456789abcdef01234567",
		DeviceName:      "CodeLocal Cloud",
		WorkspaceID:     "workspace",
		WorkspaceName:   "Product",
		ProjectRoot:     "/workspace",
		Status:          "active",
		RuntimeOnline:   true,
		Authorized:      true,
		ClientVersion:   "2.0.0",
		ProtocolVersion: 3,
		Capabilities:    map[string]any{"filesystem": true, "shell": true, "idempotency": true},
		LastSeenAt:      999,
	}

	projected := projectCloudWorkspace(source, runtimeWorkspace)
	if projected == nil {
		t.Fatal("expected projected workspace")
	}
	if projected.Key != source.Key || projected.DeviceID != source.DeviceID || projected.DeviceName != source.DeviceName || projected.WorkspaceID != source.WorkspaceID {
		t.Fatalf("product workspace identity leaked cloud runtime values: %#v", projected)
	}
	if projected.ProjectRoot != source.ProjectRoot || projected.ProjectID != source.ProjectID || projected.ProjectName != source.ProjectName {
		t.Fatalf("product project identity/path changed: %#v", projected)
	}
	if projected.Status != "active" || projected.RuntimeOnline != true || projected.Authorized != true {
		t.Fatalf("projected runtime state is not active: %#v", projected)
	}
	if projected.ProtocolVersion != runtimeWorkspace.ProtocolVersion || projected.ClientVersion != runtimeWorkspace.ClientVersion || projected.LastSeenAt != runtimeWorkspace.LastSeenAt {
		t.Fatalf("runtime metadata was not projected: %#v", projected)
	}
	if projected.Capabilities["shell"] != true || projected.Capabilities["idempotency"] != true {
		t.Fatalf("runtime capabilities were not projected: %#v", projected.Capabilities)
	}
}

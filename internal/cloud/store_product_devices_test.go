package cloud

import "testing"

func TestProductDevicesHidesOnlyManagedRuntimeCredentials(t *testing.T) {
	managed := Device{CredentialID: "managed", DeviceID: "cloud-0123456789abcdef01234567", DeviceName: "CodeLocal Cloud"}
	real := Device{CredentialID: "real", DeviceID: "macbook-pro", DeviceName: "Mong MacBook"}
	similarID := Device{CredentialID: "similar-id", DeviceID: "cloud-0123456789abcdef01234567", DeviceName: "User Cloud Device"}
	malformedManaged := Device{CredentialID: "malformed", DeviceID: "cloud-not-a-managed-runtime", DeviceName: "CodeLocal Cloud"}

	visible := ProductDevices([]Device{managed, real, similarID, malformedManaged})
	if len(visible) != 3 {
		t.Fatalf("expected 3 product devices, got %d: %#v", len(visible), visible)
	}
	seen := map[string]bool{}
	for _, device := range visible {
		seen[device.CredentialID] = true
	}
	if seen[managed.CredentialID] {
		t.Fatal("managed runtime credential leaked into product device list")
	}
	for _, id := range []string{real.CredentialID, similarID.CredentialID, malformedManaged.CredentialID} {
		if !seen[id] {
			t.Fatalf("product device %q was hidden unexpectedly", id)
		}
	}
}

func TestProductDevicesReturnsNonNilEmptySlice(t *testing.T) {
	visible := ProductDevices(nil)
	if visible == nil {
		t.Fatal("expected non-nil empty product device slice")
	}
	if len(visible) != 0 {
		t.Fatalf("expected empty product device slice, got %d", len(visible))
	}
}

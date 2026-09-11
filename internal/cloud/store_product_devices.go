package cloud

import (
	"context"
	"encoding/hex"
	"strings"
)

const managedCloudRuntimeDeviceName = "CodeLocal Cloud"

func isManagedCloudRuntimeDeviceID(value string) bool {
	value = strings.TrimSpace(value)
	const prefix = "cloud-"
	if !strings.HasPrefix(value, prefix) || len(value) != len(prefix)+24 {
		return false
	}
	_, err := hex.DecodeString(strings.TrimPrefix(value, prefix))
	return err == nil
}

// IsManagedRuntimeDevice identifies CodeLocal-owned compute credentials. Both
// the deterministic device-ID shape and managed name must match so a user
// device with a coincidentally similar ID is never hidden from account UI.
func IsManagedRuntimeDevice(device Device) bool {
	return isManagedCloudRuntimeDeviceID(device.DeviceID) && strings.TrimSpace(device.DeviceName) == managedCloudRuntimeDeviceName
}

// ProductDevices removes execution-only managed runtime credentials from
// user-facing device collections while preserving the underlying credential
// rows for authentication, runtime bootstrap and lifecycle operations.
func ProductDevices(devices []Device) []Device {
	visible := make([]Device, 0, len(devices))
	for _, device := range devices {
		if IsManagedRuntimeDevice(device) {
			continue
		}
		visible = append(visible, device)
	}
	return visible
}

func (s *Store) ListProductDevices(ctx context.Context, userID string) ([]Device, error) {
	devices, err := s.ListDevices(ctx, userID)
	if err != nil {
		return nil, err
	}
	return ProductDevices(devices), nil
}

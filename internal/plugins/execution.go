package plugins

import (
	"errors"
	"fmt"
	"strings"
)

type RuntimeBinding struct {
	ServerName string            `json:"serverName"`
	Targets    []ExecutionTarget `json:"targets"`
}

type ExecutionRoute struct {
	Target     ExecutionTarget `json:"target"`
	Connection string          `json:"connection"`
	DeviceID   string          `json:"deviceId,omitempty"`
}

// ResolveExecutionRoute is the Plugin Platform boundary between product
// metadata and an MCP execution adapter. The stable connection identifiers are
// local:<device> and cloud; callers never infer a target from an endpoint.
func ResolveExecutionRoute(supported []ExecutionTarget, requested ExecutionTarget, deviceID string) (ExecutionRoute, error) {
	if requested == "" {
		requested = ExecutionLocal
	}
	allowed := false
	for _, target := range supported {
		if target == requested {
			allowed = true
			break
		}
	}
	if !allowed {
		return ExecutionRoute{}, fmt.Errorf("plugin execution target %q is not supported", requested)
	}
	switch requested {
	case ExecutionLocal:
		deviceID = strings.TrimSpace(deviceID)
		if deviceID == "" || strings.Contains(deviceID, ":") {
			return ExecutionRoute{}, errors.New("local plugin execution requires a canonical device id")
		}
		return ExecutionRoute{Target: requested, Connection: "local:" + deviceID, DeviceID: deviceID}, nil
	case ExecutionCloud:
		return ExecutionRoute{Target: requested, Connection: "cloud"}, nil
	default:
		return ExecutionRoute{}, fmt.Errorf("unsupported plugin execution target %q", requested)
	}
}

package main

// desktopBackendContract is the stable internal capability boundary for the
// Computer Engine v3 migration. The current Go/JXA macOS backend and a future
// Swift/Objective-C AX daemon must report the same contract so the controller
// can switch implementations without changing the public MCP surface.
type desktopBackendContract struct {
	Name                 string
	NativeAXBackend      bool
	EventDrivenScene     bool
	TargetedVerification bool
	UserActivityGuard    bool
}

func applyDesktopBackendContract(capabilities map[string]any, contract desktopBackendContract) map[string]any {
	if capabilities == nil {
		capabilities = map[string]any{}
	}
	capabilities["backendContract"] = contract.Name
	capabilities["nativeAXBackend"] = contract.NativeAXBackend
	capabilities["eventDrivenScene"] = contract.EventDrivenScene
	capabilities["targetedVerification"] = contract.TargetedVerification
	capabilities["userActivityGuard"] = contract.UserActivityGuard
	return capabilities
}

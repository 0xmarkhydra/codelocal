package cloud

const (
	// MCPUsageAuthority declares the durability/decision contract for usage data.
	// The local queue is deliberately bounded and may drop telemetry under load,
	// so this dataset must never be the sole authority for billing, entitlement,
	// access control, quota enforcement or security decisions.
	MCPUsageAuthority = "telemetry-only"
	MCPUsageDurable   = false
)

func MCPUsageAuthoritative() bool { return false }

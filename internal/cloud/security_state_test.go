package cloud

import "testing"

func TestEvaluateSecuritySignals(t *testing.T) {
	base := SecuritySignal{DeviceHash: "device-a", AgentHash: "agent-a", NetworkHash: "network-a"}
	for _, tc := range []struct {
		name     string
		current  SecuritySignal
		highRisk bool
		device   bool
		agent    bool
		network  bool
	}{
		{name: "same", current: base},
		{name: "network only", current: SecuritySignal{DeviceHash: "device-a", AgentHash: "agent-a", NetworkHash: "network-b"}, network: true},
		{name: "agent only", current: SecuritySignal{DeviceHash: "device-a", AgentHash: "agent-b", NetworkHash: "network-a"}, agent: true},
		{name: "agent and network", current: SecuritySignal{DeviceHash: "device-a", AgentHash: "agent-b", NetworkHash: "network-b"}, agent: true, network: true, highRisk: true},
		{name: "device mismatch", current: SecuritySignal{DeviceHash: "device-b", AgentHash: "agent-a", NetworkHash: "network-a"}, device: true, highRisk: true},
		{name: "missing bound device", current: SecuritySignal{AgentHash: "agent-a", NetworkHash: "network-a"}, device: true, highRisk: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			decision := EvaluateSecuritySignals(base, tc.current)
			if decision.HighRisk != tc.highRisk || decision.DeviceMismatch != tc.device || decision.AgentChanged != tc.agent || decision.NetworkChanged != tc.network {
				t.Fatalf("unexpected decision: %#v", decision)
			}
		})
	}
}

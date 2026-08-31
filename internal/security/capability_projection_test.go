package security

import "testing"

func TestCapabilityProjectionHidesDeniedAndImpossiblePrompt(t *testing.T) {
	kernel, err := NewPolicyKernel(nil, NetworkApproval)
	if err != nil {
		t.Fatal(err)
	}
	caps := []CapabilityDescriptor{
		{ID: "read", Action: "read", Tool: "file"},
		{ID: "network", Action: "network", Tool: "http", NetworkTarget: "https://api.example.com"},
		{ID: "secret-expose", Action: "run", Tool: "shell", SecretMode: SecretExpose, SecretNames: []string{"TOKEN"}},
	}
	projected, err := ProjectCapabilities(kernel, CapabilityProjectionRequest{ActorID: "user", Capabilities: caps, ApprovalAvailable: false})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID != "read" || projected[0].Visibility != CapabilityAvailable {
		t.Fatalf("unexpected projection: %+v", projected)
	}
}

func TestCapabilityProjectionShowsPromptWhenApprovalIsViable(t *testing.T) {
	kernel, _ := NewPolicyKernel(nil, NetworkApproval)
	projected, err := ProjectCapabilities(kernel, CapabilityProjectionRequest{
		ActorID: "user", AgentID: "agent", ApprovalAvailable: true,
		Capabilities: []CapabilityDescriptor{{ID: "network", Action: "network", Tool: "http", NetworkTarget: "https://api.example.com"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].Visibility != CapabilityPromptable || projected[0].ApprovalKey == "" {
		t.Fatalf("promptable capability missing: %+v", projected)
	}
}

func TestCapabilityProjectionNeverAdvertisesSecretExposure(t *testing.T) {
	kernel, _ := NewPolicyKernel(nil, NetworkAllow)
	projected, err := ProjectCapabilities(kernel, CapabilityProjectionRequest{
		ActorID: "user", ApprovalAvailable: true,
		Capabilities: []CapabilityDescriptor{
			{ID: "consume", Action: "run", SecretMode: SecretConsume, SecretNames: []string{"TOKEN"}},
			{ID: "expose", Action: "run", SecretMode: SecretExpose, SecretNames: []string{"TOKEN"}},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 1 || projected[0].ID != "consume" || projected[0].Visibility != CapabilityPromptable {
		t.Fatalf("secret projection violated: %+v", projected)
	}
}

func TestCapabilityProjectionIsDeterministicAndRejectsDuplicates(t *testing.T) {
	kernel, _ := NewPolicyKernel(nil, NetworkAllow)
	projected, err := ProjectCapabilities(kernel, CapabilityProjectionRequest{ActorID: "user", Capabilities: []CapabilityDescriptor{{ID: "z", Action: "read"}, {ID: "a", Action: "read"}}})
	if err != nil {
		t.Fatal(err)
	}
	if len(projected) != 2 || projected[0].ID != "a" || projected[1].ID != "z" {
		t.Fatalf("projection order is not deterministic: %+v", projected)
	}
	if _, err := ProjectCapabilities(kernel, CapabilityProjectionRequest{ActorID: "user", Capabilities: []CapabilityDescriptor{{ID: "dup", Action: "read"}, {ID: "dup", Action: "write"}}}); err == nil {
		t.Fatal("duplicate capability ID should fail closed")
	}
}

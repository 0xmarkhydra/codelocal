package plugins

import (
	"strings"
	"testing"
)

func TestResolveExecutionRouteUsesStableConnectionNames(t *testing.T) {
	local, err := ResolveExecutionRoute([]ExecutionTarget{ExecutionLocal, ExecutionCloud}, ExecutionLocal, "mac-1")
	if err != nil || local.Connection != "local:mac-1" || local.DeviceID != "mac-1" {
		t.Fatalf("local route=%#v err=%v", local, err)
	}
	cloud, err := ResolveExecutionRoute([]ExecutionTarget{ExecutionLocal, ExecutionCloud}, ExecutionCloud, "ignored")
	if err != nil || cloud.Connection != "cloud" || cloud.DeviceID != "" {
		t.Fatalf("cloud route=%#v err=%v", cloud, err)
	}
}

func TestResolveExecutionRouteFailsClosed(t *testing.T) {
	if _, err := ResolveExecutionRoute([]ExecutionTarget{ExecutionLocal}, ExecutionCloud, ""); err == nil {
		t.Fatal("unsupported cloud execution was accepted")
	}
	if _, err := ResolveExecutionRoute([]ExecutionTarget{ExecutionLocal}, ExecutionLocal, ""); err == nil {
		t.Fatal("local execution without device was accepted")
	}
}

func TestManagedCredentialReferenceIsOpaqueAndPluginScoped(t *testing.T) {
	github := ManagedCredentialReference("github")
	if github == "" || strings.Contains(strings.ToLower(github), "github") || !IsManagedCredentialReference("github", github) {
		t.Fatalf("unexpected managed credential reference %q", github)
	}
	if IsManagedCredentialReference("notion", github) {
		t.Fatal("managed credential reference crossed Plugin boundary")
	}
}

package mcpgateway

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/gateway"
)

func defaultWorkspaceFixture(key, workspaceID, status string, lastSeenAt int64) gateway.WorkspaceView {
	return gateway.WorkspaceView{Key: key, WorkspaceID: workspaceID, Status: status, LastSeenAt: lastSeenAt}
}

func TestSelectDefaultWorkspaceKeyPrefersMostRecentlySeenActiveWorkspace(t *testing.T) {
	catalog := []gateway.WorkspaceView{
		defaultWorkspaceFixture("u::d::alpha", "alpha", "active", 100),
		defaultWorkspaceFixture("u::d::beta", "beta", "active", 500),
		defaultWorkspaceFixture("u::d::sleepy", "sleepy", "sleeping", 900),
	}
	if got := selectDefaultWorkspaceKey(catalog, ""); got != "u::d::beta" {
		t.Fatalf("key=%q want the most recently seen active workspace", got)
	}
}

func TestSelectDefaultWorkspaceKeyHonorsConfiguredIDOrKey(t *testing.T) {
	catalog := []gateway.WorkspaceView{
		defaultWorkspaceFixture("u::d::alpha", "alpha", "active", 100),
		defaultWorkspaceFixture("u::d::beta", "beta", "active", 500),
	}
	if got := selectDefaultWorkspaceKey(catalog, "alpha"); got != "u::d::alpha" {
		t.Fatalf("workspace-id override resolved to %q", got)
	}
	if got := selectDefaultWorkspaceKey(catalog, "u::d::alpha"); got != "u::d::alpha" {
		t.Fatalf("full-key override resolved to %q", got)
	}
	// An override naming a workspace that is not on this machine must not
	// resolve: routing always stays inside the authorized set.
	if got := selectDefaultWorkspaceKey(catalog, "gamma"); got != "" {
		t.Fatalf("unknown override resolved to %q want empty", got)
	}
}

func TestSelectDefaultWorkspaceKeyPrefersActiveOverMoreRecentlySeenSleeping(t *testing.T) {
	catalog := []gateway.WorkspaceView{
		defaultWorkspaceFixture("u::d::awake", "awake", "active", 100),
		defaultWorkspaceFixture("u::d::sleepy", "sleepy", "sleeping", 900),
	}
	if got := selectDefaultWorkspaceKey(catalog, ""); got != "u::d::awake" {
		t.Fatalf("key=%q want the active workspace over a newer sleeping one", got)
	}
}

func TestSelectDefaultWorkspaceKeyUsesSleepingWorkspaceWhenNothingIsActive(t *testing.T) {
	// A sleeping workspace is authorized and its runtime is online, so an
	// unattended sessionless client can still be routed to the project the
	// operator used last instead of failing before it can do anything.
	catalog := []gateway.WorkspaceView{
		defaultWorkspaceFixture("u::d::old", "old", "sleeping", 100),
		defaultWorkspaceFixture("u::d::recent", "recent", "sleeping", 900),
	}
	if got := selectDefaultWorkspaceKey(catalog, ""); got != "u::d::recent" {
		t.Fatalf("key=%q want most recently seen sleeping workspace", got)
	}
}

func TestSelectDefaultWorkspaceKeyIgnoresUnauthorizedEntries(t *testing.T) {
	// device_offline entries are not routable even when configured explicitly.
	catalog := []gateway.WorkspaceView{
		defaultWorkspaceFixture("u::d::gone", "gone", "device_offline", 9999),
	}
	if got := selectDefaultWorkspaceKey(catalog, "gone"); got != "" {
		t.Fatalf("offline device resolved to %q want empty", got)
	}
	if got := selectDefaultWorkspaceKey(catalog, ""); got != "" {
		t.Fatalf("key=%q want empty", got)
	}
}

func TestSelectDefaultWorkspaceKeySkipsManagedSystemProjects(t *testing.T) {
	catalog := []gateway.WorkspaceView{
		defaultWorkspaceFixture("u::d::system-openmontage", "system-openmontage", "active", 5000),
		defaultWorkspaceFixture("u::d::codex", "Codex-fbd20013ed", "active", 100),
	}
	if got := selectDefaultWorkspaceKey(catalog, ""); got != "u::d::codex" {
		t.Fatalf("key=%q want the operator project, not CodeLocal's managed workspace", got)
	}
	// The managed workspace stays explicitly reachable.
	if got := selectDefaultWorkspaceKey(catalog, "system-openmontage"); got != "u::d::system-openmontage" {
		t.Fatalf("explicit selection of a managed workspace resolved to %q", got)
	}
}

func TestSelectDefaultWorkspaceKeyOnlySystemProjectsResolvesEmpty(t *testing.T) {
	catalog := []gateway.WorkspaceView{
		defaultWorkspaceFixture("u::d::system-openmontage", "system-openmontage", "active", 5000),
	}
	if got := selectDefaultWorkspaceKey(catalog, ""); got != "" {
		t.Fatalf("key=%q want empty when only managed system projects exist", got)
	}
}

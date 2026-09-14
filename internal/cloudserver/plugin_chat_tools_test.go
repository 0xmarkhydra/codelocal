package cloudserver

import (
	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"testing"
)

func TestPluginConnectionSelectionNeverSilentlyFallsBack(t *testing.T) {
	items := []cloud.PluginConnection{{UserID: "A", PluginID: "github", DeviceID: "cloud"}, {UserID: "A", PluginID: "github", DeviceID: "mac"}}
	for _, selector := range []string{"", "local:other", "cloud-other"} {
		if _, err := selectPluginConnection(items, "github", selector); err == nil {
			t.Fatalf("selected ambiguous/absent %s", selector)
		}
	}
	for _, selector := range []string{"cloud", "local:mac"} {
		c, err := selectPluginConnection(items, "github", selector)
		if err != nil || c.PluginID != "github" {
			t.Fatal("explicit selection failed", err)
		}
	}
	if _, err := selectPluginConnection(items, "penpot", "cloud"); err == nil {
		t.Fatal("wrong plugin selected")
	}
}
func TestPluginApprovalAndToolErrorStatus(t *testing.T) {
	for _, tc := range []struct{ payload, want string }{
		{`{"status":"approval_required","approvalId":"opaque"}`, "approval_required"},
		{`{"isError":true,"content":[]}`, "error"},
		{`{"ok":true,"result":{"isError":true}}`, "error"},
	} {
		if got := dashboardToolResultStatus(tc.payload); got != tc.want {
			t.Fatalf("%s = %s", tc.payload, got)
		}
	}
}

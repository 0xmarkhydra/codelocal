package cloudserver

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	plugindomain "github.com/0xmarkhydra/codelocal/internal/plugins"
)

func TestPluginCatalogResponseProjectsInstallStateCapabilitiesAndConnections(t *testing.T) {
	entry, ok := plugindomain.FindBuiltin("github")
	if !ok {
		t.Fatal("github plugin missing")
	}
	hash, err := plugindomain.ManifestHash(entry.Manifest)
	if err != nil {
		t.Fatal(err)
	}
	response, err := pluginCatalogResponse([]cloud.PluginInstallation{{
		UserID: "user-1", PluginID: "github", Version: entry.Manifest.Version,
		ManifestHash: hash, State: cloud.PluginInstalled, InstalledAt: 123,
	}}, []cloud.PluginConnection{{
		UserID: "user-1", PluginID: "github", DeviceID: "mac-1", WorkspaceKey: "user-1::mac-1::repo",
		ServerName: "plugin-github", Endpoint: "https://example.com/mcp", State: cloud.PluginConnectionReady, ToolCount: 12,
	}})
	if err != nil {
		t.Fatal(err)
	}
	if response.InstalledCount != 1 {
		t.Fatalf("installed count=%d want 1", response.InstalledCount)
	}
	var github *pluginCatalogItemDTO
	for index := range response.Items {
		if response.Items[index].ID == "github" {
			github = &response.Items[index]
			break
		}
	}
	if github == nil || !github.Installed || github.UpdateAvailable || !github.SetupRequired {
		t.Fatalf("unexpected github catalog item: %#v", github)
	}
	if github.ConnectedCount != 1 || len(github.Connections) != 1 || github.Connections[0].ToolCount != 12 {
		t.Fatalf("unexpected github connections: %#v", github)
	}
	if len(github.Capabilities) < 3 {
		t.Fatalf("github capabilities missing: %#v", github.Capabilities)
	}
}

func TestPluginCatalogResponseDetectsManifestDrift(t *testing.T) {
	entry, _ := plugindomain.FindBuiltin("notion")
	response, err := pluginCatalogResponse([]cloud.PluginInstallation{{
		UserID: "user-1", PluginID: "notion", Version: entry.Manifest.Version,
		ManifestHash: "stale-hash", State: cloud.PluginInstalled,
	}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range response.Items {
		if item.ID == "notion" && !item.UpdateAvailable {
			t.Fatalf("manifest drift must require update: %#v", item)
		}
	}
}

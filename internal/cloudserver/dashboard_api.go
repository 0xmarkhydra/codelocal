package cloudserver

import (
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

const dashboardUsageScope = "MCP payload only; not full AI model/provider billing tokens"

type dashboardOverviewUserDTO struct {
	Email   string `json:"email"`
	IsAdmin bool   `json:"isAdmin"`
}

type dashboardDeviceSummaryDTO struct {
	Paired int `json:"paired"`
	Online int `json:"online"`
}

type dashboardWorkspaceDTO struct {
	DeviceID      string `json:"deviceId"`
	DeviceName    string `json:"deviceName"`
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
	Status        string `json:"status"`
	RuntimeOnline bool   `json:"runtimeOnline"`
	LastSeenAt    int64  `json:"lastSeenAt"`
}

type dashboardWorkspaceSummaryDTO struct {
	Total    int                     `json:"total"`
	Active   int                     `json:"active"`
	Sleeping int                     `json:"sleeping"`
	Offline  int                     `json:"offline"`
	Recent   []dashboardWorkspaceDTO `json:"recent"`
}

type dashboardUsageDTO struct {
	Available bool           `json:"available"`
	Estimated bool           `json:"estimated"`
	Scope     string         `json:"scope"`
	Last24h   usageWindowDTO `json:"last24h"`
	Last30d   usageWindowDTO `json:"last30d"`
	AllTime   usageWindowDTO `json:"allTime"`
}

type dashboardSkillDTO struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Version      string   `json:"version"`
	Publisher    string   `json:"publisher"`
	Scope        string   `json:"scope"`
	Kind         string   `json:"kind"`
	Tags         []string `json:"tags,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Quality      float64  `json:"quality"`
	Verified     bool     `json:"verified"`
	License      string   `json:"license,omitempty"`
}

type dashboardSkillsDTO struct {
	AutoUse bool                `json:"autoUse"`
	Items   []dashboardSkillDTO `json:"items"`
}

type dashboardOverviewDTO struct {
	User       dashboardOverviewUserDTO     `json:"user"`
	Devices    dashboardDeviceSummaryDTO    `json:"devices"`
	Workspaces dashboardWorkspaceSummaryDTO `json:"workspaces"`
	Usage      dashboardUsageDTO            `json:"usage"`
	Skills     dashboardSkillsDTO           `json:"skills"`
}

func dashboardWorkspaceStatus(status string) string {
	switch status {
	case "active":
		return "active"
	case "sleeping":
		return "sleeping"
	default:
		return "offline"
	}
}

func dashboardSkillCatalogDTO(manifests []skillintel.Manifest) dashboardSkillsDTO {
	items := make([]dashboardSkillDTO, 0, len(manifests))
	for _, manifest := range manifests {
		capabilities := make([]string, 0, len(manifest.Capabilities))
		for _, capability := range manifest.Capabilities {
			capabilities = append(capabilities, string(capability))
		}
		items = append(items, dashboardSkillDTO{
			ID: manifest.ID, Name: manifest.Name, Version: manifest.Version,
			Publisher: manifest.Publisher, Scope: string(manifest.Scope), Kind: string(manifest.Kind),
			Tags: append([]string(nil), manifest.Tags...), Capabilities: capabilities,
			Quality: manifest.Quality, Verified: manifest.Verified, License: manifest.License,
		})
	}
	return dashboardSkillsDTO{AutoUse: true, Items: items}
}

type dashboardOverviewSource struct {
	Email          string
	IsAdmin        bool
	Devices        []cloud.Device
	DeviceOnline   map[string]bool
	Workspaces     []gateway.WorkspaceView
	UsageAvailable bool
	Usage24h       cloud.MCPUsageSummary
	Usage30d       cloud.MCPUsageSummary
	UsageAll       cloud.MCPUsageSummary
	Skills         []skillintel.Manifest
}

func buildDashboardOverviewDTO(source dashboardOverviewSource) dashboardOverviewDTO {
	deviceSummary := dashboardDeviceSummaryDTO{}
	for _, device := range source.Devices {
		if device.RevokedAt != 0 {
			continue
		}
		deviceSummary.Paired++
		if source.DeviceOnline[device.DeviceID] {
			deviceSummary.Online++
		}
	}

	workspaceSummary := dashboardWorkspaceSummaryDTO{
		Total:  len(source.Workspaces),
		Recent: make([]dashboardWorkspaceDTO, 0, min(5, len(source.Workspaces))),
	}
	for index, workspace := range source.Workspaces {
		status := dashboardWorkspaceStatus(workspace.Status)
		switch status {
		case "active":
			workspaceSummary.Active++
		case "sleeping":
			workspaceSummary.Sleeping++
		default:
			workspaceSummary.Offline++
		}
		if index >= 5 {
			continue
		}
		workspaceSummary.Recent = append(workspaceSummary.Recent, dashboardWorkspaceDTO{
			DeviceID:      workspace.DeviceID,
			DeviceName:    workspace.DeviceName,
			WorkspaceID:   workspace.WorkspaceID,
			WorkspaceName: workspace.WorkspaceName,
			Status:        status,
			RuntimeOnline: workspace.RuntimeOnline,
			LastSeenAt:    workspace.LastSeenAt,
		})
	}

	return dashboardOverviewDTO{
		User:       dashboardOverviewUserDTO{Email: source.Email, IsAdmin: source.IsAdmin},
		Devices:    deviceSummary,
		Workspaces: workspaceSummary,
		Usage: dashboardUsageDTO{
			Available: source.UsageAvailable,
			Estimated: true,
			Scope:     dashboardUsageScope,
			Last24h:   usageWindow(source.Usage24h),
			Last30d:   usageWindow(source.Usage30d),
			AllTime:   usageWindow(source.UsageAll),
		},
		Skills: dashboardSkillCatalogDTO(source.Skills),
	}
}

func (s *Server) dashboardOverviewAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}

	devices, err := s.Store.ListDevices(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dashboard_devices_unavailable"})
		return
	}
	workspaces, err := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "dashboard_workspaces_unavailable"})
		return
	}

	deviceOnline := make(map[string]bool, len(devices))
	for _, device := range devices {
		if device.RevokedAt != 0 {
			continue
		}
		online, onlineErr := s.Activation.IsOnline(r.Context(), identity.User.ID, device.DeviceID)
		if onlineErr == nil {
			deviceOnline[device.DeviceID] = online
		}
	}

	now := time.Now()
	usage24h, err24h := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, now.Add(-24*time.Hour).UnixMilli())
	usage30d, err30d := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, now.Add(-30*24*time.Hour).UnixMilli())
	usageAll, errAll := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, 0)
	usageAvailable := err24h == nil && err30d == nil && errAll == nil
	if !usageAvailable {
		usage24h = cloud.MCPUsageSummary{}
		usage30d = cloud.MCPUsageSummary{}
		usageAll = cloud.MCPUsageSummary{}
	}

	skillCatalog := skillintel.DefaultEngine().Catalog()
	services := skillServicesForServer(s)
	if services.Runtime != nil {
		if snapshot, skillErr := services.Runtime.Snapshot(r.Context(), identity.User.ID); skillErr == nil && snapshot.Engine != nil {
			skillCatalog = snapshot.Engine.Catalog()
		}
	}

	webutil.JSON(w, http.StatusOK, buildDashboardOverviewDTO(dashboardOverviewSource{
		Email:          identity.User.Email,
		IsAdmin:        cloud.IsAdminEmail(identity.User.Email),
		Devices:        devices,
		DeviceOnline:   deviceOnline,
		Workspaces:     workspaces,
		UsageAvailable: usageAvailable,
		Usage24h:       usage24h,
		Usage30d:       usage30d,
		UsageAll:       usageAll,
		Skills:         skillCatalog,
	}))
}

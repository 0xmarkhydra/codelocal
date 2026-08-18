package cloudserver

import (
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
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

type dashboardUsageWindowDTO struct {
	Calls                 int64 `json:"calls"`
	InputTokensEstimated  int64 `json:"inputTokensEstimated"`
	OutputTokensEstimated int64 `json:"outputTokensEstimated"`
	TotalTokensEstimated  int64 `json:"totalTokensEstimated"`
}

type dashboardUsageDTO struct {
	Available bool                    `json:"available"`
	Estimated bool                    `json:"estimated"`
	Scope     string                  `json:"scope"`
	Last24h   dashboardUsageWindowDTO `json:"last24h"`
	Last30d   dashboardUsageWindowDTO `json:"last30d"`
	AllTime   dashboardUsageWindowDTO `json:"allTime"`
}

type dashboardOverviewDTO struct {
	User       dashboardOverviewUserDTO     `json:"user"`
	Devices    dashboardDeviceSummaryDTO    `json:"devices"`
	Workspaces dashboardWorkspaceSummaryDTO `json:"workspaces"`
	Usage      dashboardUsageDTO            `json:"usage"`
}

func dashboardUsageWindow(value cloud.MCPUsageSummary) dashboardUsageWindowDTO {
	return dashboardUsageWindowDTO{
		Calls:                 value.Calls,
		InputTokensEstimated:  value.InputTokensEst,
		OutputTokensEstimated: value.OutputTokensEst,
		TotalTokensEstimated:  value.TotalTokensEst,
	}
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
			Last24h:   dashboardUsageWindow(source.Usage24h),
			Last30d:   dashboardUsageWindow(source.Usage30d),
			AllTime:   dashboardUsageWindow(source.UsageAll),
		},
	}
}

func (s *Server) dashboardOverviewAPI(w http.ResponseWriter, r *http.Request) {
	identity, err := s.WebAuth.Identity(r)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]string{"error": "identity_unavailable"})
		return
	}
	if identity == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
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
	}))
}

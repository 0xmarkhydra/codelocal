package cloudserver

import (
	"net/http"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type deviceResourceDTO struct {
	DeviceID   string `json:"deviceId"`
	DeviceName string `json:"deviceName"`
	Status     string `json:"status"`
	CreatedAt  int64  `json:"createdAt"`
	LastSeenAt int64  `json:"lastSeenAt"`
}

type deviceResourceSummaryDTO struct {
	Paired  int `json:"paired"`
	Online  int `json:"online"`
	Revoked int `json:"revoked"`
}

type devicesResourceDTO struct {
	Summary deviceResourceSummaryDTO `json:"summary"`
	Items   []deviceResourceDTO      `json:"items"`
}

type workspaceResourceDTO struct {
	DeviceID      string `json:"deviceId"`
	DeviceName    string `json:"deviceName"`
	WorkspaceID   string `json:"workspaceId"`
	WorkspaceName string `json:"workspaceName"`
	Status        string `json:"status"`
	RuntimeOnline bool   `json:"runtimeOnline"`
	LastSeenAt    int64  `json:"lastSeenAt"`
}

type workspaceResourceSummaryDTO struct {
	Total    int `json:"total"`
	Active   int `json:"active"`
	Sleeping int `json:"sleeping"`
	Offline  int `json:"offline"`
}

type workspacesResourceDTO struct {
	Summary workspaceResourceSummaryDTO `json:"summary"`
	Items   []workspaceResourceDTO      `json:"items"`
}

func buildDevicesResourceDTO(devices []cloud.Device, onlineByDevice map[string]bool) devicesResourceDTO {
	result := devicesResourceDTO{Items: make([]deviceResourceDTO, 0, len(devices))}
	for _, device := range devices {
		status := "offline"
		if device.RevokedAt != 0 {
			status = "revoked"
			result.Summary.Revoked++
		} else {
			result.Summary.Paired++
			if onlineByDevice[device.DeviceID] {
				status = "online"
				result.Summary.Online++
			}
		}
		result.Items = append(result.Items, deviceResourceDTO{
			DeviceID:   device.DeviceID,
			DeviceName: device.DeviceName,
			Status:     status,
			CreatedAt:  device.CreatedAt,
			LastSeenAt: device.LastSeenAt,
		})
	}
	return result
}

func buildWorkspacesResourceDTO(workspaces []gateway.WorkspaceView) workspacesResourceDTO {
	result := workspacesResourceDTO{
		Summary: workspaceResourceSummaryDTO{Total: len(workspaces)},
		Items:   make([]workspaceResourceDTO, 0, len(workspaces)),
	}
	for _, workspace := range workspaces {
		status := dashboardWorkspaceStatus(workspace.Status)
		switch status {
		case "active":
			result.Summary.Active++
		case "sleeping":
			result.Summary.Sleeping++
		default:
			result.Summary.Offline++
		}
		result.Items = append(result.Items, workspaceResourceDTO{
			DeviceID:      workspace.DeviceID,
			DeviceName:    workspace.DeviceName,
			WorkspaceID:   workspace.WorkspaceID,
			WorkspaceName: workspace.WorkspaceName,
			Status:        status,
			RuntimeOnline: workspace.RuntimeOnline,
			LastSeenAt:    workspace.LastSeenAt,
		})
	}
	return result
}

func (s *Server) devicesResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	devices, err := s.Store.ListDevices(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "devices_unavailable"})
		return
	}
	onlineByDevice := make(map[string]bool, len(devices))
	for _, device := range devices {
		if device.RevokedAt != 0 {
			continue
		}
		online, onlineErr := s.Activation.IsOnline(r.Context(), identity.User.ID, device.DeviceID)
		if onlineErr == nil {
			onlineByDevice[device.DeviceID] = online
		}
	}
	webutil.JSON(w, http.StatusOK, buildDevicesResourceDTO(devices, onlineByDevice))
}

func (s *Server) workspacesResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	workspaces, err := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "workspaces_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, buildWorkspacesResourceDTO(workspaces))
}

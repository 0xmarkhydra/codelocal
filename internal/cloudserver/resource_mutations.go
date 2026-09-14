package cloudserver

import (
	"context"
	"net/http"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func mutationPublicID(value string, limit int) string {
	value = strings.TrimSpace(value)
	if value == "" || len(value) > limit || strings.ContainsAny(value, "\x00\r\n") {
		return ""
	}
	return value
}

func deviceByPublicID(devices []cloud.Device, deviceID string) *cloud.Device {
	deviceID = strings.TrimSpace(deviceID)
	for i := range devices {
		if devices[i].DeviceID == deviceID && devices[i].RevokedAt == 0 {
			value := devices[i]
			return &value
		}
	}
	return nil
}

func deviceByCredentialID(devices []cloud.Device, credentialID string) *cloud.Device {
	credentialID = strings.TrimSpace(credentialID)
	for i := range devices {
		if devices[i].CredentialID == credentialID && devices[i].RevokedAt == 0 {
			value := devices[i]
			return &value
		}
	}
	return nil
}

func workspaceByPublicID(workspaces []gateway.WorkspaceView, deviceID, workspaceID string) *gateway.WorkspaceView {
	for i := range workspaces {
		if workspaces[i].DeviceID == deviceID && workspaces[i].WorkspaceID == workspaceID {
			value := workspaces[i]
			return &value
		}
	}
	return nil
}

func (s *Server) revokeResolvedDevice(ctx context.Context, identity *webauth.Identity, device cloud.Device) (bool, error) {
	revoked, err := s.Store.RevokeDevice(ctx, identity.User.ID, device.CredentialID)
	if err != nil || !revoked {
		return revoked, err
	}
	s.disconnectCredentialEverywhere(identity.User.ID, device.CredentialID)
	if device.DeviceID != "" {
		presenceCtx, cancelPresence := context.WithTimeout(context.Background(), 2*time.Second)
		_ = s.Activation.ClearPresence(presenceCtx, identity.User.ID, device.DeviceID)
		cancelPresence()
	}
	s.Store.Audit(cloud.AuditEvent{
		UserID: identity.User.ID, Event: "device.revoked", DeviceID: device.DeviceID,
		Detail: map[string]any{"credentialId": device.CredentialID},
	})
	return true, nil
}

func (s *Server) revokeDeviceResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if !s.WebAuth.RequireFreshSecurityContext(w, r, identity, "/dashboard/devices") {
		return
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid_csrf"})
		return
	}
	deviceID := mutationPublicID(r.PathValue("deviceID"), 160)
	if deviceID == "" {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_device"})
		return
	}
	devices, err := s.Store.ListDevices(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "devices_unavailable"})
		return
	}
	device := deviceByPublicID(devices, deviceID)
	if device == nil {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "device_not_found"})
		return
	}
	revoked, err := s.revokeResolvedDevice(r.Context(), identity, *device)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "device_revoke_failed"})
		return
	}
	if !revoked {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "device_already_revoked"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "deviceId": deviceID})
}

func (s *Server) removeResolvedWorkspace(ctx context.Context, identity *webauth.Identity, deviceID, workspaceID, workspaceName string) error {
	if err := s.Workspaces.Revoke(ctx, identity.User.ID, deviceID, workspaceID); err != nil {
		return err
	}
	s.Store.Audit(cloud.AuditEvent{
		UserID: identity.User.ID, Event: "workspace.revocation_requested", DeviceID: deviceID, WorkspaceID: workspaceID,
		Detail: map[string]any{"workspaceName": workspaceName},
	})
	return nil
}

func (s *Server) removeWorkspaceResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if !s.WebAuth.RequireFreshSecurityContext(w, r, identity, "/dashboard/workspaces") {
		return
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid_csrf"})
		return
	}
	deviceID := mutationPublicID(r.PathValue("deviceID"), 160)
	workspaceID := mutationPublicID(r.PathValue("workspaceID"), 200)
	if deviceID == "" || workspaceID == "" {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_workspace"})
		return
	}
	catalog, err := s.Workspaces.Catalog(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "workspaces_unavailable"})
		return
	}
	workspace := workspaceByPublicID(catalog, deviceID, workspaceID)
	if workspace == nil {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "workspace_not_found"})
		return
	}
	if !workspace.RuntimeOnline {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "workspace_runtime_offline"})
		return
	}
	if err := s.removeResolvedWorkspace(r.Context(), identity, deviceID, workspaceID, workspace.WorkspaceName); err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "workspace_remove_failed"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{
		"ok": true, "deviceId": deviceID, "workspaceId": workspaceID,
		"message": workspace.WorkspaceName + " access removed. The project files were not changed.",
	})
}

func (s *Server) installSystemAppResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	if !s.WebAuth.RequireFreshSecurityContext(w, r, identity, "/dashboard/workspaces") {
		return
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid_csrf"})
		return
	}
	appID := mutationPublicID(r.PathValue("appID"), 80)
	deviceID := mutationPublicID(r.PathValue("deviceID"), 160)
	if appID != cloud.OpenMontageSystemProjectID || deviceID == "" {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "unsupported_system_app"})
		return
	}
	devices, err := s.Store.ListDevices(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "devices_unavailable"})
		return
	}
	if deviceByPublicID(devices, deviceID) == nil {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "device_not_found"})
		return
	}
	online, err := s.Activation.IsOnline(r.Context(), identity.User.ID, deviceID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "runtime_status_unavailable"})
		return
	}
	if !online {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "system_app_runtime_offline"})
		return
	}
	key := gateway.ClientKey(identity.User.ID, deviceID, cloud.OpenMontageWorkspaceID)
	if owner, _ := s.Coordinator.Owner(r.Context(), key); owner != "" {
		webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "deviceId": deviceID, "workspaceId": cloud.OpenMontageWorkspaceID, "message": cloud.OpenMontageName + " is ready."})
		return
	}
	requestID := cloud.RandomHex(16)
	activation := cloud.WorkspaceActivation{WorkspaceID: cloud.OpenMontageWorkspaceID, Action: "install-system-app", RequestedAt: time.Now().UnixMilli(), RequestID: requestID}
	if err := s.Activation.Request(r.Context(), identity.User.ID, deviceID, activation, 2*time.Minute); err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "system_app_install_failed"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "system_app.install_requested", DeviceID: deviceID, WorkspaceID: cloud.OpenMontageWorkspaceID, Detail: map[string]any{"appId": appID, "requestId": requestID}})
	waitCtx, cancel := context.WithTimeout(r.Context(), 90*time.Second)
	defer cancel()
	if _, err := s.Coordinator.WaitOwner(waitCtx, key); err != nil {
		webutil.JSON(w, http.StatusGatewayTimeout, map[string]string{"error": "system_app_install_timeout"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "system_app.installed", DeviceID: deviceID, WorkspaceID: cloud.OpenMontageWorkspaceID, Detail: map[string]any{"appId": appID, "requestId": requestID}})
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "deviceId": deviceID, "workspaceId": cloud.OpenMontageWorkspaceID, "message": cloud.OpenMontageName + " installed and ready."})
}

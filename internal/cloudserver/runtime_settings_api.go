package cloudserver

import (
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func runtimeScopeFromRequest(r *http.Request) cloud.RuntimeScope {
	scope := strings.TrimSpace(r.URL.Query().Get("scope"))
	if scope == "" {
		scope = strings.TrimSpace(r.FormValue("scope"))
	}
	if scope == "" {
		scope = string(cloud.RuntimeScopeGlobal)
	}
	return cloud.RuntimeScope(scope)
}

func runtimeTargetFromRequest(r *http.Request) (cloud.RuntimeScope, string, string) {
	return runtimeScopeFromRequest(r), strings.TrimSpace(first(r.URL.Query().Get("deviceId"), r.FormValue("deviceId"))), strings.TrimSpace(first(r.URL.Query().Get("workspaceId"), r.FormValue("workspaceId")))
}

func (s *Server) runtimeSettingsResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	scope, deviceID, workspaceID := runtimeTargetFromRequest(r)
	layer, err := s.Store.RuntimeSettingsLayer(r.Context(), identity.User.ID, scope, deviceID, workspaceID)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_runtime_scope"})
		return
	}
	effective, err := s.Store.ResolveRuntimeTarget(r.Context(), identity.User.ID, scope, deviceID, workspaceID)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "runtime_settings_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"scope": scope, "deviceId": deviceID, "workspaceId": workspaceID, "layer": layer, "effective": effective})
}

func (s *Server) runtimeConfigMutationAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]any{"error": "invalid_csrf"})
		return
	}
	scope, deviceID, workspaceID := runtimeTargetFromRequest(r)
	layer, err := s.Store.RuntimeSettingsLayer(r.Context(), identity.User.ID, scope, deviceID, workspaceID)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_runtime_scope"})
		return
	}
	if layer.Values == nil {
		layer.Values = map[string]string{}
	}
	key := strings.TrimSpace(r.FormValue("key"))
	if !cloud.ValidRuntimeEnvKey(key) {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_runtime_key"})
		return
	}
	if r.FormValue("action") == "delete" {
		delete(layer.Values, key)
	} else {
		layer.Values[key] = r.FormValue("value")
	}
	if err := s.Store.PutRuntimeSettingsLayer(r.Context(), identity.User.ID, layer, deviceID, workspaceID); err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "runtime_config_update_failed"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "runtime.config_updated", DeviceID: deviceID, WorkspaceID: workspaceID, Detail: map[string]any{"scope": scope, "key": key, "action": first(r.FormValue("action"), "set")}})
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) runtimeSecretMutationAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]any{"error": "invalid_csrf"})
		return
	}
	scope, deviceID, workspaceID := runtimeTargetFromRequest(r)
	name := strings.TrimSpace(r.FormValue("name"))
	if !cloud.ValidRuntimeEnvKey(name) {
		webutil.JSON(w, http.StatusBadRequest, map[string]any{"error": "invalid_runtime_key"})
		return
	}
	var err error
	if r.FormValue("action") == "delete" {
		err = s.Store.DeleteRuntimeSecret(r.Context(), identity.User.ID, scope, deviceID, workspaceID, name)
	} else {
		err = s.Store.PutRuntimeSecret(r.Context(), identity.User.ID, scope, deviceID, workspaceID, name, r.FormValue("value"))
	}
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "runtime_secret_update_failed"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "runtime.secret_updated", DeviceID: deviceID, WorkspaceID: workspaceID, Detail: map[string]any{"scope": scope, "key": name, "action": first(r.FormValue("action"), "set")}})
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

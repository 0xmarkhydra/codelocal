package cloudserver

import (
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	plugindomain "github.com/0xmarkhydra/codelocal/internal/plugins"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type pluginCatalogItemDTO struct {
	ID                string   `json:"id"`
	Name              string   `json:"name"`
	Version           string   `json:"version"`
	Description       string   `json:"description,omitempty"`
	Publisher         string   `json:"publisher"`
	PublisherVerified bool     `json:"publisherVerified"`
	Categories        []string `json:"categories,omitempty"`
	Capabilities      []string `json:"capabilities,omitempty"`
	Featured          bool     `json:"featured,omitempty"`
	Installed         bool     `json:"installed"`
	InstallationState string   `json:"installationState,omitempty"`
	InstalledAt       int64    `json:"installedAt,omitempty"`
	UpdateAvailable   bool     `json:"updateAvailable,omitempty"`
	SetupRequired     bool     `json:"setupRequired,omitempty"`
}

type pluginCatalogResponseDTO struct {
	Items          []pluginCatalogItemDTO `json:"items"`
	InstalledCount int                    `json:"installedCount"`
}

func pluginCatalogResponse(installations []cloud.PluginInstallation) (pluginCatalogResponseDTO, error) {
	installedByID := make(map[string]cloud.PluginInstallation, len(installations))
	for _, installation := range installations {
		installedByID[installation.PluginID] = installation
	}
	response := pluginCatalogResponseDTO{Items: []pluginCatalogItemDTO{}}
	for _, entry := range plugindomain.BuiltinCatalog() {
		manifest := entry.Manifest
		hash, err := plugindomain.ManifestHash(manifest)
		if err != nil {
			return pluginCatalogResponseDTO{}, err
		}
		capabilities := plugindomain.ManifestCapabilities(manifest)
		capabilityNames := make([]string, 0, len(capabilities))
		for _, capability := range capabilities {
			capabilityNames = append(capabilityNames, string(capability))
		}
		item := pluginCatalogItemDTO{
			ID: manifest.ID, Name: manifest.Name, Version: manifest.Version,
			Description: manifest.Description, Publisher: manifest.Publisher.Name,
			PublisherVerified: manifest.Publisher.Verified,
			Categories:        append([]string(nil), manifest.Categories...),
			Capabilities:      capabilityNames, Featured: entry.Featured,
			SetupRequired: manifestNeedsSetup(manifest),
		}
		if installation, ok := installedByID[manifest.ID]; ok {
			item.InstallationState = string(installation.State)
			item.Installed = installation.State == cloud.PluginInstalled
			item.InstalledAt = installation.InstalledAt
			item.UpdateAvailable = installation.Version != manifest.Version || installation.ManifestHash != hash
			if item.Installed {
				response.InstalledCount++
			}
		}
		response.Items = append(response.Items, item)
	}
	return response, nil
}

func manifestNeedsSetup(manifest plugindomain.Manifest) bool {
	for _, component := range manifest.Components {
		if component.Kind == plugindomain.ComponentAppTemplate && component.AppTemplate != nil {
			return true
		}
	}
	return false
}

func (s *Server) pluginsResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	installations, err := s.Store.ListPluginInstallations(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugins_unavailable"})
		return
	}
	response, err := pluginCatalogResponse(installations)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]string{"error": "plugin_catalog_invalid"})
		return
	}
	webutil.JSON(w, http.StatusOK, response)
}

func (s *Server) pluginMutationIdentity(w http.ResponseWriter, r *http.Request, requireFresh bool) (*webauth.Identity, bool) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return nil, false
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid_csrf"})
		return nil, false
	}
	if requireFresh && identity.RequiresReauthentication() {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "reauthentication_required"})
		return nil, false
	}
	return identity, true
}

func (s *Server) pluginInstallAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.pluginMutationIdentity(w, r, true)
	if !ok {
		return
	}
	pluginID := strings.TrimSpace(r.PathValue("pluginID"))
	entry, exists := plugindomain.FindBuiltin(pluginID)
	if !exists {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "plugin_not_found"})
		return
	}
	installation, err := cloud.NewPluginInstallation(identity.User.ID, entry.Manifest)
	if err != nil {
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "plugin_manifest_rejected", "detail": err.Error()})
		return
	}
	if err := s.Store.SetPluginInstallation(r.Context(), installation); err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_install_failed"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.installed", Detail: map[string]any{
		"pluginId": pluginID, "version": installation.Version, "manifestHash": installation.ManifestHash,
	}})
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "installation": installation})
}

func (s *Server) pluginUninstallAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.pluginMutationIdentity(w, r, false)
	if !ok {
		return
	}
	pluginID := strings.TrimSpace(r.PathValue("pluginID"))
	if _, exists := plugindomain.FindBuiltin(pluginID); !exists {
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "plugin_not_found"})
		return
	}
	removed, err := s.Store.DeletePluginInstallation(r.Context(), identity.User.ID, pluginID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "plugin_uninstall_failed"})
		return
	}
	if removed {
		s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "plugin.uninstalled", Detail: map[string]any{"pluginId": pluginID}})
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true, "removed": removed})
}

package cloudserver

import (
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func collectiveFormBool(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "1", "true", "yes", "on", "enabled":
		return true
	default:
		return false
	}
}

func collectivePreferencePayload(preference cloud.CollectivePreference) map[string]any {
	return map[string]any{
		"preference": preference,
		"rollout": map[string]any{
			"contributionAvailable": cloud.CollectiveContributionAvailable(),
			"suggestionsAvailable":  cloud.CollectiveSuggestionsAvailable(),
			"minimumContributors":   cloud.CollectiveMinimumContributors(),
		},
		"privacy": map[string]any{
			"rawCodeShared":          false,
			"conversationShared":     false,
			"projectIdentityShared":  false,
			"localReplayTrustShared": false,
		},
	}
}

func (s *Server) collectivePreferencesGet(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	preference, err := s.Store.CollectivePreference(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "collective_preferences_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, collectivePreferencePayload(preference))
}

func (s *Server) collectivePreferencesPost(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.identity(r)
	if !ok {
		webutil.JSON(w, http.StatusUnauthorized, map[string]any{"error": "unauthorized"})
		return
	}
	if !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]any{"error": "invalid_csrf"})
		return
	}
	preference, err := s.Store.SetCollectivePreference(
		r.Context(), identity.User.ID,
		collectiveFormBool(r.FormValue("contributionEnabled")),
		collectiveFormBool(r.FormValue("suggestionsEnabled")),
	)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]any{"error": "collective_preferences_update_failed"})
		return
	}
	s.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "collective.preference_updated", Detail: map[string]any{
		"contributionEnabled": preference.ContributionEnabled,
		"suggestionsEnabled":  preference.SuggestionsEnabled,
	}})
	webutil.JSON(w, http.StatusOK, collectivePreferencePayload(preference))
}

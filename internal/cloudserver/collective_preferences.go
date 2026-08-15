package cloudserver

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
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
	if next := strings.TrimSpace(r.FormValue("next")); next != "" {
		http.Redirect(w, r, webutil.SafeNext(next), http.StatusSeeOther)
		return
	}
	webutil.JSON(w, http.StatusOK, collectivePreferencePayload(preference))
}

func collectivePreferencesCard(identity *webauth.Identity, preference cloud.CollectivePreference) string {
	if identity == nil {
		return ""
	}
	contributionChecked := ""
	if preference.ContributionEnabled {
		contributionChecked = " checked"
	}
	suggestionsChecked := ""
	if preference.SuggestionsEnabled {
		suggestionsChecked = " checked"
	}
	contributionRollout := "paused"
	if cloud.CollectiveContributionAvailable() {
		contributionRollout = "available"
	}
	suggestionsRollout := "paused"
	if cloud.CollectiveSuggestionsAvailable() {
		suggestionsRollout = "available"
	}
	return `<div class="card knowledge-collective-card" style="max-width:1600px;margin:0 auto 10px;padding:16px 18px">` +
		`<div class="section-head"><div><div class="section-kicker">Collective intelligence</div><div class="title">Learn from verified engineering patterns — privately</div><div class="label">Optional. CodeLocal contributes only structured, de-identified outcome buckets; never raw code, conversations, project/repository identity, paths, symbols, root causes or machine replay trust.</div></div><span class="badge blue">Private opt-in</span></div>` +
		`<form method="post" action="/api/collective/preferences" style="display:grid;gap:10px;margin-top:14px">` +
		ui.Hidden(map[string]string{"csrf": identity.CSRF, "next": "/dashboard/knowledge"}) +
		`<label style="display:flex;gap:10px;align-items:flex-start"><input type="checkbox" name="contributionEnabled" value="1"` + contributionChecked + `><span><strong>Contribute anonymized engineering patterns</strong><span class="row-meta" style="display:block">Rollout: ` + ui.Escape(contributionRollout) + `. Turning this off withdraws your current collective contribution ledger and aggregates.</span></span></label>` +
		`<label style="display:flex;gap:10px;align-items:flex-start"><input type="checkbox" name="suggestionsEnabled" value="1"` + suggestionsChecked + `><span><strong>Receive collective suggestions</strong><span class="row-meta" style="display:block">Rollout: ` + ui.Escape(suggestionsRollout) + `. Suggestions require a privacy cohort and never override your project rules or local verification.</span></span></label>` +
		`<div><button class="btn primary" type="submit">Save collective settings</button></div></form></div>`
}

func collectiveRecommendationsCard(preference cloud.CollectivePreference, recommendations []cloud.CollectiveRecommendation) string {
	if !preference.SuggestionsEnabled {
		return ""
	}
	if !cloud.CollectiveSuggestionsAvailable() {
		return `<div class="card" style="max-width:1600px;margin:0 auto 10px;padding:14px 18px"><div class="section-kicker">Collective suggestions</div><div class="label">Your preference is saved. Recommendation rollout is currently paused on this server.</div></div>`
	}
	if len(recommendations) == 0 {
		return `<div class="card" style="max-width:1600px;margin:0 auto 10px;padding:14px 18px"><div class="section-kicker">Collective suggestions</div><div class="title">No privacy-safe cohort yet</div><div class="label">CodeLocal will only show a pattern after at least ` + fmt.Sprintf("%d", cloud.CollectiveMinimumContributors()) + ` other users independently contribute the same structured pattern.</div></div>`
	}
	var rows strings.Builder
	for _, item := range recommendations {
		checks := strings.Join(item.Fingerprint.CheckProfile, ", ")
		if checks == "" {
			checks = "verified"
		}
		rows.WriteString(`<div class="row"><div class="row-title">` + ui.Escape(strings.Title(strings.ReplaceAll(item.Fingerprint.TaskKind, "_", " "))) + ` · ` + fmt.Sprintf("%.0f%%", item.MeanUserSuccessRate*100) + ` mean success</div>` +
			`<div class="row-meta">` + fmt.Sprintf("%d", item.ContributorCount) + ` other contributors · checks ` + ui.Escape(checks) + ` · files ` + ui.Escape(item.Fingerprint.FileCountBucket) + ` · quality ` + ui.Escape(item.Fingerprint.QualityBucket) + ` · tool ` + ui.Escape(item.Fingerprint.ExecutionTool) + `. No contributor identity or project name is exposed.</div></div>`)
	}
	return `<div class="card" style="max-width:1600px;margin:0 auto 10px;padding:16px 18px"><div class="section-head"><div><div class="section-kicker">Collective suggestions</div><div class="title">Patterns with enough independent evidence</div><div class="label">These are aggregate outcome signals only. They do not override Project Brain rules, quality policy or local verification.</div></div><span class="badge blue">Cohort-safe</span></div><div style="margin-top:12px">` + rows.String() + `</div></div>`
}

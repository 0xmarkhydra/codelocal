package cloudserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

// skillJSONCSRF adapts the existing WebAuth form-token contract for JSON
// mutation APIs without weakening or duplicating CSRF validation. Browser
// clients send X-CSRF-Token; only the cloned inner request receives it as the
// csrf form/query value consumed by WebAuth.VerifyCSRF.
func skillJSONCSRF(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := strings.TrimSpace(r.Header.Get("X-CSRF-Token"))
		if token == "" {
			next.ServeHTTP(w, r)
			return
		}
		clone := r.Clone(r.Context())
		query := clone.URL.Query()
		if strings.TrimSpace(query.Get("csrf")) == "" {
			query.Set("csrf", token)
			clone.URL.RawQuery = query.Encode()
		}
		next.ServeHTTP(w, clone)
	})
}

func (s *Server) registerSkillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/skills", s.skillsManagementResourceAPI)
	personalImport := skillJSONCSRF(http.HandlerFunc(s.skillImportPersonalAPI))
	communityPublish := skillJSONCSRF(http.HandlerFunc(s.skillPublishCommunityAPI))
	mux.Handle("POST /api/v1/skills/import", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "skill-personal-import-ip", Limit: 20, Window: 10 * time.Minute}, personalImport))
	mux.Handle("POST /api/v1/skills/publish", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "skill-community-publish-ip", Limit: 10, Window: 10 * time.Minute}, communityPublish))
	mux.Handle("PATCH /api/v1/skills/{skillID}/state", skillJSONCSRF(http.HandlerFunc(s.skillUserStateAPI)))
	mux.Handle("POST /api/v1/skills/{skillID}/rating", skillJSONCSRF(http.HandlerFunc(s.skillRatingAPI)))

	mux.HandleFunc("GET /api/v1/admin/skills", s.adminSkillsResourceAPI)
	adminImport := skillJSONCSRF(http.HandlerFunc(s.adminSkillImportAPI))
	mux.Handle("POST /api/v1/admin/skills/import", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "skill-admin-import-ip", Limit: 30, Window: 10 * time.Minute}, adminImport))
	mux.Handle("POST /api/v1/admin/skills/{skillID}/versions/{version}/evaluate/start", skillJSONCSRF(http.HandlerFunc(s.adminSkillEvaluationStartAPI)))
	mux.Handle("POST /api/v1/admin/skills/{skillID}/versions/{version}/evaluate", skillJSONCSRF(http.HandlerFunc(s.adminSkillEvaluationCompleteV2API)))
	mux.Handle("POST /api/v1/admin/skills/{skillID}/versions/{version}/promote", skillJSONCSRF(http.HandlerFunc(s.adminSkillPromoteAPI)))
	mux.Handle("POST /api/v1/admin/skills/{skillID}/versions/{version}/rollback", skillJSONCSRF(http.HandlerFunc(s.adminSkillRollbackAPI)))
}

package cloudserver

import (
	"net/http"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

// dashboardJSONCSRF adapts the existing WebAuth form-token contract for JSON
// mutation APIs without weakening or duplicating CSRF validation. Browser
// clients send X-CSRF-Token; only the cloned inner request receives it as the
// csrf query value consumed by WebAuth.VerifyCSRF.
func dashboardJSONCSRF(next http.Handler) http.Handler {
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
	personalImport := dashboardJSONCSRF(http.HandlerFunc(s.skillImportPersonalAPI))
	communityPublish := dashboardJSONCSRF(http.HandlerFunc(s.skillPublishCommunityAPI))
	mux.Handle("POST /api/v1/skills/import", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "skill-personal-import-ip", Limit: 20, Window: 10 * time.Minute}, personalImport))
	mux.Handle("POST /api/v1/skills/publish", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "skill-community-publish-ip", Limit: 10, Window: 10 * time.Minute}, communityPublish))
	mux.Handle("PATCH /api/v1/skills/{skillID}/state", dashboardJSONCSRF(http.HandlerFunc(s.skillUserStateAPI)))
	mux.Handle("POST /api/v1/skills/{skillID}/rating", dashboardJSONCSRF(http.HandlerFunc(s.skillRatingAPI)))

	mux.HandleFunc("GET /api/v1/admin/skills", s.adminSkillsResourceAPI)
	adminImport := dashboardJSONCSRF(http.HandlerFunc(s.adminSkillImportAPI))
	mux.Handle("POST /api/v1/admin/skills/import", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "skill-admin-import-ip", Limit: 30, Window: 10 * time.Minute}, adminImport))
	mux.Handle("POST /api/v1/admin/skills/{skillID}/versions/{version}/evaluate/start", dashboardJSONCSRF(http.HandlerFunc(s.adminSkillEvaluationStartAPI)))
	mux.Handle("POST /api/v1/admin/skills/{skillID}/versions/{version}/evaluate", dashboardJSONCSRF(http.HandlerFunc(s.adminSkillEvaluationCompleteV2API)))
	mux.Handle("POST /api/v1/admin/skills/{skillID}/versions/{version}/promote", dashboardJSONCSRF(http.HandlerFunc(s.adminSkillPromoteAPI)))
	mux.Handle("POST /api/v1/admin/skills/{skillID}/versions/{version}/rollback", dashboardJSONCSRF(http.HandlerFunc(s.adminSkillRollbackAPI)))

	// server.go has one decomposed product-route hook here. Keep feature-owned
	// route groups separate so they can move to a neutral registry without
	// changing handler contracts.
	s.registerBlogRoutes(mux)
	s.registerMediaAssetRoutes(mux)
}

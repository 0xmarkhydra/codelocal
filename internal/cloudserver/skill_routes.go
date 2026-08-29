package cloudserver

import (
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) registerSkillRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/skills", s.skillsResourceAPI)
	mux.Handle("POST /api/v1/skills/import", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "skill-personal-import-ip", Limit: 20, Window: 10 * time.Minute}, http.HandlerFunc(s.skillImportPersonalAPI)))
	mux.Handle("POST /api/v1/skills/publish", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "skill-community-publish-ip", Limit: 10, Window: 10 * time.Minute}, http.HandlerFunc(s.skillPublishCommunityAPI)))
	mux.HandleFunc("PATCH /api/v1/skills/{skillID}/state", s.skillUserStateAPI)
	mux.HandleFunc("POST /api/v1/skills/{skillID}/rating", s.skillRatingAPI)

	mux.HandleFunc("GET /api/v1/admin/skills", s.adminSkillsResourceAPI)
	mux.Handle("POST /api/v1/admin/skills/import", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "skill-admin-import-ip", Limit: 30, Window: 10 * time.Minute}, http.HandlerFunc(s.adminSkillImportAPI)))
	mux.HandleFunc("POST /api/v1/admin/skills/{skillID}/versions/{version}/evaluate/start", s.adminSkillEvaluationStartAPI)
	mux.HandleFunc("POST /api/v1/admin/skills/{skillID}/versions/{version}/evaluate", s.adminSkillEvaluationCompleteAPI)
	mux.HandleFunc("POST /api/v1/admin/skills/{skillID}/versions/{version}/promote", s.adminSkillPromoteAPI)
	mux.HandleFunc("POST /api/v1/admin/skills/{skillID}/versions/{version}/rollback", s.adminSkillRollbackAPI)
}

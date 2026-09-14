package cloudserver

import (
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) registerForumRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/forums/topics", s.forumTopicsAPI)
	createTopic := dashboardJSONCSRF(http.HandlerFunc(s.forumTopicsAPI))
	mux.Handle("POST /api/v1/forums/topics", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "forum-topic-create-ip", Limit: 20, Window: 10 * time.Minute}, createTopic))
	mux.HandleFunc("GET /api/v1/forums/topics/{topicID}", s.forumTopicAPI)
	comment := dashboardJSONCSRF(http.HandlerFunc(s.forumCommentAPI))
	mux.Handle("POST /api/v1/forums/topics/{topicID}/comments", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "forum-comment-create-ip", Limit: 60, Window: 10 * time.Minute}, comment))
	mux.Handle("POST /api/v1/forums/topics/{topicID}/vote", dashboardJSONCSRF(http.HandlerFunc(s.forumVoteAPI)))
	mux.Handle("PATCH /api/v1/admin/forums/topics/{topicID}", dashboardJSONCSRF(http.HandlerFunc(s.adminForumTopicAPI)))
}

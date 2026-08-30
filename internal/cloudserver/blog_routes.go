package cloudserver

import (
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) registerBlogRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/blog/public", s.publicBlogPostsAPI)
	mux.HandleFunc("GET /api/v1/blog/public/{slug}", s.publicBlogPostAPI)

	mux.HandleFunc("GET /api/v1/blog/posts", s.blogPostsResourceAPI)
	createPost := dashboardJSONCSRF(http.HandlerFunc(s.blogPostsResourceAPI))
	mux.Handle("POST /api/v1/blog/posts", webutil.RateLimit(s.Store, webutil.RateLimitOptions{Scope: "blog-create-ip", Limit: 30, Window: 10 * time.Minute}, createPost))
	mux.HandleFunc("GET /api/v1/blog/posts/{postID}", s.blogPostResourceAPI)
	mux.Handle("PATCH /api/v1/blog/posts/{postID}", dashboardJSONCSRF(http.HandlerFunc(s.blogPostResourceAPI)))
	mux.Handle("POST /api/v1/blog/posts/{postID}/publish", dashboardJSONCSRF(http.HandlerFunc(s.blogPublishAPI)))
	mux.Handle("POST /api/v1/blog/posts/{postID}/unpublish", dashboardJSONCSRF(http.HandlerFunc(s.blogUnpublishAPI)))
	mux.Handle("DELETE /api/v1/blog/posts/{postID}", dashboardJSONCSRF(http.HandlerFunc(s.blogDeleteAPI)))
	mux.Handle("POST /api/v1/blog/posts/{postID}/distribution", dashboardJSONCSRF(http.HandlerFunc(s.blogDistributionAPI)))
}

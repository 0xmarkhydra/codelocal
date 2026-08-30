package cloudserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type blogCreateRequest struct {
	Slug         string          `json:"slug"`
	Title        string          `json:"title"`
	Excerpt      string          `json:"excerpt"`
	Content      json.RawMessage `json:"content"`
	CoverAssetID string          `json:"coverAssetId"`
	Category     string          `json:"category"`
	Tags         []string        `json:"tags"`
	Visibility   string          `json:"visibility"`
	SeriesID     string          `json:"seriesId"`
	SeriesPart   int             `json:"seriesPart"`
}

type blogUpdateRequest struct {
	Slug         *string          `json:"slug"`
	Title        *string          `json:"title"`
	Excerpt      *string          `json:"excerpt"`
	Content      *json.RawMessage `json:"content"`
	CoverAssetID *string          `json:"coverAssetId"`
	Category     *string          `json:"category"`
	Tags         *[]string        `json:"tags"`
	Visibility   *string          `json:"visibility"`
	SeriesID     *string          `json:"seriesId"`
	SeriesPart   *int             `json:"seriesPart"`
}

type blogDistributionRequest struct {
	Featured         bool   `json:"featured"`
	ShowOnLanding    bool   `json:"showOnLanding"`
	ModerationStatus string `json:"moderationStatus"`
}

type blogPostSummaryDTO struct {
	ID               string   `json:"id"`
	Slug             string   `json:"slug"`
	AuthorUserID     string   `json:"authorUserId"`
	AuthorEmail      string   `json:"authorEmail,omitempty"`
	Title            string   `json:"title"`
	Excerpt          string   `json:"excerpt"`
	CoverAssetID     string   `json:"coverAssetId,omitempty"`
	Category         string   `json:"category,omitempty"`
	Tags             []string `json:"tags"`
	SeriesID         string   `json:"seriesId,omitempty"`
	SeriesPart       int      `json:"seriesPart,omitempty"`
	Status           string   `json:"status"`
	Visibility       string   `json:"visibility"`
	ModerationStatus string   `json:"moderationStatus"`
	Featured         bool     `json:"featured"`
	ShowOnLanding    bool     `json:"showOnLanding"`
	Official         bool     `json:"official"`
	PublishedAt      int64    `json:"publishedAt,omitempty"`
	ScheduledAt      int64    `json:"scheduledAt,omitempty"`
	CreatedAt        int64    `json:"createdAt"`
	UpdatedAt        int64    `json:"updatedAt"`
}

func blogPostSummary(post cloud.BlogPost) blogPostSummaryDTO {
	return blogPostSummaryDTO{
		ID: post.ID, Slug: post.Slug, AuthorUserID: post.AuthorUserID, AuthorEmail: post.AuthorEmail,
		Title: post.Title, Excerpt: post.Excerpt, CoverAssetID: post.CoverAssetID, Category: post.Category,
		Tags: post.Tags, SeriesID: post.SeriesID, SeriesPart: post.SeriesPart, Status: post.Status,
		Visibility: post.Visibility, ModerationStatus: post.ModerationStatus, Featured: post.Featured,
		ShowOnLanding: post.ShowOnLanding, Official: cloud.IsAdminEmail(post.AuthorEmail),
		PublishedAt: post.PublishedAt, ScheduledAt: post.ScheduledAt, CreatedAt: post.CreatedAt, UpdatedAt: post.UpdatedAt,
	}
}

func blogPostSummaries(posts []cloud.BlogPost) []blogPostSummaryDTO {
	out := make([]blogPostSummaryDTO, 0, len(posts))
	for _, post := range posts {
		out = append(out, blogPostSummary(post))
	}
	return out
}

func (s *Server) blogAPIIdentity(w http.ResponseWriter, r *http.Request, mutation bool) (*webauth.Identity, bool) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return nil, false
	}
	if mutation && !s.WebAuth.VerifyCSRF(r) {
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "invalid_csrf"})
		return nil, false
	}
	return identity, true
}

func writeBlogAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cloud.ErrBlogNotFound):
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error": "blog_post_not_found"})
	case errors.Is(err, cloud.ErrBlogForbidden):
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error": "blog_forbidden"})
	case errors.Is(err, cloud.ErrBlogSlugConflict):
		webutil.JSON(w, http.StatusConflict, map[string]string{"error": "blog_slug_conflict"})
	case errors.Is(err, cloud.ErrBlogInvalid):
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_blog_post"})
	default:
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "blog_unavailable"})
	}
}

func (s *Server) blogPostsResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.blogAPIIdentity(w, r, r.Method != http.MethodGet)
	if !ok {
		return
	}
	admin := cloud.IsAdminEmail(identity.User.Email)
	if r.Method == http.MethodGet {
		var posts []cloud.BlogPost
		var err error
		if admin && strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("scope")), "all") {
			posts, err = s.Store.ListAllBlogPosts(r.Context(), 200)
		} else {
			posts, err = s.Store.ListBlogPostsForUser(r.Context(), identity.User.ID, 200)
		}
		if err != nil {
			writeBlogAPIError(w, err)
			return
		}
		webutil.JSON(w, http.StatusOK, map[string]any{"posts": blogPostSummaries(posts), "isAdmin": admin})
		return
	}
	var input blogCreateRequest
	if webutil.DecodeJSON(r, 600<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	post, err := s.Store.CreateBlogPost(r.Context(), cloud.BlogPostDraft{
		AuthorUserID: identity.User.ID, Slug: input.Slug, Title: input.Title, Excerpt: input.Excerpt,
		Content: input.Content, CoverAssetID: input.CoverAssetID, Category: input.Category, Tags: input.Tags,
		Visibility: input.Visibility, SeriesID: input.SeriesID, SeriesPart: input.SeriesPart,
	})
	if err != nil {
		writeBlogAPIError(w, err)
		return
	}
	webutil.JSON(w, http.StatusCreated, map[string]any{"post": post, "official": admin})
}

func (s *Server) blogPostResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.blogAPIIdentity(w, r, r.Method != http.MethodGet)
	if !ok {
		return
	}
	post, err := s.Store.BlogPostByID(r.Context(), r.PathValue("postID"))
	if err != nil {
		writeBlogAPIError(w, err)
		return
	}
	admin := cloud.IsAdminEmail(identity.User.Email)
	if post.AuthorUserID != identity.User.ID && !admin {
		writeBlogAPIError(w, cloud.ErrBlogForbidden)
		return
	}
	if r.Method == http.MethodGet {
		webutil.JSON(w, http.StatusOK, map[string]any{"post": post, "official": cloud.IsAdminEmail(post.AuthorEmail)})
		return
	}
	var input blogUpdateRequest
	if webutil.DecodeJSON(r, 600<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	update := cloud.BlogPostUpdate{
		Slug: post.Slug, Title: post.Title, Excerpt: post.Excerpt, Content: post.Content,
		CoverAssetID: post.CoverAssetID, Category: post.Category, Tags: post.Tags,
		Visibility: post.Visibility, SeriesID: post.SeriesID, SeriesPart: post.SeriesPart,
	}
	if input.Slug != nil { update.Slug = *input.Slug }
	if input.Title != nil { update.Title = *input.Title }
	if input.Excerpt != nil { update.Excerpt = *input.Excerpt }
	if input.Content != nil { update.Content = append(json.RawMessage(nil), (*input.Content)...)}
	if input.CoverAssetID != nil { update.CoverAssetID = *input.CoverAssetID }
	if input.Category != nil { update.Category = *input.Category }
	if input.Tags != nil { update.Tags = append([]string(nil), (*input.Tags)...)}
	if input.Visibility != nil { update.Visibility = *input.Visibility }
	if input.SeriesID != nil { update.SeriesID = *input.SeriesID }
	if input.SeriesPart != nil { update.SeriesPart = *input.SeriesPart }
	post, err = s.Store.UpdateBlogPost(r.Context(), identity.User.ID, admin, post.ID, update)
	if err != nil {
		writeBlogAPIError(w, err)
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"post": post})
}

func (s *Server) blogPublishAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.blogAPIIdentity(w, r, true)
	if !ok { return }
	post, err := s.Store.SetBlogPostPublished(r.Context(), identity.User.ID, cloud.IsAdminEmail(identity.User.Email), r.PathValue("postID"), true)
	if err != nil { writeBlogAPIError(w, err); return }
	webutil.JSON(w, http.StatusOK, map[string]any{"post": post})
}

func (s *Server) blogUnpublishAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.blogAPIIdentity(w, r, true)
	if !ok { return }
	post, err := s.Store.SetBlogPostPublished(r.Context(), identity.User.ID, cloud.IsAdminEmail(identity.User.Email), r.PathValue("postID"), false)
	if err != nil { writeBlogAPIError(w, err); return }
	webutil.JSON(w, http.StatusOK, map[string]any{"post": post})
}

func (s *Server) blogDeleteAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.blogAPIIdentity(w, r, true)
	if !ok { return }
	if err := s.Store.DeleteBlogPost(r.Context(), identity.User.ID, cloud.IsAdminEmail(identity.User.Email), r.PathValue("postID")); err != nil {
		writeBlogAPIError(w, err)
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (s *Server) blogDistributionAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.blogAPIIdentity(w, r, true)
	if !ok { return }
	if !cloud.IsAdminEmail(identity.User.Email) {
		writeBlogAPIError(w, cloud.ErrBlogForbidden)
		return
	}
	if !s.WebAuth.RequireFreshSecurityContext(w, r, identity, "/dashboard/blogs") {
		return
	}
	var input blogDistributionRequest
	if webutil.DecodeJSON(r, 32<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_request"})
		return
	}
	post, err := s.Store.SetBlogPostDistribution(r.Context(), r.PathValue("postID"), input.Featured, input.ShowOnLanding, input.ModerationStatus)
	if err != nil { writeBlogAPIError(w, err); return }
	webutil.JSON(w, http.StatusOK, map[string]any{"post": post})
}

func (s *Server) publicBlogPostsAPI(w http.ResponseWriter, r *http.Request) {
	posts, err := s.Store.ListPublicBlogPosts(r.Context(), 100)
	if err != nil { writeBlogAPIError(w, err); return }
	webutil.JSON(w, http.StatusOK, map[string]any{"posts": blogPostSummaries(posts)})
}

func (s *Server) publicBlogPostAPI(w http.ResponseWriter, r *http.Request) {
	requestedSlug := cloud.NormalizeBlogSlug(r.PathValue("slug"))
	post, err := s.Store.PublicBlogPostBySlug(r.Context(), requestedSlug)
	if err != nil { writeBlogAPIError(w, err); return }
	webutil.JSON(w, http.StatusOK, map[string]any{
		"post": post,
		"official": cloud.IsAdminEmail(post.AuthorEmail),
		"redirected": requestedSlug != post.Slug,
	})
}

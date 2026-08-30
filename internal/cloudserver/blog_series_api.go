package cloudserver

import (
	"errors"
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type blogSeriesCreateRequest struct {
	Slug         string `json:"slug"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	CoverAssetID string `json:"coverAssetId"`
}

type blogSeriesUpdateRequest struct {
	Slug         *string `json:"slug"`
	Title        *string `json:"title"`
	Description  *string `json:"description"`
	CoverAssetID *string `json:"coverAssetId"`
	Status       *string `json:"status"`
}

type blogPublicSeriesDTO struct {
	ID           string `json:"id"`
	Slug         string `json:"slug"`
	AuthorUserID string `json:"authorUserId"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	CoverAssetID string `json:"coverAssetId,omitempty"`
	Status       string `json:"status"`
	PostCount    int    `json:"postCount"`
	Official     bool   `json:"official"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

func blogPublicSeries(series cloud.BlogSeries) blogPublicSeriesDTO {
	return blogPublicSeriesDTO{
		ID: series.ID, Slug: series.Slug, AuthorUserID: series.AuthorUserID,
		Title: series.Title, Description: series.Description, CoverAssetID: series.CoverAssetID,
		Status: series.Status, PostCount: series.PostCount, Official: cloud.IsAdminEmail(series.AuthorEmail),
		CreatedAt: series.CreatedAt, UpdatedAt: series.UpdatedAt,
	}
}

func blogPublicSeriesList(series []cloud.BlogSeries) []blogPublicSeriesDTO {
	out := make([]blogPublicSeriesDTO, 0, len(series))
	for _, item := range series {
		out = append(out, blogPublicSeries(item))
	}
	return out
}

func writeBlogSeriesAPIError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, cloud.ErrBlogNotFound):
		webutil.JSON(w, http.StatusNotFound, map[string]string{"error":"blog_series_not_found"})
	case errors.Is(err, cloud.ErrBlogForbidden):
		webutil.JSON(w, http.StatusForbidden, map[string]string{"error":"blog_series_forbidden"})
	case errors.Is(err, cloud.ErrBlogSlugConflict):
		webutil.JSON(w, http.StatusConflict, map[string]string{"error":"blog_series_slug_conflict"})
	case errors.Is(err, cloud.ErrBlogInvalid):
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error":"invalid_blog_series"})
	default:
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error":"blog_series_unavailable"})
	}
}

func (s *Server) blogSeriesCollectionAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.blogAPIIdentity(w, r, r.Method != http.MethodGet)
	if !ok { return }
	admin := cloud.IsAdminEmail(identity.User.Email)
	if r.Method == http.MethodGet {
		var series []cloud.BlogSeries
		var err error
		if admin && strings.EqualFold(strings.TrimSpace(r.URL.Query().Get("scope")), "all") {
			series, err = s.Store.ListAllBlogSeries(r.Context(), 200)
		} else {
			series, err = s.Store.ListBlogSeriesForUser(r.Context(), identity.User.ID, 200)
		}
		if err != nil { writeBlogSeriesAPIError(w, err); return }
		webutil.JSON(w, http.StatusOK, map[string]any{"series":series,"isAdmin":admin})
		return
	}
	var input blogSeriesCreateRequest
	if webutil.DecodeJSON(r, 64<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error":"invalid_request"})
		return
	}
	series, err := s.Store.CreateBlogSeries(r.Context(), cloud.BlogSeriesDraft{AuthorUserID:identity.User.ID,Slug:input.Slug,Title:input.Title,Description:input.Description,CoverAssetID:input.CoverAssetID})
	if err != nil { writeBlogSeriesAPIError(w, err); return }
	webutil.JSON(w, http.StatusCreated, map[string]any{"series":series})
}

func (s *Server) blogSeriesResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.blogAPIIdentity(w, r, r.Method != http.MethodGet)
	if !ok { return }
	series, err := s.Store.BlogSeriesByID(r.Context(), r.PathValue("seriesID"))
	if err != nil { writeBlogSeriesAPIError(w, err); return }
	admin := cloud.IsAdminEmail(identity.User.Email)
	if series.AuthorUserID != identity.User.ID && !admin {
		writeBlogSeriesAPIError(w, cloud.ErrBlogForbidden)
		return
	}
	if r.Method == http.MethodGet {
		webutil.JSON(w, http.StatusOK, map[string]any{"series":series})
		return
	}
	var input blogSeriesUpdateRequest
	if webutil.DecodeJSON(r, 64<<10, &input) != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error":"invalid_request"})
		return
	}
	update := cloud.BlogSeriesUpdate{Slug:series.Slug,Title:series.Title,Description:series.Description,CoverAssetID:series.CoverAssetID,Status:series.Status}
	if input.Slug != nil { update.Slug = *input.Slug }
	if input.Title != nil { update.Title = *input.Title }
	if input.Description != nil { update.Description = *input.Description }
	if input.CoverAssetID != nil { update.CoverAssetID = *input.CoverAssetID }
	if input.Status != nil { update.Status = *input.Status }
	series, err = s.Store.UpdateBlogSeries(r.Context(), identity.User.ID, admin, series.ID, update)
	if err != nil { writeBlogSeriesAPIError(w, err); return }
	webutil.JSON(w, http.StatusOK, map[string]any{"series":series})
}

func (s *Server) blogSeriesDeleteAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.blogAPIIdentity(w, r, true)
	if !ok { return }
	if err := s.Store.DeleteBlogSeries(r.Context(), identity.User.ID, cloud.IsAdminEmail(identity.User.Email), r.PathValue("seriesID")); err != nil {
		writeBlogSeriesAPIError(w, err)
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"ok":true})
}

func (s *Server) publicBlogSeriesListAPI(w http.ResponseWriter, r *http.Request) {
	limit := publicBlogPageLimit(r.URL.Query().Get("limit"))
	offset := publicBlogPageOffset(r.URL.Query().Get("offset"))
	series, err := s.Store.ListPublicBlogSeriesPage(r.Context(), limit+1, offset)
	if err != nil { writeBlogSeriesAPIError(w, err); return }
	hasMore := len(series) > limit
	if hasMore {
		series = series[:limit]
	}
	webutil.JSON(w, http.StatusOK, map[string]any{
		"series": blogPublicSeriesList(series),
		"hasMore": hasMore,
		"nextOffset": offset + len(series),
	})
}

func (s *Server) publicBlogSeriesAPI(w http.ResponseWriter, r *http.Request) {
	requested := cloud.NormalizeBlogSlug(r.PathValue("slug"))
	series, err := s.Store.PublicBlogSeriesBySlug(r.Context(), requested)
	if err != nil { writeBlogSeriesAPIError(w, err); return }
	posts, err := s.Store.ListPublicBlogPostsBySeries(r.Context(), series.ID, 200)
	if err != nil { writeBlogSeriesAPIError(w, err); return }
	webutil.JSON(w, http.StatusOK, map[string]any{"series":blogPublicSeries(series),"posts":blogPostSummaries(posts),"redirected":requested!=series.Slug})
}

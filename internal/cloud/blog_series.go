package cloud

import "strings"

type BlogSeries struct {
	ID           string `json:"id"`
	Slug         string `json:"slug"`
	AuthorUserID string `json:"authorUserId"`
	AuthorEmail  string `json:"authorEmail,omitempty"`
	Title        string `json:"title"`
	Description  string `json:"description"`
	CoverAssetID string `json:"coverAssetId,omitempty"`
	Status       string `json:"status"`
	PostCount    int    `json:"postCount"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
	DeletedAt    int64  `json:"deletedAt,omitempty"`
}

type BlogSeriesDraft struct {
	AuthorUserID string
	Slug         string
	Title        string
	Description  string
	CoverAssetID string
}

type BlogSeriesUpdate struct {
	Slug         string
	Title        string
	Description  string
	CoverAssetID string
	Status       string
}

func normalizeBlogSeriesDraft(input BlogSeriesDraft) (BlogSeriesDraft, error) {
	input.AuthorUserID = strings.TrimSpace(input.AuthorUserID)
	input.Title = strings.TrimSpace(input.Title)
	input.Description = strings.TrimSpace(input.Description)
	input.CoverAssetID = strings.TrimSpace(input.CoverAssetID)
	input.Slug = NormalizeBlogSlug(input.Slug)
	if input.Slug == "" {
		input.Slug = NormalizeBlogSlug(input.Title)
	}
	if input.AuthorUserID == "" || input.Title == "" || input.Slug == "" || len(input.Title) > 200 || len(input.Slug) > 180 || len(input.Description) > 2000 || len(input.CoverAssetID) > 80 {
		return BlogSeriesDraft{}, ErrBlogInvalid
	}
	if input.CoverAssetID != "" && NormalizeMediaAssetID(input.CoverAssetID) == "" {
		return BlogSeriesDraft{}, ErrBlogInvalid
	}
	return input, nil
}

func normalizeBlogSeriesUpdate(input BlogSeriesUpdate) (BlogSeriesUpdate, error) {
	draft, err := normalizeBlogSeriesDraft(BlogSeriesDraft{AuthorUserID: "owner", Slug: input.Slug, Title: input.Title, Description: input.Description, CoverAssetID: input.CoverAssetID})
	if err != nil {
		return BlogSeriesUpdate{}, err
	}
	status := strings.TrimSpace(input.Status)
	switch status {
	case "active", "complete", "archived":
	default:
		status = "active"
	}
	return BlogSeriesUpdate{Slug: draft.Slug, Title: draft.Title, Description: draft.Description, CoverAssetID: draft.CoverAssetID, Status: status}, nil
}

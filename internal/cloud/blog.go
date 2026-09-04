package cloud

import (
	"encoding/json"
	"errors"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"
)

var (
	ErrBlogNotFound     = errors.New("blog post not found")
	ErrBlogForbidden    = errors.New("blog post forbidden")
	ErrBlogSlugConflict = errors.New("blog slug already exists")
	ErrBlogInvalid      = errors.New("invalid blog post")
)

var reservedOfficialBlogPostSlugs = map[string]struct{}{
	"why-local-execution-matters-for-ai-coding-agents":       {},
	"connect-ai-clients-without-giving-up-workspace-control": {},
	"project-brain-durable-context-for-coding-agents":        {},
	"a-practical-security-model-for-local-coding-agents":     {},
}

var reservedOfficialBlogSeriesSlugs = map[string]struct{}{
	"local-agent-foundations": {},
}

type BlogPost struct {
	ID               string          `json:"id"`
	Slug             string          `json:"slug"`
	AuthorUserID     string          `json:"authorUserId"`
	AuthorEmail      string          `json:"authorEmail,omitempty"`
	Title            string          `json:"title"`
	Excerpt          string          `json:"excerpt"`
	Content          json.RawMessage `json:"content"`
	CoverAssetID     string          `json:"coverAssetId,omitempty"`
	Category         string          `json:"category,omitempty"`
	Tags             []string        `json:"tags"`
	SeriesID         string          `json:"seriesId,omitempty"`
	SeriesPart       int             `json:"seriesPart,omitempty"`
	Status           string          `json:"status"`
	Visibility       string          `json:"visibility"`
	ModerationStatus string          `json:"moderationStatus"`
	Featured         bool            `json:"featured"`
	ShowOnLanding    bool            `json:"showOnLanding"`
	PublishedAt      int64           `json:"publishedAt,omitempty"`
	ScheduledAt      int64           `json:"scheduledAt,omitempty"`
	CreatedAt        int64           `json:"createdAt"`
	UpdatedAt        int64           `json:"updatedAt"`
	DeletedAt        int64           `json:"deletedAt,omitempty"`
}

type BlogPostDraft struct {
	AuthorUserID string
	Slug         string
	Title        string
	Excerpt      string
	Content      json.RawMessage
	CoverAssetID string
	Category     string
	Tags         []string
	Visibility   string
	SeriesID     string
	SeriesPart   int
}

type BlogPostUpdate struct {
	Slug         string
	Title        string
	Excerpt      string
	Content      json.RawMessage
	CoverAssetID string
	Category     string
	Tags         []string
	Visibility   string
	SeriesID     string
	SeriesPart   int
}

func NormalizeBlogSlug(value string) string {
	value = strings.TrimSpace(value)
	value = strings.NewReplacer("đ", "d", "Đ", "D").Replace(value)
	value = norm.NFD.String(strings.ToLower(value))

	var out strings.Builder
	dash := false
	for _, r := range value {
		if unicode.Is(unicode.Mn, r) {
			continue
		}
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			out.WriteRune(r)
			dash = false
			continue
		}
		if out.Len() > 0 && !dash {
			out.WriteByte('-')
			dash = true
		}
	}
	return strings.Trim(out.String(), "-")
}

func isReservedOfficialBlogPostSlug(value string) bool {
	_, reserved := reservedOfficialBlogPostSlugs[NormalizeBlogSlug(value)]
	return reserved
}

func isReservedOfficialBlogSeriesSlug(value string) bool {
	_, reserved := reservedOfficialBlogSeriesSlugs[NormalizeBlogSlug(value)]
	return reserved
}

func normalizeBlogTags(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" || len(value) > 60 {
			continue
		}
		key := strings.ToLower(value)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, value)
		if len(out) == 12 {
			break
		}
	}
	return out
}

func normalizeBlogVisibility(value string) string {
	switch strings.TrimSpace(value) {
	case "private", "unlisted", "public":
		return strings.TrimSpace(value)
	default:
		return "public"
	}
}

func normalizeBlogContent(value json.RawMessage) (json.RawMessage, error) {
	if len(value) == 0 {
		return json.RawMessage("[]"), nil
	}
	if len(value) > 512<<10 || !json.Valid(value) {
		return nil, ErrBlogInvalid
	}
	var blocks []json.RawMessage
	if err := json.Unmarshal(value, &blocks); err != nil {
		return nil, ErrBlogInvalid
	}
	for _, block := range blocks {
		var envelope struct {
			Type string `json:"type"`
		}
		if err := json.Unmarshal(block, &envelope); err != nil {
			return nil, ErrBlogInvalid
		}
		switch strings.TrimSpace(envelope.Type) {
		case "paragraph", "heading", "list", "code", "callout", "image":
		default:
			return nil, ErrBlogInvalid
		}
	}
	return append(json.RawMessage(nil), value...), nil
}

func normalizeBlogDraft(input BlogPostDraft) (BlogPostDraft, error) {
	input.AuthorUserID = strings.TrimSpace(input.AuthorUserID)
	input.Title = strings.TrimSpace(input.Title)
	input.Excerpt = strings.TrimSpace(input.Excerpt)
	input.Category = strings.TrimSpace(input.Category)
	input.CoverAssetID = strings.TrimSpace(input.CoverAssetID)
	input.SeriesID = strings.TrimSpace(input.SeriesID)
	if input.SeriesID == "" {
		input.SeriesPart = 0
	}
	input.Slug = NormalizeBlogSlug(input.Slug)
	if input.Slug == "" {
		input.Slug = NormalizeBlogSlug(input.Title)
	}
	if isReservedOfficialBlogPostSlug(input.Slug) {
		return BlogPostDraft{}, ErrBlogSlugConflict
	}
	content, err := normalizeBlogContent(input.Content)
	if err != nil {
		return BlogPostDraft{}, err
	}
	input.Content = content
	input.Tags = normalizeBlogTags(input.Tags)
	input.Visibility = normalizeBlogVisibility(input.Visibility)
	if input.AuthorUserID == "" || input.Title == "" || input.Slug == "" || len(input.Title) > 200 || len(input.Slug) > 180 || len(input.Excerpt) > 700 || len(input.Category) > 80 || len(input.CoverAssetID) > 220 {
		return BlogPostDraft{}, ErrBlogInvalid
	}
	if input.SeriesID != "" && input.SeriesPart <= 0 {
		return BlogPostDraft{}, ErrBlogInvalid
	}
	return input, nil
}

func normalizeBlogUpdate(input BlogPostUpdate) (BlogPostUpdate, error) {
	draft, err := normalizeBlogDraft(BlogPostDraft{
		AuthorUserID: "owner", Slug: input.Slug, Title: input.Title, Excerpt: input.Excerpt,
		Content: input.Content, CoverAssetID: input.CoverAssetID, Category: input.Category,
		Tags: input.Tags, Visibility: input.Visibility, SeriesID: input.SeriesID, SeriesPart: input.SeriesPart,
	})
	if err != nil {
		return BlogPostUpdate{}, err
	}
	return BlogPostUpdate{
		Slug: draft.Slug, Title: draft.Title, Excerpt: draft.Excerpt, Content: draft.Content,
		CoverAssetID: draft.CoverAssetID, Category: draft.Category, Tags: draft.Tags,
		Visibility: draft.Visibility, SeriesID: draft.SeriesID, SeriesPart: draft.SeriesPart,
	}, nil
}

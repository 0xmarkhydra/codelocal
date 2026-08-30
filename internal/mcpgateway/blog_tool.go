package mcpgateway

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

type BlogFileRef struct {
	FileID      string `json:"file_id"`
	DownloadURL string `json:"download_url"`
	MIMEType    string `json:"mime_type,omitempty"`
	FileName    string `json:"file_name,omitempty"`
}

type BlogMediaImporter interface {
	ImportBlogImage(context.Context, string, BlogFileRef) (cloud.MediaAsset, error)
}

func (s *Service) SetBlogMediaImporter(importer BlogMediaImporter) { s.BlogMedia = importer }

var blogToolActions = []string{
	"list", "get", "create", "update", "publish", "unpublish", "archive", "media_import",
	"series_list", "series_get", "series_create", "series_update", "series_archive",
	"set_distribution",
}

func compactBlogToolDefinitions() []compactToolDef {
	stringItems := map[string]any{"type": "string"}
	contentBlock := map[string]any{"type": "object", "additionalProperties": true}
	fileParam := map[string]any{
		"type": "object",
		"properties": map[string]any{
			"file_id":      str("ChatGPT host-issued file ID."),
			"download_url": str("ChatGPT host-issued temporary download URL."),
			"mime_type":    str("Optional MIME type hint."),
			"file_name":    str("Optional original file name."),
		},
		"required":             []string{"file_id", "download_url"},
		"additionalProperties": false,
	}
	properties := map[string]any{
		"action":           map[string]any{"type": "string", "enum": blogToolActions, "description": "Blog operation to perform."},
		"scope":            map[string]any{"type": "string", "enum": []string{"own", "all"}, "description": "For list actions. all is admin-only."},
		"limit":            integer("Maximum records to return.", 1, 200),
		"file":             fileParam,
		"postId":           str("Blog post ID returned by list/create."),
		"seriesId":         str("Blog series ID returned by series_list/series_create."),
		"slug":             str("Canonical URL slug. Omit on create to derive it from title."),
		"title":            str("Post or series title."),
		"excerpt":          str("Short article summary."),
		"content":          map[string]any{"type": "array", "items": contentBlock, "description": "Structured Blog blocks. Image blocks reference durable media by assetId."},
		"coverAssetId":     str("Existing ready durable MediaAsset ID. Blog never uploads raw S3 data itself."),
		"category":         str("Post category."),
		"tags":             array(stringItems, "Post tags."),
		"visibility":       map[string]any{"type": "string", "enum": []string{"public", "unlisted", "private"}},
		"seriesPart":       integer("Ordered part number when assigning a post to a series.", 1, 999),
		"description":      str("Series description."),
		"status":           map[string]any{"type": "string", "enum": []string{"active", "complete", "archived"}, "description": "Series status for series_update."},
		"featured":         boolean("Admin editorial featured flag."),
		"showOnLanding":    boolean("Admin editorial landing-page distribution flag."),
		"moderationStatus": map[string]any{"type": "string", "enum": []string{"clean", "pending", "hidden"}},
	}
	return []compactToolDef{{
		Name:        "blog",
		Title:       "Manage CodeLocal Blog",
		Description: "Create, edit, publish, archive and curate CodeLocal Blog posts and series using the same durable Blog/Media domain as Dashboard. action=media_import can ingest a ChatGPT conversation/generated image into the existing MediaAsset pipeline, returning an assetId for cover or inline blocks. Normal users can manage only their own content. Admin-only scope=all and set_distribution never bypass account permissions.",
		Schema:      objectSchema(properties, "action"),
		Meta:        mcp.Meta{"openai/fileParams": []string{"file"}},
		Annotations: compactAnnotations("Manage CodeLocal Blog", false, true, false),
		Execute:     executeBlogTool,
	}}
}

func blogString(args map[string]any, key string) string {
	value, _ := args[key].(string)
	return strings.TrimSpace(value)
}

func blogInt(args map[string]any, key string, fallback int) int {
	switch value := args[key].(type) {
	case float64:
		return int(value)
	case int:
		return value
	default:
		return fallback
	}
}

func blogBool(args map[string]any, key string) (bool, bool) {
	value, ok := args[key].(bool)
	return value, ok
}

func blogStrings(args map[string]any, key string) []string {
	values, ok := args[key].([]any)
	if !ok {
		if typed, typedOK := args[key].([]string); typedOK {
			return append([]string(nil), typed...)
		}
		return nil
	}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if item, ok := value.(string); ok {
			out = append(out, item)
		}
	}
	return out
}

func blogFileRef(args map[string]any) (BlogFileRef, error) {
	value, ok := args["file"].(map[string]any)
	if !ok {
		return BlogFileRef{}, errors.New("media_import requires ChatGPT file")
	}
	stringValue := func(key string) string {
		raw, _ := value[key].(string)
		return strings.TrimSpace(raw)
	}
	ref := BlogFileRef{
		FileID:      stringValue("file_id"),
		DownloadURL: stringValue("download_url"),
		MIMEType:    stringValue("mime_type"),
		FileName:    stringValue("file_name"),
	}
	if ref.FileID == "" || ref.DownloadURL == "" {
		return BlogFileRef{}, errors.New("ChatGPT file is missing file_id or download_url")
	}
	return ref, nil
}

func blogRawJSON(args map[string]any, key string) (json.RawMessage, bool, error) {
	value, ok := args[key]
	if !ok {
		return nil, false, nil
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, true, err
	}
	return raw, true, nil
}

func blogActorAdmin(ctx context.Context, service *Service, userID string) (bool, error) {
	if service == nil || service.Store == nil || service.Store.DB == nil {
		return false, errors.New("blog store unavailable")
	}
	var email string
	if err := service.Store.DB.QueryRow(ctx, `SELECT email FROM codelocal_users WHERE id=$1`, strings.TrimSpace(userID)).Scan(&email); err != nil {
		return false, err
	}
	return cloud.IsAdminEmail(email), nil
}

func blogCanonicalURL(slug string) string {
	return "/blogs/" + cloud.NormalizeBlogSlug(slug)
}

func blogSeriesCanonicalURL(slug string) string {
	return "/blogs/series/" + cloud.NormalizeBlogSlug(slug)
}

func blogToolError(err error) *mcp.CallToolResult {
	return errorResult(err)
}

func executeBlogTool(ctx context.Context, service *Service, userID string, args map[string]any, _ *mcp.CallToolRequest) (*mcp.CallToolResult, error) {
	action := blogString(args, "action")
	if action == "" {
		return blogToolError(errors.New("blog action is required")), nil
	}
	admin, err := blogActorAdmin(ctx, service, userID)
	if err != nil {
		return blogToolError(err), nil
	}
	limit := blogInt(args, "limit", 50)
	if limit < 1 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}

	switch action {
	case "media_import":
		if service.BlogMedia == nil {
			return blogToolError(errors.New("blog media importer unavailable")), nil
		}
		ref, refErr := blogFileRef(args)
		if refErr != nil {
			return blogToolError(refErr), nil
		}
		asset, importErr := service.BlogMedia.ImportBlogImage(ctx, userID, ref)
		if importErr != nil {
			return blogToolError(importErr), nil
		}
		return textResult(map[string]any{"assetId": asset.ID, "asset": asset}, false), nil

	case "list":
		var posts []cloud.BlogPost
		if blogString(args, "scope") == "all" {
			if !admin {
				return blogToolError(cloud.ErrBlogForbidden), nil
			}
			posts, err = service.Store.ListAllBlogPosts(ctx, limit)
		} else {
			posts, err = service.Store.ListBlogPostsForUser(ctx, userID, limit)
		}
		if err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"posts": posts, "count": len(posts)}, false), nil

	case "get":
		postID := blogString(args, "postId")
		if postID == "" {
			return blogToolError(errors.New("get requires postId")), nil
		}
		post, err := service.Store.BlogPostByID(ctx, postID)
		if err != nil {
			return blogToolError(err), nil
		}
		if post.AuthorUserID != userID && !admin {
			return blogToolError(cloud.ErrBlogForbidden), nil
		}
		return textResult(map[string]any{"post": post, "url": blogCanonicalURL(post.Slug)}, false), nil

	case "create":
		title := blogString(args, "title")
		if title == "" {
			return blogToolError(errors.New("create requires title")), nil
		}
		content, exists, marshalErr := blogRawJSON(args, "content")
		if marshalErr != nil {
			return blogToolError(marshalErr), nil
		}
		if !exists {
			content = json.RawMessage("[]")
		}
		post, err := service.Store.CreateBlogPost(ctx, cloud.BlogPostDraft{
			AuthorUserID: userID,
			Slug:         blogString(args, "slug"),
			Title:        title,
			Excerpt:      blogString(args, "excerpt"),
			Content:      content,
			CoverAssetID: blogString(args, "coverAssetId"),
			Category:     blogString(args, "category"),
			Tags:         blogStrings(args, "tags"),
			Visibility:   blogString(args, "visibility"),
			SeriesID:     blogString(args, "seriesId"),
			SeriesPart:   blogInt(args, "seriesPart", 0),
		})
		if err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"post": post, "url": blogCanonicalURL(post.Slug)}, false), nil

	case "update":
		postID := blogString(args, "postId")
		if postID == "" {
			return blogToolError(errors.New("update requires postId")), nil
		}
		current, err := service.Store.BlogPostByID(ctx, postID)
		if err != nil {
			return blogToolError(err), nil
		}
		update := cloud.BlogPostUpdate{
			Slug: current.Slug, Title: current.Title, Excerpt: current.Excerpt, Content: current.Content,
			CoverAssetID: current.CoverAssetID, Category: current.Category, Tags: current.Tags,
			Visibility: current.Visibility, SeriesID: current.SeriesID, SeriesPart: current.SeriesPart,
		}
		if _, ok := args["slug"]; ok {
			update.Slug = blogString(args, "slug")
		}
		if _, ok := args["title"]; ok {
			update.Title = blogString(args, "title")
		}
		if _, ok := args["excerpt"]; ok {
			update.Excerpt = blogString(args, "excerpt")
		}
		if content, ok, rawErr := blogRawJSON(args, "content"); rawErr != nil {
			return blogToolError(rawErr), nil
		} else if ok {
			update.Content = content
		}
		if _, ok := args["coverAssetId"]; ok {
			update.CoverAssetID = blogString(args, "coverAssetId")
		}
		if _, ok := args["category"]; ok {
			update.Category = blogString(args, "category")
		}
		if _, ok := args["tags"]; ok {
			update.Tags = blogStrings(args, "tags")
		}
		if _, ok := args["visibility"]; ok {
			update.Visibility = blogString(args, "visibility")
		}
		if _, ok := args["seriesId"]; ok {
			update.SeriesID = blogString(args, "seriesId")
		}
		if _, ok := args["seriesPart"]; ok {
			update.SeriesPart = blogInt(args, "seriesPart", 0)
		}
		post, err := service.Store.UpdateBlogPost(ctx, userID, admin, postID, update)
		if err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"post": post, "url": blogCanonicalURL(post.Slug)}, false), nil

	case "publish", "unpublish":
		postID := blogString(args, "postId")
		if postID == "" {
			return blogToolError(fmt.Errorf("%s requires postId", action)), nil
		}
		post, err := service.Store.SetBlogPostPublished(ctx, userID, admin, postID, action == "publish")
		if err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"post": post, "url": blogCanonicalURL(post.Slug)}, false), nil

	case "archive":
		postID := blogString(args, "postId")
		if postID == "" {
			return blogToolError(errors.New("archive requires postId")), nil
		}
		if err := service.Store.DeleteBlogPost(ctx, userID, admin, postID); err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"ok": true, "postId": postID, "status": "archived"}, false), nil

	case "series_list":
		var series []cloud.BlogSeries
		if blogString(args, "scope") == "all" {
			if !admin {
				return blogToolError(cloud.ErrBlogForbidden), nil
			}
			series, err = service.Store.ListAllBlogSeries(ctx, limit)
		} else {
			series, err = service.Store.ListBlogSeriesForUser(ctx, userID, limit)
		}
		if err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"series": series, "count": len(series)}, false), nil

	case "series_get":
		seriesID := blogString(args, "seriesId")
		if seriesID == "" {
			return blogToolError(errors.New("series_get requires seriesId")), nil
		}
		series, err := service.Store.BlogSeriesByID(ctx, seriesID)
		if err != nil {
			return blogToolError(err), nil
		}
		if series.AuthorUserID != userID && !admin {
			return blogToolError(cloud.ErrBlogForbidden), nil
		}
		return textResult(map[string]any{"series": series, "url": blogSeriesCanonicalURL(series.Slug)}, false), nil

	case "series_create":
		title := blogString(args, "title")
		if title == "" {
			return blogToolError(errors.New("series_create requires title")), nil
		}
		series, err := service.Store.CreateBlogSeries(ctx, cloud.BlogSeriesDraft{
			AuthorUserID: userID,
			Slug:         blogString(args, "slug"),
			Title:        title,
			Description:  blogString(args, "description"),
			CoverAssetID: blogString(args, "coverAssetId"),
		})
		if err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"series": series, "url": blogSeriesCanonicalURL(series.Slug)}, false), nil

	case "series_update":
		seriesID := blogString(args, "seriesId")
		if seriesID == "" {
			return blogToolError(errors.New("series_update requires seriesId")), nil
		}
		current, err := service.Store.BlogSeriesByID(ctx, seriesID)
		if err != nil {
			return blogToolError(err), nil
		}
		update := cloud.BlogSeriesUpdate{Slug: current.Slug, Title: current.Title, Description: current.Description, CoverAssetID: current.CoverAssetID, Status: current.Status}
		if _, ok := args["slug"]; ok {
			update.Slug = blogString(args, "slug")
		}
		if _, ok := args["title"]; ok {
			update.Title = blogString(args, "title")
		}
		if _, ok := args["description"]; ok {
			update.Description = blogString(args, "description")
		}
		if _, ok := args["coverAssetId"]; ok {
			update.CoverAssetID = blogString(args, "coverAssetId")
		}
		if _, ok := args["status"]; ok {
			update.Status = blogString(args, "status")
		}
		series, err := service.Store.UpdateBlogSeries(ctx, userID, admin, seriesID, update)
		if err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"series": series, "url": blogSeriesCanonicalURL(series.Slug)}, false), nil

	case "series_archive":
		seriesID := blogString(args, "seriesId")
		if seriesID == "" {
			return blogToolError(errors.New("series_archive requires seriesId")), nil
		}
		if err := service.Store.DeleteBlogSeries(ctx, userID, admin, seriesID); err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"ok": true, "seriesId": seriesID, "status": "archived"}, false), nil

	case "set_distribution":
		if !admin {
			return blogToolError(cloud.ErrBlogForbidden), nil
		}
		postID := blogString(args, "postId")
		featured, featuredOK := blogBool(args, "featured")
		showOnLanding, landingOK := blogBool(args, "showOnLanding")
		moderationStatus := blogString(args, "moderationStatus")
		if postID == "" || !featuredOK || !landingOK || moderationStatus == "" {
			return blogToolError(errors.New("set_distribution requires postId, featured, showOnLanding, and moderationStatus")), nil
		}
		post, err := service.Store.SetBlogPostDistribution(ctx, postID, featured, showOnLanding, moderationStatus)
		if err != nil {
			return blogToolError(err), nil
		}
		return textResult(map[string]any{"post": post, "url": blogCanonicalURL(post.Slug)}, false), nil
	default:
		return textResult(map[string]any{
			"error":   fmt.Sprintf("unsupported blog action %q", action),
			"code":    "CODELOCAL_TOOL_SCHEMA_MISMATCH",
			"tool":    "blog",
			"actions": blogToolActions,
		}, true), nil
	}
}

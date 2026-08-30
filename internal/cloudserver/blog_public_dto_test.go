package cloudserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestPublicBlogDTOsDoNotExposeAuthorEmail(t *testing.T) {
	const secretEmail = "creator-private@example.com"
	post := cloud.BlogPost{
		ID: "post_1", Slug: "safe-post", AuthorUserID: "user_1", AuthorEmail: secretEmail,
		Title: "Safe post", Excerpt: "Public excerpt", Content: json.RawMessage(`[{"type":"paragraph","text":"hello"}]`),
		Tags: []string{"AI"}, Status: "published", Visibility: "public", ModerationStatus: "clean",
		CreatedAt: 1, UpdatedAt: 2, PublishedAt: 2,
	}
	series := cloud.BlogSeries{
		ID: "series_1", Slug: "safe-series", AuthorUserID: "user_1", AuthorEmail: secretEmail,
		Title: "Safe series", Description: "Public series", Status: "active", PostCount: 1, CreatedAt: 1, UpdatedAt: 2,
	}

	payloads := []any{
		blogPostSummary(post),
		blogPublicPost(post),
		blogPublicSeries(series),
	}
	for _, payload := range payloads {
		encoded, err := json.Marshal(payload)
		if err != nil {
			t.Fatal(err)
		}
		text := string(encoded)
		if strings.Contains(text, secretEmail) || strings.Contains(text, "authorEmail") {
			t.Fatalf("public blog DTO leaked account email: %s", text)
		}
	}
}

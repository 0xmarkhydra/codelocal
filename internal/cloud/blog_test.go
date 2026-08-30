package cloud

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNormalizeBlogSlugKeepsStableUnicodeWords(t *testing.T) {
	if got, want := NormalizeBlogSlug("  AI Agent: Từ Project → Blog!  "), "ai-agent-từ-project-blog"; got != want {
		t.Fatalf("NormalizeBlogSlug()=%q want %q", got, want)
	}
}

func TestNormalizeBlogDraftAppliesSafeDefaults(t *testing.T) {
	input, err := normalizeBlogDraft(BlogPostDraft{
		AuthorUserID: "user-1",
		Title:        "Build with CodeLocal",
		Tags:         []string{"AI", "ai", " MCP ", ""},
	})
	if err != nil {
		t.Fatalf("normalizeBlogDraft() error=%v", err)
	}
	if input.Slug != "build-with-codelocal" {
		t.Fatalf("slug=%q", input.Slug)
	}
	if input.Visibility != "public" {
		t.Fatalf("visibility=%q", input.Visibility)
	}
	if string(input.Content) != "[]" {
		t.Fatalf("content=%q", input.Content)
	}
	if len(input.Tags) != 2 || input.Tags[0] != "AI" || input.Tags[1] != "MCP" {
		t.Fatalf("tags=%#v", input.Tags)
	}
}

func TestNormalizeBlogDraftRejectsInvalidContentAndSeriesPair(t *testing.T) {
	_, err := normalizeBlogDraft(BlogPostDraft{AuthorUserID: "user-1", Title: "Post", Content: json.RawMessage("{")})
	if !errors.Is(err, ErrBlogInvalid) {
		t.Fatalf("invalid content error=%v", err)
	}
	_, err = normalizeBlogDraft(BlogPostDraft{AuthorUserID: "user-1", Title: "Post", SeriesID: "series-1"})
	if !errors.Is(err, ErrBlogInvalid) {
		t.Fatalf("invalid series pair error=%v", err)
	}
}

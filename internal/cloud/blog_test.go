package cloud

import (
	"encoding/json"
	"errors"
	"testing"
)

func TestNormalizeBlogSlugTransliteratesVietnameseWords(t *testing.T) {
	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "vietnamese title",
			input: "  OpenClaw và DeepSeek Harness: Hướng tiếp cận khác nhau để xây AI Agent?  ",
			want:  "openclaw-va-deepseek-harness-huong-tiep-can-khac-nhau-de-xay-ai-agent",
		},
		{
			name:  "vietnamese d stroke",
			input: "Điện toán đám mây & AI",
			want:  "dien-toan-dam-may-ai",
		},
		{
			name:  "special characters",
			input: "AI Agent: Từ Project → Blog!",
			want:  "ai-agent-tu-project-blog",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := NormalizeBlogSlug(tt.input); got != tt.want {
				t.Fatalf("NormalizeBlogSlug()=%q want %q", got, tt.want)
			}
		})
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

func TestOfficialSeedSlugsAreReservedFromDurableWrites(t *testing.T) {
	for _, slug := range []string{
		"why-local-execution-matters-for-ai-coding-agents",
		"connect-ai-clients-without-giving-up-workspace-control",
		"project-brain-durable-context-for-coding-agents",
		"a-practical-security-model-for-local-coding-agents",
	} {
		_, err := normalizeBlogDraft(BlogPostDraft{AuthorUserID: "user-1", Slug: slug, Title: "Community post"})
		if !errors.Is(err, ErrBlogSlugConflict) {
			t.Fatalf("reserved post slug %q error=%v", slug, err)
		}
	}

	_, err := normalizeBlogSeriesDraft(BlogSeriesDraft{AuthorUserID: "user-1", Slug: "local-agent-foundations", Title: "Community series"})
	if !errors.Is(err, ErrBlogSlugConflict) {
		t.Fatalf("reserved series slug error=%v", err)
	}
}

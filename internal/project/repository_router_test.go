package project

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/repository"
)

func TestBuildRepositoryRouteKeepsAllRepositoriesWhenEvidenceIsWeak(t *testing.T) {
	ranked := []repositoryRouteScore{
		{Repository: repository.Checkout{ID: "web", RelativePath: "web"}, Score: 3},
		{Repository: repository.Checkout{ID: "auth", RelativePath: "backend/auth"}, Score: 1},
	}
	route := buildRepositoryRoute(ranked)
	if route.Mode != "all" {
		t.Fatalf("weak evidence must not over-focus repository context: %#v", route)
	}
	if !route.selected(ranked[0].Repository) || !route.selected(ranked[1].Repository) {
		t.Fatalf("all-mode must keep every repository available: %#v", route)
	}
}

func TestExplicitRepositoryMentionUsesWholeTokens(t *testing.T) {
	repo := repository.Checkout{ID: "auth", RelativePath: "backend/auth"}
	if explicitRepositoryMention("fix authenticate flow", repo) {
		t.Fatal("substring in authenticate must not count as explicit auth repository mention")
	}
	if !explicitRepositoryMention("fix auth flow", repo) {
		t.Fatal("whole auth token should count as an explicit repository mention")
	}
	if !explicitRepositoryMention("fix backend/auth flow", repo) {
		t.Fatal("full repository path should count as an explicit repository mention")
	}
}

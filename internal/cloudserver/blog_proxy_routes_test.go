package cloudserver

import "testing"

func TestBlogAndAuthorPresentationFamiliesAreBoundarySafe(t *testing.T) {
	for _, path := range []string{
		"/blogs", "/blogs/post", "/blogs/series/example", "/users/user_123",
	} {
		if !isNextPublicPagePath(path) {
			t.Fatalf("expected %q to use Next presentation", path)
		}
	}
	for _, path := range []string{
		"/blogger", "/blogs-private", "/users-private", "/user/user_123", "/api/v1/blog/public",
	} {
		if isNextPublicPagePath(path) {
			t.Fatalf("presentation family matcher widened to %q", path)
		}
	}
}

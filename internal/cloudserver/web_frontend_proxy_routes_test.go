package cloudserver

import "testing"

func TestNextDashboardRoutingIncludesSkillsForumsAndSingleTrailingSlash(t *testing.T) {
	for _, path := range []string{
		"/dashboard/skills",
		"/dashboard/skills/",
		"/dashboard/forums",
		"/dashboard/forums/",
		"/dashboard/forums/topic-123",
		"/dashboard/forums/topic-123/",
		"/dashboard/admin",
		"/dashboard/admin/",
	} {
		if !isNextDashboardPath(path) {
			t.Fatalf("expected %q to use Next dashboard presentation", path)
		}
	}
}

func TestNextDashboardRoutingDoesNotWidenToNestedOrDoubleSlashPaths(t *testing.T) {
	for _, path := range []string{
		"/dashboard/admin/users",
		"/dashboard/admin//",
		"/dashboard/skills/private",
		"/dashboard/forums//",
	} {
		if isNextDashboardPath(path) {
			t.Fatalf("expected %q to remain outside the Next dashboard allowlist", path)
		}
	}
}

func TestAdminAuthorizationUsesCanonicalPresentationPath(t *testing.T) {
	if got := canonicalNextPresentationPath("/dashboard/admin/"); got != "/dashboard/admin" {
		t.Fatalf("expected trailing-slash admin path to canonicalize for authorization, got %q", got)
	}
	if got := canonicalNextPresentationPath("/dashboard/admin//"); got != "/dashboard/admin//" {
		t.Fatalf("double-slash path must not be normalized into an authorized admin route, got %q", got)
	}
}

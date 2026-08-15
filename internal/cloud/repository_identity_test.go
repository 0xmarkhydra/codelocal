package cloud

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
)

func repositoryIdentityFixture(id, remote, lineage, relative string) projectidentity.Repository {
	return projectidentity.Repository{ID: id, Remote: remote, Lineage: lineage, RelativePath: relative, IdentitySource: "remote"}
}

func TestRemoteAliasPreservesCanonicalRepositoryIdentity(t *testing.T) {
	repositories := []projectidentity.Repository{
		repositoryIdentityFixture("observed-new", "github.com/acme/api-renamed", "abc123", "."),
	}
	resolution := applyRemoteAliasResolution(repositories, map[string]string{"github.com/acme/api-renamed": "repo-canonical"})
	if len(resolution.Repositories) != 1 || resolution.Repositories[0].ID != "repo-canonical" {
		t.Fatalf("remote alias did not preserve canonical repository id: %#v", resolution)
	}
	if resolution.ObservedToCanonical["observed-new"] != "repo-canonical" || resolution.RemoteAliasMatched != 1 || !resolution.RemoteMatched["observed-new"] {
		t.Fatalf("remote alias evidence/mapping lost: %#v", resolution)
	}
}

func TestProjectLineageFallbackOnlyResolvesUnmatchedUniqueLineage(t *testing.T) {
	repositories := []projectidentity.Repository{
		repositoryIdentityFixture("observed-new", "github.com/acme/new-api", "abc123", "."),
	}
	resolution := applyRemoteAliasResolution(repositories, nil)
	resolution = applyProjectLineageResolution(resolution, map[string][]string{"abc123": {"repo-canonical"}})
	if resolution.Repositories[0].ID != "repo-canonical" || resolution.LineageAliasMatched != 1 {
		t.Fatalf("project-local lineage did not rescue renamed remote: %#v", resolution)
	}
	if resolution.ObservedToCanonical["observed-new"] != "repo-canonical" {
		t.Fatalf("observed repository mapping not updated: %#v", resolution.ObservedToCanonical)
	}
}

func TestDirectRemoteAliasCannotBeOverriddenByLineage(t *testing.T) {
	repositories := []projectidentity.Repository{
		repositoryIdentityFixture("observed", "github.com/acme/api", "abc123", "."),
	}
	resolution := applyRemoteAliasResolution(repositories, map[string]string{"github.com/acme/api": "repo-from-remote"})
	resolution = applyProjectLineageResolution(resolution, map[string][]string{"abc123": {"repo-from-lineage"}})
	if resolution.Repositories[0].ID != "repo-from-remote" || resolution.LineageAliasMatched != 0 {
		t.Fatalf("weaker lineage evidence overrode direct remote alias: %#v", resolution)
	}
}

func TestDuplicateIncomingLineageFailsClosedForForkLikeSnapshot(t *testing.T) {
	repositories := []projectidentity.Repository{
		repositoryIdentityFixture("fork-a", "github.com/acme/fork-a", "same-root", "a"),
		repositoryIdentityFixture("fork-b", "github.com/acme/fork-b", "same-root", "b"),
	}
	resolution := applyRemoteAliasResolution(repositories, nil)
	resolution = applyProjectLineageResolution(resolution, map[string][]string{"same-root": {"repo-old"}})
	if resolution.LineageAliasMatched != 0 || resolution.Repositories[0].ID != "fork-a" || resolution.Repositories[1].ID != "fork-b" {
		t.Fatalf("duplicate lineage silently merged fork-like repositories: %#v", resolution)
	}
}

func TestCanonicalizeProjectRepositoriesAppliesMappingAndDeduplicates(t *testing.T) {
	repositories := []projectidentity.Repository{
		repositoryIdentityFixture("old-a", "r1", "l1", "a"),
		repositoryIdentityFixture("old-b", "r2", "l2", "b"),
	}
	canonical := CanonicalizeProjectRepositories(repositories, map[string]string{"old-a": "repo-one", "old-b": "repo-one"})
	if len(canonical) != 1 || canonical[0].ID != "repo-one" {
		t.Fatalf("downstream repository set was not canonicalized/deduplicated: %#v", canonical)
	}
}

func TestMarkerClaimRequiresValidatedSchemaAndKeepsLegacyCompatibilityExplicit(t *testing.T) {
	valid := markerClaimFromSnapshot(projectidentity.Snapshot{
		MarkerPresent: true, MarkerSchemaVersion: 1, MarkerValid: true,
		MarkerProjectID: "prj_alpha", MarkerName: "Alpha", SuggestedName: "fallback",
	})
	if valid.ProjectID != "prj_alpha" || valid.Name != "Alpha" || valid.Legacy || valid.Reason != "" {
		t.Fatalf("valid v1 marker claim rejected: %#v", valid)
	}
	invalid := markerClaimFromSnapshot(projectidentity.Snapshot{
		MarkerPresent: true, MarkerSchemaVersion: 2, MarkerValid: false, MarkerProjectID: "prj_alpha",
	})
	if invalid.ProjectID != "" || invalid.Reason != "unsupported_schema_version" {
		t.Fatalf("unsupported marker schema survived cloud validation: %#v", invalid)
	}
	legacy := markerClaimFromSnapshot(projectidentity.Snapshot{MarkerProjectID: "prj_legacy", SuggestedName: "Legacy"})
	if !legacy.Legacy || legacy.ProjectID != "prj_legacy" || legacy.Reason != "legacy_unversioned" {
		t.Fatalf("old runtime marker compatibility is not explicit/fail-closed: %#v", legacy)
	}
}

func TestCompetingRepositoryEvidenceRejectsMarkerAuthority(t *testing.T) {
	candidates := []projectCandidate{
		{ProjectID: "project-a", Matched: 1},
		{ProjectID: "project-b", Matched: 2},
	}
	if markerCandidateEvidence(candidates, "project-a") != 1 || competingMarkerProjectEvidence(candidates, "project-a") != 2 {
		t.Fatalf("marker evidence accounting is not deterministic: %#v", candidates)
	}
}

func TestRepositoryAliasMigrationSeparatesRemoteIdentityFromAmbiguousLineage(t *testing.T) {
	normalized := strings.ToLower(strings.Join(strings.Fields(repositoryIdentityMigrationSQL), " "))
	for _, required := range []string{
		"create table if not exists codelocal_repository_aliases",
		"primary key(user_id,alias_type,alias_value,repository_id)",
		"where alias_type='remote'",
		"where alias_type='lineage'",
		"foreign key(user_id,repository_id) references codelocal_repositories(user_id,repository_id)",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("repository alias migration lost invariant %q", required)
		}
	}
	if !strings.Contains(normalized, "create unique index if not exists idx_codelocal_repository_alias_remote_unique") {
		t.Fatal("remote alias is not tenant-unique")
	}
	if strings.Contains(normalized, "create unique index if not exists idx_codelocal_repository_alias_lineage") {
		t.Fatal("lineage alias must allow multiple repositories/forks")
	}
}

func TestRepositoryAliasQueriesRemainTenantAndProjectScoped(t *testing.T) {
	remote := strings.ToLower(strings.Join(strings.Fields(repositoryRemoteAliasLookupSQL), " "))
	if !strings.Contains(remote, "where user_id=$1") || !strings.Contains(remote, "alias_type='remote'") {
		t.Fatalf("remote alias lookup lost tenant scope: %s", remote)
	}
	lineage := strings.ToLower(strings.Join(strings.Fields(repositoryProjectLineageAliasSQL), " "))
	if !strings.Contains(lineage, "ra.user_id=$1") || !strings.Contains(lineage, "pr.project_id=$2") || !strings.Contains(lineage, "alias_type='lineage'") {
		t.Fatalf("lineage fallback escaped selected project scope: %s", lineage)
	}
}

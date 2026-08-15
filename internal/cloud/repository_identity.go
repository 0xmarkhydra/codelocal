package cloud

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
	"github.com/jackc/pgx/v5"
)

const repositoryIdentityMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_repository_aliases (
 user_id TEXT NOT NULL,
 alias_type TEXT NOT NULL CHECK (alias_type IN ('remote','lineage')),
 alias_value TEXT NOT NULL,
 repository_id TEXT NOT NULL,
 first_seen_at BIGINT NOT NULL,
 last_seen_at BIGINT NOT NULL,
 PRIMARY KEY(user_id,alias_type,alias_value,repository_id),
 FOREIGN KEY(user_id,repository_id) REFERENCES codelocal_repositories(user_id,repository_id) ON DELETE CASCADE
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_codelocal_repository_alias_remote_unique
 ON codelocal_repository_aliases(user_id,alias_value)
 WHERE alias_type='remote';
CREATE INDEX IF NOT EXISTS idx_codelocal_repository_alias_lineage
 ON codelocal_repository_aliases(user_id,alias_value,repository_id)
 WHERE alias_type='lineage';
INSERT INTO codelocal_repository_aliases(user_id,alias_type,alias_value,repository_id,first_seen_at,last_seen_at)
SELECT user_id,'remote',remote,repository_id,created_at,last_seen_at
FROM codelocal_repositories
WHERE remote IS NOT NULL AND BTRIM(remote)<>''
ON CONFLICT(user_id,alias_type,alias_value,repository_id) DO UPDATE SET
 last_seen_at=GREATEST(codelocal_repository_aliases.last_seen_at,EXCLUDED.last_seen_at);
INSERT INTO codelocal_repository_aliases(user_id,alias_type,alias_value,repository_id,first_seen_at,last_seen_at)
SELECT user_id,'lineage',LOWER(lineage),repository_id,created_at,last_seen_at
FROM codelocal_repositories
WHERE lineage IS NOT NULL AND BTRIM(lineage)<>''
ON CONFLICT(user_id,alias_type,alias_value,repository_id) DO UPDATE SET
 last_seen_at=GREATEST(codelocal_repository_aliases.last_seen_at,EXCLUDED.last_seen_at);
`

const repositoryRemoteAliasLookupSQL = `
SELECT alias_value,repository_id
FROM codelocal_repository_aliases
WHERE user_id=$1 AND alias_type='remote' AND alias_value=ANY($2::text[])
ORDER BY alias_value ASC,repository_id ASC`

const repositoryProjectLineageAliasSQL = `
SELECT ra.alias_value,ra.repository_id
FROM codelocal_repository_aliases ra
JOIN codelocal_project_repositories pr
 ON pr.user_id=ra.user_id AND pr.repository_id=ra.repository_id
WHERE ra.user_id=$1 AND pr.project_id=$2 AND ra.alias_type='lineage' AND ra.alias_value=ANY($3::text[])
ORDER BY ra.alias_value ASC,ra.repository_id ASC`

const repositoryLineageProjectCountSQL = `
SELECT COUNT(DISTINCT pr.project_id)::int
FROM codelocal_repository_aliases ra
JOIN codelocal_project_repositories pr
 ON pr.user_id=ra.user_id AND pr.repository_id=ra.repository_id
WHERE ra.user_id=$1 AND ra.alias_type='lineage' AND ra.alias_value=ANY($2::text[])`

type repositoryAliasResolution struct {
	Repositories        []projectidentity.Repository
	ObservedIDs         []string
	ObservedToCanonical map[string]string
	RemoteMatched       map[string]bool
	RemoteAliasMatched  int
	LineageAliasMatched int
}

func normalizeRepositoryAliases(repositories []projectidentity.Repository) []projectidentity.Repository {
	out := make([]projectidentity.Repository, 0, len(repositories))
	for _, repository := range repositories {
		repository.ID = strings.TrimSpace(repository.ID)
		repository.Remote = strings.TrimSpace(repository.Remote)
		repository.Lineage = strings.ToLower(strings.TrimSpace(repository.Lineage))
		repository.RelativePath = safeRepositoryRelativePath(repository.RelativePath)
		if repository.ID == "" || repository.RelativePath == "" {
			continue
		}
		if repository.IdentitySource != "remote" && repository.IdentitySource != "lineage" {
			continue
		}
		out = append(out, repository)
	}
	if len(out) > 64 {
		out = out[:64]
	}
	return out
}

func uniqueRepositoryLineages(repositories []projectidentity.Repository) []string {
	counts := map[string]int{}
	for _, repository := range repositories {
		if repository.Lineage != "" {
			counts[repository.Lineage]++
		}
	}
	out := []string{}
	for lineage, count := range counts {
		if count == 1 {
			out = append(out, lineage)
		}
	}
	sort.Strings(out)
	return out
}

func repositoryRemotes(repositories []projectidentity.Repository) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, repository := range repositories {
		remote := strings.TrimSpace(repository.Remote)
		if remote == "" {
			continue
		}
		if _, ok := seen[remote]; ok {
			continue
		}
		seen[remote] = struct{}{}
		out = append(out, remote)
	}
	sort.Strings(out)
	return out
}

func (s *Store) remoteRepositoryAliases(ctx context.Context, userID string, remotes []string) (map[string]string, error) {
	out := map[string]string{}
	if len(remotes) == 0 {
		return out, nil
	}
	rows, err := s.DB.Query(ctx, repositoryRemoteAliasLookupSQL, strings.TrimSpace(userID), remotes)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var remote, repositoryID string
		if err := rows.Scan(&remote, &repositoryID); err != nil {
			return nil, err
		}
		if existing := out[remote]; existing != "" && existing != repositoryID {
			return nil, errors.New("remote alias resolves to multiple repositories")
		}
		out[remote] = repositoryID
	}
	return out, rows.Err()
}

func (s *Store) projectLineageRepositoryAliases(ctx context.Context, userID, projectID string, lineages []string) (map[string][]string, error) {
	out := map[string][]string{}
	if strings.TrimSpace(projectID) == "" || len(lineages) == 0 {
		return out, nil
	}
	rows, err := s.DB.Query(ctx, repositoryProjectLineageAliasSQL, strings.TrimSpace(userID), strings.TrimSpace(projectID), lineages)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var lineage, repositoryID string
		if err := rows.Scan(&lineage, &repositoryID); err != nil {
			return nil, err
		}
		ids := out[lineage]
		duplicate := false
		for _, existing := range ids {
			if existing == repositoryID {
				duplicate = true
				break
			}
		}
		if !duplicate {
			out[lineage] = append(ids, repositoryID)
		}
	}
	return out, rows.Err()
}

func (s *Store) lineageProjectCount(ctx context.Context, userID string, lineages []string) (int, error) {
	if len(lineages) == 0 {
		return 0, nil
	}
	var count int
	err := s.DB.QueryRow(ctx, repositoryLineageProjectCountSQL, strings.TrimSpace(userID), lineages).Scan(&count)
	return count, err
}

func applyRemoteAliasResolution(repositories []projectidentity.Repository, aliases map[string]string) repositoryAliasResolution {
	resolution := repositoryAliasResolution{
		Repositories: append([]projectidentity.Repository(nil), repositories...),
		ObservedIDs:  make([]string, len(repositories)), ObservedToCanonical: map[string]string{}, RemoteMatched: map[string]bool{},
	}
	for index := range resolution.Repositories {
		repository := &resolution.Repositories[index]
		observedID := repository.ID
		resolution.ObservedIDs[index] = observedID
		canonicalID := observedID
		if aliasID := strings.TrimSpace(aliases[repository.Remote]); aliasID != "" {
			canonicalID = aliasID
			resolution.RemoteMatched[observedID] = true
			if canonicalID != observedID {
				resolution.RemoteAliasMatched++
			}
		}
		repository.ID = canonicalID
		resolution.ObservedToCanonical[observedID] = canonicalID
	}
	return resolution
}

func applyProjectLineageResolution(resolution repositoryAliasResolution, aliases map[string][]string) repositoryAliasResolution {
	incomingUnique := map[string]bool{}
	for _, lineage := range uniqueRepositoryLineages(resolution.Repositories) {
		incomingUnique[lineage] = true
	}
	for index := range resolution.Repositories {
		repository := &resolution.Repositories[index]
		observedID := resolution.ObservedIDs[index]
		if resolution.RemoteMatched[observedID] || repository.Lineage == "" || !incomingUnique[repository.Lineage] {
			continue
		}
		ids := aliases[repository.Lineage]
		if len(ids) != 1 || repository.ID == ids[0] {
			continue
		}
		repository.ID = ids[0]
		resolution.ObservedToCanonical[observedID] = ids[0]
		resolution.LineageAliasMatched++
	}
	return resolution
}

func dedupeCanonicalRepositories(repositories []projectidentity.Repository) []projectidentity.Repository {
	sort.SliceStable(repositories, func(i, j int) bool {
		if repositories[i].RelativePath != repositories[j].RelativePath {
			return repositories[i].RelativePath < repositories[j].RelativePath
		}
		return repositories[i].ID < repositories[j].ID
	})
	seen := map[string]struct{}{}
	out := make([]projectidentity.Repository, 0, len(repositories))
	for _, repository := range repositories {
		if _, exists := seen[repository.ID]; exists {
			continue
		}
		seen[repository.ID] = struct{}{}
		out = append(out, repository)
	}
	return out
}

func CanonicalizeProjectRepositories(repositories []projectidentity.Repository, mapping map[string]string) []projectidentity.Repository {
	out := append([]projectidentity.Repository(nil), repositories...)
	for index := range out {
		if canonicalID := strings.TrimSpace(mapping[out[index].ID]); canonicalID != "" {
			out[index].ID = canonicalID
		}
	}
	return dedupeCanonicalRepositories(out)
}

func upsertRepositoryAlias(ctx context.Context, tx pgx.Tx, userID, repositoryID, aliasType, aliasValue string, now int64) error {
	aliasValue = strings.TrimSpace(aliasValue)
	if aliasType == "lineage" {
		aliasValue = strings.ToLower(aliasValue)
	}
	if aliasValue == "" {
		return nil
	}
	_, err := tx.Exec(ctx, `
INSERT INTO codelocal_repository_aliases(user_id,alias_type,alias_value,repository_id,first_seen_at,last_seen_at)
VALUES($1,$2,$3,$4,$5,$5)
ON CONFLICT(user_id,alias_type,alias_value,repository_id) DO UPDATE SET
 last_seen_at=GREATEST(codelocal_repository_aliases.last_seen_at,EXCLUDED.last_seen_at)`, userID, aliasType, aliasValue, repositoryID, now)
	return err
}

func persistRepositoryAliases(ctx context.Context, tx pgx.Tx, userID string, repository projectidentity.Repository, now int64) error {
	if err := upsertRepositoryAlias(ctx, tx, userID, repository.ID, "remote", repository.Remote, now); err != nil {
		return err
	}
	return upsertRepositoryAlias(ctx, tx, userID, repository.ID, "lineage", repository.Lineage, now)
}

type markerClaim struct {
	Present   bool
	ProjectID string
	Name      string
	Version   int
	Legacy    bool
	Reason    string
}

func markerClaimFromSnapshot(snapshot projectidentity.Snapshot) markerClaim {
	if snapshot.MarkerPresent {
		if snapshot.MarkerSchemaVersion != 1 {
			return markerClaim{Present: true, Version: snapshot.MarkerSchemaVersion, Reason: "unsupported_schema_version"}
		}
		if !snapshot.MarkerValid {
			return markerClaim{Present: true, Version: snapshot.MarkerSchemaVersion, Reason: "marker_invalid"}
		}
		projectID := safeMarkerProjectID(snapshot.MarkerProjectID)
		name := safeProjectName(snapshot.MarkerName)
		if projectID == "" || strings.TrimSpace(snapshot.MarkerName) == "" {
			return markerClaim{Present: true, Version: snapshot.MarkerSchemaVersion, Reason: "marker_payload_invalid"}
		}
		return markerClaim{Present: true, ProjectID: projectID, Name: name, Version: 1}
	}
	// Compatibility for runtimes that predate marker schemaVersion. Legacy
	// claims may continue an existing project when repository evidence agrees,
	// but may never create a new project solely from the unversioned marker.
	if projectID := safeMarkerProjectID(snapshot.MarkerProjectID); projectID != "" {
		return markerClaim{Present: true, ProjectID: projectID, Name: safeProjectName(snapshot.SuggestedName), Legacy: true, Reason: "legacy_unversioned"}
	}
	return markerClaim{}
}

func markerCandidateEvidence(candidates []projectCandidate, projectID string) int {
	for _, candidate := range candidates {
		if candidate.ProjectID == projectID {
			return candidate.Matched
		}
	}
	return 0
}

func competingMarkerProjectEvidence(candidates []projectCandidate, projectID string) int {
	matched := 0
	for _, candidate := range candidates {
		if candidate.ProjectID != projectID {
			matched += candidate.Matched
		}
	}
	return matched
}

func uniqueLineageRepositoryCount(aliases map[string][]string) int {
	seen := map[string]struct{}{}
	for _, ids := range aliases {
		for _, repositoryID := range ids {
			seen[repositoryID] = struct{}{}
		}
	}
	return len(seen)
}

func (s *Store) resolveMarkerClaim(ctx context.Context, userID string, claim markerClaim, candidates []projectCandidate, repositories []projectidentity.Repository) (string, string, string, string, int, error) {
	if !claim.Present {
		return "", "", "", "", 0, nil
	}
	if claim.ProjectID == "" {
		return "", "", "rejected", claim.Reason, 0, nil
	}
	existing, err := s.projectByID(ctx, userID, claim.ProjectID)
	if err != nil {
		return "", "", "", "", 0, err
	}
	lineages := uniqueRepositoryLineages(repositories)
	if existing != nil {
		if competingMarkerProjectEvidence(candidates, claim.ProjectID) > 0 {
			return "", "", "rejected", "marker_competing_repository_evidence", 0, nil
		}
		lineageAliases, err := s.projectLineageRepositoryAliases(ctx, userID, claim.ProjectID, lineages)
		if err != nil {
			return "", "", "", "", 0, err
		}
		matched := markerCandidateEvidence(candidates, claim.ProjectID)
		lineageMatched := uniqueLineageRepositoryCount(lineageAliases)
		if matched == 0 && lineageMatched == 0 {
			return "", "", "rejected", "marker_repository_evidence_mismatch", 0, nil
		}
		status := "accepted"
		if claim.Legacy {
			status = "legacy-accepted"
		}
		return claim.ProjectID, existing.Name, status, claim.Reason, matched + lineageMatched, nil
	}
	if claim.Legacy {
		return "", "", "rejected", "legacy_marker_cannot_create_project", 0, nil
	}
	if len(repositories) == 0 {
		return "", "", "rejected", "marker_missing_repository_evidence", 0, nil
	}
	if len(candidates) > 0 {
		return "", "", "rejected", "marker_repositories_already_bound", 0, nil
	}
	lineageProjects, err := s.lineageProjectCount(ctx, userID, lineages)
	if err != nil {
		return "", "", "", "", 0, err
	}
	if lineageProjects > 0 {
		return "", "", "rejected", "marker_lineage_already_bound", 0, nil
	}
	return claim.ProjectID, claim.Name, "accepted", "", 0, nil
}

func repositoryIDs(repositories []projectidentity.Repository) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, repository := range repositories {
		if repository.ID == "" {
			continue
		}
		if _, exists := seen[repository.ID]; exists {
			continue
		}
		seen[repository.ID] = struct{}{}
		out = append(out, repository.ID)
	}
	sort.Strings(out)
	return out
}

func repositoryAliasNow() int64 { return time.Now().UnixMilli() }

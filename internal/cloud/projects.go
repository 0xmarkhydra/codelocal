package cloud

import (
	"context"
	"errors"
	"path"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
	"github.com/jackc/pgx/v5"
)

type Project struct {
	UserID     string `json:"userId"`
	ProjectID  string `json:"projectId"`
	Name       string `json:"name"`
	CreatedAt  int64  `json:"createdAt"`
	LastSeenAt int64  `json:"lastSeenAt"`
}

type ProjectBinding struct {
	ProjectID        string   `json:"projectId"`
	ProjectName      string   `json:"projectName"`
	Source           string   `json:"source"`
	Confidence       float64  `json:"confidence"`
	RepositoryIDs    []string `json:"repositoryIds,omitempty"`
	MatchedRepoCount int      `json:"matchedRepoCount,omitempty"`
}

type KnowledgeNode struct {
	ID             string  `json:"id"`
	Kind           string  `json:"kind"`
	Name           string  `json:"name"`
	Summary        string  `json:"summary,omitempty"`
	Scope          string  `json:"scope,omitempty"`
	Confidence     float64 `json:"confidence,omitempty"`
	Importance     float64 `json:"importance,omitempty"`
	LastSeenAt     int64   `json:"lastSeenAt,omitempty"`
	SourceMemoryID string  `json:"sourceMemoryId,omitempty"`
}

type KnowledgeEdge struct {
	ID         string  `json:"id"`
	From       string  `json:"from"`
	To         string  `json:"to"`
	Relation   string  `json:"relation"`
	Confidence float64 `json:"confidence,omitempty"`
	Importance float64 `json:"importance,omitempty"`
}

type KnowledgeGraph struct {
	Nodes []KnowledgeNode `json:"nodes"`
	Edges []KnowledgeEdge `json:"edges"`
	Stats map[string]int  `json:"stats"`
}

type projectCandidate struct {
	ProjectID string
	Name      string
	Matched   int
	Total     int
	Coverage  float64
}

func safeProjectName(value string) string {
	value = strings.Join(strings.Fields(strings.TrimSpace(value)), " ")
	if value == "" {
		return "Project"
	}
	runes := []rune(value)
	if len(runes) > 120 {
		value = string(runes[:120])
	}
	return value
}

func safeRepositoryRelativePath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || value == "." {
		return "."
	}
	value = path.Clean(value)
	if value == "." {
		return "."
	}
	if strings.HasPrefix(value, "/") || value == ".." || strings.HasPrefix(value, "../") {
		return ""
	}
	if len(value) > 500 {
		return ""
	}
	return value
}

func safeMarkerProjectID(value string) string {
	value = strings.TrimSpace(value)
	if len(value) < 4 || len(value) > 128 {
		return ""
	}
	for _, r := range value {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '_' || r == '-' || r == '.' {
			continue
		}
		return ""
	}
	return value
}

func remoteRepositoryIDs(repositories []projectidentity.Repository) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, repo := range repositories {
		if repo.IdentitySource != "remote" || strings.TrimSpace(repo.ID) == "" || strings.TrimSpace(repo.Remote) == "" {
			continue
		}
		if _, exists := seen[repo.ID]; exists {
			continue
		}
		seen[repo.ID] = struct{}{}
		out = append(out, repo.ID)
	}
	sort.Strings(out)
	return out
}

func repositoryLineages(repositories []projectidentity.Repository) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, repo := range repositories {
		lineage := strings.ToLower(strings.TrimSpace(repo.Lineage))
		if lineage == "" || len(lineage) > 128 {
			continue
		}
		if _, exists := seen[lineage]; exists {
			continue
		}
		seen[lineage] = struct{}{}
		out = append(out, lineage)
	}
	sort.Strings(out)
	return out
}

func chooseProjectCandidate(candidates []projectCandidate, incomingRemoteRepos int) (projectCandidate, bool) {
	if incomingRemoteRepos <= 0 {
		return projectCandidate{}, false
	}
	qualified := []projectCandidate{}
	for _, candidate := range candidates {
		if candidate.Matched <= 0 || candidate.Total <= 0 {
			continue
		}
		candidate.Coverage = float64(candidate.Matched) / float64(incomingRemoteRepos)
		if incomingRemoteRepos == 1 {
			if candidate.Matched == 1 && candidate.Total == 1 {
				qualified = append(qualified, candidate)
			}
			continue
		}
		union := incomingRemoteRepos + candidate.Total - candidate.Matched
		jaccard := 0.0
		if union > 0 {
			jaccard = float64(candidate.Matched) / float64(union)
		}
		if candidate.Matched >= 2 && candidate.Coverage >= .5 && jaccard >= .20 {
			qualified = append(qualified, candidate)
		}
	}
	if len(qualified) == 0 {
		return projectCandidate{}, false
	}
	sort.Slice(qualified, func(i, j int) bool {
		if qualified[i].Matched != qualified[j].Matched {
			return qualified[i].Matched > qualified[j].Matched
		}
		if qualified[i].Coverage != qualified[j].Coverage {
			return qualified[i].Coverage > qualified[j].Coverage
		}
		return qualified[i].ProjectID < qualified[j].ProjectID
	})
	if len(qualified) > 1 && qualified[0].Matched == qualified[1].Matched && qualified[0].Coverage == qualified[1].Coverage {
		return projectCandidate{}, false
	}
	return qualified[0], true
}

func existingProjectCandidate(candidates []projectCandidate, projectID string, incomingRemoteRepos int) (projectCandidate, bool) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return projectCandidate{}, false
	}
	for _, candidate := range candidates {
		if candidate.ProjectID != projectID || candidate.Matched <= 0 {
			continue
		}
		if incomingRemoteRepos > 0 {
			candidate.Coverage = float64(candidate.Matched) / float64(incomingRemoteRepos)
		}
		return candidate, true
	}
	return projectCandidate{}, false
}

func (s *Store) WorkspaceProject(ctx context.Context, userID, deviceID, workspaceID string) (*ProjectBinding, error) {
	var binding ProjectBinding
	err := s.DB.QueryRow(ctx, `
SELECT wp.project_id,p.name,wp.source,wp.confidence
FROM codelocal_workspace_projects wp
JOIN codelocal_projects p ON p.user_id=wp.user_id AND p.project_id=wp.project_id
WHERE wp.user_id=$1 AND wp.device_id=$2 AND wp.workspace_id=$3`, userID, deviceID, workspaceID).Scan(&binding.ProjectID, &binding.ProjectName, &binding.Source, &binding.Confidence)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &binding, nil
}

func (s *Store) projectByID(ctx context.Context, userID, projectID string) (*Project, error) {
	var project Project
	err := s.DB.QueryRow(ctx, `SELECT user_id,project_id,name,created_at,last_seen_at FROM codelocal_projects WHERE user_id=$1 AND project_id=$2`, userID, projectID).Scan(&project.UserID, &project.ProjectID, &project.Name, &project.CreatedAt, &project.LastSeenAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &project, nil
}

func (s *Store) matchingProjects(ctx context.Context, userID string, repositoryIDs []string) ([]projectCandidate, error) {
	if len(repositoryIDs) == 0 {
		return nil, nil
	}
	rows, err := s.DB.Query(ctx, `
SELECT p.project_id,p.name,
 COUNT(*) FILTER (WHERE pr.repository_id = ANY($2::text[]))::int AS matched,
 COUNT(*)::int AS total
FROM codelocal_projects p
JOIN codelocal_project_repositories pr ON pr.user_id=p.user_id AND pr.project_id=p.project_id
WHERE p.user_id=$1
GROUP BY p.project_id,p.name
HAVING COUNT(*) FILTER (WHERE pr.repository_id = ANY($2::text[])) > 0`, userID, repositoryIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []projectCandidate{}
	for rows.Next() {
		var candidate projectCandidate
		if err := rows.Scan(&candidate.ProjectID, &candidate.Name, &candidate.Matched, &candidate.Total); err != nil {
			return nil, err
		}
		out = append(out, candidate)
	}
	return out, rows.Err()
}

func (s *Store) projectLineageOverlap(ctx context.Context, userID, projectID string, lineages []string) (int, error) {
	if strings.TrimSpace(projectID) == "" || len(lineages) == 0 {
		return 0, nil
	}
	var matched int
	err := s.DB.QueryRow(ctx, `
SELECT COUNT(DISTINCT r.repository_id)::int
FROM codelocal_project_repositories pr
JOIN codelocal_repositories r ON r.user_id=pr.user_id AND r.repository_id=pr.repository_id
WHERE pr.user_id=$1 AND pr.project_id=$2 AND r.lineage=ANY($3::text[])`, userID, projectID, lineages).Scan(&matched)
	return matched, err
}

func (s *Store) ResolveWorkspaceProject(ctx context.Context, userID, deviceID, workspaceID string, snapshot projectidentity.Snapshot) (ProjectBinding, error) {
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	workspaceID = strings.TrimSpace(workspaceID)
	if userID == "" || deviceID == "" || workspaceID == "" {
		return ProjectBinding{}, errors.New("project resolution requires user, device and workspace")
	}
	if len(snapshot.Repositories) > 64 {
		snapshot.Repositories = snapshot.Repositories[:64]
	}
	now := time.Now().UnixMilli()
	name := safeProjectName(snapshot.SuggestedName)
	markerID := safeMarkerProjectID(snapshot.MarkerProjectID)
	existingBinding, err := s.WorkspaceProject(ctx, userID, deviceID, workspaceID)
	if err != nil {
		return ProjectBinding{}, err
	}
	if markerID == "" && len(snapshot.Repositories) == 0 {
		if existingBinding != nil {
			return *existingBinding, nil
		}
		return ProjectBinding{}, nil
	}
	remoteIDs := remoteRepositoryIDs(snapshot.Repositories)
	lineages := repositoryLineages(snapshot.Repositories)

	projectID := ""
	projectName := name
	source := "created"
	confidence := 1.0
	matchedCount := 0
	candidates := []projectCandidate{}

	if markerID != "" {
		projectID = markerID
		source = "marker"
		if existing, err := s.projectByID(ctx, userID, markerID); err != nil {
			return ProjectBinding{}, err
		} else if existing != nil {
			projectName = existing.Name
		}
	} else {
		candidates, err = s.matchingProjects(ctx, userID, remoteIDs)
		if err != nil {
			return ProjectBinding{}, err
		}
		if candidate, ok := chooseProjectCandidate(candidates, len(remoteIDs)); ok {
			projectID = candidate.ProjectID
			projectName = candidate.Name
			source = "repository-set"
			confidence = candidate.Coverage
			matchedCount = candidate.Matched
		}
	}
	// A previously bound workspace is a strong continuity signal when the new
	// snapshot is only a partial checkout (for example BIDDI 1/8 repos) or when
	// a remote URL changed but Git lineage stayed the same. We use this only when
	// no other project already produced a strong repository-set match, so a
	// genuinely repurposed folder can still rebind safely.
	if projectID == "" && existingBinding != nil {
		if candidate, ok := existingProjectCandidate(candidates, existingBinding.ProjectID, len(remoteIDs)); ok {
			projectID = existingBinding.ProjectID
			projectName = existingBinding.ProjectName
			source = "existing-workspace"
			matchedCount = candidate.Matched
			confidence = candidate.Coverage
			if len(remoteIDs) == 0 {
				confidence = existingBinding.Confidence
			}
		}
		if projectID == "" {
			lineageMatched, lineageErr := s.projectLineageOverlap(ctx, userID, existingBinding.ProjectID, lineages)
			if lineageErr != nil {
				return ProjectBinding{}, lineageErr
			}
			if lineageMatched > 0 {
				projectID = existingBinding.ProjectID
				projectName = existingBinding.ProjectName
				source = "existing-lineage"
				matchedCount = lineageMatched
				confidence = 1
				if len(lineages) > 0 && lineageMatched < len(lineages) {
					confidence = float64(lineageMatched) / float64(len(lineages))
				}
			}
		}
	}
	if projectID == "" {
		projectID = "prj_" + RandomHex(12)
	}

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ProjectBinding{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_projects(user_id,project_id,name,created_at,last_seen_at)
VALUES($1,$2,$3,$4,$4)
ON CONFLICT(user_id,project_id) DO UPDATE SET last_seen_at=GREATEST(codelocal_projects.last_seen_at,EXCLUDED.last_seen_at)`, userID, projectID, projectName, now); err != nil {
		return ProjectBinding{}, err
	}
	// Relative repository paths are checkout-local bindings, not repository
	// identity. Refresh this workspace map atomically on every project sync so
	// repository-scoped memory cannot be routed using a stale nested-repo path.
	if _, err := tx.Exec(ctx, `DELETE FROM codelocal_workspace_repositories WHERE user_id=$1 AND device_id=$2 AND workspace_id=$3`, userID, deviceID, workspaceID); err != nil {
		return ProjectBinding{}, err
	}
	for _, repo := range snapshot.Repositories {
		if strings.TrimSpace(repo.ID) == "" {
			continue
		}
		identitySource := strings.TrimSpace(repo.IdentitySource)
		if identitySource != "remote" && identitySource != "lineage" {
			continue
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_repositories(user_id,repository_id,remote,lineage,identity_source,created_at,last_seen_at)
VALUES($1,$2,NULLIF($3,''),NULLIF($4,''),$5,$6,$6)
ON CONFLICT(user_id,repository_id) DO UPDATE SET
 remote=COALESCE(EXCLUDED.remote,codelocal_repositories.remote),
 lineage=COALESCE(EXCLUDED.lineage,codelocal_repositories.lineage),
 last_seen_at=GREATEST(codelocal_repositories.last_seen_at,EXCLUDED.last_seen_at)`, userID, repo.ID, repo.Remote, repo.Lineage, identitySource, now); err != nil {
			return ProjectBinding{}, err
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_project_repositories(user_id,project_id,repository_id,created_at,last_seen_at)
VALUES($1,$2,$3,$4,$4)
ON CONFLICT(user_id,project_id,repository_id) DO UPDATE SET last_seen_at=GREATEST(codelocal_project_repositories.last_seen_at,EXCLUDED.last_seen_at)`, userID, projectID, repo.ID, now); err != nil {
			return ProjectBinding{}, err
		}
		if relativePath := safeRepositoryRelativePath(repo.RelativePath); relativePath != "" {
			if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_workspace_repositories(user_id,device_id,workspace_id,repository_id,relative_path,created_at,last_seen_at)
VALUES($1,$2,$3,$4,$5,$6,$6)
ON CONFLICT(user_id,device_id,workspace_id,repository_id) DO UPDATE SET
 relative_path=EXCLUDED.relative_path,last_seen_at=EXCLUDED.last_seen_at`, userID, deviceID, workspaceID, repo.ID, relativePath, now); err != nil {
				return ProjectBinding{}, err
			}
		}
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_workspace_projects(user_id,device_id,workspace_id,project_id,source,confidence,created_at,last_seen_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$7)
ON CONFLICT(user_id,device_id,workspace_id) DO UPDATE SET
 project_id=EXCLUDED.project_id,source=EXCLUDED.source,confidence=EXCLUDED.confidence,last_seen_at=EXCLUDED.last_seen_at`, userID, deviceID, workspaceID, projectID, source, confidence, now); err != nil {
		return ProjectBinding{}, err
	}
	// Project↔repository membership is the union of repository bindings still
	// reported by the project's concrete workspaces. Prune edges that no current
	// checkout supports so a repo temporarily present in a project does not
	// become a permanent identity signal or stale Knowledge Graph edge.
	if _, err := tx.Exec(ctx, `
DELETE FROM codelocal_project_repositories pr
WHERE pr.user_id=$1
 AND NOT EXISTS (
  SELECT 1
  FROM codelocal_workspace_projects wp
  JOIN codelocal_workspace_repositories wr
   ON wr.user_id=wp.user_id AND wr.device_id=wp.device_id AND wr.workspace_id=wp.workspace_id
  WHERE wp.user_id=pr.user_id
   AND wp.project_id=pr.project_id
   AND wr.repository_id=pr.repository_id
 )`, userID); err != nil {
		return ProjectBinding{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProjectBinding{}, err
	}
	allIDs := make([]string, 0, len(snapshot.Repositories))
	seen := map[string]struct{}{}
	for _, repo := range snapshot.Repositories {
		if repo.ID == "" {
			continue
		}
		if _, ok := seen[repo.ID]; ok {
			continue
		}
		seen[repo.ID] = struct{}{}
		allIDs = append(allIDs, repo.ID)
	}
	sort.Strings(allIDs)
	return ProjectBinding{ProjectID: projectID, ProjectName: projectName, Source: source, Confidence: confidence, RepositoryIDs: allIDs, MatchedRepoCount: matchedCount}, nil
}

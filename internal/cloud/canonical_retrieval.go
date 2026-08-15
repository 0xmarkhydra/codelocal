package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"
)

const canonicalKnowledgeRecallSQL = `
SELECT
 o.user_id,o.knowledge_id,o.project_id,COALESCE(o.repository_id,''),COALESCE(o.branch,''),o.knowledge_type,o.stable_key,o.cardinality,o.qualifier,
 o.status,o.privacy_classification,COALESCE(o.active_revision_id,''),o.confidence,o.importance,o.valid_from,COALESCE(o.valid_until,0),o.created_at,o.updated_at,
 r.revision_id,r.revision_number,r.subject,r.predicate,r.object,r.summary,r.semantic_fingerprint,r.confidence,r.importance,r.valid_from,COALESCE(r.valid_until,0),r.created_at
FROM codelocal_knowledge_objects o
JOIN codelocal_knowledge_revisions r
 ON r.user_id=o.user_id AND r.knowledge_id=o.knowledge_id AND r.revision_id=o.active_revision_id
WHERE o.user_id=$1
 AND o.project_id=$2
 AND o.status='active'
 AND o.privacy_classification='private_project'
 AND o.valid_from <= $5
 AND o.valid_until IS NULL
 AND r.valid_from <= $5
 AND r.valid_until IS NULL
 AND (o.repository_id IS NULL OR o.repository_id=ANY($3::text[]))
 AND (o.branch IS NULL OR ($4<>'' AND o.branch=$4))
ORDER BY o.updated_at DESC,o.knowledge_id ASC
LIMIT $6`

const (
	canonicalRecallDefaultLimit    = 6
	canonicalRecallMaxLimit        = 16
	canonicalRecallMaxRepositories = 16
	canonicalRecallCandidateLimit  = 64
)

type CanonicalKnowledgeRecallInput struct {
	UserID        string
	ProjectID     string
	RepositoryIDs []string
	Branch        string
	Limit         int
}

type CanonicalKnowledgeHit struct {
	Knowledge CanonicalKnowledge         `json:"knowledge"`
	Revision  CanonicalKnowledgeRevision `json:"revision"`
	ScopeRank int                        `json:"scopeRank"`
	TypeRank  int                        `json:"typeRank"`
}

func normalizeCanonicalRecallInput(input CanonicalKnowledgeRecallInput) (CanonicalKnowledgeRecallInput, error) {
	input.UserID = strings.TrimSpace(input.UserID)
	input.ProjectID = strings.TrimSpace(input.ProjectID)
	input.Branch = strings.TrimSpace(input.Branch)
	if input.UserID == "" || input.ProjectID == "" {
		return CanonicalKnowledgeRecallInput{}, errors.New("canonical recall requires user and project")
	}
	if len(input.Branch) > 200 {
		input.Branch = input.Branch[:200]
	}
	if input.Limit <= 0 {
		input.Limit = canonicalRecallDefaultLimit
	}
	if input.Limit > canonicalRecallMaxLimit {
		input.Limit = canonicalRecallMaxLimit
	}
	seen := map[string]struct{}{}
	repositories := make([]string, 0, min(len(input.RepositoryIDs), canonicalRecallMaxRepositories))
	for _, repositoryID := range input.RepositoryIDs {
		repositoryID = strings.TrimSpace(repositoryID)
		if repositoryID == "" {
			continue
		}
		if _, ok := seen[repositoryID]; ok {
			continue
		}
		seen[repositoryID] = struct{}{}
		repositories = append(repositories, repositoryID)
		if len(repositories) >= canonicalRecallMaxRepositories {
			break
		}
	}
	sort.Strings(repositories)
	input.RepositoryIDs = repositories
	return input, nil
}

func canonicalScopeRank(knowledge CanonicalKnowledge) int {
	repositoryScoped := strings.TrimSpace(knowledge.RepositoryID) != ""
	branchScoped := strings.TrimSpace(knowledge.Branch) != ""
	switch {
	case repositoryScoped && branchScoped:
		return 4
	case repositoryScoped:
		return 3
	case branchScoped:
		return 2
	default:
		return 1
	}
}

func canonicalTypeRank(knowledgeType string) int {
	switch strings.TrimSpace(knowledgeType) {
	case "constraint":
		return 6
	case "decision":
		return 5
	case "architecture":
		return 5
	case "project_fact":
		return 4
	case "problem":
		return 3
	case "milestone":
		return 2
	default:
		return 0
	}
}

func canonicalHitEligible(hit CanonicalKnowledgeHit, input CanonicalKnowledgeRecallInput, now int64) bool {
	knowledge := hit.Knowledge
	revision := hit.Revision
	if knowledge.UserID != input.UserID || knowledge.ProjectID != input.ProjectID || knowledge.Status != KnowledgeStatusActive {
		return false
	}
	if knowledge.PrivacyClassification != KnowledgeClassPrivateProject || knowledge.ActiveRevisionID == "" || knowledge.ActiveRevisionID != revision.RevisionID {
		return false
	}
	if knowledge.ValidFrom > now || knowledge.ValidUntil > 0 || revision.ValidFrom > now || revision.ValidUntil > 0 {
		return false
	}
	if knowledge.RepositoryID != "" {
		matched := false
		for _, repositoryID := range input.RepositoryIDs {
			if repositoryID == knowledge.RepositoryID {
				matched = true
				break
			}
		}
		if !matched {
			return false
		}
	}
	if knowledge.Branch != "" && (input.Branch == "" || knowledge.Branch != input.Branch) {
		return false
	}
	return true
}

func rankCanonicalKnowledgeHits(hits []CanonicalKnowledgeHit, input CanonicalKnowledgeRecallInput, now int64) []CanonicalKnowledgeHit {
	eligible := make([]CanonicalKnowledgeHit, 0, len(hits))
	for _, hit := range hits {
		if !canonicalHitEligible(hit, input, now) {
			continue
		}
		hit.ScopeRank = canonicalScopeRank(hit.Knowledge)
		hit.TypeRank = canonicalTypeRank(hit.Knowledge.KnowledgeType)
		eligible = append(eligible, hit)
	}
	sort.SliceStable(eligible, func(i, j int) bool {
		left, right := eligible[i], eligible[j]
		if left.ScopeRank != right.ScopeRank {
			return left.ScopeRank > right.ScopeRank
		}
		if left.TypeRank != right.TypeRank {
			return left.TypeRank > right.TypeRank
		}
		if left.Knowledge.Confidence != right.Knowledge.Confidence {
			return left.Knowledge.Confidence > right.Knowledge.Confidence
		}
		if left.Knowledge.Importance != right.Knowledge.Importance {
			return left.Knowledge.Importance > right.Knowledge.Importance
		}
		if left.Knowledge.UpdatedAt != right.Knowledge.UpdatedAt {
			return left.Knowledge.UpdatedAt > right.Knowledge.UpdatedAt
		}
		if left.Revision.CreatedAt != right.Revision.CreatedAt {
			return left.Revision.CreatedAt > right.Revision.CreatedAt
		}
		if left.Knowledge.StableKey != right.Knowledge.StableKey {
			return left.Knowledge.StableKey < right.Knowledge.StableKey
		}
		if left.Knowledge.Qualifier != right.Knowledge.Qualifier {
			return left.Knowledge.Qualifier < right.Knowledge.Qualifier
		}
		return left.Knowledge.KnowledgeID < right.Knowledge.KnowledgeID
	})
	if len(eligible) > input.Limit {
		eligible = eligible[:input.Limit]
	}
	return eligible
}

func scanCanonicalKnowledgeHit(row interface{ Scan(...any) error }) (CanonicalKnowledgeHit, error) {
	var hit CanonicalKnowledgeHit
	var subjectJSON, objectJSON []byte
	err := row.Scan(
		&hit.Knowledge.UserID, &hit.Knowledge.KnowledgeID, &hit.Knowledge.ProjectID, &hit.Knowledge.RepositoryID, &hit.Knowledge.Branch,
		&hit.Knowledge.KnowledgeType, &hit.Knowledge.StableKey, &hit.Knowledge.Cardinality, &hit.Knowledge.Qualifier,
		&hit.Knowledge.Status, &hit.Knowledge.PrivacyClassification, &hit.Knowledge.ActiveRevisionID, &hit.Knowledge.Confidence, &hit.Knowledge.Importance,
		&hit.Knowledge.ValidFrom, &hit.Knowledge.ValidUntil, &hit.Knowledge.CreatedAt, &hit.Knowledge.UpdatedAt,
		&hit.Revision.RevisionID, &hit.Revision.RevisionNumber, &subjectJSON, &hit.Revision.Predicate, &objectJSON, &hit.Revision.Summary,
		&hit.Revision.SemanticFingerprint, &hit.Revision.Confidence, &hit.Revision.Importance, &hit.Revision.ValidFrom, &hit.Revision.ValidUntil, &hit.Revision.CreatedAt,
	)
	if err != nil {
		return CanonicalKnowledgeHit{}, err
	}
	hit.Revision.UserID = hit.Knowledge.UserID
	hit.Revision.KnowledgeID = hit.Knowledge.KnowledgeID
	if err := json.Unmarshal(subjectJSON, &hit.Revision.Subject); err != nil {
		return CanonicalKnowledgeHit{}, err
	}
	if err := json.Unmarshal(objectJSON, &hit.Revision.Object); err != nil {
		return CanonicalKnowledgeHit{}, err
	}
	return hit, nil
}

func canonicalRecallDatabaseLimit(limit int) int {
	value := limit * 4
	if value < 24 {
		value = 24
	}
	if value > canonicalRecallCandidateLimit {
		value = canonicalRecallCandidateLimit
	}
	return value
}

func (s *Store) RecallCanonicalKnowledge(ctx context.Context, input CanonicalKnowledgeRecallInput) ([]CanonicalKnowledgeHit, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("canonical knowledge store unavailable")
	}
	normalized, err := normalizeCanonicalRecallInput(input)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	rows, err := s.DB.Query(ctx, canonicalKnowledgeRecallSQL,
		normalized.UserID, normalized.ProjectID, normalized.RepositoryIDs, normalized.Branch, now, canonicalRecallDatabaseLimit(normalized.Limit))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	hits := []CanonicalKnowledgeHit{}
	for rows.Next() {
		hit, err := scanCanonicalKnowledgeHit(rows)
		if err != nil {
			return nil, err
		}
		hits = append(hits, hit)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return rankCanonicalKnowledgeHits(hits, normalized, now), nil
}

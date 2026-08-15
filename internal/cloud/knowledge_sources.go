package cloud

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"path"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const (
	KnowledgeStatusActive     = "active"
	KnowledgeStatusStale      = "stale"
	KnowledgeStatusConflicted = "conflicted"
	KnowledgeStatusSuperseded = "superseded"
	KnowledgeStatusRevoked    = "revoked"

	KnowledgeClassPublicProject  = "public_project"
	KnowledgeClassTeamProject    = "team_project"
	KnowledgeClassPrivateProject = "private_project"
	KnowledgeClassLocalPrivate   = "local_private"
	KnowledgeClassSensitive      = "sensitive"
)

var (
	ErrKnowledgeInvalidScope     = errors.New("invalid knowledge source scope")
	ErrKnowledgeSourceNotFound   = errors.New("knowledge source not found")
	ErrKnowledgeRevisionConflict = errors.New("knowledge source revision conflict")
)

type KnowledgeSource struct {
	UserID           string         `json:"userId"`
	SourceID         string         `json:"sourceId"`
	ProjectID        string         `json:"projectId"`
	RepositoryID     string         `json:"repositoryId,omitempty"`
	Provider         string         `json:"provider"`
	SourceType       string         `json:"sourceType"`
	CanonicalPath    string         `json:"canonicalPath"`
	Classification   string         `json:"classification"`
	Status           string         `json:"status"`
	ActiveRevisionID string         `json:"activeRevisionId,omitempty"`
	CreatedAt        int64          `json:"createdAt"`
	LastSeenAt       int64          `json:"lastSeenAt"`
	ValidFrom        int64          `json:"validFrom"`
	ValidTo          int64          `json:"validTo,omitempty"`
	Metadata         map[string]any `json:"metadata,omitempty"`
}

type KnowledgeSourceInput struct {
	UserID         string
	ProjectID      string
	RepositoryID   string
	Provider       string
	SourceType     string
	CanonicalPath  string
	Classification string
	Metadata       map[string]any
}

type KnowledgeSourceRevision struct {
	UserID                    string         `json:"userId"`
	RevisionID                string         `json:"revisionId"`
	SourceID                  string         `json:"sourceId"`
	ContentHash               string         `json:"contentHash,omitempty"`
	SemanticHash              string         `json:"semanticHash,omitempty"`
	ParserFingerprint         string         `json:"parserFingerprint"`
	AdapterVersion            string         `json:"adapterVersion"`
	ParserVersion             string         `json:"parserVersion"`
	SemanticNormalizerVersion string         `json:"semanticNormalizerVersion"`
	GitBlobOID                string         `json:"gitBlobOid,omitempty"`
	Tombstone                 bool           `json:"tombstone"`
	CreatedAt                 int64          `json:"createdAt"`
	Metadata                  map[string]any `json:"metadata,omitempty"`
}

type KnowledgeSourceObservation struct {
	UserID         string         `json:"userId"`
	ObservationID  string         `json:"observationId"`
	SourceID       string         `json:"sourceId"`
	RevisionID     string         `json:"revisionId"`
	BaseRevisionID string         `json:"baseRevisionId,omitempty"`
	DeviceID       string         `json:"deviceId,omitempty"`
	WorkspaceID    string         `json:"workspaceId,omitempty"`
	Branch         string         `json:"branch,omitempty"`
	GitCommit      string         `json:"gitCommit,omitempty"`
	FirstSeenAt    int64          `json:"firstSeenAt"`
	LastSeenAt     int64          `json:"lastSeenAt"`
	Metadata       map[string]any `json:"metadata,omitempty"`
}

type KnowledgeSourceObservationInput struct {
	DeviceID    string
	WorkspaceID string
	Branch      string
	GitCommit   string
	Metadata    map[string]any
}

type KnowledgeSourceRevisionInput struct {
	UserID                    string
	SourceID                  string
	BaseRevisionID            string
	ContentHash               string
	SemanticHash              string
	ParserFingerprint         string
	AdapterVersion            string
	ParserVersion             string
	SemanticNormalizerVersion string
	GitBlobOID                string
	Tombstone                 bool
	Metadata                  map[string]any
	Observation               KnowledgeSourceObservationInput
}

type knowledgeRowScanner interface {
	Scan(dest ...any) error
}

const knowledgeSourceSelectSQL = `
SELECT user_id,source_id,project_id,COALESCE(repository_id,''),provider,source_type,canonical_path,classification,status,
 COALESCE(active_revision_id,''),created_at,last_seen_at,valid_from,COALESCE(valid_to,0),metadata
FROM codelocal_knowledge_sources
WHERE user_id=$1 AND source_id=$2`

const knowledgeSourceListSQL = `
SELECT user_id,source_id,project_id,COALESCE(repository_id,''),provider,source_type,canonical_path,classification,status,
 COALESCE(active_revision_id,''),created_at,last_seen_at,valid_from,COALESCE(valid_to,0),metadata
FROM codelocal_knowledge_sources
WHERE user_id=$1 AND project_id=$2 AND ($3='' OR repository_id=$3)
ORDER BY last_seen_at DESC,source_id ASC
LIMIT $4`

const knowledgeRevisionSelectSQL = `
SELECT user_id,revision_id,source_id,COALESCE(content_hash,''),COALESCE(semantic_hash,''),parser_fingerprint,
 adapter_version,parser_version,semantic_normalizer_version,COALESCE(git_blob_oid,''),tombstone,created_at,metadata
FROM codelocal_knowledge_source_revisions
WHERE user_id=$1 AND revision_id=$2`

const knowledgeRevisionListSQL = `
SELECT user_id,revision_id,source_id,COALESCE(content_hash,''),COALESCE(semantic_hash,''),parser_fingerprint,
 adapter_version,parser_version,semantic_normalizer_version,COALESCE(git_blob_oid,''),tombstone,created_at,metadata
FROM codelocal_knowledge_source_revisions
WHERE user_id=$1 AND source_id=$2
ORDER BY created_at DESC,revision_id DESC
LIMIT $3`

const knowledgeObservationListSQL = `
SELECT user_id,observation_id,source_id,revision_id,COALESCE(base_revision_id,''),COALESCE(device_id,''),COALESCE(workspace_id,''),
 COALESCE(branch,''),COALESCE(git_commit,''),first_seen_at,last_seen_at,metadata
FROM codelocal_knowledge_source_observations
WHERE user_id=$1 AND source_id=$2
ORDER BY last_seen_at DESC,observation_id DESC
LIMIT $3`

func normalizeKnowledgeToken(value string, max int) string {
	value = strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(value)), "-"))
	if len(value) > max {
		value = value[:max]
	}
	return value
}

func normalizeKnowledgeCanonicalPath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || value == "." {
		return "."
	}
	if len(value) >= 2 && value[1] == ':' && ((value[0] >= 'a' && value[0] <= 'z') || (value[0] >= 'A' && value[0] <= 'Z')) {
		return ""
	}
	value = path.Clean(value)
	if value == "." {
		return "."
	}
	if strings.HasPrefix(value, "/") || value == ".." || strings.HasPrefix(value, "../") || len(value) > 1000 {
		return ""
	}
	return value
}

func validKnowledgeClassification(value string) bool {
	switch value {
	case KnowledgeClassPublicProject, KnowledgeClassTeamProject, KnowledgeClassPrivateProject, KnowledgeClassLocalPrivate, KnowledgeClassSensitive:
		return true
	default:
		return false
	}
}

func validKnowledgeStatus(value string) bool {
	switch value {
	case KnowledgeStatusActive, KnowledgeStatusStale, KnowledgeStatusConflicted, KnowledgeStatusSuperseded, KnowledgeStatusRevoked:
		return true
	default:
		return false
	}
}

func knowledgeStatusAfterRevision(currentStatus string, tombstone bool) string {
	switch strings.TrimSpace(currentStatus) {
	case KnowledgeStatusRevoked:
		return KnowledgeStatusRevoked
	case KnowledgeStatusConflicted:
		return KnowledgeStatusConflicted
	}
	if tombstone {
		return KnowledgeStatusSuperseded
	}
	return KnowledgeStatusActive
}

func normalizeKnowledgeSourceInput(in KnowledgeSourceInput) (KnowledgeSourceInput, error) {
	in.UserID = strings.TrimSpace(in.UserID)
	in.ProjectID = strings.TrimSpace(in.ProjectID)
	in.RepositoryID = strings.TrimSpace(in.RepositoryID)
	in.Provider = normalizeKnowledgeToken(in.Provider, 80)
	in.SourceType = normalizeKnowledgeToken(in.SourceType, 80)
	in.CanonicalPath = normalizeKnowledgeCanonicalPath(in.CanonicalPath)
	in.Classification = strings.ToLower(strings.TrimSpace(in.Classification))
	if in.Classification == "" {
		in.Classification = KnowledgeClassPrivateProject
	}
	if in.UserID == "" || in.ProjectID == "" || in.Provider == "" || in.SourceType == "" || in.CanonicalPath == "" || !validKnowledgeClassification(in.Classification) {
		return KnowledgeSourceInput{}, ErrKnowledgeInvalidScope
	}
	return in, nil
}

func normalizeKnowledgeRevisionInput(in KnowledgeSourceRevisionInput) (KnowledgeSourceRevisionInput, error) {
	in.UserID = strings.TrimSpace(in.UserID)
	in.SourceID = strings.TrimSpace(in.SourceID)
	in.BaseRevisionID = strings.TrimSpace(in.BaseRevisionID)
	in.ContentHash = strings.ToLower(strings.TrimSpace(in.ContentHash))
	in.SemanticHash = strings.ToLower(strings.TrimSpace(in.SemanticHash))
	in.ParserFingerprint = strings.ToLower(strings.TrimSpace(in.ParserFingerprint))
	in.AdapterVersion = strings.TrimSpace(in.AdapterVersion)
	in.ParserVersion = strings.TrimSpace(in.ParserVersion)
	in.SemanticNormalizerVersion = strings.TrimSpace(in.SemanticNormalizerVersion)
	in.GitBlobOID = strings.TrimSpace(in.GitBlobOID)
	in.Observation.DeviceID = strings.TrimSpace(in.Observation.DeviceID)
	in.Observation.WorkspaceID = strings.TrimSpace(in.Observation.WorkspaceID)
	in.Observation.Branch = strings.TrimSpace(in.Observation.Branch)
	in.Observation.GitCommit = strings.TrimSpace(in.Observation.GitCommit)
	if in.UserID == "" || in.SourceID == "" || in.ParserFingerprint == "" || in.AdapterVersion == "" || in.ParserVersion == "" || in.SemanticNormalizerVersion == "" {
		return KnowledgeSourceRevisionInput{}, ErrKnowledgeInvalidScope
	}
	if !in.Tombstone && in.ContentHash == "" {
		return KnowledgeSourceRevisionInput{}, ErrKnowledgeInvalidScope
	}
	if in.Tombstone {
		in.ContentHash = ""
		in.SemanticHash = ""
	}
	return in, nil
}

func knowledgeStableID(prefix string, parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return prefix + hex.EncodeToString(h.Sum(nil)[:16])
}

func KnowledgeSourceID(in KnowledgeSourceInput) (string, error) {
	normalized, err := normalizeKnowledgeSourceInput(in)
	if err != nil {
		return "", err
	}
	return knowledgeStableID("ksrc_", normalized.UserID, normalized.ProjectID, normalized.RepositoryID, normalized.Provider, normalized.SourceType, normalized.CanonicalPath), nil
}

func KnowledgeRevisionID(in KnowledgeSourceRevisionInput) (string, error) {
	normalized, err := normalizeKnowledgeRevisionInput(in)
	if err != nil {
		return "", err
	}
	tombstone := "0"
	if normalized.Tombstone {
		tombstone = "1"
	}
	return knowledgeStableID("krev_", normalized.UserID, normalized.SourceID, normalized.ContentHash, normalized.SemanticHash, normalized.ParserFingerprint, normalized.AdapterVersion, normalized.ParserVersion, normalized.SemanticNormalizerVersion, tombstone), nil
}

func KnowledgeObservationID(userID, sourceID, revisionID, baseRevisionID string, in KnowledgeSourceObservationInput) string {
	return knowledgeStableID("kobs_", userID, sourceID, revisionID, baseRevisionID, in.DeviceID, in.WorkspaceID, in.Branch, in.GitCommit)
}

func knowledgeMetadata(value map[string]any) []byte {
	if value == nil {
		return []byte(`{}`)
	}
	raw, err := json.Marshal(value)
	if err != nil {
		return []byte(`{}`)
	}
	return raw
}

func scanKnowledgeSource(row knowledgeRowScanner) (*KnowledgeSource, error) {
	var out KnowledgeSource
	var metadata []byte
	if err := row.Scan(&out.UserID, &out.SourceID, &out.ProjectID, &out.RepositoryID, &out.Provider, &out.SourceType, &out.CanonicalPath, &out.Classification, &out.Status, &out.ActiveRevisionID, &out.CreatedAt, &out.LastSeenAt, &out.ValidFrom, &out.ValidTo, &metadata); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(metadata, &out.Metadata)
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	return &out, nil
}

func scanKnowledgeRevision(row knowledgeRowScanner) (*KnowledgeSourceRevision, error) {
	var out KnowledgeSourceRevision
	var metadata []byte
	if err := row.Scan(&out.UserID, &out.RevisionID, &out.SourceID, &out.ContentHash, &out.SemanticHash, &out.ParserFingerprint, &out.AdapterVersion, &out.ParserVersion, &out.SemanticNormalizerVersion, &out.GitBlobOID, &out.Tombstone, &out.CreatedAt, &metadata); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(metadata, &out.Metadata)
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	return &out, nil
}

func scanKnowledgeObservation(row knowledgeRowScanner) (*KnowledgeSourceObservation, error) {
	var out KnowledgeSourceObservation
	var metadata []byte
	if err := row.Scan(&out.UserID, &out.ObservationID, &out.SourceID, &out.RevisionID, &out.BaseRevisionID, &out.DeviceID, &out.WorkspaceID, &out.Branch, &out.GitCommit, &out.FirstSeenAt, &out.LastSeenAt, &metadata); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	_ = json.Unmarshal(metadata, &out.Metadata)
	if out.Metadata == nil {
		out.Metadata = map[string]any{}
	}
	return &out, nil
}

func (s *Store) UpsertKnowledgeSource(ctx context.Context, input KnowledgeSourceInput) (*KnowledgeSource, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("knowledge store unavailable")
	}
	normalized, err := normalizeKnowledgeSourceInput(input)
	if err != nil {
		return nil, err
	}
	sourceID, _ := KnowledgeSourceID(normalized)
	now := time.Now().UnixMilli()
	row := s.DB.QueryRow(ctx, `
INSERT INTO codelocal_knowledge_sources(
 user_id,source_id,project_id,repository_id,provider,source_type,canonical_path,classification,status,created_at,last_seen_at,valid_from,metadata)
SELECT $1,$2,$3,NULLIF($4,''),$5,$6,$7,$8,$9,$10,$10,$10,$11
WHERE EXISTS (SELECT 1 FROM codelocal_projects WHERE user_id=$1 AND project_id=$3)
 AND ($4='' OR EXISTS (
   SELECT 1 FROM codelocal_project_repositories
   WHERE user_id=$1 AND project_id=$3 AND repository_id=$4
 ))
ON CONFLICT(user_id,source_id) DO UPDATE SET
 last_seen_at=EXCLUDED.last_seen_at,
 classification=CASE
  WHEN EXCLUDED.classification='sensitive' THEN 'sensitive'
  WHEN codelocal_knowledge_sources.classification='sensitive' THEN codelocal_knowledge_sources.classification
  WHEN EXCLUDED.classification='local_private' THEN 'local_private'
  WHEN codelocal_knowledge_sources.classification='local_private' THEN codelocal_knowledge_sources.classification
  WHEN EXCLUDED.classification='private_project' THEN 'private_project'
  WHEN codelocal_knowledge_sources.classification='private_project' THEN codelocal_knowledge_sources.classification
  WHEN EXCLUDED.classification='team_project' THEN 'team_project'
  ELSE codelocal_knowledge_sources.classification
 END,
 metadata=EXCLUDED.metadata
RETURNING user_id,source_id,project_id,COALESCE(repository_id,''),provider,source_type,canonical_path,classification,status,
 COALESCE(active_revision_id,''),created_at,last_seen_at,valid_from,COALESCE(valid_to,0),metadata`,
		normalized.UserID, sourceID, normalized.ProjectID, normalized.RepositoryID, normalized.Provider, normalized.SourceType, normalized.CanonicalPath, normalized.Classification, KnowledgeStatusActive, now, knowledgeMetadata(normalized.Metadata))
	out, err := scanKnowledgeSource(row)
	if err != nil {
		return nil, err
	}
	if out == nil {
		return nil, ErrKnowledgeSourceNotFound
	}
	return out, nil
}

func (s *Store) KnowledgeSource(ctx context.Context, userID, sourceID string) (*KnowledgeSource, error) {
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(sourceID) == "" {
		return nil, ErrKnowledgeInvalidScope
	}
	return scanKnowledgeSource(s.DB.QueryRow(ctx, knowledgeSourceSelectSQL, strings.TrimSpace(userID), strings.TrimSpace(sourceID)))
}

func (s *Store) ListKnowledgeSources(ctx context.Context, userID, projectID, repositoryID string, limit int) ([]KnowledgeSource, error) {
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(projectID) == "" {
		return nil, ErrKnowledgeInvalidScope
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.DB.Query(ctx, knowledgeSourceListSQL, strings.TrimSpace(userID), strings.TrimSpace(projectID), strings.TrimSpace(repositoryID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeSource{}
	for rows.Next() {
		item, err := scanKnowledgeSource(rows)
		if err != nil {
			return nil, err
		}
		if item != nil {
			out = append(out, *item)
		}
	}
	return out, rows.Err()
}

func knowledgeRevisionConflict(currentRevisionID, baseRevisionID string) bool {
	currentRevisionID = strings.TrimSpace(currentRevisionID)
	baseRevisionID = strings.TrimSpace(baseRevisionID)
	if currentRevisionID == "" {
		return baseRevisionID != ""
	}
	return baseRevisionID == "" || currentRevisionID != baseRevisionID
}

func upsertKnowledgeObservation(ctx context.Context, tx pgx.Tx, userID, sourceID, revisionID, baseRevisionID string, in KnowledgeSourceObservationInput, now int64) error {
	observationID := KnowledgeObservationID(userID, sourceID, revisionID, baseRevisionID, in)
	_, err := tx.Exec(ctx, `
INSERT INTO codelocal_knowledge_source_observations(
 user_id,observation_id,source_id,revision_id,base_revision_id,device_id,workspace_id,branch,git_commit,first_seen_at,last_seen_at,metadata)
VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,$10,$11)
ON CONFLICT(user_id,observation_id) DO UPDATE SET
 last_seen_at=EXCLUDED.last_seen_at,
 metadata=EXCLUDED.metadata`, userID, observationID, sourceID, revisionID, baseRevisionID, strings.TrimSpace(in.DeviceID), strings.TrimSpace(in.WorkspaceID), strings.TrimSpace(in.Branch), strings.TrimSpace(in.GitCommit), now, knowledgeMetadata(in.Metadata))
	return err
}

func (s *Store) AppendKnowledgeSourceRevision(ctx context.Context, input KnowledgeSourceRevisionInput) (*KnowledgeSourceRevision, error) {
	if s == nil || s.DB == nil {
		return nil, errors.New("knowledge store unavailable")
	}
	normalized, err := normalizeKnowledgeRevisionInput(input)
	if err != nil {
		return nil, err
	}
	revisionID, _ := KnowledgeRevisionID(normalized)
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)

	var currentRevisionID, currentStatus string
	err = tx.QueryRow(ctx, `
SELECT COALESCE(active_revision_id,''),status
FROM codelocal_knowledge_sources
WHERE user_id=$1 AND source_id=$2
FOR UPDATE`, normalized.UserID, normalized.SourceID).Scan(&currentRevisionID, &currentStatus)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, ErrKnowledgeSourceNotFound
	}
	if err != nil {
		return nil, err
	}

	existing, err := scanKnowledgeRevision(tx.QueryRow(ctx, knowledgeRevisionSelectSQL, normalized.UserID, revisionID))
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	if currentRevisionID == revisionID && existing != nil {
		if err := upsertKnowledgeObservation(ctx, tx, normalized.UserID, normalized.SourceID, revisionID, normalized.BaseRevisionID, normalized.Observation, now); err != nil {
			return nil, err
		}
		if err := tx.Commit(ctx); err != nil {
			return nil, err
		}
		return existing, nil
	}
	if knowledgeRevisionConflict(currentRevisionID, normalized.BaseRevisionID) {
		return nil, fmt.Errorf("%w: current=%s base=%s", ErrKnowledgeRevisionConflict, currentRevisionID, normalized.BaseRevisionID)
	}

	if existing == nil {
		_, err = tx.Exec(ctx, `
INSERT INTO codelocal_knowledge_source_revisions(
 user_id,revision_id,source_id,content_hash,semantic_hash,parser_fingerprint,adapter_version,parser_version,
 semantic_normalizer_version,git_blob_oid,tombstone,created_at,metadata)
VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,NULLIF($10,''),$11,$12,$13)`,
			normalized.UserID, revisionID, normalized.SourceID, normalized.ContentHash, normalized.SemanticHash, normalized.ParserFingerprint,
			normalized.AdapterVersion, normalized.ParserVersion, normalized.SemanticNormalizerVersion, normalized.GitBlobOID,
			normalized.Tombstone, now, knowledgeMetadata(normalized.Metadata))
		if err != nil {
			return nil, err
		}
	}

	status := knowledgeStatusAfterRevision(currentStatus, normalized.Tombstone)
	var validTo any
	if normalized.Tombstone || status == KnowledgeStatusRevoked {
		validTo = now
	}
	// Revoke is explicit user policy, not content state. New observations and
	// revisions may continue to advance for provenance, but only an explicit
	// revalidation may make the source eligible again.
	if _, err := tx.Exec(ctx, `
UPDATE codelocal_knowledge_sources
SET active_revision_id=$3,status=$4,last_seen_at=$5,valid_to=$6
WHERE user_id=$1 AND source_id=$2`, normalized.UserID, normalized.SourceID, revisionID, status, now, validTo); err != nil {
		return nil, err
	}
	if err := upsertKnowledgeObservation(ctx, tx, normalized.UserID, normalized.SourceID, revisionID, normalized.BaseRevisionID, normalized.Observation, now); err != nil {
		return nil, err
	}
	out, err := scanKnowledgeRevision(tx.QueryRow(ctx, knowledgeRevisionSelectSQL, normalized.UserID, revisionID))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) ListKnowledgeSourceRevisions(ctx context.Context, userID, sourceID string, limit int) ([]KnowledgeSourceRevision, error) {
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(sourceID) == "" {
		return nil, ErrKnowledgeInvalidScope
	}
	if limit <= 0 {
		limit = 50
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.DB.Query(ctx, knowledgeRevisionListSQL, strings.TrimSpace(userID), strings.TrimSpace(sourceID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeSourceRevision{}
	for rows.Next() {
		item, err := scanKnowledgeRevision(rows)
		if err != nil {
			return nil, err
		}
		if item != nil {
			out = append(out, *item)
		}
	}
	return out, rows.Err()
}

func (s *Store) ListKnowledgeSourceObservations(ctx context.Context, userID, sourceID string, limit int) ([]KnowledgeSourceObservation, error) {
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" || strings.TrimSpace(sourceID) == "" {
		return nil, ErrKnowledgeInvalidScope
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 1000 {
		limit = 1000
	}
	rows, err := s.DB.Query(ctx, knowledgeObservationListSQL, strings.TrimSpace(userID), strings.TrimSpace(sourceID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeSourceObservation{}
	for rows.Next() {
		item, err := scanKnowledgeObservation(rows)
		if err != nil {
			return nil, err
		}
		if item != nil {
			out = append(out, *item)
		}
	}
	return out, rows.Err()
}

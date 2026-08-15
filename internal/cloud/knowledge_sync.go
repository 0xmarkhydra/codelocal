package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
	"github.com/jackc/pgx/v5"
)

type KnowledgeConflict struct {
	UserID              string         `json:"userId"`
	ConflictID          string         `json:"conflictId"`
	SourceID            string         `json:"sourceId"`
	ActiveRevisionID    string         `json:"activeRevisionId"`
	CandidateRevisionID string         `json:"candidateRevisionId"`
	Branch              string         `json:"branch,omitempty"`
	Status              string         `json:"status"`
	CreatedAt           int64          `json:"createdAt"`
	UpdatedAt           int64          `json:"updatedAt"`
	ResolvedAt          int64          `json:"resolvedAt,omitempty"`
	Metadata            map[string]any `json:"metadata,omitempty"`
}

type KnowledgeManifestSyncResult struct {
	RootHash        string            `json:"rootHash"`
	ActiveRevisions map[string]string `json:"activeRevisions"`
	Synced          int               `json:"synced"`
	Conflicts       int               `json:"conflicts"`
	Tombstones      int               `json:"tombstones"`
	Disabled        bool              `json:"disabled,omitempty"`
}

type workspaceKnowledgeObservation struct {
	SourceID          string
	Provider          string
	SourceType        string
	CanonicalPath     string
	ActiveRevisionID  string
	ObservedRevision  string
	ParserFingerprint string
	AdapterVersion    string
	ParserVersion     string
	NormalizerVersion string
}

func knowledgeConflictID(userID, sourceID, branch, activeRevisionID, candidateRevisionID string) string {
	return knowledgeStableID("kconf_", userID, sourceID, strings.TrimSpace(branch), activeRevisionID, candidateRevisionID)
}

func knowledgeAdvanceAllowed(activeRevisionID, candidateRevisionID, baseRevisionID string) bool {
	activeRevisionID = strings.TrimSpace(activeRevisionID)
	candidateRevisionID = strings.TrimSpace(candidateRevisionID)
	baseRevisionID = strings.TrimSpace(baseRevisionID)
	return activeRevisionID == "" || activeRevisionID == candidateRevisionID || activeRevisionID == baseRevisionID
}

func (s *Store) recordKnowledgeConflictCandidate(ctx context.Context, input KnowledgeSourceRevisionInput, activeRevisionID string) (*KnowledgeSourceRevision, error) {
	normalized, err := normalizeKnowledgeRevisionInput(input)
	if err != nil {
		return nil, err
	}
	candidateRevisionID, err := KnowledgeRevisionID(normalized)
	if err != nil {
		return nil, err
	}
	if candidateRevisionID == strings.TrimSpace(activeRevisionID) {
		return s.AppendKnowledgeSourceRevision(ctx, normalized)
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	now := time.Now().UnixMilli()
	existing, err := scanKnowledgeRevision(tx.QueryRow(ctx, knowledgeRevisionSelectSQL, normalized.UserID, candidateRevisionID))
	if err != nil {
		return nil, err
	}
	if existing == nil {
		_, err = tx.Exec(ctx, `
INSERT INTO codelocal_knowledge_source_revisions(
 user_id,revision_id,source_id,content_hash,semantic_hash,parser_fingerprint,adapter_version,parser_version,
 semantic_normalizer_version,git_blob_oid,tombstone,created_at,metadata)
VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,NULLIF($10,''),$11,$12,$13)`,
			normalized.UserID, candidateRevisionID, normalized.SourceID, normalized.ContentHash, normalized.SemanticHash,
			normalized.ParserFingerprint, normalized.AdapterVersion, normalized.ParserVersion, normalized.SemanticNormalizerVersion,
			normalized.GitBlobOID, normalized.Tombstone, now, knowledgeMetadata(normalized.Metadata))
		if err != nil {
			return nil, err
		}
	}
	if err := upsertKnowledgeObservation(ctx, tx, normalized.UserID, normalized.SourceID, candidateRevisionID, normalized.BaseRevisionID, normalized.Observation, now); err != nil {
		return nil, err
	}
	branch := strings.TrimSpace(normalized.Observation.Branch)
	conflictID := knowledgeConflictID(normalized.UserID, normalized.SourceID, branch, activeRevisionID, candidateRevisionID)
	_, err = tx.Exec(ctx, `
INSERT INTO codelocal_knowledge_conflicts(
 user_id,conflict_id,source_id,active_revision_id,candidate_revision_id,branch,status,created_at,updated_at,metadata)
VALUES($1,$2,$3,$4,$5,NULLIF($6,''),'open',$7,$7,$8)
ON CONFLICT(user_id,conflict_id) DO UPDATE SET updated_at=EXCLUDED.updated_at,metadata=EXCLUDED.metadata,branch=EXCLUDED.branch`,
		normalized.UserID, conflictID, normalized.SourceID, activeRevisionID, candidateRevisionID, branch, now,
		knowledgeMetadata(map[string]any{"baseRevisionId": normalized.BaseRevisionID, "deviceId": normalized.Observation.DeviceID, "workspaceId": normalized.Observation.WorkspaceID, "branch": branch}))
	if err != nil {
		return nil, err
	}
	if _, err := tx.Exec(ctx, `
UPDATE codelocal_knowledge_sources
SET status=CASE WHEN status='revoked' THEN status ELSE $3 END,last_seen_at=$4
WHERE user_id=$1 AND source_id=$2`, normalized.UserID, normalized.SourceID, KnowledgeStatusConflicted, now); err != nil {
		return nil, err
	}
	out, err := scanKnowledgeRevision(tx.QueryRow(ctx, knowledgeRevisionSelectSQL, normalized.UserID, candidateRevisionID))
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) workspaceKnowledgeObservations(ctx context.Context, userID, projectID, deviceID, workspaceID string) ([]workspaceKnowledgeObservation, error) {
	rows, err := s.DB.Query(ctx, `
SELECT DISTINCT ON (src.source_id)
 src.source_id,src.provider,src.source_type,src.canonical_path,COALESCE(src.active_revision_id,''),obs.revision_id,
 rev.parser_fingerprint,rev.adapter_version,rev.parser_version,rev.semantic_normalizer_version
FROM codelocal_knowledge_sources src
JOIN codelocal_knowledge_source_observations obs
 ON obs.user_id=src.user_id AND obs.source_id=src.source_id
JOIN codelocal_knowledge_source_revisions rev
 ON rev.user_id=obs.user_id AND rev.revision_id=obs.revision_id
WHERE src.user_id=$1 AND src.project_id=$2 AND obs.device_id=$3 AND obs.workspace_id=$4
ORDER BY src.source_id,obs.last_seen_at DESC,obs.observation_id DESC`, userID, projectID, deviceID, workspaceID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []workspaceKnowledgeObservation{}
	for rows.Next() {
		var item workspaceKnowledgeObservation
		if err := rows.Scan(&item.SourceID, &item.Provider, &item.SourceType, &item.CanonicalPath, &item.ActiveRevisionID, &item.ObservedRevision, &item.ParserFingerprint, &item.AdapterVersion, &item.ParserVersion, &item.NormalizerVersion); err != nil {
			return nil, err
		}
		out = append(out, item)
	}
	return out, rows.Err()
}

func (s *Store) SyncKnowledgeManifest(ctx context.Context, userID, deviceID, workspaceID, projectID string, repositories []projectidentity.Repository, manifest projectbrain.Manifest) (KnowledgeManifestSyncResult, error) {
	result := KnowledgeManifestSyncResult{RootHash: strings.TrimSpace(manifest.RootHash), ActiveRevisions: map[string]string{}}
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	workspaceID = strings.TrimSpace(workspaceID)
	projectID = strings.TrimSpace(projectID)
	if s == nil || s.DB == nil || userID == "" || deviceID == "" || workspaceID == "" || projectID == "" || result.RootHash == "" {
		return result, ErrKnowledgeInvalidScope
	}
	canonicalManifest := projectbrain.NewManifest(manifest.Sources)
	if canonicalManifest.RootHash != result.RootHash {
		return result, fmt.Errorf("%w: knowledge manifest root mismatch", ErrKnowledgeInvalidScope)
	}
	manifest.Sources = canonicalManifest.Sources
	if len(manifest.Sources) > 512 {
		return result, fmt.Errorf("%w: too many knowledge sources", ErrKnowledgeInvalidScope)
	}
	for _, source := range manifest.Sources {
		if !projectbrain.CloudSafeSource(source) {
			return result, fmt.Errorf("%w: local-private knowledge cannot sync to cloud", ErrKnowledgeInvalidScope)
		}
	}
	observedBefore, err := s.workspaceKnowledgeObservations(ctx, userID, projectID, deviceID, workspaceID)
	if err != nil {
		return result, err
	}
	incomingCanonical := make(map[string]struct{}, len(manifest.Sources))
	for _, source := range manifest.Sources {
		clientKey := projectbrain.SourceIdentityKey(source)
		repositoryID, canonicalPath := projectbrain.RepositoryScopeForPath(source.Path, repositories)
		if canonicalPath == "" {
			return result, ErrKnowledgeInvalidScope
		}
		canonicalKey := strings.Join([]string{canonicalPath, source.Provider, source.SourceType}, "\x00")
		incomingCanonical[canonicalKey] = struct{}{}
		stored, err := s.UpsertKnowledgeSource(ctx, KnowledgeSourceInput{
			UserID: userID, ProjectID: projectID, RepositoryID: repositoryID,
			Provider: source.Provider, SourceType: source.SourceType, CanonicalPath: canonicalPath, Classification: source.Classification,
			Metadata: map[string]any{"scopePath": source.ScopePath, "workspacePath": source.Path, "size": source.Size, "manifestRoot": result.RootHash},
		})
		if err != nil {
			return result, err
		}
		revisionInput := KnowledgeSourceRevisionInput{
			UserID: userID, SourceID: stored.SourceID, BaseRevisionID: source.BaseRevisionID,
			ContentHash: source.ContentHash, ParserFingerprint: source.ParserFingerprint,
			AdapterVersion: source.AdapterVersion, ParserVersion: source.ParserVersion, SemanticNormalizerVersion: source.SemanticNormalizerVersion,
			Metadata:    map[string]any{"scopePath": source.ScopePath, "size": source.Size},
			Observation: KnowledgeSourceObservationInput{DeviceID: deviceID, WorkspaceID: workspaceID, Metadata: map[string]any{"manifestRoot": result.RootHash}},
		}
		candidateID, err := KnowledgeRevisionID(revisionInput)
		if err != nil {
			return result, err
		}
		if knowledgeAdvanceAllowed(stored.ActiveRevisionID, candidateID, source.BaseRevisionID) {
			if stored.ActiveRevisionID != "" && stored.ActiveRevisionID == candidateID {
				revisionInput.BaseRevisionID = stored.ActiveRevisionID
			}
			if _, err := s.AppendKnowledgeSourceRevision(ctx, revisionInput); err != nil {
				return result, err
			}
			result.ActiveRevisions[clientKey] = candidateID
			result.Synced++
			continue
		}
		if _, err := s.recordKnowledgeConflictCandidate(ctx, revisionInput, stored.ActiveRevisionID); err != nil {
			return result, err
		}
		result.ActiveRevisions[clientKey] = stored.ActiveRevisionID
		result.Conflicts++
	}

	for _, previous := range observedBefore {
		key := strings.Join([]string{previous.CanonicalPath, previous.Provider, previous.SourceType}, "\x00")
		if _, exists := incomingCanonical[key]; exists || previous.ActiveRevisionID == "" || previous.ActiveRevisionID != previous.ObservedRevision {
			continue
		}
		tombstone := KnowledgeSourceRevisionInput{
			UserID: userID, SourceID: previous.SourceID, BaseRevisionID: previous.ActiveRevisionID,
			ParserFingerprint: previous.ParserFingerprint, AdapterVersion: previous.AdapterVersion,
			ParserVersion: previous.ParserVersion, SemanticNormalizerVersion: previous.NormalizerVersion, Tombstone: true,
			Metadata:    map[string]any{"reason": "manifest_source_removed"},
			Observation: KnowledgeSourceObservationInput{DeviceID: deviceID, WorkspaceID: workspaceID, Metadata: map[string]any{"manifestRoot": result.RootHash}},
		}
		if _, err := s.AppendKnowledgeSourceRevision(ctx, tombstone); err != nil {
			if errors.Is(err, ErrKnowledgeRevisionConflict) {
				continue
			}
			return result, err
		}
		result.Tombstones++
	}
	return result, nil
}

func (s *Store) ListKnowledgeConflicts(ctx context.Context, userID, projectID string, limit int) ([]KnowledgeConflict, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if s == nil || s.DB == nil || userID == "" || projectID == "" {
		return nil, ErrKnowledgeInvalidScope
	}
	if limit <= 0 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	rows, err := s.DB.Query(ctx, `
SELECT c.user_id,c.conflict_id,c.source_id,c.active_revision_id,c.candidate_revision_id,COALESCE(c.branch,''),c.status,c.created_at,c.updated_at,COALESCE(c.resolved_at,0),c.metadata
FROM codelocal_knowledge_conflicts c
JOIN codelocal_knowledge_sources s ON s.user_id=c.user_id AND s.source_id=c.source_id
WHERE c.user_id=$1 AND s.project_id=$2
ORDER BY CASE WHEN c.status='open' THEN 0 ELSE 1 END,c.updated_at DESC
LIMIT $3`, userID, projectID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []KnowledgeConflict{}
	for rows.Next() {
		var item KnowledgeConflict
		var metadata []byte
		if err := rows.Scan(&item.UserID, &item.ConflictID, &item.SourceID, &item.ActiveRevisionID, &item.CandidateRevisionID, &item.Branch, &item.Status, &item.CreatedAt, &item.UpdatedAt, &item.ResolvedAt, &metadata); err != nil {
			return nil, err
		}
		_ = jsonUnmarshalMetadata(metadata, &item.Metadata)
		out = append(out, item)
	}
	return out, rows.Err()
}

func jsonUnmarshalMetadata(raw []byte, target *map[string]any) error {
	if target == nil {
		return nil
	}
	if len(raw) == 0 {
		*target = map[string]any{}
		return nil
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return err
	}
	if value == nil {
		value = map[string]any{}
	}
	*target = value
	return nil
}

func (s *Store) ResolveKnowledgeConflict(ctx context.Context, userID, conflictID, winnerRevisionID string) error {
	userID = strings.TrimSpace(userID)
	conflictID = strings.TrimSpace(conflictID)
	winnerRevisionID = strings.TrimSpace(winnerRevisionID)
	if s == nil || s.DB == nil || userID == "" || conflictID == "" || winnerRevisionID == "" {
		return ErrKnowledgeInvalidScope
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var sourceID, activeRevisionID, candidateRevisionID, status, branch string
	if err := tx.QueryRow(ctx, `
SELECT source_id,active_revision_id,candidate_revision_id,status,COALESCE(branch,'')
FROM codelocal_knowledge_conflicts
WHERE user_id=$1 AND conflict_id=$2
FOR UPDATE`, userID, conflictID).Scan(&sourceID, &activeRevisionID, &candidateRevisionID, &status, &branch); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrKnowledgeSourceNotFound
		}
		return err
	}
	if status != "open" {
		return nil
	}
	if winnerRevisionID != activeRevisionID && winnerRevisionID != candidateRevisionID {
		return fmt.Errorf("winner revision is not part of conflict")
	}
	now := time.Now().UnixMilli()
	if branch != "" {
		if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_knowledge_branch_heads(user_id,source_id,branch,revision_id,updated_at)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(user_id,source_id,branch) DO UPDATE SET revision_id=EXCLUDED.revision_id,updated_at=EXCLUDED.updated_at`, userID, sourceID, branch, winnerRevisionID, now); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(ctx, `UPDATE codelocal_knowledge_conflicts SET status='resolved',resolved_at=$3,updated_at=$3,metadata=metadata || jsonb_build_object('winnerRevisionId',$4) WHERE user_id=$1 AND conflict_id=$2`, userID, conflictID, now, winnerRevisionID); err != nil {
		return err
	}

	var currentGlobal, currentStatus string
	if err := tx.QueryRow(ctx, `SELECT COALESCE(active_revision_id,''),status FROM codelocal_knowledge_sources WHERE user_id=$1 AND source_id=$2 FOR UPDATE`, userID, sourceID).Scan(&currentGlobal, &currentStatus); err != nil {
		return err
	}
	globalWinner := currentGlobal
	if branch == "" || currentGlobal == activeRevisionID {
		globalWinner = winnerRevisionID
	}
	var globalTombstone bool
	if globalWinner != "" {
		if err := tx.QueryRow(ctx, `SELECT tombstone FROM codelocal_knowledge_source_revisions WHERE user_id=$1 AND revision_id=$2 AND source_id=$3`, userID, globalWinner, sourceID).Scan(&globalTombstone); err != nil {
			return err
		}
	}
	var remainingOpen int
	if err := tx.QueryRow(ctx, `SELECT COUNT(*)::int FROM codelocal_knowledge_conflicts WHERE user_id=$1 AND source_id=$2 AND status='open'`, userID, sourceID).Scan(&remainingOpen); err != nil {
		return err
	}
	newStatus := KnowledgeStatusActive
	var validTo any
	if globalTombstone {
		newStatus = KnowledgeStatusSuperseded
		validTo = now
	}
	if remainingOpen > 0 {
		newStatus = KnowledgeStatusConflicted
	}
	if currentStatus == KnowledgeStatusRevoked {
		newStatus = KnowledgeStatusRevoked
	}
	if _, err := tx.Exec(ctx, `UPDATE codelocal_knowledge_sources
SET active_revision_id=$3,
    status=$4,
    last_seen_at=$5,
    valid_to=CASE WHEN status='revoked' THEN valid_to ELSE $6 END
WHERE user_id=$1 AND source_id=$2`, userID, sourceID, globalWinner, newStatus, now, validTo); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

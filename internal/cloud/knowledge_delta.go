package cloud

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
	"github.com/jackc/pgx/v5"
)

const maxKnowledgeDeltaItems = 128

type deltaPreparedSource struct {
	Source        projectbrain.Source
	ClientKey     string
	SourceID      string
	RepositoryID  string
	CanonicalPath string
	SourceInput   KnowledgeSourceInput
	IsRemoval     bool
}

type deltaSourceState struct {
	ActiveRevisionID string
	Status           string
}

type deltaRevisionMeta struct {
	SourceID                  string
	ParserFingerprint         string
	AdapterVersion            string
	ParserVersion             string
	SemanticNormalizerVersion string
	Tombstone                 bool
}

type deltaMutation struct {
	Prepared     deltaPreparedSource
	Revision     KnowledgeSourceRevisionInput
	CandidateID  string
	HeadBefore   string
	Branch       string
	Conflict     bool
	UpdateGlobal bool
	NewTombstone bool
}

func prepareKnowledgeDeltaSource(userID, projectID string, repositories []projectidentity.Repository, source projectbrain.Source, removal bool) (deltaPreparedSource, error) {
	if !projectbrain.CloudSafeSource(source) {
		return deltaPreparedSource{}, fmt.Errorf("%w: local-private knowledge cannot sync to cloud", ErrKnowledgeInvalidScope)
	}
	repositoryID, canonicalPath := projectbrain.RepositoryScopeForPath(source.Path, repositories)
	if canonicalPath == "" {
		return deltaPreparedSource{}, ErrKnowledgeInvalidScope
	}
	sourceInput, err := normalizeKnowledgeSourceInput(KnowledgeSourceInput{
		UserID: userID, ProjectID: projectID, RepositoryID: repositoryID,
		Provider: source.Provider, SourceType: source.SourceType, CanonicalPath: canonicalPath, Classification: source.Classification,
		Metadata: map[string]any{"scopePath": source.ScopePath, "workspacePath": source.Path, "size": source.Size},
	})
	if err != nil {
		return deltaPreparedSource{}, err
	}
	sourceID, err := KnowledgeSourceID(sourceInput)
	if err != nil {
		return deltaPreparedSource{}, err
	}
	return deltaPreparedSource{
		Source: source, ClientKey: projectbrain.BaseRevisionKey(source), SourceID: sourceID,
		RepositoryID: repositoryID, CanonicalPath: canonicalPath, SourceInput: sourceInput, IsRemoval: removal,
	}, nil
}

func branchDecision(globalHead, branchHead, branch, baseRevisionID, candidateID string) (head string, firstBranch, allowed, updateGlobal bool) {
	globalHead = strings.TrimSpace(globalHead)
	branchHead = strings.TrimSpace(branchHead)
	branch = strings.TrimSpace(branch)
	baseRevisionID = strings.TrimSpace(baseRevisionID)
	candidateID = strings.TrimSpace(candidateID)
	if branch == "" {
		return globalHead, false, knowledgeAdvanceAllowed(globalHead, candidateID, baseRevisionID), true
	}
	if branchHead == "" {
		head = baseRevisionID
		if head == "" && candidateID == globalHead {
			head = globalHead
		}
		// The first observation of a branch is a fork, not a concurrency
		// conflict. If the supplied base is no longer the global head, keep the
		// fork branch-local instead of overwriting another branch.
		updateGlobal = globalHead == "" || candidateID == globalHead || (baseRevisionID != "" && globalHead == baseRevisionID)
		return head, true, true, updateGlobal
	}
	return branchHead, false, knowledgeAdvanceAllowed(branchHead, candidateID, baseRevisionID), globalHead == branchHead
}

func runKnowledgeExecBatch(ctx context.Context, tx pgx.Tx, batch *pgx.Batch, count int) error {
	if count == 0 {
		return nil
	}
	results := tx.SendBatch(ctx, batch)
	var firstErr error
	for index := 0; index < count; index++ {
		if _, err := results.Exec(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	if err := results.Close(); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func (s *Store) SyncKnowledgeDelta(ctx context.Context, userID, deviceID, workspaceID, projectID string, repositories []projectidentity.Repository, delta projectbrain.ManifestDelta) (KnowledgeManifestSyncResult, error) {
	result := KnowledgeManifestSyncResult{RootHash: strings.TrimSpace(delta.RootHash), ActiveRevisions: map[string]string{}}
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	workspaceID = strings.TrimSpace(workspaceID)
	projectID = strings.TrimSpace(projectID)
	if s == nil || s.DB == nil || userID == "" || deviceID == "" || workspaceID == "" || projectID == "" || result.RootHash == "" {
		return result, ErrKnowledgeInvalidScope
	}
	if len(delta.Sources)+len(delta.Removed) > maxKnowledgeDeltaItems {
		return result, fmt.Errorf("%w: knowledge delta batch too large", ErrKnowledgeInvalidScope)
	}

	prepared := make([]deltaPreparedSource, 0, len(delta.Sources)+len(delta.Removed))
	seen := map[string]struct{}{}
	for _, source := range delta.Sources {
		item, err := prepareKnowledgeDeltaSource(userID, projectID, repositories, source, false)
		if err != nil {
			return result, err
		}
		if _, duplicate := seen[item.SourceID]; duplicate {
			return result, fmt.Errorf("%w: duplicate knowledge delta source", ErrKnowledgeInvalidScope)
		}
		seen[item.SourceID] = struct{}{}
		prepared = append(prepared, item)
	}
	for _, source := range delta.Removed {
		item, err := prepareKnowledgeDeltaSource(userID, projectID, repositories, source, true)
		if err != nil {
			return result, err
		}
		if _, duplicate := seen[item.SourceID]; duplicate {
			return result, fmt.Errorf("%w: source cannot be changed and removed in one delta", ErrKnowledgeInvalidScope)
		}
		seen[item.SourceID] = struct{}{}
		prepared = append(prepared, item)
	}
	if len(prepared) == 0 {
		return result, nil
	}
	sort.Slice(prepared, func(i, j int) bool { return prepared[i].SourceID < prepared[j].SourceID })

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	now := time.Now().UnixMilli()

	// Upsert all changed sources in one protocol batch. Source IDs are
	// deterministic, so RETURNING is unnecessary and the source rows can be
	// locked/read together immediately afterwards.
	var upserts pgx.Batch
	upsertCount := 0
	for _, item := range prepared {
		if item.IsRemoval {
			continue
		}
		input := item.SourceInput
		metadata := map[string]any{"scopePath": item.Source.ScopePath, "workspacePath": item.Source.Path, "size": item.Source.Size, "manifestRoot": result.RootHash}
		upserts.Queue(`
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
 metadata=EXCLUDED.metadata`,
			input.UserID, item.SourceID, input.ProjectID, input.RepositoryID, input.Provider, input.SourceType, input.CanonicalPath,
			input.Classification, KnowledgeStatusActive, now, knowledgeMetadata(metadata))
		upsertCount++
	}
	if err := runKnowledgeExecBatch(ctx, tx, &upserts, upsertCount); err != nil {
		return result, err
	}

	sourceIDs := make([]string, 0, len(prepared))
	for _, item := range prepared {
		sourceIDs = append(sourceIDs, item.SourceID)
	}
	states := map[string]deltaSourceState{}
	rows, err := tx.Query(ctx, `
SELECT source_id,COALESCE(active_revision_id,''),status
FROM codelocal_knowledge_sources
WHERE user_id=$1 AND source_id=ANY($2::text[])
ORDER BY source_id
FOR UPDATE`, userID, sourceIDs)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var sourceID string
		var state deltaSourceState
		if err := rows.Scan(&sourceID, &state.ActiveRevisionID, &state.Status); err != nil {
			rows.Close()
			return result, err
		}
		states[sourceID] = state
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()
	for _, item := range prepared {
		if _, exists := states[item.SourceID]; !exists && !item.IsRemoval {
			return result, ErrKnowledgeSourceNotFound
		}
	}

	branchHeads := map[string]string{}
	rows, err = tx.Query(ctx, `
SELECT source_id,branch,revision_id
FROM codelocal_knowledge_branch_heads
WHERE user_id=$1 AND source_id=ANY($2::text[])
FOR UPDATE`, userID, sourceIDs)
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var sourceID, branch, revisionID string
		if err := rows.Scan(&sourceID, &branch, &revisionID); err != nil {
			rows.Close()
			return result, err
		}
		branchHeads[sourceID+"\x00"+branch] = revisionID
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return result, err
	}
	rows.Close()

	revisionIDs := map[string]struct{}{}
	for _, item := range prepared {
		state, exists := states[item.SourceID]
		if !exists {
			continue
		}
		if state.ActiveRevisionID != "" {
			revisionIDs[state.ActiveRevisionID] = struct{}{}
		}
		if base := strings.TrimSpace(item.Source.BaseRevisionID); base != "" {
			revisionIDs[base] = struct{}{}
		}
		if branch := strings.TrimSpace(item.Source.Branch); branch != "" {
			if head := branchHeads[item.SourceID+"\x00"+branch]; head != "" {
				revisionIDs[head] = struct{}{}
			}
		}
	}
	revisionIDList := make([]string, 0, len(revisionIDs))
	for revisionID := range revisionIDs {
		revisionIDList = append(revisionIDList, revisionID)
	}
	sort.Strings(revisionIDList)
	revisions := map[string]deltaRevisionMeta{}
	if len(revisionIDList) > 0 {
		rows, err = tx.Query(ctx, `
SELECT revision_id,source_id,parser_fingerprint,adapter_version,parser_version,semantic_normalizer_version,tombstone
FROM codelocal_knowledge_source_revisions
WHERE user_id=$1 AND revision_id=ANY($2::text[])`, userID, revisionIDList)
		if err != nil {
			return result, err
		}
		for rows.Next() {
			var revisionID string
			var meta deltaRevisionMeta
			if err := rows.Scan(&revisionID, &meta.SourceID, &meta.ParserFingerprint, &meta.AdapterVersion, &meta.ParserVersion, &meta.SemanticNormalizerVersion, &meta.Tombstone); err != nil {
				rows.Close()
				return result, err
			}
			revisions[revisionID] = meta
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return result, err
		}
		rows.Close()
	}

	mutations := make([]deltaMutation, 0, len(prepared))
	for _, item := range prepared {
		state, exists := states[item.SourceID]
		if !exists {
			// Removing a source the server has never seen is already converged.
			continue
		}
		baseRevisionID := strings.TrimSpace(item.Source.BaseRevisionID)
		if baseRevisionID != "" {
			meta, ok := revisions[baseRevisionID]
			if !ok || meta.SourceID != item.SourceID {
				return result, fmt.Errorf("%w: base revision does not belong to knowledge source", ErrKnowledgeInvalidScope)
			}
		}
		branch := strings.TrimSpace(item.Source.Branch)
		branchHead := ""
		if branch != "" {
			branchHead = branchHeads[item.SourceID+"\x00"+branch]
		}

		if !item.IsRemoval {
			revisionInput := KnowledgeSourceRevisionInput{
				UserID: userID, SourceID: item.SourceID, BaseRevisionID: baseRevisionID,
				ContentHash: item.Source.ContentHash, ParserFingerprint: item.Source.ParserFingerprint,
				AdapterVersion: item.Source.AdapterVersion, ParserVersion: item.Source.ParserVersion, SemanticNormalizerVersion: item.Source.SemanticNormalizerVersion,
				Metadata: map[string]any{"scopePath": item.Source.ScopePath, "size": item.Source.Size},
				Observation: KnowledgeSourceObservationInput{
					DeviceID: deviceID, WorkspaceID: workspaceID, Branch: branch, GitCommit: item.Source.GitCommit,
					Metadata: map[string]any{"manifestRoot": result.RootHash},
				},
			}
			revisionInput, err = normalizeKnowledgeRevisionInput(revisionInput)
			if err != nil {
				return result, err
			}
			candidateID, err := KnowledgeRevisionID(revisionInput)
			if err != nil {
				return result, err
			}
			head, _, allowed, updateGlobal := branchDecision(state.ActiveRevisionID, branchHead, branch, baseRevisionID, candidateID)
			mutations = append(mutations, deltaMutation{
				Prepared: item, Revision: revisionInput, CandidateID: candidateID, HeadBefore: head, Branch: branch,
				Conflict: !allowed, UpdateGlobal: updateGlobal,
			})
			continue
		}

		globalHead := strings.TrimSpace(state.ActiveRevisionID)
		if globalHead == "" {
			continue
		}
		templateRevisionID := globalHead
		if branch != "" && branchHead != "" {
			templateRevisionID = branchHead
		} else if branch != "" && baseRevisionID != "" {
			templateRevisionID = baseRevisionID
		}
		template, ok := revisions[templateRevisionID]
		if !ok || template.SourceID != item.SourceID {
			return result, fmt.Errorf("%w: removal base revision unavailable", ErrKnowledgeInvalidScope)
		}
		revisionInput := KnowledgeSourceRevisionInput{
			UserID: userID, SourceID: item.SourceID, BaseRevisionID: baseRevisionID,
			ParserFingerprint: template.ParserFingerprint, AdapterVersion: template.AdapterVersion,
			ParserVersion: template.ParserVersion, SemanticNormalizerVersion: template.SemanticNormalizerVersion, Tombstone: true,
			Metadata: map[string]any{"reason": "manifest_source_removed"},
			Observation: KnowledgeSourceObservationInput{
				DeviceID: deviceID, WorkspaceID: workspaceID, Branch: branch, GitCommit: item.Source.GitCommit,
				Metadata: map[string]any{"manifestRoot": result.RootHash},
			},
		}
		revisionInput, err = normalizeKnowledgeRevisionInput(revisionInput)
		if err != nil {
			return result, err
		}
		candidateID, err := KnowledgeRevisionID(revisionInput)
		if err != nil {
			return result, err
		}
		head, _, allowed, updateGlobal := branchDecision(globalHead, branchHead, branch, baseRevisionID, candidateID)
		mutations = append(mutations, deltaMutation{
			Prepared: item, Revision: revisionInput, CandidateID: candidateID, HeadBefore: head, Branch: branch,
			Conflict: !allowed, UpdateGlobal: updateGlobal, NewTombstone: head != candidateID,
		})
	}

	var writes pgx.Batch
	writeCount := 0
	for _, mutation := range mutations {
		input := mutation.Revision
		writes.Queue(`
INSERT INTO codelocal_knowledge_source_revisions(
 user_id,revision_id,source_id,content_hash,semantic_hash,parser_fingerprint,adapter_version,parser_version,
 semantic_normalizer_version,git_blob_oid,tombstone,created_at,metadata)
VALUES($1,$2,$3,NULLIF($4,''),NULLIF($5,''),$6,$7,$8,$9,NULLIF($10,''),$11,$12,$13)
ON CONFLICT(user_id,revision_id) DO NOTHING`,
			input.UserID, mutation.CandidateID, input.SourceID, input.ContentHash, input.SemanticHash, input.ParserFingerprint,
			input.AdapterVersion, input.ParserVersion, input.SemanticNormalizerVersion, input.GitBlobOID,
			input.Tombstone, now, knowledgeMetadata(input.Metadata))
		writeCount++

		observationID := KnowledgeObservationID(input.UserID, input.SourceID, mutation.CandidateID, input.BaseRevisionID, input.Observation)
		writes.Queue(`
INSERT INTO codelocal_knowledge_source_observations(
 user_id,observation_id,source_id,revision_id,base_revision_id,device_id,workspace_id,branch,git_commit,first_seen_at,last_seen_at,metadata)
VALUES($1,$2,$3,$4,NULLIF($5,''),NULLIF($6,''),NULLIF($7,''),NULLIF($8,''),NULLIF($9,''),$10,$10,$11)
ON CONFLICT(user_id,observation_id) DO UPDATE SET last_seen_at=EXCLUDED.last_seen_at,metadata=EXCLUDED.metadata`,
			input.UserID, observationID, input.SourceID, mutation.CandidateID, input.BaseRevisionID,
			input.Observation.DeviceID, input.Observation.WorkspaceID, mutation.Branch, input.Observation.GitCommit, now, knowledgeMetadata(input.Observation.Metadata))
		writeCount++

		state := states[mutation.Prepared.SourceID]
		if mutation.Conflict {
			conflictID := knowledgeConflictID(userID, mutation.Prepared.SourceID, mutation.Branch, mutation.HeadBefore, mutation.CandidateID)
			writes.Queue(`
INSERT INTO codelocal_knowledge_conflicts(
 user_id,conflict_id,source_id,active_revision_id,candidate_revision_id,branch,status,created_at,updated_at,metadata)
VALUES($1,$2,$3,$4,$5,NULLIF($6,''),'open',$7,$7,$8)
ON CONFLICT(user_id,conflict_id) DO UPDATE SET updated_at=EXCLUDED.updated_at,metadata=EXCLUDED.metadata,branch=EXCLUDED.branch`,
				userID, conflictID, mutation.Prepared.SourceID, mutation.HeadBefore, mutation.CandidateID, mutation.Branch, now,
				knowledgeMetadata(map[string]any{"baseRevisionId": input.BaseRevisionID, "deviceId": deviceID, "workspaceId": workspaceID, "branch": mutation.Branch}))
			writeCount++
			writes.Queue(`UPDATE codelocal_knowledge_sources SET status=CASE WHEN status='revoked' THEN status ELSE $3 END,last_seen_at=$4 WHERE user_id=$1 AND source_id=$2`, userID, mutation.Prepared.SourceID, KnowledgeStatusConflicted, now)
			writeCount++
			result.ActiveRevisions[mutation.Prepared.ClientKey] = mutation.HeadBefore
			result.Conflicts++
			continue
		}

		if mutation.Branch != "" {
			writes.Queue(`
INSERT INTO codelocal_knowledge_branch_heads(user_id,source_id,branch,revision_id,updated_at)
VALUES($1,$2,$3,$4,$5)
ON CONFLICT(user_id,source_id,branch) DO UPDATE SET revision_id=EXCLUDED.revision_id,updated_at=EXCLUDED.updated_at`, userID, mutation.Prepared.SourceID, mutation.Branch, mutation.CandidateID, now)
			writeCount++
		}
		if mutation.UpdateGlobal {
			status := knowledgeStatusAfterRevision(state.Status, input.Tombstone)
			var validTo any
			if input.Tombstone || status == KnowledgeStatusRevoked {
				validTo = now
			}
			writes.Queue(`
UPDATE codelocal_knowledge_sources
SET active_revision_id=$3,
    status=CASE WHEN status='revoked' THEN status ELSE $4 END,
    last_seen_at=$5,
    valid_to=CASE WHEN status='revoked' THEN valid_to ELSE $6 END
WHERE user_id=$1 AND source_id=$2`, userID, mutation.Prepared.SourceID, mutation.CandidateID, status, now, validTo)
			writeCount++
		} else {
			writes.Queue(`UPDATE codelocal_knowledge_sources SET last_seen_at=$3 WHERE user_id=$1 AND source_id=$2`, userID, mutation.Prepared.SourceID, now)
			writeCount++
		}
		result.ActiveRevisions[mutation.Prepared.ClientKey] = mutation.CandidateID
		if mutation.Prepared.IsRemoval {
			if mutation.NewTombstone {
				result.Tombstones++
			}
		} else {
			result.Synced++
		}
	}
	if err := runKnowledgeExecBatch(ctx, tx, &writes, writeCount); err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

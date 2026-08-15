package memory

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct {
	db              *pgxpool.Pool
	embedder        Embedder
	mu              sync.RWMutex
	vectorAvailable bool
	vectorDimension int
	embeddingColumn bool
	graphEnabled    bool
}

func NewStore(db *pgxpool.Pool, embedder Embedder) *Store { return &Store{db: db, embedder: embedder} }
func (s *Store) Enabled() bool                            { return s != nil && s.db != nil }

func (s *Store) EmbeddingProvider() string {
	if s == nil || s.embedder == nil {
		return ""
	}
	return s.embedder.Name()
}

func (s *Store) EmbeddingModel() string {
	if s == nil || s.embedder == nil {
		return ""
	}
	return s.embedder.Model()
}

func (s *Store) VectorAvailable() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vectorAvailable
}

func (s *Store) VectorDimension() int {
	if s == nil {
		return 0
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.vectorDimension
}

func (s *Store) setVector(available bool, dimension int) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.vectorAvailable = available
	s.vectorDimension = dimension
}

func (s *Store) setEmbeddingColumn(available bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.embeddingColumn = available
}

func (s *Store) EmbeddingColumnAvailable() bool {
	if s == nil {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.embeddingColumn
}

func (s *Store) ProbeVector(ctx context.Context) bool {
	if !s.Enabled() {
		return false
	}
	if _, err := s.db.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		slog.Warn("memory vector extension unavailable; lexical recall remains enabled", "error", err)
		s.setVector(false, 0)
		s.setEmbeddingColumn(false)
		return false
	}
	var columnExists bool
	if err := s.db.QueryRow(ctx, `SELECT EXISTS(
 SELECT 1 FROM information_schema.columns
 WHERE table_schema=current_schema() AND table_name='codelocal_memories' AND column_name='embedding'
)`).Scan(&columnExists); err != nil {
		slog.Warn("memory vector column probe failed; lexical recall remains enabled", "error", err)
	}
	dimension := 0
	if columnExists {
		if err := s.db.QueryRow(ctx, `SELECT COALESCE(MAX(embedding_dimension),0) FROM codelocal_memories WHERE embedding_dimension IS NOT NULL AND embedding_dimension > 0`).Scan(&dimension); err != nil {
			slog.Warn("memory vector dimension probe failed; semantic recall will resume after the next successful embedding", "error", err)
			dimension = 0
		}
	}
	s.setEmbeddingColumn(columnExists)
	s.setVector(true, dimension)
	return true
}

func (s *Store) ensureVectorColumn(ctx context.Context, dimension int) bool {
	if dimension <= 0 || !s.VectorAvailable() {
		return false
	}
	existing := s.VectorDimension()
	if existing != 0 && existing != dimension {
		return false
	}
	if existing == dimension {
		return true
	}
	if _, err := s.db.Exec(ctx, fmt.Sprintf(`ALTER TABLE codelocal_memories ADD COLUMN IF NOT EXISTS embedding vector(%d)`, dimension)); err != nil {
		slog.Warn("memory vector column unavailable; lexical recall remains enabled", "error", err)
		s.setVector(false, 0)
		s.setEmbeddingColumn(false)
		return false
	}
	s.setEmbeddingColumn(true)
	if _, err := s.db.Exec(ctx, `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_codelocal_memories_vector ON codelocal_memories USING hnsw (embedding vector_cosine_ops)`); err != nil {
		slog.Warn("memory vector index unavailable; lexical recall remains enabled", "error", err)
		s.setVector(false, 0)
		return false
	}
	s.setVector(true, dimension)
	return true
}

func vectorLiteral(vector []float32) string {
	parts := make([]string, len(vector))
	for i, value := range vector {
		parts[i] = strconv.FormatFloat(float64(value), 'g', -1, 32)
	}
	return "[" + strings.Join(parts, ",") + "]"
}

func encodeList(values []string) string {
	raw, _ := json.Marshal(SanitizeList(values, 50))
	return string(raw)
}

func decodeList(raw []byte) []string {
	var values []string
	_ = json.Unmarshal(raw, &values)
	return values
}

func validLevel(level Level) bool {
	return level == LevelEvent || level == LevelScenario || level == LevelWorkspace
}

func normalizeScore(value, fallback float64) float64 {
	if value < 0 || value > 1 {
		return fallback
	}
	return value
}

func embedderModel(embedder Embedder, vector []float32) string {
	if embedder == nil || len(vector) == 0 {
		return ""
	}
	return embedder.Model()
}

func (s *Store) clearEmbeddingIfPresent(ctx context.Context, id string) error {
	if !s.Enabled() || strings.TrimSpace(id) == "" {
		return nil
	}
	if !s.EmbeddingColumnAvailable() {
		_, err := s.db.Exec(ctx, `UPDATE codelocal_memories SET embedding_model=NULL,embedding_dimension=NULL WHERE id=$1`, id)
		return err
	}
	_, err := s.db.Exec(ctx, `UPDATE codelocal_memories SET embedding=NULL,embedding_model=NULL,embedding_dimension=NULL WHERE id=$1`, id)
	return err
}

func (s *Store) Ingest(ctx context.Context, input IngestInput) (Record, error) {
	if !s.Enabled() {
		return Record{}, errors.New("memory store is disabled")
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.Summary = SanitizeText(input.Summary, 2000)
	input.Kind = strings.ToLower(SanitizeText(input.Kind, 80))
	input.SourceType = strings.ToLower(SanitizeText(input.SourceType, 40))
	if input.SourceType == "" {
		input.SourceType = "task"
	}
	scope, workspaceID, scopeErr := normalizeScope(input.Scope, input.WorkspaceID)
	if scopeErr != nil {
		return Record{}, scopeErr
	}
	input.Scope = scope
	input.WorkspaceID = workspaceID
	if input.UserID == "" || input.Summary == "" {
		return Record{}, errors.New("memory user and summary are required")
	}
	if !validLevel(input.Level) {
		return Record{}, errors.New("invalid memory level")
	}
	input.Confidence = normalizeScore(input.Confidence, .7)
	input.Importance = normalizeScore(input.Importance, .5)
	idempotency := SanitizeText(input.IdempotencyKey, 200)
	if idempotency == "" {
		idempotency = IdempotencyKey(input.UserID, string(input.Scope), input.WorkspaceID, string(input.Level), input.Kind, input.SourceType, input.TaskID, input.Summary)
	}
	var vector []float32
	if s.embedder != nil {
		vectors, err := s.embedder.Embed(ctx, []string{input.Summary})
		if err != nil {
			slog.Warn("memory embedding failed; storing lexical memory", "provider", s.embedder.Name(), "model", s.embedder.Model(), "error", err)
		} else if len(vectors) == 1 && len(vectors[0]) > 0 {
			vector = vectors[0]
			if !s.ensureVectorColumn(ctx, len(vector)) {
				vector = nil
			}
		}
	}
	now := time.Now().UnixMilli()
	id := ""
	if input.Scope == ScopeGlobal {
		id = IdempotencyKey(input.UserID, "global", idempotency)[:32]
	} else {
		// Preserve the pre-graph workspace ID formula so existing rows are updated
		// instead of duplicated after the scope migration.
		id = IdempotencyKey(input.UserID, input.WorkspaceID, idempotency)[:32]
	}
	// Clear any previous vector before changing the durable text. If the new
	// embedding cannot be produced or written, recall safely falls back to
	// lexical ranking instead of pairing new text with stale semantic meaning.
	if err := s.clearEmbeddingIfPresent(ctx, id); err != nil {
		return Record{}, fmt.Errorf("clear stale memory embedding: %w", err)
	}
	var record Record
	var filesRaw, symbolsRaw []byte
	err := s.db.QueryRow(ctx, `
INSERT INTO codelocal_memories(id,user_id,workspace_id,scope,task_id,level,kind,source_type,summary,branch,files,symbols,confidence,importance,idempotency_key,embedding_model,embedding_dimension,created_at,updated_at,last_used_at)
VALUES($1,$2,NULLIF($3,''),$4,NULLIF($5,''),$6,NULLIF($7,''),$8,$9,NULLIF($10,''),$11::jsonb,$12::jsonb,$13,$14,$15,NULLIF($16,''),NULLIF($17,0),$18,$19,$20)
ON CONFLICT(id) DO UPDATE SET
 workspace_id=EXCLUDED.workspace_id,scope=EXCLUDED.scope,task_id=EXCLUDED.task_id,level=EXCLUDED.level,kind=EXCLUDED.kind,source_type=EXCLUDED.source_type,summary=EXCLUDED.summary,branch=EXCLUDED.branch,files=EXCLUDED.files,symbols=EXCLUDED.symbols,
 confidence=EXCLUDED.confidence,importance=EXCLUDED.importance,updated_at=EXCLUDED.updated_at,last_used_at=EXCLUDED.last_used_at
RETURNING id,user_id,COALESCE(workspace_id,''),scope,COALESCE(task_id,''),level,COALESCE(kind,''),source_type,summary,COALESCE(branch,''),files,symbols,confidence,importance,created_at,updated_at,last_used_at`,
		id, input.UserID, input.WorkspaceID, input.Scope, input.TaskID, input.Level, input.Kind, input.SourceType, input.Summary, input.Branch, encodeList(input.Files), encodeList(input.Symbols), input.Confidence, input.Importance, idempotency, embedderModel(s.embedder, vector), len(vector), now, now, now,
	).Scan(&record.ID, &record.UserID, &record.WorkspaceID, &record.Scope, &record.TaskID, &record.Level, &record.Kind, &record.SourceType, &record.Summary, &record.Branch, &filesRaw, &symbolsRaw, &record.Confidence, &record.Importance, &record.CreatedAt, &record.UpdatedAt, &record.LastUsedAt)
	if err != nil {
		return Record{}, err
	}
	record.Files = decodeList(filesRaw)
	record.Symbols = decodeList(symbolsRaw)
	if len(vector) > 0 {
		if _, err := s.db.Exec(ctx, `UPDATE codelocal_memories SET embedding=$1::vector,embedding_model=$2,embedding_dimension=$3 WHERE id=$4`, vectorLiteral(vector), s.embedder.Model(), len(vector), record.ID); err != nil {
			slog.Warn("memory vector write failed; lexical memory remains stored", "error", err)
		}
	}
	if s.GraphEnabled() {
		if err := s.projectRecordToGraph(ctx, record); err != nil {
			// Graph is an acceleration/association layer. A graph projection failure
			// must never lose the durable vector/lexical memory that was already stored.
			slog.Warn("memory graph projection failed; base memory remains stored", "memoryId", record.ID, "error", err)
		}
	}
	return record, nil
}

func (s *Store) Recall(ctx context.Context, input RecallInput) ([]Record, error) {
	if !s.Enabled() {
		return nil, nil
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Query = SanitizeText(input.Query, 1200)
	if input.UserID == "" || input.Query == "" {
		return nil, nil
	}
	limit := input.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	candidateLimit := max(20, limit*4)
	rows, err := s.db.Query(ctx, `
SELECT id,user_id,COALESCE(workspace_id,''),scope,COALESCE(task_id,''),level,COALESCE(kind,''),source_type,summary,COALESCE(branch,''),files,symbols,confidence,importance,created_at,updated_at,last_used_at,
       ts_rank_cd(to_tsvector('simple',summary),plainto_tsquery('simple',$3)) AS lexical_score
FROM codelocal_memories
WHERE user_id=$1 AND (scope='global' OR ($2<>'' AND scope='workspace' AND workspace_id=$2))
ORDER BY lexical_score DESC,CASE WHEN scope='workspace' THEN 0 ELSE 1 END,created_at DESC
LIMIT $4`, input.UserID, input.WorkspaceID, input.Query, candidateLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]Record{}
	for rows.Next() {
		var record Record
		var filesRaw, symbolsRaw []byte
		if err := rows.Scan(&record.ID, &record.UserID, &record.WorkspaceID, &record.Scope, &record.TaskID, &record.Level, &record.Kind, &record.SourceType, &record.Summary, &record.Branch, &filesRaw, &symbolsRaw, &record.Confidence, &record.Importance, &record.CreatedAt, &record.UpdatedAt, &record.LastUsedAt, &record.LexicalScore); err != nil {
			return nil, err
		}
		record.Files = decodeList(filesRaw)
		record.Symbols = decodeList(symbolsRaw)
		byID[record.ID] = record
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if s.embedder != nil && s.VectorAvailable() && s.VectorDimension() > 0 {
		var queryVector []float32
		var embedErr error
		if queryEmbedder, ok := s.embedder.(QueryEmbedder); ok {
			queryVector, embedErr = queryEmbedder.EmbedQuery(ctx, input.Query)
		} else {
			var vectors [][]float32
			vectors, embedErr = s.embedder.Embed(ctx, []string{input.Query})
			if embedErr == nil && len(vectors) == 1 {
				queryVector = vectors[0]
			}
		}
		if embedErr == nil && len(queryVector) == s.VectorDimension() {
			semantic, queryErr := s.db.Query(ctx, `
SELECT id,user_id,COALESCE(workspace_id,''),scope,COALESCE(task_id,''),level,COALESCE(kind,''),source_type,summary,COALESCE(branch,''),files,symbols,confidence,importance,created_at,updated_at,last_used_at,
       1-(embedding <=> $3::vector) AS vector_score
FROM codelocal_memories
WHERE user_id=$1 AND (scope='global' OR ($2<>'' AND scope='workspace' AND workspace_id=$2)) AND embedding IS NOT NULL
ORDER BY embedding <=> $3::vector,CASE WHEN scope='workspace' THEN 0 ELSE 1 END
LIMIT $4`, input.UserID, input.WorkspaceID, vectorLiteral(queryVector), candidateLimit)
			if queryErr == nil {
				for semantic.Next() {
					var record Record
					var filesRaw, symbolsRaw []byte
					if err := semantic.Scan(&record.ID, &record.UserID, &record.WorkspaceID, &record.Scope, &record.TaskID, &record.Level, &record.Kind, &record.SourceType, &record.Summary, &record.Branch, &filesRaw, &symbolsRaw, &record.Confidence, &record.Importance, &record.CreatedAt, &record.UpdatedAt, &record.LastUsedAt, &record.VectorScore); err != nil {
						semantic.Close()
						return nil, err
					}
					record.Files = decodeList(filesRaw)
					record.Symbols = decodeList(symbolsRaw)
					if existing, ok := byID[record.ID]; ok {
						existing.VectorScore = record.VectorScore
						byID[record.ID] = existing
					} else {
						byID[record.ID] = record
					}
				}
				semantic.Close()
			}
		}
	}
	candidates := make([]Record, 0, len(byID))
	for _, record := range byID {
		candidates = append(candidates, record)
	}
	ranked := Rank(candidates, input, time.Now().UnixMilli())
	if len(ranked) > 0 {
		ids := make([]string, 0, len(ranked))
		for _, record := range ranked {
			ids = append(ids, record.ID)
		}
		_, _ = s.db.Exec(ctx, `UPDATE codelocal_memories SET last_used_at=$1 WHERE id=ANY($2)`, time.Now().UnixMilli(), ids)
	}
	return ranked, nil
}

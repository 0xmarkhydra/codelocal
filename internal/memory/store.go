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
}

func NewStore(db *pgxpool.Pool, embedder Embedder) *Store { return &Store{db: db, embedder: embedder} }
func (s *Store) Enabled() bool                            { return s != nil && s.db != nil }

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

func (s *Store) ProbeVector(ctx context.Context) bool {
	if !s.Enabled() {
		return false
	}
	if _, err := s.db.Exec(ctx, `CREATE EXTENSION IF NOT EXISTS vector`); err != nil {
		slog.Warn("memory vector extension unavailable; lexical recall remains enabled", "error", err)
		s.setVector(false, 0)
		return false
	}
	s.setVector(true, 0)
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
		return false
	}
	if _, err := s.db.Exec(ctx, `CREATE INDEX IF NOT EXISTS idx_codelocal_memories_vector ON codelocal_memories USING hnsw (embedding vector_cosine_ops)`); err != nil {
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

func (s *Store) Ingest(ctx context.Context, input IngestInput) (Record, error) {
	if !s.Enabled() {
		return Record{}, errors.New("memory store is disabled")
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Summary = SanitizeText(input.Summary, 2000)
	if input.UserID == "" || input.WorkspaceID == "" || input.Summary == "" {
		return Record{}, errors.New("memory user, workspace and summary are required")
	}
	if !validLevel(input.Level) {
		return Record{}, errors.New("invalid memory level")
	}
	input.Confidence = normalizeScore(input.Confidence, .7)
	input.Importance = normalizeScore(input.Importance, .5)
	idempotency := SanitizeText(input.IdempotencyKey, 200)
	if idempotency == "" {
		idempotency = IdempotencyKey(input.UserID, input.WorkspaceID, string(input.Level), input.TaskID, input.Summary)
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
	id := IdempotencyKey(input.UserID, input.WorkspaceID, idempotency)[:32]
	var record Record
	var filesRaw, symbolsRaw []byte
	err := s.db.QueryRow(ctx, `
INSERT INTO codelocal_memories(id,user_id,workspace_id,task_id,level,summary,branch,files,symbols,confidence,importance,idempotency_key,embedding_model,embedding_dimension,created_at,last_used_at)
VALUES($1,$2,$3,NULLIF($4,''),$5,$6,NULLIF($7,''),$8::jsonb,$9::jsonb,$10,$11,$12,NULLIF($13,''),NULLIF($14,0),$15,$16)
ON CONFLICT(user_id,workspace_id,idempotency_key) WHERE idempotency_key IS NOT NULL DO UPDATE SET
 task_id=EXCLUDED.task_id,level=EXCLUDED.level,summary=EXCLUDED.summary,branch=EXCLUDED.branch,files=EXCLUDED.files,symbols=EXCLUDED.symbols,
 confidence=EXCLUDED.confidence,importance=EXCLUDED.importance,last_used_at=EXCLUDED.last_used_at
RETURNING id,user_id,workspace_id,COALESCE(task_id,''),level,summary,COALESCE(branch,''),files,symbols,confidence,importance,created_at,last_used_at`,
		id, input.UserID, input.WorkspaceID, input.TaskID, input.Level, input.Summary, input.Branch, encodeList(input.Files), encodeList(input.Symbols), input.Confidence, input.Importance, idempotency, embedderModel(s.embedder, vector), len(vector), now, now,
	).Scan(&record.ID, &record.UserID, &record.WorkspaceID, &record.TaskID, &record.Level, &record.Summary, &record.Branch, &filesRaw, &symbolsRaw, &record.Confidence, &record.Importance, &record.CreatedAt, &record.LastUsedAt)
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
	return record, nil
}

func (s *Store) Recall(ctx context.Context, input RecallInput) ([]Record, error) {
	if !s.Enabled() {
		return nil, nil
	}
	input.UserID = strings.TrimSpace(input.UserID)
	input.WorkspaceID = strings.TrimSpace(input.WorkspaceID)
	input.Query = SanitizeText(input.Query, 1200)
	if input.UserID == "" || input.WorkspaceID == "" || input.Query == "" {
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
SELECT id,user_id,workspace_id,COALESCE(task_id,''),level,summary,COALESCE(branch,''),files,symbols,confidence,importance,created_at,last_used_at,
       ts_rank_cd(to_tsvector('simple',summary),plainto_tsquery('simple',$3)) AS lexical_score
FROM codelocal_memories
WHERE user_id=$1 AND workspace_id=$2
ORDER BY lexical_score DESC,created_at DESC
LIMIT $4`, input.UserID, input.WorkspaceID, input.Query, candidateLimit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	byID := map[string]Record{}
	for rows.Next() {
		var record Record
		var filesRaw, symbolsRaw []byte
		if err := rows.Scan(&record.ID, &record.UserID, &record.WorkspaceID, &record.TaskID, &record.Level, &record.Summary, &record.Branch, &filesRaw, &symbolsRaw, &record.Confidence, &record.Importance, &record.CreatedAt, &record.LastUsedAt, &record.LexicalScore); err != nil {
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
		vectors, err := s.embedder.Embed(ctx, []string{input.Query})
		if err == nil && len(vectors) == 1 && len(vectors[0]) == s.VectorDimension() {
			semantic, queryErr := s.db.Query(ctx, `
SELECT id,user_id,workspace_id,COALESCE(task_id,''),level,summary,COALESCE(branch,''),files,symbols,confidence,importance,created_at,last_used_at,
       1-(embedding <=> $3::vector) AS vector_score
FROM codelocal_memories
WHERE user_id=$1 AND workspace_id=$2 AND embedding IS NOT NULL
ORDER BY embedding <=> $3::vector
LIMIT $4`, input.UserID, input.WorkspaceID, vectorLiteral(vectors[0]), candidateLimit)
			if queryErr == nil {
				for semantic.Next() {
					var record Record
					var filesRaw, symbolsRaw []byte
					if err := semantic.Scan(&record.ID, &record.UserID, &record.WorkspaceID, &record.TaskID, &record.Level, &record.Summary, &record.Branch, &filesRaw, &symbolsRaw, &record.Confidence, &record.Importance, &record.CreatedAt, &record.LastUsedAt, &record.VectorScore); err != nil {
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
var _ = time.Now

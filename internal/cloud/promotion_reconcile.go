package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

const reconcileExplicitProjectMemoriesSQL = `
SELECT m.id,COALESCE(m.kind,''),m.summary,m.confidence,m.importance,m.symbols
FROM codelocal_memories m
WHERE m.user_id=$1
 AND m.project_id=$2
 AND m.scope='project'
 AND m.source_type='conversation'
 AND m.kind IN ('decision','project_fact','constraint','goal','milestone','problem')
 AND m.confidence >= 0.9
 AND m.importance >= 0.65
 AND NOT EXISTS (
  SELECT 1
  FROM codelocal_knowledge_promotion_sources ps
  JOIN codelocal_knowledge_promotion_candidates pc
   ON pc.user_id=ps.user_id AND pc.candidate_id=ps.candidate_id
  WHERE ps.user_id=m.user_id
   AND ps.memory_id=m.id
   AND ps.source_type='explicit_user_memory'
   AND pc.summary=m.summary
   AND pc.confidence >= m.confidence
   AND pc.importance >= m.importance
 )
ORDER BY m.updated_at DESC,m.id ASC
LIMIT $3`

type explicitMemoryReconcileRow struct {
	MemoryID   string
	Kind       string
	Summary    string
	Confidence float64
	Importance float64
	Symbols    []string
}

func explicitMemoryReconcileLimit() int {
	value := envInt("CODELOCAL_KNOWLEDGE_RECONCILE_LIMIT", 50)
	if value < 1 {
		return 1
	}
	if value > 200 {
		return 200
	}
	return value
}

func singleMemoryKeySymbol(symbols []string) (string, bool) {
	key := ""
	count := 0
	for _, symbol := range symbols {
		symbol = strings.TrimSpace(symbol)
		if !strings.HasPrefix(symbol, "memory-key:") {
			continue
		}
		count++
		value := strings.ToLower(strings.Join(strings.Fields(strings.TrimSpace(strings.TrimPrefix(symbol, "memory-key:"))), " "))
		if value == "" {
			return "", false
		}
		key = value
	}
	if count != 1 {
		return "", false
	}
	return key, true
}

func explicitMemoryPromotionInputFromReconcileRow(userID, projectID string, row explicitMemoryReconcileRow) (ExplicitMemoryPromotionInput, bool) {
	sourceKey, ok := singleMemoryKeySymbol(row.Symbols)
	if !ok {
		return ExplicitMemoryPromotionInput{}, false
	}
	record := longmemory.Record{
		ID:         strings.TrimSpace(row.MemoryID),
		UserID:     strings.TrimSpace(userID),
		ProjectID:  strings.TrimSpace(projectID),
		Scope:      longmemory.ScopeProject,
		Kind:       strings.TrimSpace(row.Kind),
		SourceType: "conversation",
		Summary:    row.Summary,
		Symbols:    append([]string(nil), row.Symbols...),
		Confidence: row.Confidence,
		Importance: row.Importance,
	}
	return ExplicitMemoryPromotionInputForRecord(userID, projectID, record, sourceKey)
}

func scanExplicitMemoryReconcileRow(row interface{ Scan(...any) error }) (explicitMemoryReconcileRow, error) {
	var value explicitMemoryReconcileRow
	var symbolsJSON []byte
	if err := row.Scan(&value.MemoryID, &value.Kind, &value.Summary, &value.Confidence, &value.Importance, &symbolsJSON); err != nil {
		return explicitMemoryReconcileRow{}, err
	}
	if err := json.Unmarshal(symbolsJSON, &value.Symbols); err != nil {
		return explicitMemoryReconcileRow{}, err
	}
	return value, nil
}

func (s *Store) reconcileExplicitProjectMemories(ctx context.Context, userID, projectID string) (int, error) {
	if s == nil || s.DB == nil {
		return 0, errors.New("explicit memory reconciliation unavailable")
	}
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" || projectID == "" {
		return 0, errors.New("explicit memory reconciliation requires user and project")
	}
	rows, err := s.DB.Query(ctx, reconcileExplicitProjectMemoriesSQL, userID, projectID, explicitMemoryReconcileLimit())
	if err != nil {
		return 0, err
	}
	inputs := []ExplicitMemoryPromotionInput{}
	for rows.Next() {
		row, scanErr := scanExplicitMemoryReconcileRow(rows)
		if scanErr != nil {
			rows.Close()
			return 0, scanErr
		}
		input, ok := explicitMemoryPromotionInputFromReconcileRow(userID, projectID, row)
		if ok {
			inputs = append(inputs, input)
		}
	}
	if err := rows.Err(); err != nil {
		rows.Close()
		return 0, err
	}
	rows.Close()
	processed := 0
	for _, input := range inputs {
		candidate, err := s.StageExplicitMemoryCandidate(ctx, input)
		if err != nil {
			return processed, err
		}
		if candidate == nil {
			continue
		}
		if err := s.evaluateAndPromoteCandidate(ctx, candidate); err != nil {
			return processed, err
		}
		processed++
	}
	return processed, nil
}

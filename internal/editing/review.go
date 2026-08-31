package editing

import (
	"errors"
	"sort"
	"strings"
	"time"
)

type ReviewDisposition string
const ( ReviewPending ReviewDisposition = "pending"; ReviewKeep ReviewDisposition = "keep"; ReviewRevert ReviewDisposition = "revert" )
var ( ErrInvalidReview = errors.New("invalid change review"); ErrStaleReviewRevision = errors.New("stale change review revision"); ErrReviewItemNotFound = errors.New("change review item not found") )

type ReviewChange struct { Path string `json:"path"`; BeforeHash string `json:"beforeHash,omitempty"`; AfterHash string `json:"afterHash,omitempty"`; Created bool `json:"created,omitempty"`; Disposition ReviewDisposition `json:"disposition"` }
type ReviewBatch struct { ID string `json:"id"`; PatchSetID string `json:"patchSetId"`; Revision uint64 `json:"revision"`; Changes []ReviewChange `json:"changes"`; CreatedAt time.Time `json:"createdAt"`; UpdatedAt time.Time `json:"updatedAt"` }

func NewReviewBatch(id, patchSetID string, changes []ReviewChange) (ReviewBatch, error) { id, patchSetID = strings.TrimSpace(id), strings.TrimSpace(patchSetID); if id == "" || patchSetID == "" || len(changes) == 0 { return ReviewBatch{}, ErrInvalidReview }; seen := map[string]struct{}{}; normalized := make([]ReviewChange, 0, len(changes)); for _, change := range changes { change.Path = strings.TrimSpace(change.Path); if change.Path == "" { return ReviewBatch{}, ErrInvalidReview }; if _, exists := seen[change.Path]; exists { return ReviewBatch{}, ErrInvalidReview }; seen[change.Path] = struct{}{}; change.Disposition = ReviewPending; normalized = append(normalized, change) }; sort.Slice(normalized, func(i, j int) bool { return normalized[i].Path < normalized[j].Path }); now := time.Now().UTC(); return ReviewBatch{ID: id, PatchSetID: patchSetID, Revision: 1, Changes: normalized, CreatedAt: now, UpdatedAt: now}, nil }

// Decide records post-execution review intent; actual revert remains behind the
// Patch/Savepoint policy path. CAS prevents stale UI decisions from winning.
func (batch ReviewBatch) Decide(path string, expectedRevision uint64, disposition ReviewDisposition) (ReviewBatch, error) { if batch.Revision != expectedRevision { return batch, ErrStaleReviewRevision }; if disposition != ReviewKeep && disposition != ReviewRevert { return batch, ErrInvalidReview }; path = strings.TrimSpace(path); found := false; for i := range batch.Changes { if batch.Changes[i].Path == path { batch.Changes[i].Disposition = disposition; found = true; break } }; if !found { return batch, ErrReviewItemNotFound }; batch.Revision++; batch.UpdatedAt = time.Now().UTC(); return batch, nil }
func (batch ReviewBatch) Complete() bool { if len(batch.Changes) == 0 { return false }; for _, change := range batch.Changes { if change.Disposition == ReviewPending { return false } }; return true }

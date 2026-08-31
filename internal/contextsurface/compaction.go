package contextsurface

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"sort"
	"strings"
)

const CompactionVersion = 1

var ErrCompactionMandatoryOverflow = errors.New("context compaction budget cannot preserve mandatory evidence")

type compactionCandidate struct {
	index int
	score int
	cost  int
}

type CompactionRecord struct {
	Version           int      `json:"version"`
	ID                string   `json:"id"`
	SourceFingerprint string   `json:"sourceFingerprint"`
	TargetTokens      int      `json:"targetTokens"`
	OriginalTokens    int      `json:"originalTokens"`
	VisibleTokens     int      `json:"visibleTokens"`
	HiddenTokens      int      `json:"hiddenTokens"`
	KeptItemIDs       []string `json:"keptItemIds,omitempty"`
	HiddenItemIDs     []string `json:"hiddenItemIds,omitempty"`
	MarkerItemID      string   `json:"markerItemId,omitempty"`
	// HiddenItems is process-local convenience only. Durable storage should keep
	// the descriptor above and resolve original evidence from the event/artifact
	// store by ID instead of duplicating raw model/tool content.
	HiddenItems []Item `json:"-"`
}

// Compact creates a reversible, model-free checkpoint. Required evidence is
// never hidden. Lower-priority visible items are replaced by one small marker;
// the returned record names exactly which evidence IDs must be resolvable for
// later expansion.
func Compact(surface Surface, targetTokens int) (Surface, CompactionRecord, error) {
	if targetTokens <= 0 || surface.MandatoryOverflow {
		return Surface{}, CompactionRecord{}, ErrCompactionMandatoryOverflow
	}
	originalTokens := totalItemTokens(surface.Items)
	if originalTokens <= targetTokens {
		return surface, CompactionRecord{Version: CompactionVersion, SourceFingerprint: surface.Fingerprint, TargetTokens: targetTokens, OriginalTokens: originalTokens, VisibleTokens: originalTokens}, nil
	}

	requiredTokens := 0
	keep := make(map[int]bool, len(surface.Items))
	for index, item := range surface.Items {
		if !item.Required {
			continue
		}
		requiredTokens += itemTokens(item)
		keep[index] = true
	}
	if requiredTokens > targetTokens {
		return Surface{}, CompactionRecord{}, ErrCompactionMandatoryOverflow
	}

	candidates := []compactionCandidate{}
	for index, item := range surface.Items {
		if item.Required {
			continue
		}
		candidates = append(candidates, compactionCandidate{index: index, score: compactionScore(item), cost: itemTokens(item)})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].index < candidates[j].index
	})

	markerReserve := minInt(48, maxInt(16, targetTokens/5))
	itemBudget := targetTokens - markerReserve
	if itemBudget < requiredTokens {
		itemBudget = requiredTokens
	}
	used := requiredTokens
	for _, candidate := range candidates {
		if candidate.cost <= 0 || used+candidate.cost > itemBudget {
			continue
		}
		keep[candidate.index] = true
		used += candidate.cost
	}

	for {
		hiddenItems, keptIDs := partitionCompactionItems(surface.Items, keep)
		if len(hiddenItems) == 0 {
			return surface, CompactionRecord{Version: CompactionVersion, SourceFingerprint: surface.Fingerprint, TargetTokens: targetTokens, OriginalTokens: originalTokens, VisibleTokens: originalTokens, KeptItemIDs: keptIDs}, nil
		}
		hiddenIDs := itemIDs(hiddenItems)
		recordID := compactionDigest(surface.Fingerprint, strings.Join(hiddenIDs, "\x00"), integerString(targetTokens))
		marker := compactionMarker(recordID, len(hiddenItems), totalItemTokens(hiddenItems))
		visible := visibleCompactionItems(surface.Items, keep)
		visibleTokens := totalItemTokens(visible) + itemTokens(marker)
		if visibleTokens <= targetTokens {
			visible = append(visible, marker)
			compacted := rebuildCompactedSurface(surface, visible, hiddenIDs, targetTokens)
			return compacted, CompactionRecord{
				Version:           CompactionVersion,
				ID:                recordID,
				SourceFingerprint: surface.Fingerprint,
				TargetTokens:      targetTokens,
				OriginalTokens:    originalTokens,
				VisibleTokens:     compacted.Budget.EstimatedTokens,
				HiddenTokens:      totalItemTokens(hiddenItems),
				KeptItemIDs:       keptIDs,
				HiddenItemIDs:     hiddenIDs,
				MarkerItemID:      marker.ID,
				HiddenItems:       append([]Item(nil), hiddenItems...),
			}, nil
		}

		removable := removableCompactionCandidates(candidates, keep)
		if len(removable) == 0 {
			return Surface{}, CompactionRecord{}, ErrCompactionMandatoryOverflow
		}
		keep[removable[0].index] = false
	}
}

// Expand returns process-local hidden evidence. After restart callers should
// resolve HiddenItemIDs against the durable event/artifact store instead.
func Expand(record CompactionRecord, ids []string) []Item {
	wanted := map[string]struct{}{}
	for _, id := range ids {
		id = strings.TrimSpace(id)
		if id != "" {
			wanted[id] = struct{}{}
		}
	}
	all := len(wanted) == 0
	out := []Item{}
	for _, item := range record.HiddenItems {
		if !all {
			if _, ok := wanted[item.ID]; !ok {
				continue
			}
		}
		out = append(out, item)
	}
	return out
}

func compactionScore(item Item) int {
	base := 0
	switch item.Lane {
	case LaneActive:
		base = 4000
	case LaneObservation:
		base = 3200
	case LaneBrainRelevant:
		base = 2400
	case LaneRecent:
		base = 1200
	case LaneBrainMandatory:
		base = 10_000
	}
	priority := item.Priority
	if priority > 500 {
		priority = 500
	}
	if priority < -500 {
		priority = -500
	}
	return base + priority
}

func partitionCompactionItems(items []Item, keep map[int]bool) ([]Item, []string) {
	hidden := []Item{}
	keptIDs := []string{}
	for index, item := range items {
		if keep[index] {
			keptIDs = append(keptIDs, item.ID)
			continue
		}
		hidden = append(hidden, item)
	}
	return hidden, keptIDs
}

func visibleCompactionItems(items []Item, keep map[int]bool) []Item {
	out := []Item{}
	for index, item := range items {
		if keep[index] {
			out = append(out, item)
		}
	}
	return out
}

func removableCompactionCandidates(candidates []compactionCandidate, keep map[int]bool) []compactionCandidate {
	out := []compactionCandidate{}
	for _, candidate := range candidates {
		if keep[candidate.index] {
			out = append(out, candidate)
		}
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].score != out[j].score {
			return out[i].score < out[j].score
		}
		return out[i].index > out[j].index
	})
	return out
}

func compactionMarker(recordID string, count, hiddenTokens int) Item {
	return Item{
		ID:       "checkpoint:" + recordID,
		Lane:     LaneRecent,
		Text:     "Context checkpoint " + recordID + " hides " + integerString(count) + " lower-priority evidence items (~" + integerString(hiddenTokens) + " tokens). Expand/search this checkpoint only if needed.",
		Source:   "context-checkpoint:" + recordID,
		Priority: -100,
		Trust:    "verified",
	}
}

func rebuildCompactedSurface(original Surface, items []Item, hiddenIDs []string, targetTokens int) Surface {
	out := original
	out.Items = append([]Item(nil), items...)
	out.Budget = Budget{MaxTokens: targetTokens}
	for _, item := range out.Items {
		cost := itemTokens(item)
		out.Budget.EstimatedTokens += cost
		switch item.Lane {
		case LaneBrainMandatory, LaneBrainRelevant:
			out.Budget.BrainTokens += cost
		case LaneActive:
			out.Budget.ActiveTokens += cost
		case LaneRecent:
			out.Budget.RecentTokens += cost
		case LaneObservation:
			out.Budget.ObservationTokens += cost
		}
	}
	out.OmittedItemIDs = uniqueStrings(append(append([]string(nil), original.OmittedItemIDs...), hiddenIDs...))
	out.Budget.DroppedItems = len(out.OmittedItemIDs)
	out.Truncated = true
	out.Fingerprint = fingerprintSurface(out)
	return out
}

func fingerprintSurface(surface Surface) string {
	parts := []string{surface.BrainFingerprint}
	for _, item := range surface.Items {
		parts = append(parts, string(item.Lane), item.ID, item.Text)
	}
	for _, id := range surface.OmittedItemIDs {
		parts = append(parts, "omitted", id)
	}
	if surface.MandatoryOverflow {
		parts = append(parts, "mandatory-overflow")
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func totalItemTokens(items []Item) int {
	total := 0
	for _, item := range items {
		total += itemTokens(item)
	}
	return total
}

func itemIDs(items []Item) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, item.ID)
	}
	return out
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func compactionDigest(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(part))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

func integerString(value int) string {
	if value == 0 {
		return "0"
	}
	negative := value < 0
	if negative {
		value = -value
	}
	buf := [32]byte{}
	index := len(buf)
	for value > 0 {
		index--
		buf[index] = byte('0' + value%10)
		value /= 10
	}
	if negative {
		index--
		buf[index] = '-'
	}
	return string(buf[index:])
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}

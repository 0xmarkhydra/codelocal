package projectbrain

import (
	"encoding/json"
	"strings"
)

const (
	DefaultKnowledgeSyncBatchSources = 64
	DefaultKnowledgeSyncBatchBytes   = 128 << 10
)

type ManifestDelta struct {
	RootHash string   `json:"rootHash"`
	Sources  []Source `json:"sources,omitempty"`
	Removed  []Source `json:"removed,omitempty"`
}

func baseRevisionForSource(baseRevisions map[string]string, source Source) string {
	if len(baseRevisions) == 0 {
		return ""
	}
	if value := baseRevisions[BaseRevisionKey(source)]; value != "" {
		return value
	}
	identityKey := SourceIdentityKey(source)
	if strings.TrimSpace(source.Branch) != "" {
		prefix := identityKey + "\x00branch:"
		for key := range baseRevisions {
			if strings.HasPrefix(key, prefix) {
				// This source has already migrated to branch-scoped cursors. A new
				// branch must establish its own base instead of borrowing another
				// branch's legacy cursor.
				return ""
			}
		}
	}
	// Migration fallback for state files written before branch-aware cursors.
	// It is used only until this source records its first scoped branch cursor.
	return baseRevisions[identityKey]
}

func DiffManifest(previous, current Manifest, baseRevisions map[string]string) ManifestDelta {
	previousByKey := make(map[string]Source, len(previous.Sources))
	for _, source := range previous.Sources {
		previousByKey[SourceIdentityKey(source)] = source
	}
	currentByKey := make(map[string]Source, len(current.Sources))
	for _, source := range current.Sources {
		key := SourceIdentityKey(source)
		currentByKey[key] = source
		prior, existed := previousByKey[key]
		if !existed || SourceFingerprint(prior) != SourceFingerprint(source) {
			source.BaseRevisionID = baseRevisionForSource(baseRevisions, source)
		}
	}
	changed := make([]Source, 0)
	for _, source := range current.Sources {
		key := SourceIdentityKey(source)
		prior, existed := previousByKey[key]
		if existed && SourceFingerprint(prior) == SourceFingerprint(source) {
			continue
		}
		source.BaseRevisionID = baseRevisionForSource(baseRevisions, source)
		changed = append(changed, source)
	}
	removed := make([]Source, 0)
	for _, source := range previous.Sources {
		key := SourceIdentityKey(source)
		if _, exists := currentByKey[key]; exists {
			continue
		}
		source.BaseRevisionID = baseRevisionForSource(baseRevisions, source)
		removed = append(removed, source)
	}
	return ManifestDelta{RootHash: current.RootHash, Sources: changed, Removed: removed}
}

func ChunkManifestDelta(delta ManifestDelta, maxSources, maxBytes int) []ManifestDelta {
	if maxSources <= 0 {
		maxSources = DefaultKnowledgeSyncBatchSources
	}
	if maxBytes <= 0 {
		maxBytes = DefaultKnowledgeSyncBatchBytes
	}
	type item struct {
		removed bool
		source  Source
	}
	items := make([]item, 0, len(delta.Sources)+len(delta.Removed))
	for _, source := range delta.Sources {
		items = append(items, item{source: source})
	}
	for _, source := range delta.Removed {
		items = append(items, item{removed: true, source: source})
	}
	if len(items) == 0 {
		return nil
	}
	chunks := []ManifestDelta{}
	current := ManifestDelta{RootHash: delta.RootHash}
	currentBytes := len(delta.RootHash) + 64
	flush := func() {
		if len(current.Sources) == 0 && len(current.Removed) == 0 {
			return
		}
		chunks = append(chunks, current)
		current = ManifestDelta{RootHash: delta.RootHash}
		currentBytes = len(delta.RootHash) + 64
	}
	for _, value := range items {
		raw, _ := json.Marshal(value.source)
		cost := len(raw) + 16
		count := len(current.Sources) + len(current.Removed)
		if count > 0 && (count >= maxSources || currentBytes+cost > maxBytes) {
			flush()
		}
		if value.removed {
			current.Removed = append(current.Removed, value.source)
		} else {
			current.Sources = append(current.Sources, value.source)
		}
		currentBytes += cost
	}
	flush()
	return chunks
}

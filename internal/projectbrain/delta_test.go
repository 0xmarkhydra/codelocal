package projectbrain

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
)

func deltaSource(path, hash string) Source {
	return Source{
		Path: path, Provider: "agents", SourceType: "instructions", ScopePath: ".", Classification: "private_project",
		ContentHash: hash, ParserFingerprint: "parser", AdapterVersion: "1", ParserVersion: "1", SemanticNormalizerVersion: "1", Size: 64,
	}
}

func TestDiffManifestCarriesDurableBaseForChangedRemovedAndReaddedSource(t *testing.T) {
	original := deltaSource("AGENTS.md", "hash-a")
	previous := NewManifest([]Source{original})
	changed := deltaSource("AGENTS.md", "hash-b")
	current := NewManifest([]Source{changed})
	key := SourceIdentityKey(original)

	delta := DiffManifest(previous, current, map[string]string{key: "rev-a"})
	if len(delta.Sources) != 1 || delta.Sources[0].BaseRevisionID != "rev-a" || len(delta.Removed) != 0 {
		t.Fatalf("changed source lost durable base: %#v", delta)
	}

	removed := DiffManifest(current, NewManifest(nil), map[string]string{key: "rev-b"})
	if len(removed.Removed) != 1 || removed.Removed[0].BaseRevisionID != "rev-b" {
		t.Fatalf("removed source lost base revision: %#v", removed)
	}

	// After the server acknowledges the tombstone, a restart may load only the
	// persisted empty manifest plus tombstone base. Re-adding the file must form
	// a linear tombstone -> content transition rather than a false conflict.
	readded := DiffManifest(NewManifest(nil), previous, map[string]string{key: "rev-tombstone"})
	if len(readded.Sources) != 1 || readded.Sources[0].BaseRevisionID != "rev-tombstone" {
		t.Fatalf("re-added source did not use persisted tombstone base: %#v", readded)
	}
}

func TestDiffManifestUsesBranchScopedBaseAndNeverBorrowsAnotherBranch(t *testing.T) {
	original := deltaSource("AGENTS.md", "hash-a")
	original.Branch = "main"
	changedMain := deltaSource("AGENTS.md", "hash-b")
	changedMain.Branch = "main"
	mainKey := BaseRevisionKey(original)
	feature := original
	feature.Branch = "feat/payment"
	featureKey := BaseRevisionKey(feature)
	bases := map[string]string{
		SourceIdentityKey(original): "legacy-rev",
		mainKey:                     "main-rev",
		featureKey:                  "feature-rev",
	}
	delta := DiffManifest(NewManifest([]Source{original}), NewManifest([]Source{changedMain}), bases)
	if len(delta.Sources) != 1 || delta.Sources[0].BaseRevisionID != "main-rev" {
		t.Fatalf("main branch did not use its scoped cursor: %#v", delta)
	}
	newBranch := changedMain
	newBranch.Branch = "release"
	if got := baseRevisionForSource(bases, newBranch); got != "" {
		t.Fatalf("new branch borrowed another branch cursor: %q", got)
	}
	legacyOnly := map[string]string{SourceIdentityKey(original): "legacy-rev"}
	if got := baseRevisionForSource(legacyOnly, newBranch); got != "legacy-rev" {
		t.Fatalf("legacy cursor migration fallback lost: %q", got)
	}
}

func TestDiffManifestOmitsUnchangedSource(t *testing.T) {
	manifest := NewManifest([]Source{deltaSource("AGENTS.md", "same")})
	delta := DiffManifest(manifest, manifest, map[string]string{SourceIdentityKey(manifest.Sources[0]): "rev"})
	if len(delta.Sources) != 0 || len(delta.Removed) != 0 {
		t.Fatalf("unchanged manifest produced delta: %#v", delta)
	}
}

func TestSourceProvenanceDoesNotChangeManifestFingerprint(t *testing.T) {
	base := deltaSource("backend/AGENTS.md", "hash")
	withProvenance := base
	withProvenance.Branch = "feat/payment"
	withProvenance.GitCommit = "abcdef"
	if SourceFingerprint(base) != SourceFingerprint(withProvenance) {
		t.Fatal("branch/commit provenance must not split content manifest identity")
	}
	if NewManifest([]Source{base}).RootHash != NewManifest([]Source{withProvenance}).RootHash {
		t.Fatal("branch/commit provenance must not change manifest root")
	}
}

func TestChunkManifestDeltaKeepsRequestsBounded(t *testing.T) {
	sources := make([]Source, 0, 320)
	for i := 0; i < 320; i++ {
		source := deltaSource(fmt.Sprintf("modules/%03d/%s/AGENTS.md", i, strings.Repeat("long-path-", 10)), fmt.Sprintf("hash-%03d", i))
		source.BaseRevisionID = fmt.Sprintf("revision-%03d", i)
		sources = append(sources, source)
	}
	delta := ManifestDelta{RootHash: strings.Repeat("a", 64), Sources: sources}
	chunks := ChunkManifestDelta(delta, 64, 128<<10)
	if len(chunks) < 5 {
		t.Fatalf("expected multiple bounded chunks, got %d", len(chunks))
	}
	seen := 0
	for index, chunk := range chunks {
		count := len(chunk.Sources) + len(chunk.Removed)
		if count == 0 || count > 64 {
			t.Fatalf("chunk %d item count=%d", index, count)
		}
		raw, err := json.Marshal(chunk)
		if err != nil {
			t.Fatal(err)
		}
		if len(raw) > 160<<10 {
			t.Fatalf("chunk %d unexpectedly large: %d bytes", index, len(raw))
		}
		seen += count
	}
	if seen != len(sources) {
		t.Fatalf("chunking lost sources: got=%d want=%d", seen, len(sources))
	}
}

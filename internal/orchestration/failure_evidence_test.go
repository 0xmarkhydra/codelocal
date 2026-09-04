package orchestration

import (
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestFailureEvidenceDurableAndIdempotent(t *testing.T) {
	events := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	store, err := NewFailureEvidenceStore(events, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	input := FailureEvidenceBundle{OccurrenceID: "occ-1", AgentID: "agent", FailureSignature: "fail_123", ContextFingerprint: "ctx", PatchRefs: []string{"patch:b", "patch:a", "patch:a"}, LastMutationRef: "patch:b"}
	first, err := store.Record(input)
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Record(input)
	if err != nil {
		t.Fatal(err)
	}
	if first.OccurrenceID != second.OccurrenceID || len(store.List()) != 1 {
		t.Fatalf("evidence was not idempotent: first=%+v second=%+v list=%+v", first, second, store.List())
	}
	recovered, err := NewFailureEvidenceStore(events, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	got, ok := recovered.Get("occ-1")
	if !ok || got.LastMutationRef != "patch:b" || len(got.PatchRefs) != 2 {
		t.Fatalf("recovery lost evidence: %+v", got)
	}
}

func TestFailureEvidenceAttributionPrefersMutationThenPatch(t *testing.T) {
	bundle := FailureEvidenceBundle{OccurrenceID: "occ", AgentID: "agent", FailureSignature: "sig", LastMutationRef: "mutation:7", PatchRefs: []string{"patch:1"}}
	attribution := AttributeFailure(bundle)
	if attribution.ResponsibleRef != "mutation:7" || attribution.Confidence < .8 {
		t.Fatalf("unexpected mutation attribution: %+v", attribution)
	}
	bundle.LastMutationRef = ""
	attribution = AttributeFailure(bundle)
	if attribution.ResponsibleRef != "patch:1" || attribution.ReasonCode != "latest_patch_before_failure" {
		t.Fatalf("unexpected patch attribution: %+v", attribution)
	}
}

func TestFailureEvidenceRejectsRawInvalidBundle(t *testing.T) {
	store, err := NewFailureEvidenceStore(nil, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Record(FailureEvidenceBundle{OccurrenceID: "occ"}); err == nil {
		t.Fatal("missing failure signature should fail closed")
	}
}

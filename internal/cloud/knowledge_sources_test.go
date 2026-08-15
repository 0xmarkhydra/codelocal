package cloud

import (
	"errors"
	"strings"
	"testing"
)

func testKnowledgeSourceInput() KnowledgeSourceInput {
	return KnowledgeSourceInput{
		UserID:         "user-1",
		ProjectID:      "project-1",
		RepositoryID:   "repo-1",
		Provider:       "Generic",
		SourceType:     "Instructions",
		CanonicalPath:  "./docs/../AGENTS.md",
		Classification: KnowledgeClassPrivateProject,
	}
}

func testKnowledgeRevisionInput() KnowledgeSourceRevisionInput {
	return KnowledgeSourceRevisionInput{
		UserID:                    "user-1",
		SourceID:                  "ksrc-1",
		ContentHash:               "ABC123",
		SemanticHash:              "DEF456",
		ParserFingerprint:         "PARSER-FP",
		AdapterVersion:            "generic@1",
		ParserVersion:             "1",
		SemanticNormalizerVersion: "1",
		GitBlobOID:                "blob-1",
	}
}

func TestKnowledgeSourceIDNormalizesPathAndProvider(t *testing.T) {
	first := testKnowledgeSourceInput()
	second := first
	second.Provider = "  generic  "
	second.SourceType = "instructions"
	second.CanonicalPath = "AGENTS.md"

	firstID, err := KnowledgeSourceID(first)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := KnowledgeSourceID(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstID != secondID {
		t.Fatalf("equivalent source identities diverged: %q != %q", firstID, secondID)
	}

	normalized, err := normalizeKnowledgeSourceInput(first)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.CanonicalPath != "AGENTS.md" || normalized.Provider != "generic" || normalized.SourceType != "instructions" {
		t.Fatalf("unexpected normalization: %#v", normalized)
	}
}

func TestKnowledgeSourceIDSeparatesTenantProjectAndRepository(t *testing.T) {
	base := testKnowledgeSourceInput()
	baseID, err := KnowledgeSourceID(base)
	if err != nil {
		t.Fatal(err)
	}
	variants := []KnowledgeSourceInput{base, base, base}
	variants[0].UserID = "user-2"
	variants[1].ProjectID = "project-2"
	variants[2].RepositoryID = "repo-2"
	for i, variant := range variants {
		id, err := KnowledgeSourceID(variant)
		if err != nil {
			t.Fatalf("variant %d: %v", i, err)
		}
		if id == baseID {
			t.Fatalf("variant %d collapsed into base identity %q", i, baseID)
		}
	}
}

func TestKnowledgeSourceValidationRejectsUnsafeScope(t *testing.T) {
	cases := []KnowledgeSourceInput{
		{UserID: "user", Provider: "generic", SourceType: "instructions", CanonicalPath: "AGENTS.md"},
		{UserID: "user", ProjectID: "project", Provider: "generic", SourceType: "instructions", CanonicalPath: "../AGENTS.md"},
		{UserID: "user", ProjectID: "project", Provider: "generic", SourceType: "instructions", CanonicalPath: "/tmp/AGENTS.md"},
		{UserID: "user", ProjectID: "project", Provider: "generic", SourceType: "instructions", CanonicalPath: `C:\\tmp\\AGENTS.md`},
		{UserID: "user", ProjectID: "project", Provider: "generic", SourceType: "instructions", CanonicalPath: "AGENTS.md", Classification: "secret"},
	}
	for i, input := range cases {
		if _, err := normalizeKnowledgeSourceInput(input); !errors.Is(err, ErrKnowledgeInvalidScope) {
			t.Fatalf("case %d error=%v want ErrKnowledgeInvalidScope", i, err)
		}
	}
}

func TestKnowledgeRevisionIDDedupesAcrossObservationProvenance(t *testing.T) {
	first := testKnowledgeRevisionInput()
	first.BaseRevisionID = "base-a"
	first.Observation = KnowledgeSourceObservationInput{DeviceID: "mac-a", WorkspaceID: "workspace-a", Branch: "dev", GitCommit: "commit-a"}
	second := first
	second.BaseRevisionID = "base-b"
	second.Observation = KnowledgeSourceObservationInput{DeviceID: "pc-b", WorkspaceID: "workspace-b", Branch: "release", GitCommit: "commit-b"}

	firstID, err := KnowledgeRevisionID(first)
	if err != nil {
		t.Fatal(err)
	}
	secondID, err := KnowledgeRevisionID(second)
	if err != nil {
		t.Fatal(err)
	}
	if firstID != secondID {
		t.Fatalf("same content/parser revision should dedupe across devices/branches: %q != %q", firstID, secondID)
	}

	firstObservation := KnowledgeObservationID(first.UserID, first.SourceID, firstID, first.BaseRevisionID, first.Observation)
	secondObservation := KnowledgeObservationID(second.UserID, second.SourceID, secondID, second.BaseRevisionID, second.Observation)
	if firstObservation == secondObservation {
		t.Fatal("different provenance observations must remain distinct")
	}
}

func TestKnowledgeRevisionFingerprintInvalidatesOnMeaningfulProcessorChange(t *testing.T) {
	base := testKnowledgeRevisionInput()
	baseID, err := KnowledgeRevisionID(base)
	if err != nil {
		t.Fatal(err)
	}
	variants := []KnowledgeSourceRevisionInput{base, base, base}
	variants[0].ContentHash = "changed-content"
	variants[1].ParserVersion = "2"
	variants[2].SemanticNormalizerVersion = "2"
	for i, variant := range variants {
		id, err := KnowledgeRevisionID(variant)
		if err != nil {
			t.Fatalf("variant %d: %v", i, err)
		}
		if id == baseID {
			t.Fatalf("variant %d did not invalidate revision fingerprint", i)
		}
	}

	gitOnly := base
	gitOnly.GitBlobOID = "different-blob-metadata"
	gitOnlyID, err := KnowledgeRevisionID(gitOnly)
	if err != nil {
		t.Fatal(err)
	}
	if gitOnlyID != baseID {
		t.Fatalf("git provenance must not split identical content/parser revision: %q != %q", gitOnlyID, baseID)
	}
}

func TestKnowledgeRevisionValidationAndTombstone(t *testing.T) {
	missingContent := testKnowledgeRevisionInput()
	missingContent.ContentHash = ""
	if _, err := normalizeKnowledgeRevisionInput(missingContent); !errors.Is(err, ErrKnowledgeInvalidScope) {
		t.Fatalf("missing content hash error=%v", err)
	}

	tombstone := testKnowledgeRevisionInput()
	tombstone.Tombstone = true
	normalized, err := normalizeKnowledgeRevisionInput(tombstone)
	if err != nil {
		t.Fatal(err)
	}
	if normalized.ContentHash != "" || normalized.SemanticHash != "" {
		t.Fatalf("tombstone retained content identity: %#v", normalized)
	}
}

func TestKnowledgeRevisionConflictRequiresCurrentBase(t *testing.T) {
	cases := []struct {
		current string
		base    string
		want    bool
	}{
		{"", "", false},
		{"", "unexpected", true},
		{"rev-a", "", true},
		{"rev-a", "rev-a", false},
		{"rev-a", "rev-b", true},
	}
	for _, tc := range cases {
		if got := knowledgeRevisionConflict(tc.current, tc.base); got != tc.want {
			t.Fatalf("conflict(%q,%q)=%v want %v", tc.current, tc.base, got, tc.want)
		}
	}
}

func TestKnowledgeReadQueriesAreTenantScoped(t *testing.T) {
	queries := map[string]string{
		"source":        knowledgeSourceSelectSQL,
		"source-list":   knowledgeSourceListSQL,
		"revision":      knowledgeRevisionSelectSQL,
		"revision-list": knowledgeRevisionListSQL,
		"observations":  knowledgeObservationListSQL,
	}
	for name, query := range queries {
		normalized := strings.ToLower(strings.Join(strings.Fields(query), " "))
		if !strings.Contains(normalized, "where user_id=$1") {
			t.Fatalf("%s query lost first-class tenant scope: %s", name, normalized)
		}
	}
}

func TestKnowledgeStatusesRemainClosedSet(t *testing.T) {
	for _, status := range []string{KnowledgeStatusActive, KnowledgeStatusStale, KnowledgeStatusConflicted, KnowledgeStatusSuperseded, KnowledgeStatusRevoked} {
		if !validKnowledgeStatus(status) {
			t.Fatalf("expected valid status %q", status)
		}
	}
	if validKnowledgeStatus("done") {
		t.Fatal("unexpected open-ended knowledge status")
	}
}

func TestKnowledgeRevisionStatePreservesExplicitRevoke(t *testing.T) {
	if got := knowledgeStatusAfterRevision(KnowledgeStatusRevoked, false); got != KnowledgeStatusRevoked {
		t.Fatalf("content revision resurrected revoked source: %q", got)
	}
	if got := knowledgeStatusAfterRevision(KnowledgeStatusRevoked, true); got != KnowledgeStatusRevoked {
		t.Fatalf("tombstone replaced explicit revoke policy: %q", got)
	}
	if got := knowledgeStatusAfterRevision(KnowledgeStatusActive, true); got != KnowledgeStatusSuperseded {
		t.Fatalf("tombstone content state=%q want superseded", got)
	}
	if got := knowledgeStatusAfterRevision(KnowledgeStatusSuperseded, false); got != KnowledgeStatusActive {
		t.Fatalf("re-added content did not reactivate non-revoked source: %q", got)
	}
}

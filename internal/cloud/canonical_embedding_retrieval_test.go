package cloud

import (
	"strings"
	"testing"
)

func TestCanonicalEmbeddingRecallSQLIsTenantScopedAndCurrentOnly(t *testing.T) {
	lower := strings.ToLower(canonicalEmbeddingRecallSQL)
	for _, required := range []string{
		"e.user_id=$1",
		"e.project_id=$2",
		"e.provider=$3",
		"e.model=$4",
		"e.model_version=$5",
		"e.dimensions=$6",
		"o.active_revision_id=e.revision_id",
		"o.status='active'",
		"o.privacy_classification='private_project'",
		"o.repository_id=any($8::text[])",
		"o.branch=$9",
		"e.embedding <=> $7::vector",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("semantic recall query missing %q: %s", required, lower)
		}
	}
}

func TestCanonicalSemanticRecallLimitIsBounded(t *testing.T) {
	if got := canonicalSemanticRecallLimit(0); got != 6 {
		t.Fatalf("default semantic recall limit=%d want 6", got)
	}
	if got := canonicalSemanticRecallLimit(99); got != 16 {
		t.Fatalf("max semantic recall limit=%d want 16", got)
	}
	if got := canonicalSemanticRecallLimit(3); got != 3 {
		t.Fatalf("semantic recall limit=%d want 3", got)
	}
}

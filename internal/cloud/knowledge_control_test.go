package cloud

import (
	"strings"
	"testing"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

func TestKnowledgeControlQueriesRemainTenantScoped(t *testing.T) {
	for name, query := range map[string]string{
		"revoke source":    revokeKnowledgeSourceSQL,
		"memory lifecycle": setMemoryLifecycleSQL,
	} {
		if !strings.Contains(query, "user_id=$1") {
			t.Fatalf("%s query is not tenant scoped: %s", name, query)
		}
	}
}

func TestValidMemoryLifecycleClosedSet(t *testing.T) {
	for _, status := range []longmemory.LifecycleStatus{
		longmemory.LifecycleObserved, longmemory.LifecycleConfirmed, longmemory.LifecycleActive,
		longmemory.LifecycleStale, longmemory.LifecycleSuperseded, longmemory.LifecycleInvalidated,
	} {
		if !validMemoryLifecycle(status) {
			t.Fatalf("valid lifecycle rejected: %q", status)
		}
	}
	if validMemoryLifecycle("trusted_by_model") {
		t.Fatal("unknown lifecycle must be rejected")
	}
}

package cloud

import (
	"strings"
	"testing"
)

func TestLegacyOperationalNoiseCleanupQuarantinesWithoutDeletingCanonicalHistory(t *testing.T) {
	query := quarantineLegacyOperationalNoiseSQL
	for _, required := range []string{
		"source_type='task'",
		"lifecycle_status NOT IN ('invalidated','superseded')",
		"Files edited for task:%",
		"Verification evidence refreshed;%",
		"Fresh verification evidence reached the ready quality gate for task:%",
		"Agent quality gate reached ready state for task:%",
		"failed while working on task:",
		"SET lifecycle_status='invalidated'",
		"UPDATE codelocal_memory_nodes",
		"SET valid_to=$1,last_seen_at=$1",
		"UPDATE codelocal_memory_edges",
		"DELETE FROM codelocal_memory_node_aliases",
		"DELETE FROM codelocal_memory_sources",
		"LIMIT $2",
	} {
		if !strings.Contains(query, required) {
			t.Fatalf("legacy cleanup contract missing %q", required)
		}
	}
	if strings.Contains(query, "DELETE FROM codelocal_memories") {
		t.Fatal("legacy cleanup must quarantine canonical memory history instead of deleting it")
	}
}

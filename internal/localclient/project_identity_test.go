package localclient

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
)

func TestStableProjectIDUsesOnlyValidatedV1Marker(t *testing.T) {
	repositories := []projectidentity.Repository{{ID: "repo-a", IdentitySource: "remote"}}
	valid := projectidentity.Snapshot{
		MarkerPresent: true, MarkerSchemaVersion: 1, MarkerValid: true, MarkerProjectID: "prj_explicit", Repositories: repositories,
	}
	if got := stableProjectID(valid); got != "prj_explicit" {
		t.Fatalf("validated marker not used: %q", got)
	}
	for name, snapshot := range map[string]projectidentity.Snapshot{
		"invalid": {MarkerPresent: true, MarkerSchemaVersion: 1, MarkerValid: false, MarkerProjectID: "prj_bad", Repositories: repositories},
		"future":  {MarkerPresent: true, MarkerSchemaVersion: 2, MarkerValid: true, MarkerProjectID: "prj_future", Repositories: repositories},
		"legacy":  {MarkerProjectID: "prj_legacy", Repositories: repositories},
	} {
		t.Run(name, func(t *testing.T) {
			got := stableProjectID(snapshot)
			if got == snapshot.MarkerProjectID || !strings.HasPrefix(got, "reposet_") {
				t.Fatalf("unvalidated marker influenced local project identity: got=%q snapshot=%#v", got, snapshot)
			}
		})
	}
}

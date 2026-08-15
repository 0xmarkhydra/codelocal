package cloudserver

import (
	"errors"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestSchemaMigrationPayloadReportsCurrentTargetAndPlanHash(t *testing.T) {
	target := cloud.LatestSchemaMigrationVersion()
	status := cloud.SchemaMigrationStatus{
		CurrentVersion: target, TargetVersion: target, AppliedCount: target, UpToDate: true,
		ProjectBrainPlanHash: cloud.ProjectBrainMigrationPlanHash(),
	}
	payload := schemaMigrationPayload(status, nil)
	if payload["available"] != true || payload["upToDate"] != true || payload["currentVersion"] != target || payload["targetVersion"] != target || payload["appliedCount"] != target {
		t.Fatalf("unexpected schema status payload: %#v", payload)
	}
	if hash, _ := payload["projectBrainPlanHash"].(string); len(hash) != 64 {
		t.Fatalf("schema plan hash missing: %#v", payload)
	}
}

func TestSchemaMigrationPayloadFailsClosedWithoutDatabaseStatus(t *testing.T) {
	payload := schemaMigrationPayload(cloud.SchemaMigrationStatus{}, errors.New("db unavailable"))
	if payload["available"] != false || payload["upToDate"] != false || payload["targetVersion"] != cloud.LatestSchemaMigrationVersion() {
		t.Fatalf("schema status did not fail closed: %#v", payload)
	}
	if hash, _ := payload["projectBrainPlanHash"].(string); hash != cloud.ProjectBrainMigrationPlanHash() {
		t.Fatalf("fallback plan hash mismatch: %#v", payload)
	}
}

package cloud

import (
	"context"
	"strings"
	"testing"
	"time"
)

func healthyCanonicalHealthRow() knowledgeHealthCanonicalRow {
	return knowledgeHealthCanonicalRow{
		KnowledgeID: "knw-a", ActiveRevisionID: "rev-a", Status: KnowledgeStatusActive, Privacy: KnowledgeClassPrivateProject,
		RevisionID: "rev-a", Summary: "Project uses server-first architecture.", Predicate: "uses-architecture",
		Subject: map[string]any{"type": "system", "id": "project-brain"}, Object: map[string]any{"type": "architecture", "value": "server-first"},
		ProvenanceCount: 1, ActiveRevisionCount: 1,
	}
}

func TestKnowledgeHealthCriticalSignalsOverrideDegradedSignals(t *testing.T) {
	health := knowledgeHealthFromSignals("user-a", "project-a", knowledgeHealthSignals{
		SecretViolations: 1, ConflictedCandidates: 4, StalePendingCandidates: 2,
	}, 123)
	if health.Status != knowledgeHealthCritical || health.AutoPromotionEnabled {
		t.Fatalf("critical signal did not open circuit breaker: %#v", health)
	}
	if strings.Join(health.ReasonCodes, ",") != "secret_payload_detected" {
		t.Fatalf("critical health should persist only applicable compact reason codes: %#v", health.ReasonCodes)
	}
}

func TestKnowledgeHealthDegradedSignalsRemainObservableWithoutHardBlock(t *testing.T) {
	health := knowledgeHealthFromSignals("user-a", "project-a", knowledgeHealthSignals{ConflictedCandidates: 2, StalePendingCandidates: 3}, 123)
	if health.Status != knowledgeHealthDegraded || !health.AutoPromotionEnabled {
		t.Fatalf("degraded health incorrectly became a hard breaker: %#v", health)
	}
	if strings.Join(health.ReasonCodes, ",") != "promotion_conflicts,stale_pending_candidates" {
		t.Fatalf("degraded reason codes are not deterministic: %#v", health.ReasonCodes)
	}
}

func TestKnowledgeHealthDurableLearningDegradedBlocksReadinessWithoutHardPromotionBreaker(t *testing.T) {
	health := knowledgeHealthFromSignals("user-a", "project-a", knowledgeHealthSignals{DurableLearningDegraded: true}, 123)
	if health.Status != knowledgeHealthDegraded || !health.AutoPromotionEnabled {
		t.Fatalf("degraded durable learning should degrade readiness without hard-blocking promotion: %#v", health)
	}
	if strings.Join(health.ReasonCodes, ",") != "durable_learning_pipeline_degraded" {
		t.Fatalf("durable learning degraded reason not persisted deterministically: %#v", health.ReasonCodes)
	}
}

func TestKnowledgeHealthDurableLearningCriticalOpensPromotionAndHybridCircuitBreaker(t *testing.T) {
	health := knowledgeHealthFromSignals("user-a", "project-a", knowledgeHealthSignals{DurableLearningCritical: true}, 123)
	if health.Status != knowledgeHealthCritical || health.AutoPromotionEnabled {
		t.Fatalf("critical durable learning did not open circuit breaker: %#v", health)
	}
	if strings.Join(health.ReasonCodes, ",") != "durable_learning_pipeline_critical" {
		t.Fatalf("durable learning critical reason not persisted deterministically: %#v", health.ReasonCodes)
	}
}

func TestKnowledgeHealthDetectsCanonicalIntegrityNoisePrivacyAndSecrets(t *testing.T) {
	valid := healthyCanonicalHealthRow()
	broken := healthyCanonicalHealthRow()
	broken.KnowledgeID = "broken"
	broken.ActiveRevisionID = "rev-broken"
	broken.RevisionID = "rev-other"
	broken.ActiveRevisionCount = 2
	broken.ProvenanceCount = 0
	broken.Privacy = KnowledgeClassSensitive
	broken.Predicate = "terminal-progress"
	broken.Object = map[string]any{"type": "config", "api_key": "super-secret-value"}
	signals := evaluateKnowledgeHealthSignals([]knowledgeHealthCanonicalRow{valid, broken}, 1, 1, false)
	if signals.CanonicalIntegrityViolations != 1 || signals.OperationalNoiseViolations != 1 || signals.SecretViolations != 1 || signals.PrivacyViolations != 1 {
		t.Fatalf("health monitor missed structural violations: %#v", signals)
	}
	health := knowledgeHealthFromSignals("user-a", "project-a", signals, 456)
	if health.Status != knowledgeHealthCritical || health.AutoPromotionEnabled {
		t.Fatalf("structural violations failed to trip hard breaker: %#v", health)
	}
}

func TestKnowledgeHealthSecretDetectorAllowsRedactedPayloadOnly(t *testing.T) {
	if !healthValueContainsSecret(map[string]any{"api_key": "sk-live-secret"}) {
		t.Fatal("probable secret-bearing structured payload was not detected")
	}
	if healthValueContainsSecret(map[string]any{"api_key": "[REDACTED]"}) {
		t.Fatal("already-redacted payload should not trip secret health signal")
	}
	if !healthStringContainsSecret("Authorization: Bearer abcdef123456") {
		t.Fatal("bearer token in canonical text was not detected")
	}
	if healthStringContainsSecret("Bearer [REDACTED]") {
		t.Fatal("redacted bearer marker should be safe")
	}
}

func TestKnowledgeHealthTruncatedScanFailsClosed(t *testing.T) {
	health := knowledgeHealthFromSignals("user-a", "project-a", knowledgeHealthSignals{ScanTruncated: true}, 123)
	if health.Status != knowledgeHealthCritical || health.AutoPromotionEnabled || !health.ScanTruncated {
		t.Fatalf("incomplete health evidence must fail closed: %#v", health)
	}
	if len(health.ReasonCodes) != 1 || health.ReasonCodes[0] != "health_scan_truncated" {
		t.Fatalf("unexpected truncated-scan reason: %#v", health.ReasonCodes)
	}
}

func TestKnowledgeHealthQueriesAreTenantScopedBoundedAndPersistNoRawPayload(t *testing.T) {
	canonical := strings.ToLower(strings.Join(strings.Fields(knowledgeHealthCanonicalScanSQL), " "))
	if !strings.Contains(canonical, "where o.user_id=$1 and o.project_id=$2") || !strings.Contains(canonical, "limit $3") {
		t.Fatalf("canonical health scan lost tenant/project bound: %s", canonical)
	}
	promotions := strings.ToLower(strings.Join(strings.Fields(knowledgeHealthPromotionStatsSQL), " "))
	if !strings.Contains(promotions, "where user_id=$1 and project_id=$2") {
		t.Fatalf("promotion health stats lost tenant/project scope: %s", promotions)
	}
	projects := strings.ToLower(strings.Join(strings.Fields(recentKnowledgeHealthProjectsSQL), " "))
	if !strings.Contains(projects, "limit $1") {
		t.Fatalf("periodic health project scan became unbounded: %s", projects)
	}
	upsert := strings.ToLower(strings.Join(strings.Fields(upsertKnowledgeHealthSQL), " "))
	for _, forbidden := range []string{"summary", "subject", "object", "semantic_fingerprint", "last_error"} {
		if strings.Contains(upsert, forbidden) {
			t.Fatalf("health persistence leaked raw knowledge field %q: %s", forbidden, upsert)
		}
	}
	migration := strings.ToLower(strings.Join(strings.Fields(knowledgeHealthMigrationSQL), " "))
	if !strings.Contains(migration, "status in ('healthy','degraded','critical')") || !strings.Contains(migration, "reason_codes text[]") {
		t.Fatalf("health migration lost closed lifecycle/reason codes: %s", migration)
	}
}

func TestKnowledgeHealthLimitsAndIntervalsStayBounded(t *testing.T) {
	t.Setenv("CODELOCAL_KNOWLEDGE_HEALTH_SCAN_LIMIT", "999999")
	t.Setenv("CODELOCAL_KNOWLEDGE_HEALTH_PROJECT_LIMIT", "999999")
	t.Setenv("CODELOCAL_KNOWLEDGE_HEALTH_PENDING_LIMIT", "999999")
	t.Setenv("CODELOCAL_KNOWLEDGE_HEALTH_INTERVAL_MS", "1")
	if knowledgeHealthScanLimit() != 5000 || knowledgeHealthProjectLimit() != 500 || knowledgeHealthPendingLimit() != 100 {
		t.Fatalf("health work bounds escaped: scan=%d projects=%d pending=%d", knowledgeHealthScanLimit(), knowledgeHealthProjectLimit(), knowledgeHealthPendingLimit())
	}
	if knowledgeHealthInterval() != 30*time.Second {
		t.Fatalf("health interval minimum lost: %s", knowledgeHealthInterval())
	}
}

func TestKnowledgeHealthWorkerExitsWithStoreLifecycle(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	store := &Store{ctx: ctx}
	store.wg.Add(1)
	cancel()
	go store.knowledgeHealthWorker()
	done := make(chan struct{})
	go func() {
		store.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(500 * time.Millisecond):
		t.Fatal("knowledge health worker did not stop with Store context")
	}
}

package mcpgateway

import (
	"fmt"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func semanticGateMetrics(attempts int64) cloud.CanonicalSemanticCanaryMetrics {
	return cloud.CanonicalSemanticCanaryMetrics{AttemptsTotal: attempts}
}

func TestSemanticCanaryCircuitCollectsBeforeMinimumAndAllowsHealthyCohort(t *testing.T) {
	collecting := semanticCanaryDecisionFromMetrics(semanticGateMetrics(semanticCanaryGateMinSamples - 1))
	if !collecting.Allow || collecting.Status != "collecting" {
		t.Fatalf("low-sample cohort should collect: %#v", collecting)
	}
	healthy := semanticGateMetrics(100)
	healthy.ReadinessErrorCount = 2
	healthy.RecallErrorCount = 2
	healthy.TimeoutCount = 2
	healthy.SlowCount = 10
	healthy.ReadinessBlockedCount = 20
	decision := semanticCanaryDecisionFromMetrics(healthy)
	if !decision.Allow || decision.Status != "healthy" {
		t.Fatalf("healthy canary cohort was blocked: %#v", decision)
	}
}

func TestSemanticCanaryCircuitBlocksBadAggregateRates(t *testing.T) {
	cases := []struct {
		name    string
		metrics cloud.CanonicalSemanticCanaryMetrics
		status  string
	}{
		{name: "errors", metrics: cloud.CanonicalSemanticCanaryMetrics{AttemptsTotal: 100, ReadinessErrorCount: 6, RecallErrorCount: 5}, status: "blocked_error_rate"},
		{name: "timeouts", metrics: cloud.CanonicalSemanticCanaryMetrics{AttemptsTotal: 100, TimeoutCount: 6}, status: "blocked_timeout_rate"},
		{name: "slow", metrics: cloud.CanonicalSemanticCanaryMetrics{AttemptsTotal: 100, SlowCount: 21}, status: "blocked_slow_rate"},
		{name: "readiness", metrics: cloud.CanonicalSemanticCanaryMetrics{AttemptsTotal: 100, ReadinessBlockedCount: 81}, status: "blocked_readiness_rate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decision := semanticCanaryDecisionFromMetrics(tc.metrics)
			if decision.Allow || decision.Status != tc.status {
				t.Fatalf("bad aggregate did not trip breaker: got=%#v want=%q", decision, tc.status)
			}
		})
	}
}

func TestSemanticCanaryCircuitThresholdsAreStrict(t *testing.T) {
	metrics := semanticGateMetrics(100)
	metrics.ReadinessErrorCount = 10
	metrics.TimeoutCount = 5
	metrics.SlowCount = 20
	metrics.ReadinessBlockedCount = 80
	decision := semanticCanaryDecisionFromMetrics(metrics)
	if !decision.Allow || decision.Status != "healthy" {
		t.Fatalf("exact thresholds should remain allowed: %#v", decision)
	}
}

func TestSemanticCanaryCircuitAllowsSingleHalfOpenProbeAfterCooldown(t *testing.T) {
	key := semanticCanaryGateKey("user-a", "project-a")
	blocked := cloud.CanonicalSemanticCanaryMetrics{AttemptsTotal: 100, TimeoutCount: 20}
	s := &Service{
		Store: &cloud.Store{},
		semanticCanaryGates: map[string]semanticCanaryGateEntry{
			key: {Metrics: blocked, Available: true, ExpiresAt: time.Now().Add(time.Minute), NextProbeAt: time.Now().Add(-time.Second)},
		},
	}
	first := s.semanticCanaryGate(cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a"})
	if !first.Allow || first.Status != "half_open_probe" {
		t.Fatalf("expired cooldown did not permit a half-open probe: %#v", first)
	}
	second := s.semanticCanaryGate(cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a"})
	if second.Allow || second.Status != "blocked_timeout_rate" {
		t.Fatalf("breaker allowed repeated probe before cooldown: %#v", second)
	}
}

func TestSemanticCanaryGateCacheIsBoundedAndPreservesCurrentKey(t *testing.T) {
	s := &Service{semanticCanaryGates: map[string]semanticCanaryGateEntry{}}
	now := time.Now()
	for index := 0; index < semanticCanaryGateMaxEntries+20; index++ {
		key := fmt.Sprintf("user-%d\x00project-%d", index, index)
		s.semanticCanaryGates[key] = semanticCanaryGateEntry{Available: true, ExpiresAt: now.Add(time.Duration(index+1) * time.Second)}
	}
	keep := "keep\x00project"
	s.semanticCanaryGates[keep] = semanticCanaryGateEntry{Refreshing: true}
	s.trimSemanticCanaryGatesLocked(now, keep)
	if len(s.semanticCanaryGates) > semanticCanaryGateMaxEntries {
		t.Fatalf("semantic canary cache grew beyond bound: %d", len(s.semanticCanaryGates))
	}
	if _, ok := s.semanticCanaryGates[keep]; !ok {
		t.Fatal("cache trim evicted current refreshing key")
	}
}

func TestSemanticCanaryGateKeyDoesNotExposeRawSeparatorAmbiguity(t *testing.T) {
	left := semanticCanaryGateKey(" user ", " project ")
	right := semanticCanaryGateKey("user", "project")
	if left != right || left != "user\x00project" {
		t.Fatalf("unexpected semantic canary cache key: left=%q right=%q", left, right)
	}
}

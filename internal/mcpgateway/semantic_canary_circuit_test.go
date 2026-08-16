package mcpgateway

import (
	"fmt"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func semanticCanaryWindow(samples ...semanticCanaryOutcome) semanticCanaryRollingWindow {
	window := semanticCanaryRollingWindow{}
	for _, sample := range samples {
		window = window.append(sample)
	}
	return window
}

func repeatedSemanticOutcome(count int, sample semanticCanaryOutcome) semanticCanaryRollingWindow {
	window := semanticCanaryRollingWindow{}
	for index := 0; index < count; index++ {
		window = window.append(sample)
	}
	return window
}

func TestSemanticCanaryCircuitCollectsThenAllowsHealthyRollingWindow(t *testing.T) {
	collecting := repeatedSemanticOutcome(semanticCanaryGateMinSamples-1, semanticCanaryOutcome{Applied: true}).decision()
	if !collecting.Allow || collecting.Status != "collecting" {
		t.Fatalf("low-sample rolling window should collect: %#v", collecting)
	}
	healthy := repeatedSemanticOutcome(semanticCanaryGateWindowSize, semanticCanaryOutcome{Applied: true}).decision()
	if !healthy.Allow || healthy.Status != "healthy" {
		t.Fatalf("healthy rolling window was blocked: %#v", healthy)
	}
}

func TestSemanticCanaryCircuitBlocksRecentBadRates(t *testing.T) {
	cases := []struct {
		name   string
		bad    semanticCanaryOutcome
		count  int
		status string
	}{
		{name: "errors", bad: semanticCanaryOutcome{Error: true}, count: 6, status: "blocked_error_rate"},
		{name: "timeouts", bad: semanticCanaryOutcome{Timeout: true}, count: 3, status: "blocked_timeout_rate"},
		{name: "slow", bad: semanticCanaryOutcome{Slow: true}, count: 11, status: "blocked_slow_rate"},
		{name: "readiness", bad: semanticCanaryOutcome{ReadinessBlocked: true}, count: 41, status: "blocked_readiness_rate"},
		{name: "no value", bad: semanticCanaryOutcome{NoValue: true}, count: 46, status: "blocked_no_value_rate"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			window := repeatedSemanticOutcome(semanticCanaryGateWindowSize-tc.count, semanticCanaryOutcome{Applied: true})
			for index := 0; index < tc.count; index++ {
				window = window.append(tc.bad)
			}
			decision := window.decision()
			if decision.Allow || decision.Status != tc.status {
				t.Fatalf("bad recent cohort did not trip breaker: got=%#v want=%q", decision, tc.status)
			}
		})
	}
}

func TestSemanticCanaryCircuitReactsToBurstWithoutWaitingForDBRefresh(t *testing.T) {
	key := semanticCanaryGateKey("user-a", "project-a")
	s := &Service{Store: &cloud.Store{}, semanticCanaryGates: map[string]semanticCanaryGateEntry{
		key: {Available: true, ExpiresAt: time.Now().Add(time.Minute), Window: repeatedSemanticOutcome(semanticCanaryGateMinSamples, semanticCanaryOutcome{Applied: true})},
	}}
	input := cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a"}
	for index := 0; index < 3; index++ {
		s.observeSemanticCanaryOutcome(input, cloud.SemanticCanaryReasonTimeout, 450*time.Millisecond, false)
	}
	decision := s.semanticCanaryGate(input)
	if decision.Allow || decision.Status != "blocked_timeout_rate" {
		t.Fatalf("recent timeout burst did not trip in-memory breaker immediately: %#v", decision)
	}
}

func TestSemanticCanaryCircuitHonorsRetryTTLAfterRefreshFailure(t *testing.T) {
	key := semanticCanaryGateKey("user-a", "project-a")
	s := &Service{Store: &cloud.Store{}, semanticCanaryGates: map[string]semanticCanaryGateEntry{}}
	s.finishSemanticCanaryGateRefresh(key, cloud.CanonicalSemanticCanaryMetrics{}, false)
	decision := s.semanticCanaryGate(cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a"})
	if decision.Allow || decision.Status != "cache_retry_wait" {
		t.Fatalf("failed refresh ignored retry TTL: %#v", decision)
	}
}

func TestSemanticCanaryCircuitHalfOpenSuccessResetsBadRollingWindow(t *testing.T) {
	key := semanticCanaryGateKey("user-a", "project-a")
	blocked := repeatedSemanticOutcome(semanticCanaryGateWindowSize, semanticCanaryOutcome{Timeout: true})
	s := &Service{Store: &cloud.Store{}, semanticCanaryGates: map[string]semanticCanaryGateEntry{
		key: {Available: true, ExpiresAt: time.Now().Add(time.Minute), NextProbeAt: time.Now().Add(-time.Second), Window: blocked},
	}}
	input := cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a"}
	probe := s.semanticCanaryGate(input)
	if !probe.Allow || probe.Status != "half_open_probe" {
		t.Fatalf("expired cooldown did not permit half-open probe: %#v", probe)
	}
	s.observeSemanticCanaryOutcome(input, cloud.SemanticCanaryReasonReady, 50*time.Millisecond, true)
	after := s.semanticCanaryGate(input)
	if !after.Allow || after.Status != "collecting" {
		t.Fatalf("successful half-open probe did not reset bad rolling history: %#v", after)
	}
}

func TestSemanticCanaryCircuitHalfOpenFailureKeepsBreakerBlocked(t *testing.T) {
	key := semanticCanaryGateKey("user-a", "project-a")
	blocked := repeatedSemanticOutcome(semanticCanaryGateWindowSize, semanticCanaryOutcome{Timeout: true})
	s := &Service{Store: &cloud.Store{}, semanticCanaryGates: map[string]semanticCanaryGateEntry{
		key: {Available: true, ExpiresAt: time.Now().Add(time.Minute), NextProbeAt: time.Now().Add(-time.Second), Window: blocked},
	}}
	input := cloud.CanonicalKnowledgeRecallInput{UserID: "user-a", ProjectID: "project-a"}
	if probe := s.semanticCanaryGate(input); !probe.Allow || probe.Status != "half_open_probe" {
		t.Fatalf("half-open probe was not allowed: %#v", probe)
	}
	s.observeSemanticCanaryOutcome(input, cloud.SemanticCanaryReasonTimeout, 450*time.Millisecond, false)
	after := s.semanticCanaryGate(input)
	if after.Allow || after.Status != "blocked_timeout_rate" {
		t.Fatalf("failed half-open probe reopened breaker: %#v", after)
	}
}

func TestSemanticCanaryRollingWindowIsBounded(t *testing.T) {
	window := repeatedSemanticOutcome(semanticCanaryGateWindowSize+25, semanticCanaryOutcome{Applied: true})
	if len(window.Samples) != semanticCanaryGateWindowSize {
		t.Fatalf("rolling window size=%d want=%d", len(window.Samples), semanticCanaryGateWindowSize)
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

func TestSemanticCanaryGateKeyNormalizesWhitespace(t *testing.T) {
	left := semanticCanaryGateKey(" user ", " project ")
	right := semanticCanaryGateKey("user", "project")
	if left != right || left != "user\x00project" {
		t.Fatalf("unexpected semantic canary cache key: left=%q right=%q", left, right)
	}
}

func TestSemanticCanaryOutcomeClassificationCountsNoValueAndFailures(t *testing.T) {
	cases := []struct {
		reason string
		check  func(semanticCanaryOutcome) bool
	}{
		{cloud.SemanticCanaryReasonReady, func(o semanticCanaryOutcome) bool { return o.Applied }},
		{cloud.SemanticCanaryReasonNotReady, func(o semanticCanaryOutcome) bool { return o.ReadinessBlocked }},
		{cloud.SemanticCanaryReasonReadinessError, func(o semanticCanaryOutcome) bool { return o.Error }},
		{cloud.SemanticCanaryReasonRecallError, func(o semanticCanaryOutcome) bool { return o.Error }},
		{cloud.SemanticCanaryReasonTimeout, func(o semanticCanaryOutcome) bool { return o.Timeout }},
		{cloud.SemanticCanaryReasonNoSimilarity, func(o semanticCanaryOutcome) bool { return o.NoValue }},
		{cloud.SemanticCanaryReasonNoUniqueClaims, func(o semanticCanaryOutcome) bool { return o.NoValue }},
	}
	for _, tc := range cases {
		outcome := semanticCanaryOutcomeFrom(tc.reason, 10*time.Millisecond, false)
		if !tc.check(outcome) {
			t.Fatalf("reason=%q classified incorrectly: %#v", tc.reason, outcome)
		}
	}
}

func TestSemanticCanaryWindowAppendCopiesTrimmedTail(t *testing.T) {
	window := semanticCanaryWindow(semanticCanaryOutcome{Applied: true})
	for index := 0; index < semanticCanaryGateWindowSize+5; index++ {
		window = window.append(semanticCanaryOutcome{NoValue: index%2 == 0})
	}
	if len(window.Samples) != semanticCanaryGateWindowSize {
		t.Fatalf("unexpected rolling window length: %d", len(window.Samples))
	}
}

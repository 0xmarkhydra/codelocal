package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestCompactHealthAge(t *testing.T) {
	for _, tc := range []struct {
		milliseconds int64
		want         string
	}{
		{0, "0s"},
		{45_000, "45s"},
		{120_000, "2m"},
		{3_900_000, "1h05m"},
	} {
		if got := compactHealthAge(tc.milliseconds); got != tc.want {
			t.Fatalf("age(%d)=%q want %q", tc.milliseconds, got, tc.want)
		}
	}
}

func TestDurableLearningHealthCardUsesAggregateMetricsOnly(t *testing.T) {
	card := durableLearningHealthCard(cloud.DurableOutboxHealth{
		Status: "degraded", PendingCount: 3, ProcessingCount: 1, RetryingCount: 2,
		DeadCount: 1, ProcessedLastHour: 14, OldestActiveAgeMS: 180_000, MaxAttempts: 7,
	})
	for _, required := range []string{"DEGRADED", "Pending 3", "Processing 1", "Retrying 2", "Dead 1", "Processed last hour 14", "Oldest active 3m"} {
		if !strings.Contains(card, required) {
			t.Fatalf("durable learning card missing %q: %s", required, card)
		}
	}
	for _, forbidden := range []string{"payload", "last_error", "dedupe", "outbox_id", "MaxAttempts 7"} {
		if strings.Contains(card, forbidden) {
			t.Fatalf("durable learning card exposed operational detail %q: %s", forbidden, card)
		}
	}
}

func TestDurableLearningCriticalCardDoesNotClaimCodingIsBlocked(t *testing.T) {
	card := durableLearningHealthCard(cloud.DurableOutboxHealth{Status: "critical", DeadCount: 12})
	if !strings.Contains(card, "Coding actions remain available") || strings.Contains(strings.ToLower(card), "coding blocked") {
		t.Fatalf("durable learning health confused learning freshness with coding availability: %s", card)
	}
}

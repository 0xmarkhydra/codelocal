package cloud

import (
	"strings"
	"testing"
	"time"
)

func TestDurableOutboxBackoffIsBoundedExponential(t *testing.T) {
	base := 500 * time.Millisecond
	maximum := 5 * time.Second
	cases := []struct {
		attempt int
		want    time.Duration
	}{
		{attempt: 1, want: 500 * time.Millisecond},
		{attempt: 2, want: time.Second},
		{attempt: 3, want: 2 * time.Second},
		{attempt: 4, want: 4 * time.Second},
		{attempt: 5, want: 5 * time.Second},
		{attempt: 20, want: 5 * time.Second},
	}
	for _, tc := range cases {
		if got := durableOutboxBackoff(tc.attempt, base, maximum); got != tc.want {
			t.Fatalf("attempt %d backoff=%s want=%s", tc.attempt, got, tc.want)
		}
	}
}

func TestChooseExperienceRepositoryUsesDeepestUniqueBinding(t *testing.T) {
	bindings := []experienceRepositoryBinding{
		{ID: "root", RelativePath: "."},
		{ID: "api", RelativePath: "apps/api"},
		{ID: "web", RelativePath: "apps/web"},
	}
	if got := chooseExperienceRepository([]string{"apps/api/main.go", "apps/api/auth.go"}, bindings); got != "api" {
		t.Fatalf("deepest repository=%q want api", got)
	}
	if got := chooseExperienceRepository([]string{"apps/api/main.go", "apps/web/page.tsx"}, bindings); got != "" {
		t.Fatalf("multi-repository experience must remain project-scoped, got %q", got)
	}
	if got := chooseExperienceRepository([]string{"README.md"}, bindings); got != "root" {
		t.Fatalf("workspace-root file repository=%q want root", got)
	}
}

func TestDurableOutboxMigrationKeepsTenantDedupeAndClosedStatus(t *testing.T) {
	normalized := strings.ToLower(strings.Join(strings.Fields(durableOutboxMigrationSQL), " "))
	for _, required := range []string{
		"primary key(user_id,outbox_id)",
		"unique(user_id,event_type,dedupe_key)",
		"status in ('pending','processing','processed','dead')",
		"foreign key(user_id) references codelocal_users(id) on delete cascade",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("durable outbox migration lost %q: %s", required, normalized)
		}
	}
}

func TestDurableOutboxClaimUsesLeaseAndSkipLocked(t *testing.T) {
	normalized := strings.ToLower(strings.Join(strings.Fields(claimDurableOutboxSQL), " "))
	for _, required := range []string{
		"for update skip locked",
		"status='processing'",
		"lease_until=$3",
		"worker_id=$4",
		"attempts=o.attempts+1",
	} {
		if !strings.Contains(normalized, required) {
			t.Fatalf("durable outbox claim lost %q: %s", required, normalized)
		}
	}
}

func TestDurableOutboxHealthEscalatesBacklogRetriesAndDeadLetters(t *testing.T) {
	t.Setenv("CODELOCAL_OUTBOX_DEGRADED_AGE_MS", "120000")
	t.Setenv("CODELOCAL_OUTBOX_CRITICAL_AGE_MS", "1800000")
	t.Setenv("CODELOCAL_OUTBOX_CRITICAL_DEAD_COUNT", "10")
	t.Setenv("CODELOCAL_OUTBOX_MAX_ATTEMPTS", "12")
	now := int64(2_000_000)
	cases := []struct {
		name   string
		sample durableOutboxHealthSample
		want   string
	}{
		{name: "healthy", sample: durableOutboxHealthSample{ProcessedLastHour: 4}, want: "healthy"},
		{name: "retry pressure", sample: durableOutboxHealthSample{PendingCount: 1, RetryingCount: 1, MaxAttempts: 6, OldestCreatedAt: now - 10_000}, want: "degraded"},
		{name: "old backlog", sample: durableOutboxHealthSample{PendingCount: 1, OldestCreatedAt: now - 120_000}, want: "degraded"},
		{name: "dead letter", sample: durableOutboxHealthSample{DeadCount: 1}, want: "degraded"},
		{name: "critical backlog", sample: durableOutboxHealthSample{PendingCount: 1, OldestCreatedAt: now - 1_800_000}, want: "critical"},
		{name: "critical dead letters", sample: durableOutboxHealthSample{DeadCount: 10}, want: "critical"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := durableOutboxHealthFromSample(tc.sample, now)
			if got.Status != tc.want {
				t.Fatalf("health=%q want %q: %#v", got.Status, tc.want, got)
			}
		})
	}
}

func TestDurableOutboxHealthQueryIsAggregateOnly(t *testing.T) {
	lower := strings.ToLower(durableOutboxHealthSelect)
	for _, required := range []string{
		"status='pending'",
		"status='processing'",
		"status='dead'",
		"processed_at >= $1",
		"min(created_at)",
		"max(attempts)",
	} {
		if !strings.Contains(lower, required) {
			t.Fatalf("outbox health query missing %q", required)
		}
	}
	for _, forbidden := range []string{"payload", "last_error", "dedupe_key", "event_type", "outbox_id"} {
		if strings.Contains(lower, forbidden) {
			t.Fatalf("outbox health query exposes private operational field %q", forbidden)
		}
	}
	if !strings.Contains(strings.ToLower(durableOutboxHealthUserSQL), "where user_id=$2") {
		t.Fatalf("user outbox health query lost tenant scope: %s", durableOutboxHealthUserSQL)
	}
}

package cloud

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestMergeMCPUsage(t *testing.T) {
	got := mergeMCPUsage(
		MCPUsageEvent{UserID: "user-1", Calls: 2, InputBytes: 10, OutputBytes: 20, InputTokensEst: 3, OutputTokensEst: 4, CreatedAt: 100},
		MCPUsageEvent{UserID: "user-1", Calls: 5, InputBytes: 30, OutputBytes: 40, InputTokensEst: 6, OutputTokensEst: 7, CreatedAt: 200},
	)
	if got.Calls != 7 || got.InputBytes != 40 || got.OutputBytes != 60 || got.InputTokensEst != 9 || got.OutputTokensEst != 11 || got.CreatedAt != 200 {
		t.Fatalf("unexpected aggregate: %#v", got)
	}
}

func TestUsageEventFromStream(t *testing.T) {
	raw, err := json.Marshal(MCPUsageEvent{UserID: "user-1", Tool: "read_files"})
	if err != nil {
		t.Fatal(err)
	}
	batchID, event, err := usageEventFromStream(redis.XMessage{ID: "1-0", Values: map[string]any{"batchId": "batch-1", "event": string(raw)}})
	if err != nil {
		t.Fatal(err)
	}
	if batchID != "batch-1" || event.Calls != 1 || event.CreatedAt == 0 {
		t.Fatalf("unexpected decoded event: batch=%q event=%#v", batchID, event)
	}

	if _, _, err := usageEventFromStream(redis.XMessage{ID: "2-0", Values: map[string]any{"event": `{}`}}); err == nil {
		t.Fatal("missing userId should be rejected")
	}
}

func TestRecordMCPUsageNeverBlocksWhenLocalQueueIsFull(t *testing.T) {
	store := &Store{usageQ: make(chan MCPUsageEvent, 1)}
	if err := store.RecordMCPUsage(context.Background(), MCPUsageEvent{UserID: "user-1"}); err != nil {
		t.Fatal(err)
	}

	done := make(chan struct{})
	go func() {
		_ = store.RecordMCPUsage(context.Background(), MCPUsageEvent{UserID: "user-1"})
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(100 * time.Millisecond):
		t.Fatal("RecordMCPUsage blocked on a full local queue")
	}
	if dropped := store.usageDropped.Load(); dropped != 1 {
		t.Fatalf("dropped telemetry count = %d, want 1", dropped)
	}
}

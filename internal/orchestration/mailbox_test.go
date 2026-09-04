package orchestration

import (
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

func TestMailboxDeliverySurvivesRestartWithoutBecomingNewWork(t *testing.T) {
	store := runtimeevents.NewStore(filepath.Join(t.TempDir(), "events"))
	box, err := NewMailbox(store, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	message, err := box.Queue(MailboxMessage{ID: "m1", SenderAgentID: "lead", RecipientAgentID: "worker", Content: "inspect auth"})
	if err != nil {
		t.Fatal(err)
	}
	delivered, err := box.Deliver("worker", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 1 || delivered[0].ID != message.ID || delivered[0].Status != MailboxDelivered {
		t.Fatalf("delivered = %#v", delivered)
	}

	recovered, err := NewMailbox(store, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	redelivery, err := recovered.Deliver("worker", time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	if len(redelivery) != 0 {
		t.Fatalf("delivered message became new work again: %#v", redelivery)
	}
	outstanding := recovered.Outstanding("worker")
	if len(outstanding) != 1 || outstanding[0].Status != MailboxDelivered {
		t.Fatalf("outstanding = %#v", outstanding)
	}
	acked, err := recovered.Acknowledge("m1", outstanding[0].Revision, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	again, err := recovered.Acknowledge("m1", outstanding[0].Revision, time.Time{})
	if err != nil {
		t.Fatalf("ack must be idempotent: %v", err)
	}
	if again.Revision != acked.Revision || again.Status != MailboxAcknowledged {
		t.Fatalf("second ack = %#v", again)
	}
}

func TestMailboxRejectsStaleAckAndExpiresQueuedWork(t *testing.T) {
	box, err := NewMailbox(nil, "workspace", "task")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	message, err := box.Queue(MailboxMessage{ID: "m2", SenderAgentID: "lead", RecipientAgentID: "worker", Content: "old work", ExpiresAt: now.Add(-time.Second)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := box.Acknowledge(message.ID, message.Revision, now); !errors.Is(err, ErrInvalidMailboxMessage) {
		t.Fatalf("queued message should not ack directly: %v", err)
	}
	delivered, err := box.Deliver("worker", now)
	if err != nil {
		t.Fatal(err)
	}
	if len(delivered) != 0 {
		t.Fatalf("expired message delivered: %#v", delivered)
	}
	got, _ := box.Message(message.ID)
	if got.Status != MailboxExpired {
		t.Fatalf("message = %#v", got)
	}
}

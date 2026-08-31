package orchestration

import (
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
)

type MailboxStatus string

const (
	MailboxQueued       MailboxStatus = "queued"
	MailboxDelivered    MailboxStatus = "delivered"
	MailboxAcknowledged MailboxStatus = "acknowledged"
	MailboxExpired      MailboxStatus = "expired"

	eventMailboxQueued  = "mailbox.queued"
	eventMailboxUpdated = "mailbox.updated"
)

var (
	ErrInvalidMailboxMessage = errors.New("invalid mailbox message")
	ErrMailboxMessageExists  = errors.New("mailbox message already exists")
	ErrMailboxMessageMissing = errors.New("mailbox message not found")
	ErrStaleMailboxMessage   = errors.New("stale mailbox message revision")
)

type MailboxMessage struct {
	ID               string        `json:"id"`
	Revision         uint64        `json:"revision"`
	SenderAgentID    string        `json:"senderAgentId"`
	RecipientAgentID string        `json:"recipientAgentId"`
	Content          string        `json:"content"`
	DeliveryMode     string        `json:"deliveryMode,omitempty"`
	Status           MailboxStatus `json:"status"`
	CreatedAt        time.Time     `json:"createdAt"`
	UpdatedAt        time.Time     `json:"updatedAt"`
	DeliveredAt      time.Time     `json:"deliveredAt,omitempty"`
	AcknowledgedAt   time.Time     `json:"acknowledgedAt,omitempty"`
	ExpiresAt        time.Time     `json:"expiresAt,omitempty"`
}

type Mailbox struct {
	mu           sync.RWMutex
	events       *runtimeevents.Store
	workspaceKey string
	taskID       string
	messages     map[string]MailboxMessage
}

func NewMailbox(events *runtimeevents.Store, workspaceKey, taskID string) (*Mailbox, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	taskID = strings.TrimSpace(taskID)
	if workspaceKey == "" || taskID == "" {
		return nil, ErrInvalidMailboxMessage
	}
	box := &Mailbox{events: events, workspaceKey: workspaceKey, taskID: taskID, messages: map[string]MailboxMessage{}}
	if events == nil {
		return box, nil
	}
	stored, err := events.List(workspaceKey, taskID, 0, 5000)
	if err != nil {
		return nil, err
	}
	for _, event := range stored {
		if event.Type != eventMailboxQueued && event.Type != eventMailboxUpdated {
			continue
		}
		var message MailboxMessage
		if err := mailboxDecode(event.Payload["message"], &message); err != nil {
			return nil, err
		}
		box.messages[message.ID] = message
	}
	return box, nil
}

func (m *Mailbox) Queue(message MailboxMessage) (MailboxMessage, error) {
	message = normalizeMailboxMessage(message)
	if message.ID == "" || message.SenderAgentID == "" || message.RecipientAgentID == "" || message.Content == "" {
		return MailboxMessage{}, ErrInvalidMailboxMessage
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if existing, ok := m.messages[message.ID]; ok {
		if sameMailboxMessage(existing, message) {
			return existing, nil
		}
		return MailboxMessage{}, ErrMailboxMessageExists
	}
	if err := m.appendLocked(eventMailboxQueued, message, "mailbox:queue:"+message.ID); err != nil {
		return MailboxMessage{}, err
	}
	m.messages[message.ID] = message
	return message, nil
}

// Deliver moves queued work to delivered durably before returning it. After a
// crash, delivered-but-unacknowledged messages remain visible as outstanding
// work but are never presented again as newly queued work.
func (m *Mailbox) Deliver(recipientAgentID string, now time.Time) ([]MailboxMessage, error) {
	recipientAgentID = strings.TrimSpace(recipientAgentID)
	if recipientAgentID == "" {
		return nil, ErrInvalidMailboxMessage
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	ids := make([]string, 0)
	for id, message := range m.messages {
		if message.RecipientAgentID == recipientAgentID && message.Status == MailboxQueued {
			ids = append(ids, id)
		}
	}
	sort.Slice(ids, func(i, j int) bool {
		a, b := m.messages[ids[i]], m.messages[ids[j]]
		if a.CreatedAt.Equal(b.CreatedAt) {
			return a.ID < b.ID
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	out := make([]MailboxMessage, 0, len(ids))
	for _, id := range ids {
		message := m.messages[id]
		if !message.ExpiresAt.IsZero() && !now.Before(message.ExpiresAt) {
			message.Status = MailboxExpired
			message.Revision++
			message.UpdatedAt = now
			if err := m.persistUpdateLocked(message); err != nil {
				return out, err
			}
			m.messages[id] = message
			continue
		}
		message.Status = MailboxDelivered
		message.Revision++
		message.DeliveredAt = now
		message.UpdatedAt = now
		if err := m.persistUpdateLocked(message); err != nil {
			return out, err
		}
		m.messages[id] = message
		out = append(out, message)
	}
	return out, nil
}

func (m *Mailbox) Acknowledge(messageID string, expectedRevision uint64, now time.Time) (MailboxMessage, error) {
	messageID = strings.TrimSpace(messageID)
	m.mu.Lock()
	defer m.mu.Unlock()
	message, ok := m.messages[messageID]
	if !ok {
		return MailboxMessage{}, ErrMailboxMessageMissing
	}
	if message.Status == MailboxAcknowledged {
		return message, nil
	}
	if expectedRevision == 0 || expectedRevision != message.Revision {
		return MailboxMessage{}, ErrStaleMailboxMessage
	}
	if message.Status != MailboxDelivered {
		return MailboxMessage{}, ErrInvalidMailboxMessage
	}
	if now.IsZero() {
		now = time.Now().UTC()
	} else {
		now = now.UTC()
	}
	message.Status = MailboxAcknowledged
	message.Revision++
	message.AcknowledgedAt = now
	message.UpdatedAt = now
	if err := m.persistUpdateLocked(message); err != nil {
		return MailboxMessage{}, err
	}
	m.messages[messageID] = message
	return message, nil
}

func (m *Mailbox) Outstanding(recipientAgentID string) []MailboxMessage {
	recipientAgentID = strings.TrimSpace(recipientAgentID)
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []MailboxMessage{}
	for _, message := range m.messages {
		if message.RecipientAgentID != recipientAgentID {
			continue
		}
		if message.Status == MailboxQueued || message.Status == MailboxDelivered {
			out = append(out, message)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].CreatedAt.Equal(out[j].CreatedAt) {
			return out[i].ID < out[j].ID
		}
		return out[i].CreatedAt.Before(out[j].CreatedAt)
	})
	return out
}

func (m *Mailbox) Message(messageID string) (MailboxMessage, bool) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	message, ok := m.messages[strings.TrimSpace(messageID)]
	return message, ok
}

func (m *Mailbox) persistUpdateLocked(message MailboxMessage) error {
	return m.appendLocked(eventMailboxUpdated, message, "mailbox:update:"+message.ID+":"+strconv.FormatUint(message.Revision, 10))
}

func (m *Mailbox) appendLocked(eventType string, message MailboxMessage, idempotencyKey string) error {
	if m.events == nil {
		return nil
	}
	_, _, err := m.events.Append(m.workspaceKey, m.taskID, runtimeevents.Event{
		Type:           eventType,
		TaskID:         m.taskID,
		AgentID:        message.RecipientAgentID,
		IdempotencyKey: idempotencyKey,
		Payload:        map[string]any{"message": mailboxPayload(message)},
	})
	return err
}

func normalizeMailboxMessage(message MailboxMessage) MailboxMessage {
	message.ID = strings.TrimSpace(message.ID)
	message.SenderAgentID = strings.TrimSpace(message.SenderAgentID)
	message.RecipientAgentID = strings.TrimSpace(message.RecipientAgentID)
	message.Content = strings.TrimSpace(message.Content)
	message.DeliveryMode = strings.TrimSpace(message.DeliveryMode)
	if message.Status == "" {
		message.Status = MailboxQueued
	}
	if message.Revision == 0 {
		message.Revision = 1
	}
	now := time.Now().UTC()
	if message.CreatedAt.IsZero() {
		message.CreatedAt = now
	} else {
		message.CreatedAt = message.CreatedAt.UTC()
	}
	if message.UpdatedAt.IsZero() {
		message.UpdatedAt = message.CreatedAt
	} else {
		message.UpdatedAt = message.UpdatedAt.UTC()
	}
	if !message.ExpiresAt.IsZero() {
		message.ExpiresAt = message.ExpiresAt.UTC()
	}
	return message
}

func sameMailboxMessage(a, b MailboxMessage) bool {
	return a.ID == b.ID && a.Revision == b.Revision && a.SenderAgentID == b.SenderAgentID && a.RecipientAgentID == b.RecipientAgentID && a.Content == b.Content && a.DeliveryMode == b.DeliveryMode && a.Status == b.Status && a.ExpiresAt.Equal(b.ExpiresAt)
}

func mailboxPayload(value any) any {
	raw, _ := json.Marshal(value)
	var payload any
	_ = json.Unmarshal(raw, &payload)
	return payload
}

func mailboxDecode(value any, target any) error {
	raw, err := json.Marshal(value)
	if err != nil {
		return ErrInvalidMailboxMessage
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return ErrInvalidMailboxMessage
	}
	return nil
}

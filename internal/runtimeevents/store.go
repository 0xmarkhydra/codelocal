package runtimeevents

import (
	"bufio"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	codelocalstate "github.com/0xmarkhydra/codelocal/internal/state"
)

const (
	defaultListLimit = 500
	maxListLimit     = 5000
)

type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) *Store {
	root = strings.TrimSpace(root)
	if root == "" {
		root = filepath.Join(codelocalstate.Dir(), "runtime-events")
	}
	return &Store{root: root}
}

func digest(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func (s *Store) taskDir(workspaceKey, taskID string) string {
	return filepath.Join(s.root, digest("workspace", workspaceKey), digest("task", taskID))
}

func (s *Store) eventPath(workspaceKey, taskID string) string {
	return filepath.Join(s.taskDir(workspaceKey, taskID), "events.jsonl")
}

func (s *Store) snapshotPath(workspaceKey, taskID string) string {
	return filepath.Join(s.taskDir(workspaceKey, taskID), "snapshot.json")
}

func validateScope(workspaceKey, taskID string) error {
	if strings.TrimSpace(workspaceKey) == "" || strings.TrimSpace(taskID) == "" {
		return ErrInvalidEvent
	}
	return nil
}

func readEvents(path string) ([]Event, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	defer f.Close()

	events := []Event{}
	scanner := bufio.NewScanner(f)
	buffer := make([]byte, 64*1024)
	scanner.Buffer(buffer, 4*1024*1024)
	line := 0
	var previous uint64
	for scanner.Scan() {
		line++
		raw := strings.TrimSpace(scanner.Text())
		if raw == "" {
			continue
		}
		var event Event
		if err := json.Unmarshal([]byte(raw), &event); err != nil {
			return nil, fmt.Errorf("%w at line %d: %v", ErrCorruptLog, line, err)
		}
		if event.SchemaVersion != SchemaVersion || event.Sequence == 0 || event.ID == "" || event.Type == "" || event.TaskID == "" {
			return nil, fmt.Errorf("%w at line %d: invalid envelope", ErrCorruptLog, line)
		}
		if previous != 0 && event.Sequence != previous+1 {
			return nil, fmt.Errorf("%w at line %d: non-contiguous sequence", ErrCorruptLog, line)
		}
		previous = event.Sequence
		events = append(events, event)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return events, nil
}

func cloneEvent(event Event) Event {
	if event.Payload == nil {
		return event
	}
	payload := make(map[string]any, len(event.Payload))
	for key, value := range event.Payload {
		payload[key] = value
	}
	event.Payload = payload
	return event
}

func eventID(taskID string, sequence uint64, event Event) string {
	return "evt_" + digest(taskID, fmt.Sprintf("%d", sequence), event.Type, event.Timestamp.Format(timeLayout), event.IdempotencyKey)
}

const timeLayout = "2006-01-02T15:04:05.999999999Z07:00"

// Append adds one ordered durable event. When IdempotencyKey matches an existing
// event for the task, Append returns the original event with appended=false.
func (s *Store) Append(workspaceKey, taskID string, event Event) (stored Event, appended bool, err error) {
	if err := validateScope(workspaceKey, taskID); err != nil {
		return Event{}, false, err
	}
	event, err = normalizeEvent(event, taskID)
	if err != nil {
		return Event{}, false, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	path := s.eventPath(workspaceKey, taskID)
	events, err := readEvents(path)
	if err != nil {
		return Event{}, false, err
	}
	if event.IdempotencyKey != "" {
		for _, existing := range events {
			if existing.IdempotencyKey == event.IdempotencyKey {
				return cloneEvent(existing), false, nil
			}
	}

	sequence := uint64(1)
	if len(events) > 0 {
		sequence = events[len(events)-1].Sequence + 1
	}
	event.Sequence = sequence
	if event.ID == "" {
		event.ID = eventID(taskID, sequence, event)
	}
	if err := codelocalstate.AppendJSONL(path, event); err != nil {
		return Event{}, false, err
	}
	return cloneEvent(event), true, nil
}

func (s *Store) List(workspaceKey, taskID string, afterSequence uint64, limit int) ([]Event, error) {
	if err := validateScope(workspaceKey, taskID); err != nil {
		return nil, err
	}
	if limit <= 0 {
		limit = defaultListLimit
	}
	if limit > maxListLimit {
		limit = maxListLimit
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := readEvents(s.eventPath(workspaceKey, taskID))
	if err != nil {
		return nil, err
	}
	out := make([]Event, 0, min(limit, len(events)))
	for _, event := range events {
		if event.Sequence <= afterSequence {
			continue
		}
		out = append(out, cloneEvent(event))
		if len(out) == limit {
			break
		}
	}
	return out, nil
}

func (s *Store) LatestSequence(workspaceKey, taskID string) (uint64, error) {
	if err := validateScope(workspaceKey, taskID); err != nil {
		return 0, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := readEvents(s.eventPath(workspaceKey, taskID))
	if err != nil || len(events) == 0 {
		return 0, err
	}
	return events[len(events)-1].Sequence, nil
}

func (s *Store) SaveSnapshot(workspaceKey, taskID string, snapshot Snapshot) (Snapshot, error) {
	if err := validateScope(workspaceKey, taskID); err != nil {
		return Snapshot{}, err
	}
	snapshot, err := normalizeSnapshot(snapshot, taskID)
	if err != nil {
		return Snapshot{}, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	events, err := readEvents(s.eventPath(workspaceKey, taskID))
	if err != nil {
		return Snapshot{}, err
	}
	latest := uint64(0)
	if len(events) > 0 {
		latest = events[len(events)-1].Sequence
	}
	if snapshot.Sequence > latest {
		return Snapshot{}, ErrInvalidEvent
	}
	if err := codelocalstate.WriteJSONAtomic(s.snapshotPath(workspaceKey, taskID), snapshot); err != nil {
		return Snapshot{}, err
	}
	return snapshot, nil
}

func (s *Store) LoadSnapshot(workspaceKey, taskID string) (Snapshot, bool, error) {
	if err := validateScope(workspaceKey, taskID); err != nil {
		return Snapshot{}, false, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	var snapshot Snapshot
	if err := codelocalstate.ReadJSON(s.snapshotPath(workspaceKey, taskID), &snapshot); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Snapshot{}, false, nil
		}
		return Snapshot{}, false, err
	}
	if snapshot.SchemaVersion != SchemaVersion || strings.TrimSpace(snapshot.TaskID) != strings.TrimSpace(taskID) {
		return Snapshot{}, false, ErrCorruptLog
	}
	return snapshot, true, nil
}

// Replay returns the latest snapshot (when present) and all durable events that
// occurred after it. Consumers rebuild projections from this pair.
func (s *Store) Replay(workspaceKey, taskID string, limit int) (Snapshot, bool, []Event, error) {
	snapshot, found, err := s.LoadSnapshot(workspaceKey, taskID)
	if err != nil {
		return Snapshot{}, false, nil, err
	}
	after := uint64(0)
	if found {
		after = snapshot.Sequence
	}
	events, err := s.List(workspaceKey, taskID, after, limit)
	if err != nil {
		return Snapshot{}, false, nil, err
	}
	return snapshot, found, events, nil
}

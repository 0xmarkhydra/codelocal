package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/state"
)

type Status string

const (
	Started   Status = "started"
	Completed Status = "completed"
	Failed    Status = "failed"
)

type Entry struct {
	Key         string `json:"key"`
	Operation   string `json:"operation"`
	StartedAt   int64  `json:"startedAt"`
	CompletedAt int64  `json:"completedAt,omitempty"`
	Status      Status `json:"status"`
	Result      any    `json:"result,omitempty"`
	Error       string `json:"error,omitempty"`
}

type Journal struct {
	mu         sync.Mutex
	file       string
	maxEntries int
	loaded     bool
	entries    map[string]Entry
}

func New(workspaceKey string) *Journal {
	digest := sha256.Sum256([]byte(workspaceKey))
	name := hex.EncodeToString(digest[:12]) + ".json"
	return &Journal{file: filepath.Join(state.Dir(), "journals", name), maxEntries: 1000, entries: map[string]Entry{}}
}

func NewAt(file string, maxEntries int) *Journal {
	if maxEntries <= 0 {
		maxEntries = 1000
	}
	return &Journal{file: file, maxEntries: maxEntries, entries: map[string]Entry{}}
}

func (j *Journal) loadLocked() error {
	if j.loaded {
		return nil
	}
	var entries []Entry
	if err := state.ReadJSON(j.file, &entries); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	for _, entry := range entries {
		if entry.Key != "" {
			j.entries[entry.Key] = entry
		}
	}
	j.loaded = true
	return nil
}

func (j *Journal) flushLocked() error {
	entries := make([]Entry, 0, len(j.entries))
	for _, entry := range j.entries {
		entries = append(entries, entry)
	}
	sort.Slice(entries, func(i, k int) bool { return entries[i].StartedAt > entries[k].StartedAt })
	if len(entries) > j.maxEntries {
		entries = entries[:j.maxEntries]
	}
	j.entries = make(map[string]Entry, len(entries))
	for _, entry := range entries {
		j.entries[entry.Key] = entry
	}
	return state.WriteJSONAtomic(j.file, entries)
}

func (j *Journal) Get(key string) (*Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.loadLocked(); err != nil {
		return nil, err
	}
	entry, ok := j.entries[key]
	if !ok {
		return nil, nil
	}
	copy := entry
	return &copy, nil
}

func (j *Journal) Start(key, operation string) (Entry, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.loadLocked(); err != nil {
		return Entry{}, err
	}
	if existing, ok := j.entries[key]; ok && existing.Status != Failed {
		return existing, nil
	}
	entry := Entry{Key: key, Operation: operation, StartedAt: time.Now().UnixMilli(), Status: Started}
	j.entries[key] = entry
	return entry, j.flushLocked()
}

func (j *Journal) Complete(key string, result any) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.loadLocked(); err != nil {
		return err
	}
	entry, ok := j.entries[key]
	if !ok {
		return nil
	}
	entry.Status = Completed
	entry.CompletedAt = time.Now().UnixMilli()
	entry.Result = result
	entry.Error = ""
	j.entries[key] = entry
	return j.flushLocked()
}

func (j *Journal) Fail(key, message string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.loadLocked(); err != nil {
		return err
	}
	entry, ok := j.entries[key]
	if !ok {
		return nil
	}
	entry.Status = Failed
	entry.CompletedAt = time.Now().UnixMilli()
	entry.Error = message
	j.entries[key] = entry
	return j.flushLocked()
}

// Abandon removes a provisional idempotency record. Approval-required and
// policy-blocked responses are not completed side effects and may contain
// short-lived approval tokens, so they must never be persisted as replayable
// results.
func (j *Journal) Abandon(key string) error {
	j.mu.Lock()
	defer j.mu.Unlock()
	if err := j.loadLocked(); err != nil {
		return err
	}
	if _, ok := j.entries[key]; !ok {
		return nil
	}
	delete(j.entries, key)
	return j.flushLocked()
}

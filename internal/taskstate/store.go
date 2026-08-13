package taskstate

import (
	"sort"
	"strings"
	"sync"
	"time"
)

// State is CodeLocal's compact working memory for one ChatGPT MCP session and
// workspace. It is intentionally operational rather than conversational: the
// model remains responsible for reasoning while CodeLocal remembers the facts
// that are expensive to rediscover on every turn.
type State struct {
	UserID       string    `json:"-"`
	SessionID    string    `json:"sessionId"`
	WorkspaceKey string    `json:"workspaceKey"`
	Task         string    `json:"task,omitempty"`
	Branch       string    `json:"branch,omitempty"`
	TouchedFiles []string  `json:"touchedFiles,omitempty"`
	RecentChecks []string  `json:"recentChecks,omitempty"`
	RecentErrors []string  `json:"recentErrors,omitempty"`
	LastAction   string    `json:"lastAction,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// Patch updates only fields that are meaningful for working memory. Empty
// scalar values are ignored so an incidental tool call cannot erase context.
type Patch struct {
	Task          string
	Branch        string
	TouchedFiles  []string
	RecentChecks  []string
	RecentErrors  []string
	LastAction    string
	ReplaceErrors bool
}

type Store struct {
	mu     sync.RWMutex
	states map[string]State
	limit  int
}

func New(limit int) *Store {
	if limit <= 0 {
		limit = 256
	}
	return &Store{states: map[string]State{}, limit: limit}
}

func stateKey(userID, sessionID, workspaceKey string) string {
	return strings.TrimSpace(userID) + "\x00" + strings.TrimSpace(sessionID) + "\x00" + strings.TrimSpace(workspaceKey)
}

func normalizeList(values []string, max int) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if max > 0 && len(out) >= max {
			break
		}
	}
	return out
}

func mergeRecent(existing, added []string, max int) []string {
	combined := append(append([]string{}, added...), existing...)
	return normalizeList(combined, max)
}

func (s *Store) Get(userID, sessionID, workspaceKey string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	state, ok := s.states[stateKey(userID, sessionID, workspaceKey)]
	return state, ok
}

func (s *Store) Update(userID, sessionID, workspaceKey string, patch Patch) State {
	if s == nil {
		return State{}
	}
	key := stateKey(userID, sessionID, workspaceKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	state := s.states[key]
	state.UserID = strings.TrimSpace(userID)
	state.SessionID = strings.TrimSpace(sessionID)
	state.WorkspaceKey = strings.TrimSpace(workspaceKey)
	if value := strings.TrimSpace(patch.Task); value != "" {
		state.Task = value
	}
	if value := strings.TrimSpace(patch.Branch); value != "" {
		state.Branch = value
	}
	state.TouchedFiles = mergeRecent(state.TouchedFiles, patch.TouchedFiles, 24)
	state.RecentChecks = mergeRecent(state.RecentChecks, patch.RecentChecks, 16)
	if patch.ReplaceErrors {
		state.RecentErrors = normalizeList(patch.RecentErrors, 12)
	} else {
		state.RecentErrors = mergeRecent(state.RecentErrors, patch.RecentErrors, 12)
	}
	if value := strings.TrimSpace(patch.LastAction); value != "" {
		state.LastAction = value
	}
	state.UpdatedAt = time.Now().UTC()
	s.states[key] = state
	s.compactLocked()
	return state
}

func (s *Store) Delete(userID, sessionID, workspaceKey string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.states, stateKey(userID, sessionID, workspaceKey))
	s.mu.Unlock()
}

func (s *Store) compactLocked() {
	if len(s.states) <= s.limit {
		return
	}
	type item struct {
		key string
		at  time.Time
	}
	items := make([]item, 0, len(s.states))
	for key, state := range s.states {
		items = append(items, item{key: key, at: state.UpdatedAt})
	}
	sort.Slice(items, func(i, j int) bool { return items[i].at.Before(items[j].at) })
	remove := len(items) - s.limit
	for i := 0; i < remove; i++ {
		delete(s.states, items[i].key)
	}
}

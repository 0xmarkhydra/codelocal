package taskstate

import (
	"crypto/sha256"
	"encoding/hex"
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
	UserID                 string    `json:"-"`
	SessionID              string    `json:"sessionId"`
	WorkspaceKey           string    `json:"workspaceKey"`
	TaskID                 string    `json:"taskId,omitempty"`
	Task                   string    `json:"task,omitempty"`
	Branch                 string    `json:"branch,omitempty"`
	TouchedFiles           []string  `json:"touchedFiles,omitempty"`
	RecentChecks           []string  `json:"recentChecks,omitempty"`
	RecentErrors           []string  `json:"recentErrors,omitempty"`
	LastAction             string    `json:"lastAction,omitempty"`
	AgentPhase             string    `json:"agentPhase,omitempty"`
	AgentIteration         int       `json:"agentIteration,omitempty"`
	RecoveryAttempts       int       `json:"recoveryAttempts,omitempty"`
	LastOutcome            string    `json:"lastOutcome,omitempty"`
	NextAction             string    `json:"nextAction,omitempty"`
	PassedChecks           []string  `json:"passedChecks,omitempty"`
	RequiredChecks         []string  `json:"requiredChecks,omitempty"`
	VerificationSeen       bool      `json:"verificationSeen,omitempty"`
	DiagnosticRegression   int       `json:"diagnosticRegression,omitempty"`
	DiffObserved           bool      `json:"diffObserved,omitempty"`
	QualityScore           int       `json:"qualityScore,omitempty"`
	QualityStatus          string    `json:"qualityStatus,omitempty"`
	RulesHash              string    `json:"rulesHash,omitempty"`
	ContextHash            string    `json:"contextHash,omitempty"`
	RuleMutationBlocked    bool      `json:"ruleMutationBlocked,omitempty"`
	OmittedRequiredRuleIDs []string  `json:"omittedRequiredRuleIds,omitempty"`
	UpdatedAt              time.Time `json:"updatedAt"`
}

// Patch updates only fields that are meaningful for working memory. Empty
// scalar values are ignored so an incidental tool call cannot erase context.
type Patch struct {
	TaskID                 string
	Task                   string
	Branch                 string
	TouchedFiles           []string
	RecentChecks           []string
	RecentErrors           []string
	LastAction             string
	ReplaceErrors          bool
	AgentPhase             string
	AgentIteration         *int
	RecoveryAttempts       *int
	LastOutcome            string
	NextAction             string
	PassedChecks           []string
	ReplacePassedChecks    bool
	RequiredChecks         []string
	ReplaceRequiredChecks  bool
	VerificationSeen       *bool
	DiagnosticRegression   *int
	DiffObserved           *bool
	QualityScore           *int
	QualityStatus          string
	RulesHash              string
	ContextHash            string
	RuleMutationBlocked    *bool
	OmittedRequiredRuleIDs []string
	ReplaceOmittedRuleIDs  bool
}

type Store struct {
	mu     sync.RWMutex
	states map[string]State
	limit  int
	ttl    time.Duration
}

func New(limit int) *Store {
	return NewWithTTL(limit, 24*time.Hour)
}

func NewWithTTL(limit int, ttl time.Duration) *Store {
	if limit <= 0 {
		limit = 256
	}
	if ttl <= 0 {
		ttl = 24 * time.Hour
	}
	return &Store{states: map[string]State{}, limit: limit, ttl: ttl}
}

func stateKey(userID, sessionID, workspaceKey string) string {
	return strings.TrimSpace(userID) + "\x00" + strings.TrimSpace(sessionID) + "\x00" + strings.TrimSpace(workspaceKey)
}

func cloneState(state State) State {
	state.TouchedFiles = append([]string(nil), state.TouchedFiles...)
	state.RecentChecks = append([]string(nil), state.RecentChecks...)
	state.RecentErrors = append([]string(nil), state.RecentErrors...)
	state.PassedChecks = append([]string(nil), state.PassedChecks...)
	state.RequiredChecks = append([]string(nil), state.RequiredChecks...)
	state.OmittedRequiredRuleIDs = append([]string(nil), state.OmittedRequiredRuleIDs...)
	return state
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

func (s *Store) expired(state State, now time.Time) bool {
	return !state.UpdatedAt.IsZero() && s.ttl > 0 && now.Sub(state.UpdatedAt) > s.ttl
}

func (s *Store) Get(userID, sessionID, workspaceKey string) (State, bool) {
	if s == nil {
		return State{}, false
	}
	key := stateKey(userID, sessionID, workspaceKey)
	s.mu.RLock()
	state, ok := s.states[key]
	expired := ok && s.expired(state, time.Now().UTC())
	if ok && !expired {
		state = cloneState(state)
	}
	s.mu.RUnlock()
	if !expired {
		return state, ok
	}
	s.mu.Lock()
	if current, exists := s.states[key]; exists && s.expired(current, time.Now().UTC()) {
		delete(s.states, key)
	}
	s.mu.Unlock()
	return State{}, false
}

// LatestTask returns the most recently updated non-expired state with a task for
// the user/workspace, regardless of MCP session. maxAge bounds session-rotation
// recovery so unrelated work much later cannot inherit a stale task.
func (s *Store) LatestTask(userID, workspaceKey string, maxAge time.Duration) (State, bool) {
	if s == nil {
		return State{}, false
	}
	userID = strings.TrimSpace(userID)
	workspaceKey = strings.TrimSpace(workspaceKey)
	now := time.Now().UTC()
	s.mu.RLock()
	defer s.mu.RUnlock()
	var latest State
	found := false
	for _, state := range s.states {
		if state.UserID != userID || state.WorkspaceKey != workspaceKey || strings.TrimSpace(state.Task) == "" || s.expired(state, now) {
			continue
		}
		if maxAge > 0 && (state.UpdatedAt.IsZero() || now.Sub(state.UpdatedAt) > maxAge) {
			continue
		}
		if !found || state.UpdatedAt.After(latest.UpdatedAt) {
			latest = state
			found = true
		}
	}
	if !found {
		return State{}, false
	}
	return cloneState(latest), true
}

func newTaskID(userID, sessionID, workspaceKey, task string, now time.Time) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{strings.TrimSpace(userID), strings.TrimSpace(sessionID), strings.TrimSpace(workspaceKey), strings.TrimSpace(task), now.UTC().Format(time.RFC3339Nano)}, "\x00")))
	return "task_" + hex.EncodeToString(sum[:8])
}

func resetAgentState(state *State) {
	if state == nil {
		return
	}
	state.TaskID = ""
	state.TouchedFiles = nil
	state.RecentChecks = nil
	state.RecentErrors = nil
	state.LastAction = ""
	state.AgentPhase = "plan"
	state.AgentIteration = 0
	state.RecoveryAttempts = 0
	state.LastOutcome = ""
	state.NextAction = ""
	state.PassedChecks = nil
	state.RequiredChecks = nil
	state.VerificationSeen = false
	state.DiagnosticRegression = 0
	state.DiffObserved = false
	state.QualityScore = 0
	state.QualityStatus = ""
	state.RulesHash = ""
	state.ContextHash = ""
	state.RuleMutationBlocked = false
	state.OmittedRequiredRuleIDs = nil
}

func (s *Store) Update(userID, sessionID, workspaceKey string, patch Patch) State {
	if s == nil {
		return State{}
	}
	key := stateKey(userID, sessionID, workspaceKey)
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	state := s.states[key]
	if s.expired(state, now) {
		state = State{}
	}
	state.UserID = strings.TrimSpace(userID)
	state.SessionID = strings.TrimSpace(sessionID)
	state.WorkspaceKey = strings.TrimSpace(workspaceKey)
	if value := strings.TrimSpace(patch.Task); value != "" {
		if state.Task != "" && state.Task != value {
			resetAgentState(&state)
		}
		state.Task = value
		if id := strings.TrimSpace(patch.TaskID); id != "" {
			state.TaskID = id
		} else if state.TaskID == "" {
			state.TaskID = newTaskID(userID, sessionID, workspaceKey, value, now)
		}
	} else if id := strings.TrimSpace(patch.TaskID); id != "" && state.TaskID == "" {
		state.TaskID = id
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
	if value := strings.TrimSpace(patch.AgentPhase); value != "" {
		state.AgentPhase = value
	}
	if patch.AgentIteration != nil {
		state.AgentIteration = max(0, *patch.AgentIteration)
	}
	if patch.RecoveryAttempts != nil {
		state.RecoveryAttempts = max(0, *patch.RecoveryAttempts)
	}
	if value := strings.TrimSpace(patch.LastOutcome); value != "" {
		state.LastOutcome = value
	}
	if value := strings.TrimSpace(patch.NextAction); value != "" {
		state.NextAction = value
	}
	if patch.ReplacePassedChecks {
		state.PassedChecks = normalizeList(patch.PassedChecks, 12)
	} else {
		state.PassedChecks = mergeRecent(state.PassedChecks, patch.PassedChecks, 12)
	}
	if patch.ReplaceRequiredChecks {
		state.RequiredChecks = normalizeList(patch.RequiredChecks, 12)
	} else {
		state.RequiredChecks = mergeRecent(state.RequiredChecks, patch.RequiredChecks, 12)
	}
	if patch.VerificationSeen != nil {
		state.VerificationSeen = *patch.VerificationSeen
	}
	if patch.DiagnosticRegression != nil {
		state.DiagnosticRegression = *patch.DiagnosticRegression
	}
	if patch.DiffObserved != nil {
		state.DiffObserved = *patch.DiffObserved
	}
	if patch.QualityScore != nil {
		state.QualityScore = max(0, min(100, *patch.QualityScore))
	}
	if value := strings.TrimSpace(patch.QualityStatus); value != "" {
		state.QualityStatus = value
	}
	if value := strings.TrimSpace(patch.RulesHash); value != "" {
		state.RulesHash = value
	}
	if value := strings.TrimSpace(patch.ContextHash); value != "" {
		state.ContextHash = value
	}
	if patch.RuleMutationBlocked != nil {
		state.RuleMutationBlocked = *patch.RuleMutationBlocked
	}
	if patch.ReplaceOmittedRuleIDs {
		state.OmittedRequiredRuleIDs = normalizeList(patch.OmittedRequiredRuleIDs, 64)
	} else if len(patch.OmittedRequiredRuleIDs) > 0 {
		state.OmittedRequiredRuleIDs = mergeRecent(state.OmittedRequiredRuleIDs, patch.OmittedRequiredRuleIDs, 64)
	}
	state.UpdatedAt = now
	s.states[key] = state
	s.compactLocked(now)
	return cloneState(state)
}

func (s *Store) Delete(userID, sessionID, workspaceKey string) {
	if s == nil {
		return
	}
	s.mu.Lock()
	delete(s.states, stateKey(userID, sessionID, workspaceKey))
	s.mu.Unlock()
}

func (s *Store) compactLocked(now time.Time) {
	for key, state := range s.states {
		if s.expired(state, now) {
			delete(s.states, key)
		}
	}
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

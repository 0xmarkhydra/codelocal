package approval

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/security"
	"github.com/0xmarkhydra/codelocal/internal/state"
)

const approvalStateVersion = 2

type Remembered struct {
	ID              string   `json:"id"`
	WorkspaceKey    string   `json:"workspaceKey"`
	SessionID       string   `json:"sessionId,omitempty"`
	ActionKey       string   `json:"actionKey"`
	Label           string   `json:"label"`
	RedactedCommand string   `json:"redactedCommand"`
	RiskLevel       string   `json:"riskLevel"`
	MatchedRules    []string `json:"matchedRules"`
	CreatedAt       int64    `json:"createdAt"`
	LastUsedAt      int64    `json:"lastUsedAt"`
	ExpiresAt       int64    `json:"expiresAt,omitempty"`
	UseCount        int      `json:"useCount"`
}

type fileState struct {
	Version      int          `json:"version"`
	WorkspaceKey string       `json:"workspaceKey"`
	Approvals    []Remembered `json:"approvals"`
}

type Memory struct {
	RootDir string
	ttl     time.Duration
}

func approvalGrantTTL() time.Duration {
	ttl := 30 * time.Minute
	for _, key := range []string{"CODELOCAL_APPROVAL_GRANT_TTL_MS", "CODELOCAL_CHAT_APPROVAL_TTL_MS"} {
		raw := strings.TrimSpace(os.Getenv(key))
		if raw == "" {
			continue
		}
		if parsed, err := time.ParseDuration(raw + "ms"); err == nil && parsed > 0 {
			return parsed
		}
	}
	return ttl
}

func New() *Memory {
	return &Memory{RootDir: filepath.Join(state.Dir(), "approvals"), ttl: approvalGrantTTL()}
}

func workspaceHash(key string) string {
	sum := sha256.Sum256([]byte(key))
	return hex.EncodeToString(sum[:])[:24]
}

func randomID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}
	return hex.EncodeToString(sha256.New().Sum([]byte(time.Now().String())))
}

func (m *Memory) fileFor(key string) string {
	return filepath.Join(m.RootDir, workspaceHash(key)+".json")
}

func (m *Memory) read(key string) (fileState, error) {
	var data fileState
	if err := state.ReadJSON(m.fileFor(key), &data); err != nil {
		if os.IsNotExist(err) {
			return fileState{Version: approvalStateVersion, WorkspaceKey: key}, nil
		}
		return fileState{}, err
	}
	data.Version = approvalStateVersion
	data.WorkspaceKey = key
	filtered := data.Approvals[:0]
	for _, entry := range data.Approvals {
		if entry.WorkspaceKey == key && entry.ActionKey != "" {
			filtered = append(filtered, entry)
		}
	}
	data.Approvals = filtered
	return data, nil
}

func approvalEntryKey(entry Remembered) string {
	return strings.TrimSpace(entry.SessionID) + "\x00" + strings.TrimSpace(entry.ActionKey)
}

func (m *Memory) write(key string, entries []Remembered) error {
	unique := map[string]Remembered{}
	for _, entry := range entries {
		unique[approvalEntryKey(entry)] = entry
	}
	out := make([]Remembered, 0, len(unique))
	for _, entry := range unique {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsedAt > out[j].LastUsedAt })
	return state.WriteJSONAtomic(m.fileFor(key), fileState{Version: approvalStateVersion, WorkspaceKey: key, Approvals: out})
}

func approvalRiskRank(value security.RiskLevel) int {
	switch value {
	case security.RiskSafe:
		return 0
	case security.RiskReview:
		return 1
	case security.RiskHigh:
		return 2
	case security.RiskCritical:
		return 3
	case security.RiskBlocked:
		return 4
	default:
		return -1
	}
}

func rememberedAllows(entry Remembered, sessionID, actionKey string, currentRisk security.RiskLevel, now int64) bool {
	if strings.TrimSpace(entry.SessionID) == "" || entry.SessionID != strings.TrimSpace(sessionID) {
		return false
	}
	if entry.ActionKey != strings.TrimSpace(actionKey) || entry.ExpiresAt <= now {
		return false
	}
	ceiling := security.RiskLevel(strings.TrimSpace(entry.RiskLevel))
	currentRank, ceilingRank := approvalRiskRank(currentRisk), approvalRiskRank(ceiling)
	return currentRank >= 0 && ceilingRank >= 0 && currentRank <= ceilingRank
}

func (m *Memory) Find(workspaceKey, sessionID, actionKey string, currentRisk security.RiskLevel) (*Remembered, error) {
	data, err := m.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	for _, entry := range data.Approvals {
		if rememberedAllows(entry, sessionID, actionKey, currentRisk, now) {
			copy := entry
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *Memory) Remember(workspaceKey, sessionID string, decision security.Decision) (*Remembered, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" || decision.ApprovalPolicy != security.ApprovalRememberable || decision.ApprovalKey == "" {
		return nil, nil
	}
	data, err := m.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	ttl := m.ttl
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	entry := Remembered{
		ID: randomID(), WorkspaceKey: workspaceKey, SessionID: sessionID, ActionKey: decision.ApprovalKey,
		Label: decision.ApprovalLabel, RedactedCommand: decision.RedactedCommand, RiskLevel: string(decision.RiskLevel),
		MatchedRules: append([]string(nil), decision.MatchedRules...), CreatedAt: now, LastUsedAt: now,
		ExpiresAt: now + ttl.Milliseconds(), UseCount: 1,
	}
	if entry.Label == "" {
		entry.Label = decision.RedactedCommand
	}
	for _, previous := range data.Approvals {
		if previous.SessionID == entry.SessionID && previous.ActionKey == entry.ActionKey {
			entry.ID = previous.ID
			entry.CreatedAt = previous.CreatedAt
			entry.UseCount = previous.UseCount + 1
		}
	}
	next := make([]Remembered, 0, len(data.Approvals)+1)
	for _, item := range data.Approvals {
		if item.SessionID != entry.SessionID || item.ActionKey != entry.ActionKey {
			next = append(next, item)
		}
	}
	next = append(next, entry)
	if err := m.write(workspaceKey, next); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (m *Memory) Touch(workspaceKey, sessionID, actionKey string) (*Remembered, error) {
	data, err := m.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	for i := range data.Approvals {
		if data.Approvals[i].SessionID == strings.TrimSpace(sessionID) && data.Approvals[i].ActionKey == strings.TrimSpace(actionKey) && data.Approvals[i].ExpiresAt > now {
			data.Approvals[i].LastUsedAt = now
			data.Approvals[i].UseCount++
			if err := m.write(workspaceKey, data.Approvals); err != nil {
				return nil, err
			}
			copy := data.Approvals[i]
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *Memory) List(workspaceKey string) ([]Remembered, error) {
	if workspaceKey != "" {
		data, err := m.read(workspaceKey)
		return data.Approvals, err
	}
	names, err := os.ReadDir(m.RootDir)
	if os.IsNotExist(err) {
		return []Remembered{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := []Remembered{}
	for _, name := range names {
		if name.IsDir() || !strings.HasSuffix(name.Name(), ".json") {
			continue
		}
		var data fileState
		if state.ReadJSON(filepath.Join(m.RootDir, name.Name()), &data) != nil || data.WorkspaceKey == "" {
			continue
		}
		for _, entry := range data.Approvals {
			if entry.WorkspaceKey == data.WorkspaceKey && entry.ActionKey != "" {
				out = append(out, entry)
			}
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsedAt > out[j].LastUsedAt })
	return out, nil
}

func (m *Memory) Revoke(identifier, workspaceKey string) (int, error) {
	keys := []string{}
	if workspaceKey != "" {
		keys = append(keys, workspaceKey)
	} else {
		all, err := m.List("")
		if err != nil {
			return 0, err
		}
		seen := map[string]struct{}{}
		for _, entry := range all {
			if _, ok := seen[entry.WorkspaceKey]; !ok {
				seen[entry.WorkspaceKey] = struct{}{}
				keys = append(keys, entry.WorkspaceKey)
			}
		}
	}
	removed := 0
	for _, key := range keys {
		data, err := m.read(key)
		if err != nil {
			return removed, err
		}
		next := make([]Remembered, 0, len(data.Approvals))
		for _, entry := range data.Approvals {
			if entry.ID == identifier || entry.ActionKey == identifier {
				removed++
				continue
			}
			next = append(next, entry)
		}
		if len(next) != len(data.Approvals) {
			if err := m.write(key, next); err != nil {
				return removed, err
			}
		}
	}
	return removed, nil
}

func (m *Memory) Reset(workspaceKey string) (int, error) {
	if workspaceKey != "" {
		data, err := m.read(workspaceKey)
		if err != nil {
			return 0, err
		}
		if err := os.Remove(m.fileFor(workspaceKey)); err != nil && !os.IsNotExist(err) {
			return 0, err
		}
		return len(data.Approvals), nil
	}
	all, err := m.List("")
	if err != nil {
		return 0, err
	}
	if err := os.RemoveAll(m.RootDir); err != nil {
		return 0, err
	}
	return len(all), nil
}

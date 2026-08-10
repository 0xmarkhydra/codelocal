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

type Remembered struct {
	ID              string   `json:"id"`
	WorkspaceKey    string   `json:"workspaceKey"`
	ActionKey       string   `json:"actionKey"`
	Label           string   `json:"label"`
	RedactedCommand string   `json:"redactedCommand"`
	RiskLevel       string   `json:"riskLevel"`
	MatchedRules    []string `json:"matchedRules"`
	CreatedAt       int64    `json:"createdAt"`
	LastUsedAt      int64    `json:"lastUsedAt"`
	UseCount        int      `json:"useCount"`
}

type fileState struct {
	Version      int          `json:"version"`
	WorkspaceKey string       `json:"workspaceKey"`
	Approvals    []Remembered `json:"approvals"`
}

type Memory struct{ RootDir string }

func New() *Memory { return &Memory{RootDir: filepath.Join(state.Dir(), "approvals")} }

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
			return fileState{Version: 1, WorkspaceKey: key}, nil
		}
		return fileState{}, err
	}
	data.Version = 1
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

func (m *Memory) write(key string, entries []Remembered) error {
	unique := map[string]Remembered{}
	for _, entry := range entries {
		unique[entry.ActionKey] = entry
	}
	out := make([]Remembered, 0, len(unique))
	for _, entry := range unique {
		out = append(out, entry)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].LastUsedAt > out[j].LastUsedAt })
	return state.WriteJSONAtomic(m.fileFor(key), fileState{Version: 1, WorkspaceKey: key, Approvals: out})
}

func (m *Memory) Find(workspaceKey, actionKey string) (*Remembered, error) {
	data, err := m.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	for _, entry := range data.Approvals {
		if entry.ActionKey == actionKey {
			copy := entry
			return &copy, nil
		}
	}
	return nil, nil
}

func (m *Memory) Remember(workspaceKey string, decision security.Decision) (*Remembered, error) {
	if decision.ApprovalPolicy != security.ApprovalRememberable || decision.ApprovalKey == "" {
		return nil, nil
	}
	data, err := m.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	entry := Remembered{ID: randomID(), WorkspaceKey: workspaceKey, ActionKey: decision.ApprovalKey, Label: decision.ApprovalLabel, RedactedCommand: decision.RedactedCommand, RiskLevel: string(decision.RiskLevel), MatchedRules: append([]string(nil), decision.MatchedRules...), CreatedAt: now, LastUsedAt: now, UseCount: 1}
	if entry.Label == "" {
		entry.Label = decision.RedactedCommand
	}
	for _, previous := range data.Approvals {
		if previous.ActionKey == entry.ActionKey {
			entry.ID = previous.ID
			entry.CreatedAt = previous.CreatedAt
			entry.UseCount = previous.UseCount + 1
		}
	}
	next := make([]Remembered, 0, len(data.Approvals)+1)
	for _, item := range data.Approvals {
		if item.ActionKey != entry.ActionKey {
			next = append(next, item)
		}
	}
	next = append(next, entry)
	if err := m.write(workspaceKey, next); err != nil {
		return nil, err
	}
	return &entry, nil
}

func (m *Memory) Touch(workspaceKey, actionKey string) (*Remembered, error) {
	data, err := m.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	for i := range data.Approvals {
		if data.Approvals[i].ActionKey == actionKey {
			data.Approvals[i].LastUsedAt = time.Now().UnixMilli()
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

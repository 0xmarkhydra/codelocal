package projectbrain

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
	"github.com/0xmarkhydra/codelocal/internal/state"
)

const syncStateVersion = 1

type WorkspaceSyncState struct {
	WorkspaceID     string                   `json:"workspaceId"`
	ProjectIdentity projectidentity.Snapshot `json:"projectIdentity,omitempty"`
	Manifest        Manifest                 `json:"manifest"`
	BaseRevisions   map[string]string        `json:"baseRevisions,omitempty"`
	SyncedRoot      string                   `json:"syncedRoot,omitempty"`
	UpdatedAt       int64                    `json:"updatedAt"`
}

type syncStateFile struct {
	Version    int                           `json:"version"`
	Workspaces map[string]WorkspaceSyncState `json:"workspaces"`
}

type SyncStateStore struct {
	Path string
	mu   sync.Mutex
}

func NewSyncStateStore() *SyncStateStore {
	return &SyncStateStore{Path: filepath.Join(state.Dir(), "project-brain", "sync-state-v1.json")}
}

func NewSyncStateStoreAt(path string) *SyncStateStore { return &SyncStateStore{Path: path} }

func cloneWorkspaceSyncState(input WorkspaceSyncState) WorkspaceSyncState {
	raw, _ := json.Marshal(input)
	var out WorkspaceSyncState
	_ = json.Unmarshal(raw, &out)
	if out.BaseRevisions == nil {
		out.BaseRevisions = map[string]string{}
	}
	return out
}

func (s *SyncStateStore) readLocked() (syncStateFile, error) {
	data := syncStateFile{Version: syncStateVersion, Workspaces: map[string]WorkspaceSyncState{}}
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return data, nil
	}
	if err := state.ReadJSON(s.Path, &data); err != nil {
		if os.IsNotExist(err) {
			return syncStateFile{Version: syncStateVersion, Workspaces: map[string]WorkspaceSyncState{}}, nil
		}
		return syncStateFile{}, err
	}
	data.Version = syncStateVersion
	if data.Workspaces == nil {
		data.Workspaces = map[string]WorkspaceSyncState{}
	}
	return data, nil
}

func (s *SyncStateStore) Workspace(workspaceID string) (WorkspaceSyncState, bool, error) {
	workspaceID = strings.TrimSpace(workspaceID)
	if workspaceID == "" {
		return WorkspaceSyncState{}, false, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.readLocked()
	if err != nil {
		return WorkspaceSyncState{}, false, err
	}
	value, ok := data.Workspaces[workspaceID]
	if !ok {
		return WorkspaceSyncState{}, false, nil
	}
	return cloneWorkspaceSyncState(value), true, nil
}

func (s *SyncStateStore) Put(value WorkspaceSyncState) error {
	value.WorkspaceID = strings.TrimSpace(value.WorkspaceID)
	if value.WorkspaceID == "" || s == nil || strings.TrimSpace(s.Path) == "" {
		return nil
	}
	if value.BaseRevisions == nil {
		value.BaseRevisions = map[string]string{}
	}
	if value.UpdatedAt <= 0 {
		value.UpdatedAt = time.Now().UnixMilli()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.readLocked()
	if err != nil {
		return err
	}
	data.Workspaces[value.WorkspaceID] = cloneWorkspaceSyncState(value)
	return state.WriteJSONAtomic(s.Path, data)
}

func (s *SyncStateStore) Prune(authorized []string) error {
	if s == nil || strings.TrimSpace(s.Path) == "" {
		return nil
	}
	allowed := map[string]struct{}{}
	for _, workspaceID := range authorized {
		if workspaceID = strings.TrimSpace(workspaceID); workspaceID != "" {
			allowed[workspaceID] = struct{}{}
		}
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.readLocked()
	if err != nil {
		return err
	}
	changed := false
	for workspaceID := range data.Workspaces {
		if _, ok := allowed[workspaceID]; !ok {
			delete(data.Workspaces, workspaceID)
			changed = true
		}
	}
	if !changed {
		return nil
	}
	return state.WriteJSONAtomic(s.Path, data)
}

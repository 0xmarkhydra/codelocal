package taskexecution

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	codelocalstate "github.com/0xmarkhydra/codelocal/internal/state"
)

var ErrLeaseHeld = errors.New("task execution bundle is leased by another owner")

type Store struct {
	root string
	mu   sync.Mutex
}

func NewStore(root string) *Store {
	root = strings.TrimSpace(root)
	if root == "" {
		root = filepath.Join(codelocalstate.Dir(), "task-execution")
	}
	return &Store{root: root}
}

func digestKey(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:24]
}

func (s *Store) bundlePath(workspaceKey, taskID string) string {
	workspace := digestKey("workspace", workspaceKey)
	task := digestKey("task", taskID)
	return filepath.Join(s.root, workspace, task+".json")
}

func validateBundle(bundle Bundle) error {
	if strings.TrimSpace(bundle.TaskID) == "" || strings.TrimSpace(bundle.WorkspaceKey) == "" {
		return errors.New("taskId and workspaceKey are required")
	}
	if bundle.Provider == "" {
		return errors.New("execution provider is required")
	}
	return nil
}

func (s *Store) Put(bundle Bundle) (Bundle, error) {
	if err := validateBundle(bundle); err != nil {
		return Bundle{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	now := time.Now().UTC()
	if bundle.SchemaVersion == 0 {
		bundle.SchemaVersion = 1
	}
	if bundle.ID == "" {
		bundle.ID = "exec_" + digestKey(bundle.WorkspaceKey, bundle.TaskID)
	}
	if bundle.CreatedAt.IsZero() {
		bundle.CreatedAt = now
	}
	bundle.UpdatedAt = now
	if err := codelocalstate.WriteJSONAtomic(s.bundlePath(bundle.WorkspaceKey, bundle.TaskID), bundle); err != nil {
		return Bundle{}, err
	}
	return bundle.Clone(), nil
}

func (s *Store) Get(workspaceKey, taskID string) (Bundle, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	var bundle Bundle
	if err := codelocalstate.ReadJSON(s.bundlePath(workspaceKey, taskID), &bundle); err != nil {
		if os.IsNotExist(err) {
			return Bundle{}, false, nil
		}
		return Bundle{}, false, err
	}
	return bundle.Clone(), true, nil
}

func (s *Store) Delete(workspaceKey, taskID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := os.Remove(s.bundlePath(workspaceKey, taskID))
	if os.IsNotExist(err) {
		return nil
	}
	return err
}

func (s *Store) Claim(workspaceKey, taskID, ownerID string, ttl time.Duration) (Bundle, error) {
	ownerID = strings.TrimSpace(ownerID)
	if ownerID == "" {
		return Bundle{}, errors.New("lease owner is required")
	}
	if ttl <= 0 {
		ttl = 15 * time.Minute
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.bundlePath(workspaceKey, taskID)
	var bundle Bundle
	if err := codelocalstate.ReadJSON(path, &bundle); err != nil {
		return Bundle{}, err
	}
	now := time.Now().UTC()
	if bundle.Lease.OwnerID != "" && bundle.Lease.OwnerID != ownerID && bundle.Lease.ExpiresAt.After(now) {
		return Bundle{}, ErrLeaseHeld
	}
	bundle.Lease = Lease{OwnerID: ownerID, ExpiresAt: now.Add(ttl)}
	bundle.UpdatedAt = now
	if err := codelocalstate.WriteJSONAtomic(path, bundle); err != nil {
		return Bundle{}, err
	}
	return bundle.Clone(), nil
}

func (s *Store) Release(workspaceKey, taskID, ownerID string) (Bundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.bundlePath(workspaceKey, taskID)
	var bundle Bundle
	if err := codelocalstate.ReadJSON(path, &bundle); err != nil {
		return Bundle{}, err
	}
	ownerID = strings.TrimSpace(ownerID)
	if bundle.Lease.OwnerID != "" && bundle.Lease.OwnerID != ownerID {
		return Bundle{}, ErrLeaseHeld
	}
	bundle.Lease = Lease{}
	bundle.UpdatedAt = time.Now().UTC()
	if err := codelocalstate.WriteJSONAtomic(path, bundle); err != nil {
		return Bundle{}, err
	}
	return bundle.Clone(), nil
}

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

var (
	ErrLeaseHeld        = errors.New("task execution bundle is leased by another owner")
	ErrLeaseGeneration  = errors.New("task execution lease generation is stale")
	ErrRevisionConflict = errors.New("task execution bundle revision conflict")
)

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

func normalizedRevision(bundle Bundle) uint64 {
	if bundle.Revision == 0 {
		return 1
	}
	return bundle.Revision
}

func readBundle(path string) (Bundle, bool, error) {
	var bundle Bundle
	if err := codelocalstate.ReadJSON(path, &bundle); err != nil {
		if os.IsNotExist(err) {
			return Bundle{}, false, nil
		}
		return Bundle{}, false, err
	}
	bundle.Revision = normalizedRevision(bundle)
	return bundle, true, nil
}

func prepareBundle(bundle Bundle, existing *Bundle, now time.Time) Bundle {
	if bundle.SchemaVersion == 0 {
		bundle.SchemaVersion = 1
	}
	if bundle.ID == "" {
		if existing != nil && existing.ID != "" {
			bundle.ID = existing.ID
		} else {
			bundle.ID = "exec_" + digestKey(bundle.WorkspaceKey, bundle.TaskID)
		}
	}
	if bundle.CreatedAt.IsZero() {
		if existing != nil && !existing.CreatedAt.IsZero() {
			bundle.CreatedAt = existing.CreatedAt
		} else {
			bundle.CreatedAt = now
		}
	}
	if existing == nil {
		bundle.Revision = 1
	} else {
		bundle.Revision = normalizedRevision(*existing) + 1
	}
	bundle.UpdatedAt = now
	return bundle
}

// Put is the backwards-compatible unconditional writer. New V3 mutation paths
// should use PutCAS so stale task projections cannot overwrite newer state.
func (s *Store) Put(bundle Bundle) (Bundle, error) {
	if err := validateBundle(bundle); err != nil {
		return Bundle{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.bundlePath(bundle.WorkspaceKey, bundle.TaskID)
	existing, found, err := readBundle(path)
	if err != nil {
		return Bundle{}, err
	}
	now := time.Now().UTC()
	if found {
		bundle = prepareBundle(bundle, &existing, now)
	} else {
		bundle = prepareBundle(bundle, nil, now)
	}
	if err := codelocalstate.WriteJSONAtomic(path, bundle); err != nil {
		return Bundle{}, err
	}
	return bundle.Clone(), nil
}

// PutCAS writes only when expectedRevision matches the durable bundle revision.
// expectedRevision=0 means create-only. This is the V3 mutation primitive.
func (s *Store) PutCAS(bundle Bundle, expectedRevision uint64) (Bundle, error) {
	if err := validateBundle(bundle); err != nil {
		return Bundle{}, err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.bundlePath(bundle.WorkspaceKey, bundle.TaskID)
	existing, found, err := readBundle(path)
	if err != nil {
		return Bundle{}, err
	}
	if !found {
		if expectedRevision != 0 {
			return Bundle{}, ErrRevisionConflict
		}
		bundle = prepareBundle(bundle, nil, time.Now().UTC())
	} else {
		if normalizedRevision(existing) != expectedRevision {
			return Bundle{}, ErrRevisionConflict
		}
		bundle = prepareBundle(bundle, &existing, time.Now().UTC())
	}
	if err := codelocalstate.WriteJSONAtomic(path, bundle); err != nil {
		return Bundle{}, err
	}
	return bundle.Clone(), nil
}

func (s *Store) Get(workspaceKey, taskID string) (Bundle, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	bundle, found, err := readBundle(s.bundlePath(workspaceKey, taskID))
	if err != nil || !found {
		return Bundle{}, found, err
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
	bundle, found, err := readBundle(path)
	if err != nil {
		return Bundle{}, err
	}
	if !found {
		return Bundle{}, os.ErrNotExist
	}
	now := time.Now().UTC()
	if bundle.Lease.OwnerID != "" && bundle.Lease.OwnerID != ownerID && bundle.Lease.ExpiresAt.After(now) {
		return Bundle{}, ErrLeaseHeld
	}
	generation := bundle.Lease.Generation + 1
	if generation == 0 {
		generation = 1
	}
	bundle.Lease = Lease{OwnerID: ownerID, Generation: generation, ExpiresAt: now.Add(ttl)}
	bundle.Revision = normalizedRevision(bundle) + 1
	bundle.UpdatedAt = now
	if err := codelocalstate.WriteJSONAtomic(path, bundle); err != nil {
		return Bundle{}, err
	}
	return bundle.Clone(), nil
}

// Release is retained for compatibility. V3 workers should call ReleaseLease
// with the generation they acquired, which fences stale owners after takeover.
func (s *Store) Release(workspaceKey, taskID, ownerID string) (Bundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.bundlePath(workspaceKey, taskID)
	bundle, found, err := readBundle(path)
	if err != nil {
		return Bundle{}, err
	}
	if !found {
		return Bundle{}, os.ErrNotExist
	}
	ownerID = strings.TrimSpace(ownerID)
	if bundle.Lease.OwnerID != "" && bundle.Lease.OwnerID != ownerID {
		return Bundle{}, ErrLeaseHeld
	}
	bundle.Lease.OwnerID = ""
	bundle.Lease.ExpiresAt = time.Time{}
	bundle.Revision = normalizedRevision(bundle) + 1
	bundle.UpdatedAt = time.Now().UTC()
	if err := codelocalstate.WriteJSONAtomic(path, bundle); err != nil {
		return Bundle{}, err
	}
	return bundle.Clone(), nil
}

func (s *Store) ReleaseLease(workspaceKey, taskID, ownerID string, generation uint64) (Bundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path := s.bundlePath(workspaceKey, taskID)
	bundle, found, err := readBundle(path)
	if err != nil {
		return Bundle{}, err
	}
	if !found {
		return Bundle{}, os.ErrNotExist
	}
	ownerID = strings.TrimSpace(ownerID)
	if bundle.Lease.OwnerID != ownerID {
		return Bundle{}, ErrLeaseHeld
	}
	if generation == 0 || bundle.Lease.Generation != generation {
		return Bundle{}, ErrLeaseGeneration
	}
	bundle.Lease.OwnerID = ""
	bundle.Lease.ExpiresAt = time.Time{}
	bundle.Revision = normalizedRevision(bundle) + 1
	bundle.UpdatedAt = time.Now().UTC()
	if err := codelocalstate.WriteJSONAtomic(path, bundle); err != nil {
		return Bundle{}, err
	}
	return bundle.Clone(), nil
}

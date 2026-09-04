package editing

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
	"github.com/0xmarkhydra/codelocal/internal/security"
	codelocalstate "github.com/0xmarkhydra/codelocal/internal/state"
)

type SavepointState string

const (
	SavepointCaptured SavepointState = "captured"
	SavepointSealed   SavepointState = "sealed"
)

var (
	ErrInvalidSavepoint = errors.New("invalid savepoint")
	ErrSavepointMissing = errors.New("savepoint not found")
	ErrSavepointStale   = errors.New("stale savepoint revision or workspace state")
	ErrSavepointState   = errors.New("invalid savepoint state")
	ErrSavepointBlob    = errors.New("savepoint blob is corrupt")
)

type SavepointProvenance struct {
	TaskID          string `json:"taskId"`
	AgentID         string `json:"agentId,omitempty"`
	ActivationID    string `json:"activationId,omitempty"`
	TraceID         string `json:"traceId,omitempty"`
	RuntimeSequence uint64 `json:"runtimeSequence,omitempty"`
}

type SavepointEntry struct {
	Path          string      `json:"path"`
	ExistedBefore bool        `json:"existedBefore"`
	BeforeHash    string      `json:"beforeHash,omitempty"`
	BeforeBlob    string      `json:"beforeBlob,omitempty"`
	BeforeMode    os.FileMode `json:"beforeMode,omitempty"`
	ExistsAfter   bool        `json:"existsAfter"`
	AfterHash     string      `json:"afterHash,omitempty"`
	AfterBlob     string      `json:"afterBlob,omitempty"`
	AfterMode     os.FileMode `json:"afterMode,omitempty"`
}

type Savepoint struct {
	ID           string              `json:"id"`
	WorkspaceKey string              `json:"workspaceKey"`
	Revision     uint64              `json:"revision"`
	State        SavepointState      `json:"state"`
	Provenance   SavepointProvenance `json:"provenance"`
	Entries      []SavepointEntry    `json:"entries"`
	CreatedAt    time.Time           `json:"createdAt"`
	UpdatedAt    time.Time           `json:"updatedAt"`
}

type SavepointCaptureRequest struct {
	ID           string
	WorkspaceKey string
	Provenance   SavepointProvenance
	Paths        []string
}

type SavepointStore struct {
	root string
	fs   *localfs.FS
}

func NewSavepointStore(root string, fs *localfs.FS) (*SavepointStore, error) {
	if fs == nil || strings.TrimSpace(fs.Root) == "" {
		return nil, ErrInvalidSavepoint
	}
	root = strings.TrimSpace(root)
	if root == "" {
		root = filepath.Join(codelocalstate.Dir(), "savepoints")
	}
	if err := codelocalstate.EnsurePrivateDir(root); err != nil {
		return nil, err
	}
	return &SavepointStore{root: root, fs: fs}, nil
}

func (s *SavepointStore) Capture(req SavepointCaptureRequest) (Savepoint, error) {
	req.ID = strings.TrimSpace(req.ID)
	req.WorkspaceKey = strings.TrimSpace(req.WorkspaceKey)
	req.Provenance.TaskID = strings.TrimSpace(req.Provenance.TaskID)
	if req.ID == "" || req.WorkspaceKey == "" || req.Provenance.TaskID == "" || len(req.Paths) == 0 || len(req.Paths) > 100 {
		return Savepoint{}, ErrInvalidSavepoint
	}
	if _, found, err := s.Load(req.WorkspaceKey, req.ID); err != nil {
		return Savepoint{}, err
	} else if found {
		return Savepoint{}, ErrInvalidSavepoint
	}
	paths, err := normalizeSavepointPaths(req.Paths)
	if err != nil {
		return Savepoint{}, err
	}
	now := time.Now().UTC()
	point := Savepoint{ID: req.ID, WorkspaceKey: req.WorkspaceKey, Revision: 1, State: SavepointCaptured, Provenance: req.Provenance, CreatedAt: now, UpdatedAt: now}
	for _, path := range paths {
		data, mode, exists, err := s.readWorkspace(path)
		if err != nil {
			return Savepoint{}, err
		}
		entry := SavepointEntry{Path: path, ExistedBefore: exists}
		if exists {
			entry.BeforeHash = localfs.Hash(data)
			entry.BeforeBlob, err = s.storeBlob(req.WorkspaceKey, data)
			if err != nil {
				return Savepoint{}, err
			}
			entry.BeforeMode = mode
		}
		point.Entries = append(point.Entries, entry)
	}
	if err := s.writeManifest(point); err != nil {
		return Savepoint{}, err
	}
	return cloneSavepoint(point), nil
}

// Seal records the exact agent-produced post-state. expectedAfter must contain
// every captured path; an empty hash means the agent expected the path absent.
// The current workspace must exactly match these expected outcomes, otherwise a
// concurrent user edit is fenced out and the savepoint remains unsealed.
func (s *SavepointStore) Seal(workspaceKey, id string, expectedRevision uint64, expectedAfter map[string]string) (Savepoint, error) {
	point, found, err := s.Load(workspaceKey, id)
	if err != nil {
		return Savepoint{}, err
	}
	if !found {
		return Savepoint{}, ErrSavepointMissing
	}
	if expectedRevision == 0 || point.Revision != expectedRevision {
		return Savepoint{}, ErrSavepointStale
	}
	if point.State != SavepointCaptured {
		return Savepoint{}, ErrSavepointState
	}
	if len(expectedAfter) != len(point.Entries) {
		return Savepoint{}, ErrInvalidSavepoint
	}
	for index := range point.Entries {
		entry := &point.Entries[index]
		expectedHash, ok := expectedAfter[entry.Path]
		if !ok {
			return Savepoint{}, ErrInvalidSavepoint
		}
		expectedHash = strings.TrimSpace(expectedHash)
		data, mode, exists, err := s.readWorkspace(entry.Path)
		if err != nil {
			return Savepoint{}, err
		}
		if exists != (expectedHash != "") {
			return Savepoint{}, ErrSavepointStale
		}
		if exists && localfs.Hash(data) != expectedHash {
			return Savepoint{}, ErrSavepointStale
		}
		entry.ExistsAfter = exists
		if exists {
			entry.AfterHash = expectedHash
			entry.AfterBlob, err = s.storeBlob(point.WorkspaceKey, data)
			if err != nil {
				return Savepoint{}, err
			}
			entry.AfterMode = mode
		}
	}
	point.Revision++
	point.State = SavepointSealed
	point.UpdatedAt = time.Now().UTC()
	if err := s.writeManifest(point); err != nil {
		return Savepoint{}, err
	}
	return cloneSavepoint(point), nil
}

func (s *SavepointStore) Load(workspaceKey, id string) (Savepoint, bool, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	id = strings.TrimSpace(id)
	if workspaceKey == "" || id == "" {
		return Savepoint{}, false, ErrInvalidSavepoint
	}
	var point Savepoint
	if err := codelocalstate.ReadJSON(s.manifestPath(workspaceKey, id), &point); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return Savepoint{}, false, nil
		}
		return Savepoint{}, false, err
	}
	if err := validateSavepoint(point, workspaceKey, id); err != nil {
		return Savepoint{}, false, err
	}
	return cloneSavepoint(point), true, nil
}

func (s *SavepointStore) readWorkspace(relative string) ([]byte, os.FileMode, bool, error) {
	if security.IsSensitivePath(relative) {
		return nil, 0, false, fmt.Errorf("access blocked by sensitive-path policy: %s", relative)
	}
	path, err := s.fs.Existing(relative)
	if err != nil {
		if os.IsNotExist(err) {
			if err := validateAbsentWorkspacePath(s.fs.Root, relative); err != nil {
				return nil, 0, false, err
			}
			return nil, 0, false, nil
		}
		return nil, 0, false, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return nil, 0, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, 0, false, ErrInvalidSavepoint
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, 0, false, err
	}
	if len(data) > maxReconcileFileBytes {
		return nil, 0, false, ErrReconcileTooLarge
	}
	return data, info.Mode().Perm(), true, nil
}

func validateAbsentWorkspacePath(root, relative string) error {
	relative = strings.TrimSpace(filepath.ToSlash(relative))
	if relative == "" || filepath.IsAbs(relative) || relative == ".." || strings.HasPrefix(relative, "../") || strings.Contains(relative, "/../") {
		return ErrInvalidSavepoint
	}
	candidate := filepath.Clean(filepath.Join(root, filepath.FromSlash(relative)))
	rel, err := filepath.Rel(root, candidate)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return ErrInvalidSavepoint
	}
	ancestor := filepath.Dir(candidate)
	for {
		real, err := filepath.EvalSymlinks(ancestor)
		if err == nil {
			realRel, relErr := filepath.Rel(root, real)
			if relErr != nil || realRel == ".." || strings.HasPrefix(realRel, ".."+string(filepath.Separator)) || filepath.IsAbs(realRel) {
				return ErrInvalidSavepoint
			}
			return nil
		}
		if !os.IsNotExist(err) {
			return err
		}
		next := filepath.Dir(ancestor)
		if next == ancestor {
			return ErrInvalidSavepoint
		}
		ancestor = next
	}
}

func normalizeSavepointPaths(paths []string) ([]string, error) {
	seen := map[string]struct{}{}
	out := []string{}
	for _, path := range paths {
		normalized, err := normalizeReconcilePath(path)
		if err != nil || security.IsSensitivePath(normalized) {
			return nil, ErrInvalidSavepoint
		}
		if _, ok := seen[normalized]; ok {
			continue
		}
		seen[normalized] = struct{}{}
		out = append(out, normalized)
	}
	if len(out) == 0 {
		return nil, ErrInvalidSavepoint
	}
	sort.Strings(out)
	return out, nil
}

func (s *SavepointStore) storeBlob(workspaceKey string, data []byte) (string, error) {
	hash := localfs.Hash(data)
	path := filepath.Join(s.workspaceDir(workspaceKey), "blobs", hash+".blob")
	if existing, err := os.ReadFile(path); err == nil {
		if localfs.Hash(existing) != hash {
			return "", ErrSavepointBlob
		}
		return hash, nil
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if err := codelocalstate.EnsurePrivateDir(filepath.Dir(path)); err != nil {
		return "", err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".blob-*.tmp")
	if err != nil {
		return "", err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if err := tmp.Chmod(0o600); err != nil {
		tmp.Close()
		return "", err
	}
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Sync(); err != nil {
		tmp.Close()
		return "", err
	}
	if err := tmp.Close(); err != nil {
		return "", err
	}
	if err := os.Rename(tmpName, path); err != nil {
		return "", err
	}
	if err := os.Chmod(path, 0o600); err != nil && !errors.Is(err, os.ErrPermission) {
		return "", err
	}
	return hash, nil
}

func (s *SavepointStore) loadBlob(workspaceKey, hash string) ([]byte, error) {
	hash = strings.TrimSpace(hash)
	if len(hash) != 64 {
		return nil, ErrSavepointBlob
	}
	data, err := os.ReadFile(filepath.Join(s.workspaceDir(workspaceKey), "blobs", hash+".blob"))
	if err != nil {
		return nil, err
	}
	if localfs.Hash(data) != hash {
		return nil, ErrSavepointBlob
	}
	return data, nil
}

func (s *SavepointStore) workspaceDir(workspaceKey string) string {
	return filepath.Join(s.root, "workspace_"+savepointDigest(workspaceKey))
}

func (s *SavepointStore) manifestPath(workspaceKey, id string) string {
	return filepath.Join(s.workspaceDir(workspaceKey), "manifests", "savepoint_"+savepointDigest(id)+".json")
}

func (s *SavepointStore) writeManifest(point Savepoint) error {
	return codelocalstate.WriteJSONAtomic(s.manifestPath(point.WorkspaceKey, point.ID), point)
}

func validateSavepoint(point Savepoint, workspaceKey, id string) error {
	if point.ID != id || point.WorkspaceKey != workspaceKey || point.Revision == 0 || point.CreatedAt.IsZero() || point.UpdatedAt.IsZero() || point.Provenance.TaskID == "" || len(point.Entries) == 0 {
		return ErrInvalidSavepoint
	}
	if point.State != SavepointCaptured && point.State != SavepointSealed {
		return ErrInvalidSavepoint
	}
	for _, entry := range point.Entries {
		if _, err := normalizeReconcilePath(entry.Path); err != nil {
			return ErrInvalidSavepoint
		}
		if entry.ExistedBefore && (entry.BeforeHash == "" || entry.BeforeBlob == "") {
			return ErrInvalidSavepoint
		}
		if point.State == SavepointSealed && entry.ExistsAfter && (entry.AfterHash == "" || entry.AfterBlob == "") {
			return ErrInvalidSavepoint
		}
	}
	return nil
}

func cloneSavepoint(point Savepoint) Savepoint {
	point.Entries = append([]SavepointEntry(nil), point.Entries...)
	return point
}

func savepointDigest(value string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(value)))
	return hex.EncodeToString(sum[:])[:24]
}

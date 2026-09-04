package editing

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
	"github.com/0xmarkhydra/codelocal/internal/security"
)

type PatchState string

const (
	PatchProposed   PatchState = "proposed"
	PatchValidated  PatchState = "validated"
	PatchStale      PatchState = "stale"
	PatchApplied    PatchState = "applied"
	PatchReverted   PatchState = "reverted"
	PatchConflicted PatchState = "conflicted"
	PatchRejected   PatchState = "rejected"
)

var (
	ErrInvalidPatchSet = errors.New("invalid patch set")
	ErrPatchStaleBase  = errors.New("patch set has a stale base")
	ErrPatchConflict   = errors.New("patch set conflicts with current workspace state")
)

// PatchFile is deliberately optimistic-concurrency-first. Every file must state
// either the exact hash it was based on or that the path was expected to be
// absent. Unguarded writes are not valid PatchSet V2 mutations.
type PatchFile struct {
	Path           string `json:"path"`
	ExpectedHash   string `json:"expectedHash,omitempty"`
	ExpectedAbsent bool   `json:"expectedAbsent,omitempty"`
	Edits          []Edit `json:"edits,omitempty"`
	Content        *string `json:"content,omitempty"`
}

type PatchProvenance struct {
	TaskID       string `json:"taskId"`
	AgentID      string `json:"agentId,omitempty"`
	ActivationID string `json:"activationId,omitempty"`
	WorktreeID   string `json:"worktreeId,omitempty"`
	TraceID      string `json:"traceId,omitempty"`
}

type PatchSet struct {
	ID               string          `json:"id"`
	BaseSHA          string          `json:"baseSha,omitempty"`
	Provenance       PatchProvenance `json:"provenance"`
	Files            []PatchFile     `json:"files"`
	State            PatchState      `json:"state"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	VerificationRefs []string        `json:"verificationRefs,omitempty"`
	Failure          string          `json:"failure,omitempty"`
}

type patchBackup struct {
	path   string
	before []byte
	mode   os.FileMode
}

type patchCreate struct {
	path    string
	content []byte
	mode    os.FileMode
}

func normalizePatchSet(set PatchSet) (PatchSet, error) {
	set.ID = strings.TrimSpace(set.ID)
	set.BaseSHA = strings.TrimSpace(set.BaseSHA)
	set.Provenance.TaskID = strings.TrimSpace(set.Provenance.TaskID)
	set.Provenance.AgentID = strings.TrimSpace(set.Provenance.AgentID)
	set.Provenance.ActivationID = strings.TrimSpace(set.Provenance.ActivationID)
	set.Provenance.WorktreeID = strings.TrimSpace(set.Provenance.WorktreeID)
	set.Provenance.TraceID = strings.TrimSpace(set.Provenance.TraceID)
	if set.State == "" {
		set.State = PatchProposed
	}
	if set.ID == "" || set.Provenance.TaskID == "" || len(set.Files) == 0 || len(set.Files) > 100 {
		return PatchSet{}, ErrInvalidPatchSet
	}
	seen := map[string]struct{}{}
	for index := range set.Files {
		file := &set.Files[index]
		file.Path = strings.TrimSpace(filepath.ToSlash(file.Path))
		file.ExpectedHash = strings.TrimSpace(file.ExpectedHash)
		if file.Path == "" || filepath.IsAbs(file.Path) || file.Path == ".." || strings.HasPrefix(file.Path, "../") || strings.Contains(file.Path, "/../") {
			return PatchSet{}, fmt.Errorf("%w: unsafe path", ErrInvalidPatchSet)
		}
		if security.IsSensitivePath(file.Path) {
			return PatchSet{}, fmt.Errorf("%w: sensitive path %s", ErrInvalidPatchSet, file.Path)
		}
		if _, exists := seen[file.Path]; exists {
			return PatchSet{}, fmt.Errorf("%w: duplicate path %s", ErrInvalidPatchSet, file.Path)
		}
		seen[file.Path] = struct{}{}
		if file.ExpectedAbsent == (file.ExpectedHash != "") {
			return PatchSet{}, fmt.Errorf("%w: %s must specify exactly one of expectedHash or expectedAbsent", ErrInvalidPatchSet, file.Path)
		}
		if file.Content != nil && len(file.Edits) > 0 {
			return PatchSet{}, fmt.Errorf("%w: %s cannot use content and edits together", ErrInvalidPatchSet, file.Path)
		}
		if file.ExpectedAbsent && file.Content == nil {
			return PatchSet{}, fmt.Errorf("%w: new file %s requires content", ErrInvalidPatchSet, file.Path)
		}
		if !file.ExpectedAbsent && file.Content == nil && len(file.Edits) == 0 {
			return PatchSet{}, fmt.Errorf("%w: existing file %s requires edits or content", ErrInvalidPatchSet, file.Path)
		}
	}
	now := time.Now().UTC()
	if set.CreatedAt.IsZero() {
		set.CreatedAt = now
	} else {
		set.CreatedAt = set.CreatedAt.UTC()
	}
	set.UpdatedAt = now
	return set, nil
}

func patchFail(set PatchSet, state PatchState, err error) (PatchSet, error) {
	set.State = state
	set.UpdatedAt = time.Now().UTC()
	set.Failure = err.Error()
	return set, err
}

// ValidatePatchSet validates current workspace state without mutation. It is
// intentionally repeated during ApplyPatchSet; validation evidence is useful
// for preview/UI but is never treated as a lock or write authorization.
func (e *Engine) ValidatePatchSet(input PatchSet) (PatchSet, error) {
	set, err := normalizePatchSet(input)
	if err != nil {
		return patchFail(input, PatchRejected, err)
	}
	for _, file := range set.Files {
		path, err := e.FS.WritePath(file.Path)
		if err != nil {
			return patchFail(set, PatchRejected, err)
		}
		data, readErr := os.ReadFile(path)
		if file.ExpectedAbsent {
			if readErr == nil {
				return patchFail(set, PatchConflicted, fmt.Errorf("%w: %s now exists", ErrPatchConflict, file.Path))
			}
			if !os.IsNotExist(readErr) {
				return patchFail(set, PatchRejected, readErr)
			}
			continue
		}
		if readErr != nil {
			if os.IsNotExist(readErr) {
				return patchFail(set, PatchStale, fmt.Errorf("%w: %s no longer exists", ErrPatchStaleBase, file.Path))
			}
			return patchFail(set, PatchRejected, readErr)
		}
		if localfs.Hash(data) != file.ExpectedHash {
			return patchFail(set, PatchStale, fmt.Errorf("%w: %s hash mismatch", ErrPatchStaleBase, file.Path))
		}
	}
	set.State = PatchValidated
	set.Failure = ""
	set.UpdatedAt = time.Now().UTC()
	return set, nil
}

func fullReplacementEdit(length int, content string) Edit {
	start, end := 0, length
	return Edit{StartOffset: &start, EndOffset: &end, Replacement: content}
}

func writeNewExclusive(path string, data []byte, mode os.FileMode) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		if os.IsExist(err) {
			return ErrPatchConflict
		}
		return err
	}
	remove := true
	defer func() {
		_ = f.Close()
		if remove {
			_ = os.Remove(path)
		}
	}()
	if _, err := f.Write(data); err != nil {
		return err
	}
	if err := f.Sync(); err != nil {
		return err
	}
	if err := f.Close(); err != nil {
		return err
	}
	remove = false
	return nil
}

// ApplyPatchSet re-validates immediately before mutation, delegates guarded
// existing-file edits to the proven Engine.Apply path, creates new files with
// O_EXCL so an untracked/user-created path can never be overwritten, and rolls
// back existing edits plus earlier creates if a later create fails.
func (e *Engine) ApplyPatchSet(input PatchSet) (PatchSet, map[string]any, error) {
	set, err := e.ValidatePatchSet(input)
	if err != nil {
		return set, nil, err
	}

	existing := make([]FileEdit, 0, len(set.Files))
	backups := make([]patchBackup, 0, len(set.Files))
	creates := make([]patchCreate, 0, len(set.Files))
	for _, file := range set.Files {
		path, err := e.FS.WritePath(file.Path)
		if err != nil {
			set, err = patchFail(set, PatchRejected, err)
			return set, nil, err
		}
		if file.ExpectedAbsent {
			creates = append(creates, patchCreate{path: path, content: []byte(*file.Content), mode: 0o644})
			continue
		}
		before, err := os.ReadFile(path)
		if err != nil {
			set, err = patchFail(set, PatchStale, fmt.Errorf("%w: %s changed before apply", ErrPatchStaleBase, file.Path))
			return set, nil, err
		}
		if localfs.Hash(before) != file.ExpectedHash {
			set, err = patchFail(set, PatchStale, fmt.Errorf("%w: %s changed before apply", ErrPatchStaleBase, file.Path))
			return set, nil, err
		}
		mode := os.FileMode(0o644)
		if info, statErr := os.Stat(path); statErr == nil {
			mode = info.Mode().Perm()
		}
		backups = append(backups, patchBackup{path: path, before: before, mode: mode})
		edits := file.Edits
		if file.Content != nil {
			edits = []Edit{fullReplacementEdit(len(before), *file.Content)}
		}
		existing = append(existing, FileEdit{Path: file.Path, ExpectedHash: file.ExpectedHash, Edits: edits})
	}

	result := map[string]any{"changed": []map[string]any{}, "created": []string{}, "atomicValidation": true, "rollbackOnFailure": true, "optimisticConcurrency": true}
	if len(existing) > 0 {
		existingResult, applyErr := e.Apply(existing)
		if applyErr != nil {
			state := PatchRejected
			if strings.Contains(applyErr.Error(), "hash mismatch") || strings.Contains(applyErr.Error(), "changed since read") {
				state = PatchStale
				applyErr = fmt.Errorf("%w: %v", ErrPatchStaleBase, applyErr)
			}
			set, applyErr = patchFail(set, state, applyErr)
			return set, nil, applyErr
		}
		result["existing"] = existingResult
	}

	createdPaths := make([]string, 0, len(creates))
	for _, create := range creates {
		if createErr := writeNewExclusive(create.path, create.content, create.mode); createErr != nil {
			for index := len(createdPaths) - 1; index >= 0; index-- {
				_ = os.Remove(createdPaths[index])
			}
			for index := len(backups) - 1; index >= 0; index-- {
				_ = writeAtomic(backups[index].path, backups[index].before, backups[index].mode)
			}
			if errors.Is(createErr, ErrPatchConflict) {
				set, createErr = patchFail(set, PatchConflicted, fmt.Errorf("%w: path appeared during apply", ErrPatchConflict))
			} else {
				set, createErr = patchFail(set, PatchReverted, createErr)
			}
			return set, nil, createErr
		}
		createdPaths = append(createdPaths, create.path)
	}
	createdRelative := make([]string, 0, len(createdPaths))
	for _, path := range createdPaths {
		createdRelative = append(createdRelative, e.FS.Rel(path))
	}
	result["created"] = createdRelative
	set.State = PatchApplied
	set.Failure = ""
	set.UpdatedAt = time.Now().UTC()
	return set, result, nil
}

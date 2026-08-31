package editing

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

const maxReconcileFileBytes = 8 * 1024 * 1024

type ReconcileState string

const (
	ReconcileAgentOnly ReconcileState = "agent_only"
	ReconcileUserOnly  ReconcileState = "user_only"
	ReconcileConverged ReconcileState = "converged"
	ReconcileMerged    ReconcileState = "merged"
	ReconcileConflict  ReconcileState = "conflict"
)

var (
	ErrInvalidReconcileInput = errors.New("invalid reconciliation input")
	ErrReconcileTooLarge     = errors.New("reconciliation input exceeds size limit")
	ErrReconcileTool         = errors.New("three-way merge tool failed")
)

type ReconcileInput struct {
	Path   string `json:"path"`
	Base   []byte `json:"-"`
	Latest []byte `json:"-"`
	Agent  []byte `json:"-"`
}

type ReconcileResult struct {
	Path            string         `json:"path"`
	State           ReconcileState `json:"state"`
	CanApply        bool           `json:"canApply"`
	BaseHash        string         `json:"baseHash"`
	LatestHash      string         `json:"latestHash"`
	AgentHash       string         `json:"agentHash"`
	ResultHash      string         `json:"resultHash,omitempty"`
	Content         []byte         `json:"-"`
	ConflictPreview []byte         `json:"-"`
}

// Reconcile performs a pure three-way reconciliation. It never writes to the
// authoritative checkout. A successful result can later be converted into a
// PatchSet guarded by LatestHash so user edits that occur after reconciliation
// are still fenced out by PatchSet V2 optimistic concurrency.
func Reconcile(ctx context.Context, input ReconcileInput) (ReconcileResult, error) {
	path, err := normalizeReconcilePath(input.Path)
	if err != nil {
		return ReconcileResult{}, err
	}
	if len(input.Base) > maxReconcileFileBytes || len(input.Latest) > maxReconcileFileBytes || len(input.Agent) > maxReconcileFileBytes {
		return ReconcileResult{}, ErrReconcileTooLarge
	}
	result := ReconcileResult{
		Path:       path,
		BaseHash:   reconcileHash(input.Base),
		LatestHash: reconcileHash(input.Latest),
		AgentHash:  reconcileHash(input.Agent),
	}

	latestChanged := !bytes.Equal(input.Latest, input.Base)
	agentChanged := !bytes.Equal(input.Agent, input.Base)
	switch {
	case !latestChanged && !agentChanged:
		result.State = ReconcileConverged
		result.ResultHash = result.LatestHash
		return result, nil
	case !latestChanged && agentChanged:
		result.State = ReconcileAgentOnly
		result.CanApply = true
		result.Content = append([]byte(nil), input.Agent...)
		result.ResultHash = result.AgentHash
		return result, nil
	case latestChanged && !agentChanged:
		result.State = ReconcileUserOnly
		result.ResultHash = result.LatestHash
		return result, nil
	case bytes.Equal(input.Latest, input.Agent):
		result.State = ReconcileConverged
		result.ResultHash = result.LatestHash
		return result, nil
	}

	if reconcileBinary(input.Base) || reconcileBinary(input.Latest) || reconcileBinary(input.Agent) {
		result.State = ReconcileConflict
		return result, nil
	}
	merged, conflict, err := mergeTextThreeWay(ctx, input.Latest, input.Base, input.Agent)
	if err != nil {
		return ReconcileResult{}, err
	}
	if conflict {
		result.State = ReconcileConflict
		result.ConflictPreview = boundConflictPreview(merged)
		return result, nil
	}
	result.State = ReconcileMerged
	result.CanApply = true
	result.Content = append([]byte(nil), merged...)
	result.ResultHash = reconcileHash(merged)
	return result, nil
}

func normalizeReconcilePath(path string) (string, error) {
	path = strings.TrimSpace(filepath.ToSlash(path))
	if path == "" || filepath.IsAbs(path) {
		return "", ErrInvalidReconcileInput
	}
	path = filepath.ToSlash(filepath.Clean(path))
	if path == "." || path == ".." || strings.HasPrefix(path, "../") || strings.Contains(path, "/../") {
		return "", ErrInvalidReconcileInput
	}
	return path, nil
}

func reconcileHash(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func reconcileBinary(data []byte) bool {
	return bytes.IndexByte(data, 0) >= 0
}

func mergeTextThreeWay(ctx context.Context, latest, base, agent []byte) ([]byte, bool, error) {
	dir, err := os.MkdirTemp("", "codelocal-reconcile-*")
	if err != nil {
		return nil, false, err
	}
	defer os.RemoveAll(dir)
	paths := []string{filepath.Join(dir, "latest"), filepath.Join(dir, "base"), filepath.Join(dir, "agent")}
	values := [][]byte{latest, base, agent}
	for index, path := range paths {
		if err := os.WriteFile(path, values[index], 0o600); err != nil {
			return nil, false, err
		}
	}
	cmd := exec.CommandContext(ctx, "git", "merge-file", "-p", paths[0], paths[1], paths[2])
	cmd.Env = append(os.Environ(), "PAGER=cat", "GIT_PAGER=cat", "CI=1")
	out, runErr := cmd.Output()
	if runErr == nil {
		return out, false, nil
	}
	var exitErr *exec.ExitError
	if errors.As(runErr, &exitErr) {
		if exitErr.ExitCode() == 1 {
			return out, true, nil
		}
		return nil, false, fmt.Errorf("%w: git merge-file exit %d: %s", ErrReconcileTool, exitErr.ExitCode(), strings.TrimSpace(string(exitErr.Stderr)))
	}
	return nil, false, fmt.Errorf("%w: %v", ErrReconcileTool, runErr)
}

func boundConflictPreview(data []byte) []byte {
	const maxPreview = 64 * 1024
	if len(data) <= maxPreview {
		return append([]byte(nil), data...)
	}
	preview := append([]byte(nil), data[:maxPreview]...)
	return append(preview, []byte("\n… conflict preview truncated …\n")...)
}

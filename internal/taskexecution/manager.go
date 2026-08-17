package taskexecution

import (
	"context"
	"errors"
	"os"
	"sort"
	"strings"
	"time"
)

var (
	ErrExecutionProviderMismatch = errors.New("existing task execution bundle uses a different provider")
	ErrRepositoryCoverageChanged = errors.New("existing task execution bundle has different repository coverage")
	ErrBindingUnavailable        = errors.New("existing task execution binding is unavailable")
)

type Manager struct {
	Store     *Store
	Worktrees *LocalWorktreeProvider
}

func NewManager(store *Store, worktrees *LocalWorktreeProvider) *Manager {
	if store == nil {
		store = NewStore("")
	}
	if worktrees == nil {
		worktrees = NewLocalWorktreeProvider("")
	}
	return &Manager{Store: store, Worktrees: worktrees}
}

func repositoryCoverage(bindings []RepositoryBinding) []string {
	out := make([]string, 0, len(bindings))
	for _, binding := range bindings {
		out = append(out, strings.TrimSpace(binding.RepositoryID)+"\x00"+strings.TrimSpace(binding.RepositoryPath))
	}
	sort.Strings(out)
	return out
}

func requestCoverage(req PrepareRequest) []string {
	out := make([]string, 0, len(req.Repositories))
	for _, repo := range req.Repositories {
		out = append(out, strings.TrimSpace(repo.ID)+"\x00"+strings.TrimSpace(repo.RelativePath))
	}
	sort.Strings(out)
	return out
}

func sameStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func validateBindings(ctx context.Context, bundle Bundle) error {
	for _, binding := range bundle.RepositoryBindings {
		info, err := os.Stat(binding.LocalPath)
		if err != nil || !info.IsDir() {
			return ErrBindingUnavailable
		}
		if !gitCommandOK(ctx, binding.LocalPath, "rev-parse", "--is-inside-work-tree") {
			return ErrBindingUnavailable
		}
	}
	return nil
}

func (m *Manager) EnsureLocal(ctx context.Context, req PrepareRequest, ownerID string, leaseTTL time.Duration) (Bundle, error) {
	existing, ok, err := m.Store.Get(req.WorkspaceKey, req.TaskID)
	if err != nil {
		return Bundle{}, err
	}
	if ok {
		if existing.Provider != ProviderLocalWorktree {
			return Bundle{}, ErrExecutionProviderMismatch
		}
		if !sameStrings(repositoryCoverage(existing.RepositoryBindings), requestCoverage(req)) {
			return Bundle{}, ErrRepositoryCoverageChanged
		}
		if err := validateBindings(ctx, existing); err != nil {
			return Bundle{}, err
		}
		return m.Store.Claim(req.WorkspaceKey, req.TaskID, ownerID, leaseTTL)
	}

	bundle, err := m.Worktrees.Prepare(ctx, req)
	if err != nil {
		return Bundle{}, err
	}
	bundle, err = m.Store.Put(bundle)
	if err != nil {
		return Bundle{}, err
	}
	return m.Store.Claim(req.WorkspaceKey, req.TaskID, ownerID, leaseTTL)
}

func (m *Manager) SetState(workspaceKey, taskID, ownerID string, next State) (Bundle, error) {
	bundle, ok, err := m.Store.Get(workspaceKey, taskID)
	if err != nil {
		return Bundle{}, err
	}
	if !ok {
		return Bundle{}, os.ErrNotExist
	}
	if bundle.Lease.OwnerID != "" && strings.TrimSpace(bundle.Lease.OwnerID) != strings.TrimSpace(ownerID) {
		return Bundle{}, ErrLeaseHeld
	}
	bundle.State = next
	return m.Store.Put(bundle)
}

func (m *Manager) Release(workspaceKey, taskID, ownerID string) (Bundle, error) {
	return m.Store.Release(workspaceKey, taskID, ownerID)
}

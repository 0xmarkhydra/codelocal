package taskexecution

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/repository"
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

func existingCoverage(bindings []RepositoryBinding) map[string]RepositoryBinding {
	out := map[string]RepositoryBinding{}
	for _, binding := range bindings {
		out[strings.TrimSpace(binding.RepositoryID)+"\x00"+strings.TrimSpace(binding.RepositoryPath)] = binding
	}
	return out
}

func missingRepositories(existing []RepositoryBinding, requested []repository.Checkout) ([]repository.Checkout, error) {
	coverage := existingCoverage(existing)
	byPath, byID := map[string]string{}, map[string]string{}
	for _, binding := range existing {
		byPath[strings.TrimSpace(binding.RepositoryPath)] = strings.TrimSpace(binding.RepositoryID)
		byID[strings.TrimSpace(binding.RepositoryID)] = strings.TrimSpace(binding.RepositoryPath)
	}
	missing := []repository.Checkout{}
	for _, repo := range requested {
		id, path := strings.TrimSpace(repo.ID), strings.TrimSpace(repo.RelativePath)
		if _, ok := coverage[id+"\x00"+path]; ok {
			continue
		}
		if knownID, ok := byPath[path]; ok && knownID != id {
			return nil, ErrRepositoryCoverageChanged
		}
		if knownPath, ok := byID[id]; ok && knownPath != path {
			return nil, ErrRepositoryCoverageChanged
		}
		missing = append(missing, repo)
	}
	return missing, nil
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
	if !ok {
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
	if existing.Provider != ProviderLocalWorktree {
		return Bundle{}, ErrExecutionProviderMismatch
	}
	if err := validateBindings(ctx, existing); err != nil {
		return Bundle{}, err
	}
	missing, err := missingRepositories(existing.RepositoryBindings, req.Repositories)
	if err != nil {
		return Bundle{}, err
	}
	claimed, err := m.Store.Claim(req.WorkspaceKey, req.TaskID, ownerID, leaseTTL)
	if err != nil {
		return Bundle{}, err
	}
	if len(missing) == 0 {
		return claimed, nil
	}
	expansionReq := req
	expansionReq.Repositories = missing
	expansion, err := m.Worktrees.Prepare(ctx, expansionReq)
	if err != nil {
		_, _ = m.Store.Release(req.WorkspaceKey, req.TaskID, ownerID)
		return Bundle{}, err
	}
	claimed.RepositoryBindings = append(claimed.RepositoryBindings, expansion.RepositoryBindings...)
	return m.Store.Put(claimed)
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

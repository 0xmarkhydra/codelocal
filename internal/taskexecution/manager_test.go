package taskexecution

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/repository"
)

func TestManagerPersistsAndResumesTaskExecutionBundle(t *testing.T) {
	ctx := context.Background()
	repo := makeTaskRepo(t)
	store := NewStore(filepath.Join(t.TempDir(), "state"))
	provider := NewLocalWorktreeProvider(filepath.Join(t.TempDir(), "worktrees"))
	manager := NewManager(store, provider)
	req := PrepareRequest{
		TaskID: "task-a", WorkspaceKey: "workspace", WorkspaceID: "ws", ProjectID: "project",
		Repositories: []repository.Checkout{repo},
	}

	first, err := manager.EnsureLocal(ctx, req, "agent-a", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if first.Lease.OwnerID != "agent-a" || len(first.RepositoryBindings) != 1 {
		t.Fatalf("unexpected first bundle: %#v", first)
	}
	if _, err := manager.EnsureLocal(ctx, req, "agent-b", time.Minute); !errors.Is(err, ErrLeaseHeld) {
		t.Fatalf("second owner must not take an active task: %v", err)
	}
	if _, err := manager.Release(req.WorkspaceKey, req.TaskID, "agent-a"); err != nil {
		t.Fatal(err)
	}
	resumed, err := manager.EnsureLocal(ctx, req, "agent-b", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	if resumed.RepositoryBindings[0].LocalPath != first.RepositoryBindings[0].LocalPath {
		t.Fatal("task resume did not reuse its persisted execution binding")
	}
}

func TestManagerRejectsRepositoryCoverageChangeOnResume(t *testing.T) {
	ctx := context.Background()
	firstRepo := makeTaskRepo(t)
	secondRepo := makeTaskRepo(t)
	secondRepo.ID = "repo-b"
	secondRepo.RelativePath = "backend/auth"
	manager := NewManager(
		NewStore(filepath.Join(t.TempDir(), "state")),
		NewLocalWorktreeProvider(filepath.Join(t.TempDir(), "worktrees")),
	)
	req := PrepareRequest{TaskID: "task-a", WorkspaceKey: "workspace", Repositories: []repository.Checkout{firstRepo}}
	if _, err := manager.EnsureLocal(ctx, req, "agent-a", time.Minute); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Release(req.WorkspaceKey, req.TaskID, "agent-a"); err != nil {
		t.Fatal(err)
	}
	req.Repositories = []repository.Checkout{firstRepo, secondRepo}
	if _, err := manager.EnsureLocal(ctx, req, "agent-a", time.Minute); !errors.Is(err, ErrRepositoryCoverageChanged) {
		t.Fatalf("repository coverage mutation must be explicit, got %v", err)
	}
}

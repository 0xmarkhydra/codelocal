package runtime

import (
	"context"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
	"github.com/0xmarkhydra/codelocal/internal/workspace"
)

const knowledgeSyncInterval = 10 * time.Second

type gitProvenance struct {
	Branch string
	Commit string
}

func projectIdentityPresent(identity projectidentity.Snapshot) bool {
	return strings.TrimSpace(identity.SuggestedName) != "" || strings.TrimSpace(identity.MarkerProjectID) != "" || len(identity.Repositories) > 0
}

func gitOutputAt(root string, args ...string) string {
	cmd := exec.Command("git", append([]string{"-C", root}, args...)...)
	cmd.Env = append(os.Environ(), "PAGER=cat", "GIT_PAGER=cat", "CI=1")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func repositoryProvenance(root string, identity projectidentity.Snapshot) map[string]gitProvenance {
	out := map[string]gitProvenance{}
	for _, repository := range identity.Repositories {
		repoRoot := root
		if rel := strings.TrimSpace(repository.RelativePath); rel != "" && rel != "." {
			repoRoot = filepath.Join(root, filepath.FromSlash(rel))
		}
		out[repository.ID] = gitProvenance{
			Branch: gitOutputAt(repoRoot, "branch", "--show-current"),
			Commit: gitOutputAt(repoRoot, "rev-parse", "HEAD"),
		}
	}
	if len(out) == 0 {
		out[""] = gitProvenance{Branch: gitOutputAt(root, "branch", "--show-current"), Commit: gitOutputAt(root, "rev-parse", "HEAD")}
	}
	return out
}

func applySourceProvenance(source projectbrain.Source, identity projectidentity.Snapshot, provenance map[string]gitProvenance) projectbrain.Source {
	repositoryID := projectbrain.RepositoryIDForPath(source.Path, identity.Repositories)
	value, ok := provenance[repositoryID]
	if !ok {
		value = provenance[""]
	}
	source.Branch = value.Branch
	source.GitCommit = value.Commit
	return source
}

func applyDeltaProvenance(delta projectbrain.ManifestDelta, identity projectidentity.Snapshot, provenance map[string]gitProvenance) projectbrain.ManifestDelta {
	out := delta
	out.Sources = append([]projectbrain.Source(nil), delta.Sources...)
	out.Removed = append([]projectbrain.Source(nil), delta.Removed...)
	for index := range out.Sources {
		out.Sources[index] = applySourceProvenance(out.Sources[index], identity, provenance)
	}
	for index := range out.Removed {
		out.Removed[index] = applySourceProvenance(out.Removed[index], identity, provenance)
	}
	return out
}

func applyManifestProvenance(manifest projectbrain.Manifest, identity projectidentity.Snapshot, provenance map[string]gitProvenance) projectbrain.Manifest {
	out := manifest
	out.Sources = append([]projectbrain.Source(nil), manifest.Sources...)
	for index := range out.Sources {
		out.Sources[index] = applySourceProvenance(out.Sources[index], identity, provenance)
	}
	return out
}

func (r *Runtime) activeWorker(workspaceID string) *WorkspaceWorker {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.workers[workspaceID]
}

func (r *Runtime) identityForWorkspace(w workspace.Workspace) projectidentity.Snapshot {
	r.mu.Lock()
	identity, ok := r.projectIdentities[w.WorkspaceID]
	r.mu.Unlock()
	if ok && projectIdentityPresent(identity) {
		return identity
	}
	if r.brainSync != nil {
		if stored, found, err := r.brainSync.Workspace(w.WorkspaceID); err == nil && found && projectIdentityPresent(stored.ProjectIdentity) {
			identity = stored.ProjectIdentity
			r.mu.Lock()
			r.projectIdentities[w.WorkspaceID] = identity
			r.mu.Unlock()
			return identity
		}
	}
	identity = projectidentity.Discover(w.LocalPath, w.WorkspaceName)
	r.mu.Lock()
	r.projectIdentities[w.WorkspaceID] = identity
	r.mu.Unlock()
	if r.brainSync != nil {
		state, _, err := r.brainSync.Workspace(w.WorkspaceID)
		if err == nil {
			state.WorkspaceID = w.WorkspaceID
			state.ProjectIdentity = identity
			if state.BaseRevisions == nil {
				state.BaseRevisions = map[string]string{}
			}
			if persistErr := r.brainSync.Put(state); persistErr != nil {
				slog.Debug("project identity persistence failed; runtime remains usable", "workspaceId", w.WorkspaceID, "error", persistErr)
			}
		}
	}
	return identity
}

func (r *Runtime) activeKnowledgeManifest(worker *WorkspaceWorker) (projectbrain.Manifest, error) {
	if worker == nil || worker.Engine == nil || worker.Engine.Project == nil {
		return projectbrain.Manifest{}, nil
	}
	if projectMap, ok := worker.Engine.Project.CachedMap(); ok {
		return projectbrain.CloudSafeManifest(projectbrain.FromProjectMap(projectMap)), nil
	}
	// This is an explicit activation/invalidated-workspace path, not a periodic
	// timer scan. Once built, background sync only reads CachedMap until the
	// project engine is invalidated by actual work or refreshed by context use.
	projectMap, err := worker.Engine.Project.Map(false)
	if err != nil {
		return projectbrain.Manifest{}, err
	}
	return projectbrain.CloudSafeManifest(projectbrain.FromProjectMap(projectMap)), nil
}

func (r *Runtime) syncKnowledgeWorkspace(ctx context.Context, w workspace.Workspace, force bool) error {
	if !r.projectBrainCloudEnabled() {
		return nil
	}
	worker := r.activeWorker(w.WorkspaceID)
	if worker == nil {
		return nil
	}
	r.knowledgeMu.Lock()
	defer r.knowledgeMu.Unlock()

	manifest, err := r.activeKnowledgeManifest(worker)
	if err != nil || manifest.RootHash == "" {
		return err
	}
	state := projectbrain.WorkspaceSyncState{WorkspaceID: w.WorkspaceID, BaseRevisions: map[string]string{}}
	if r.brainSync != nil {
		if cached, ok, loadErr := r.brainSync.Workspace(w.WorkspaceID); loadErr != nil {
			slog.Warn("project brain local sync state unreadable; using safe bootstrap", "workspaceId", w.WorkspaceID, "error", loadErr)
		} else if ok {
			state = cached
		}
	}
	if state.BaseRevisions == nil {
		state.BaseRevisions = map[string]string{}
	}
	identity := r.identityForWorkspace(w)
	if force {
		// Activation is an explicit workspace-use boundary, so it is the right
		// place to refresh nested repository identity without scanning sleeping
		// workspaces on the runtime heartbeat.
		identity = projectidentity.Discover(w.LocalPath, w.WorkspaceName)
		r.mu.Lock()
		r.projectIdentities[w.WorkspaceID] = identity
		r.mu.Unlock()
	}
	state.ProjectIdentity = identity
	if !force && state.SyncedRoot == manifest.RootHash {
		return nil
	}
	previous := state.Manifest
	if state.SyncedRoot == "" {
		// A missing acknowledgement means we cannot assume the server saw the
		// locally cached manifest. Bootstrap by sending all current safe sources.
		previous = projectbrain.Manifest{}
	}
	provenance := repositoryProvenance(w.LocalPath, identity)
	observedManifest := applyManifestProvenance(manifest, identity, provenance)
	observedPrevious := applyManifestProvenance(previous, identity, provenance)
	delta := projectbrain.DiffManifest(observedPrevious, observedManifest, state.BaseRevisions)
	if len(delta.Sources) == 0 && len(delta.Removed) == 0 {
		state.Manifest = manifest
		state.SyncedRoot = manifest.RootHash
		state.UpdatedAt = time.Now().UnixMilli()
		if r.brainSync != nil {
			return r.brainSync.Put(state)
		}
		return nil
	}
	chunks := projectbrain.ChunkManifestDelta(delta, projectbrain.DefaultKnowledgeSyncBatchSources, projectbrain.DefaultKnowledgeSyncBatchBytes)
	for _, chunk := range chunks {
		var response cloud.KnowledgeManifestSyncResult
		payload := map[string]any{
			"workspaceId":     w.WorkspaceID,
			"projectIdentity": identity,
			"delta":           chunk,
		}
		if err := r.post(ctx, "/api/client/knowledge/sync", payload, &response); err != nil {
			return err
		}
		if response.Disabled {
			// Server-side kill switch/canary decision changed after the last control
			// plane refresh. Do not acknowledge local deltas; keep them pending for
			// a future enabled cohort and stop background attempts immediately.
			r.setProjectBrainCloudEnabled(false)
			return nil
		}
		for key, revisionID := range response.ActiveRevisions {
			if strings.TrimSpace(key) != "" && strings.TrimSpace(revisionID) != "" {
				state.BaseRevisions[key] = revisionID
			}
		}
		// Persist acknowledgement progress after every bounded batch. If a later
		// batch fails, retry uses fresh bases instead of manufacturing conflicts.
		state.UpdatedAt = time.Now().UnixMilli()
		if r.brainSync != nil {
			if err := r.brainSync.Put(state); err != nil {
				slog.Debug("project brain local batch cursor persistence failed", "workspaceId", w.WorkspaceID, "error", err)
			}
		}
	}
	state.Manifest = manifest
	state.SyncedRoot = manifest.RootHash
	state.UpdatedAt = time.Now().UnixMilli()
	if r.brainSync != nil {
		if err := r.brainSync.Put(state); err != nil {
			return err
		}
	}
	r.mu.Lock()
	r.knowledgeManifests[w.WorkspaceID] = manifest
	r.knowledgeBaseRevisions[w.WorkspaceID] = copyRevisionMap(state.BaseRevisions)
	r.syncedKnowledgeRoots[w.WorkspaceID] = state.SyncedRoot
	r.mu.Unlock()
	return nil
}

func (r *Runtime) runKnowledgeSyncLoop(ctx context.Context) {
	ticker := time.NewTicker(knowledgeSyncInterval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			r.mu.Lock()
			workers := make([]*WorkspaceWorker, 0, len(r.workers))
			for _, worker := range r.workers {
				workers = append(workers, worker)
			}
			r.mu.Unlock()
			for _, worker := range workers {
				if worker == nil {
					continue
				}
				syncCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
				err := r.syncKnowledgeWorkspace(syncCtx, worker.Workspace, false)
				cancel()
				if err != nil && !errorsIsContext(err) {
					slog.Debug("project brain background sync delayed; runtime remains usable", "workspaceId", worker.Workspace.WorkspaceID, "error", err)
				}
			}
		}
	}
}

func errorsIsContext(err error) bool {
	return err == context.Canceled || err == context.DeadlineExceeded
}

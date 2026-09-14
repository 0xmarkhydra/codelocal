package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/mcpconfig"
)

type runtimeConfigCache struct {
	DeviceID   string                                 `json:"deviceId,omitempty"`
	Workspaces map[string]cloud.RuntimeConfigSnapshot `json:"workspaces"`
	UpdatedAt  int64                                  `json:"updatedAt"`
}

func runtimeConfigCachePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".codelocal", "runtime", "config.json"), nil
}

func saveRuntimeConfigCache(cache runtimeConfigCache) error {
	path, err := runtimeConfigCachePath()
	if err != nil {
		return err
	}
	if cache.Workspaces == nil {
		cache.Workspaces = map[string]cloud.RuntimeConfigSnapshot{}
	}
	cache.UpdatedAt = time.Now().UnixMilli()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	raw, err := json.MarshalIndent(cache, "", "  ")
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, raw, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func loadRuntimeConfigCache(deviceID string) map[string]cloud.RuntimeConfigSnapshot {
	path, err := runtimeConfigCachePath()
	if err != nil {
		return map[string]cloud.RuntimeConfigSnapshot{}
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		return map[string]cloud.RuntimeConfigSnapshot{}
	}
	var cache runtimeConfigCache
	if json.Unmarshal(raw, &cache) != nil || cache.DeviceID != deviceID || cache.Workspaces == nil {
		return map[string]cloud.RuntimeConfigSnapshot{}
	}
	return cache.Workspaces
}

func resolveRuntimeSnapshot(snapshot cloud.RuntimeConfigSnapshot) cloud.RuntimeConfigSnapshot {
	for index, project := range snapshot.SystemProjects {
		if project.ID != cloud.OpenMontageSystemProjectID {
			continue
		}
		project.Name = cloud.OpenMontageName
		project.Source = cloud.OpenMontageSource
		project.SystemApp = true
		project.Managed = true
		project.Hidden = false
		snapshot.SystemProjects[index] = project
	}
	return snapshot
}

func managedRuntimeSystemProjects(settings map[string]cloud.RuntimeMaterializedConfig) []cloud.RuntimeSystemProject {
	projects := map[string]cloud.RuntimeSystemProject{}
	for _, materialized := range settings {
		for _, project := range materialized.Snapshot.SystemProjects {
			id := strings.TrimSpace(project.ID)
			if id == "" || !project.Enabled || !project.Managed {
				continue
			}
			project.ID = id
			projects[id] = project
		}
	}
	out := make([]cloud.RuntimeSystemProject, 0, len(projects))
	for _, project := range projects {
		out = append(out, project)
	}
	return out
}

func validateManagedSystemProject(project cloud.RuntimeSystemProject) error {
	if project.ID != "openmontage" {
		return fmt.Errorf("unsupported managed system project: %s", project.ID)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return err
	}
	wantPath := filepath.Join(home, ".codelocal", "system-projects", "openmontage")
	wantSource := "https://github.com/calesthio/OpenMontage.git"
	if filepath.Clean(project.Path) != filepath.Clean(wantPath) || project.Source != wantSource {
		return errors.New("managed OpenMontage source or path does not match the CodeLocal system project")
	}
	return nil
}

func runSystemProjectGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_PAGER=cat", "CI=1")
	output, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("git %s: %w: %s", strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return nil
}

func materializeManagedSystemProject(ctx context.Context, project cloud.RuntimeSystemProject) error {
	if err := validateManagedSystemProject(project); err != nil {
		return err
	}
	if info, err := os.Stat(project.Path); err == nil {
		if !info.IsDir() {
			return fmt.Errorf("system project path is not a directory: %s", project.Path)
		}
		if _, err := os.Stat(filepath.Join(project.Path, ".git")); err != nil {
			return fmt.Errorf("system project exists but is not a Git checkout: %s", project.Path)
		}
		return nil
	} else if !os.IsNotExist(err) {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(project.Path), 0o700); err != nil {
		return err
	}
	tmp := project.Path + ".installing"
	_ = os.RemoveAll(tmp)
	defer os.RemoveAll(tmp)
	if err := runSystemProjectGit(ctx, filepath.Dir(project.Path), "clone", "--depth", "1", project.Source, tmp); err != nil {
		return err
	}
	return os.Rename(tmp, project.Path)
}

func (r *Runtime) InstallSystemApp(ctx context.Context, appID string) (*WorkspaceWorker, error) {
	if appID != cloud.OpenMontageSystemProjectID {
		return nil, fmt.Errorf("unsupported system app: %s", appID)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	project := cloud.RuntimeSystemProject{
		ID: cloud.OpenMontageSystemProjectID, Name: cloud.OpenMontageName,
		Path: filepath.Join(home, ".codelocal", "system-projects", "openmontage"), Source: cloud.OpenMontageSource,
		SystemApp: true, Managed: true, Hidden: false, Enabled: true,
	}
	r.systemProjectSyncMu.Lock()
	defer r.systemProjectSyncMu.Unlock()
	if err := materializeManagedSystemProject(ctx, project); err != nil {
		return nil, err
	}
	entry, err := r.Registry.EnsureSystem(project.ID, project.Name, project.Path)
	if err != nil {
		return nil, err
	}
	if _, err := r.SyncRegistry(ctx, true); err != nil {
		return nil, err
	}
	return r.Activate(ctx, entry.WorkspaceID)
}

func runtimeConfigEnvironment(snapshot cloud.RuntimeConfigSnapshot) map[string]string {
	out := map[string]string{}
	for key, value := range snapshot.Values {
		if cloud.ValidRuntimeEnvKey(strings.TrimSpace(key)) {
			out[key] = value
		}
	}
	return out
}

func runtimeSecretRedactValues(secrets map[string]string) []string {
	out := make([]string, 0, len(secrets))
	for _, value := range secrets {
		if strings.TrimSpace(value) != "" {
			out = append(out, value)
		}
	}
	return out
}

func (r *Runtime) applyRuntimeSettings(settings map[string]cloud.RuntimeMaterializedConfig) {
	if settings == nil {
		settings = map[string]cloud.RuntimeMaterializedConfig{}
	}
	cache := runtimeConfigCache{DeviceID: r.Options.Credential.DeviceID, Workspaces: map[string]cloud.RuntimeConfigSnapshot{}}
	active := map[string]*WorkspaceWorker{}
	r.mu.Lock()
	for workspaceID, materialized := range settings {
		materialized.Snapshot = resolveRuntimeSnapshot(materialized.Snapshot)
		settings[workspaceID] = materialized
		cache.Workspaces[workspaceID] = materialized.Snapshot
		if worker := r.workers[workspaceID]; worker != nil && worker.Engine != nil {
			worker.Engine.SetRuntimeEnvironment(runtimeConfigEnvironment(materialized.Snapshot), materialized.Secrets)
			active[workspaceID] = worker
		}
	}
	r.runtimeSettings = settings
	r.mu.Unlock()
	for workspaceID, worker := range active {
		materialized := settings[workspaceID]
		go r.reconcileWorkerMCP(worker, materialized.MCPServers)
	}
	// Applying runtime settings must never create System App files. Installation
	// is an explicit user action handled by the realtime runtime control lane.
	if err := saveRuntimeConfigCache(cache); err != nil {
		return
	}
}

func (r *Runtime) reconcileWorkerMCP(worker *WorkspaceWorker, desired []mcpconfig.MaterializedServer) {
	if worker == nil || worker.Engine == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	defer cancel()
	actual := worker.Engine.ReconcileMCPServers(ctx, desired)
	if len(actual) == 0 && len(desired) == 0 {
		return
	}
	reportCtx, reportCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer reportCancel()
	_ = r.post(reportCtx, "/api/client/mcp/status", map[string]any{"actual": actual}, nil)
}

func (r *Runtime) runtimeSetting(workspaceID string) cloud.RuntimeMaterializedConfig {
	r.mu.Lock()
	defer r.mu.Unlock()
	if setting, ok := r.runtimeSettings[workspaceID]; ok {
		return setting
	}
	if snapshot, ok := loadRuntimeConfigCache(r.Options.Credential.DeviceID)[workspaceID]; ok {
		return cloud.RuntimeMaterializedConfig{Snapshot: resolveRuntimeSnapshot(snapshot)}
	}
	return cloud.RuntimeMaterializedConfig{}
}

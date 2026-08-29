package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
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
	home, err := os.UserHomeDir()
	if err != nil {
		return snapshot
	}
	for index, project := range snapshot.SystemProjects {
		if project.ID != "openmontage" {
			continue
		}
		project.Path = filepath.Join(home, ".codelocal", "system-projects", "openmontage")
		project.Source = "https://github.com/calesthio/OpenMontage.git"
		project.Managed, project.Hidden = true, true
		snapshot.SystemProjects[index] = project
	}
	return snapshot
}

func managedRuntimeSystemProjects(settings map[string]cloud.RuntimeMaterializedConfig) []cloud.RuntimeSystemProject {
	projects := map[string]cloud.RuntimeSystemProject{}
	for _, materialized := range settings {
		for _, project := range materialized.Snapshot.SystemProjects {
			if !project.Enabled || !project.Managed || strings.TrimSpace(project.ID) == "" {
				continue
			}
			projects[project.ID] = project
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

func (r *Runtime) materializeRuntimeSystemProjects(settings map[string]cloud.RuntimeMaterializedConfig) {
	projects := managedRuntimeSystemProjects(settings)
	if len(projects) == 0 {
		return
	}
	go func() {
		r.systemProjectSyncMu.Lock()
		defer r.systemProjectSyncMu.Unlock()
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
		defer cancel()
		for _, project := range projects {
			if err := materializeManagedSystemProject(ctx, project); err != nil {
				slog.Warn("managed system project materialization failed", "projectId", project.ID, "error", err)
				continue
			}
			slog.Debug("managed system project ready", "projectId", project.ID, "path", project.Path)
		}
	}()
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
	r.mu.Lock()
	for workspaceID, materialized := range settings {
		materialized.Snapshot = resolveRuntimeSnapshot(materialized.Snapshot)
		settings[workspaceID] = materialized
		cache.Workspaces[workspaceID] = materialized.Snapshot
		if worker := r.workers[workspaceID]; worker != nil && worker.Engine != nil {
			worker.Engine.SetRuntimeEnvironment(runtimeConfigEnvironment(materialized.Snapshot), materialized.Secrets)
		}
	}
	r.runtimeSettings = settings
	r.mu.Unlock()
	r.materializeRuntimeSystemProjects(settings)
	if err := saveRuntimeConfigCache(cache); err != nil {
		// Runtime config is an optimization layer; losing the metadata cache must
		// never take an otherwise healthy local workspace offline.
		return
	}
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

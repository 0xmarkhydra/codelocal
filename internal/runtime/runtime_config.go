package runtime

import (
	"encoding/json"
	"os"
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

func runtimeConfigEnvironment(snapshot cloud.RuntimeConfigSnapshot, secrets map[string]string) map[string]string {
	out := map[string]string{}
	for key, value := range snapshot.Values {
		if cloud.ValidRuntimeEnvKey(strings.TrimSpace(key)) {
			out[key] = value
		}
	}
	for key, value := range secrets {
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
			worker.Engine.SetRuntimeEnvironment(runtimeConfigEnvironment(materialized.Snapshot, materialized.Secrets), runtimeSecretRedactValues(materialized.Secrets))
		}
	}
	r.runtimeSettings = settings
	r.mu.Unlock()
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

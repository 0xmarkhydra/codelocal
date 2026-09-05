package cloud

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/plugins"
	"github.com/jackc/pgx/v5"
)

type PluginInstallationState string

const (
	PluginInstalled PluginInstallationState = "installed"
	PluginDisabled  PluginInstallationState = "disabled"
)

type PluginInstallation struct {
	UserID       string                  `json:"userId"`
	PluginID     string                  `json:"pluginId"`
	Version      string                  `json:"version"`
	ManifestHash string                  `json:"manifestHash"`
	State        PluginInstallationState `json:"state"`
	InstalledAt  int64                   `json:"installedAt"`
	UpdatedAt    int64                   `json:"updatedAt"`
}

func NewPluginInstallation(userID string, manifest plugins.Manifest) (PluginInstallation, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return PluginInstallation{}, fmt.Errorf("plugin installation user is required")
	}
	manifestHash, err := plugins.ManifestHash(manifest)
	if err != nil {
		return PluginInstallation{}, err
	}
	now := time.Now().UnixMilli()
	return PluginInstallation{
		UserID:       userID,
		PluginID:     manifest.ID,
		Version:      manifest.Version,
		ManifestHash: manifestHash,
		State:        PluginInstalled,
		InstalledAt:  now,
		UpdatedAt:    now,
	}, nil
}

func (s *Store) SetPluginInstallation(ctx context.Context, installation PluginInstallation) error {
	if s == nil || s.DB == nil {
		return fmt.Errorf("cloud store is unavailable")
	}
	installation.UserID = strings.TrimSpace(installation.UserID)
	installation.PluginID = strings.TrimSpace(installation.PluginID)
	installation.Version = strings.TrimSpace(installation.Version)
	installation.ManifestHash = strings.TrimSpace(installation.ManifestHash)
	if installation.UserID == "" || installation.PluginID == "" || installation.Version == "" || installation.ManifestHash == "" {
		return fmt.Errorf("plugin installation identity is required")
	}
	if installation.State == "" {
		installation.State = PluginInstalled
	}
	if installation.State != PluginInstalled && installation.State != PluginDisabled {
		return fmt.Errorf("invalid plugin installation state %q", installation.State)
	}
	now := time.Now().UnixMilli()
	if installation.InstalledAt <= 0 {
		installation.InstalledAt = now
	}
	installation.UpdatedAt = now
	_, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_plugin_installations (
  user_id, plugin_id, version, manifest_hash, state, installed_at, updated_at
) VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (user_id, plugin_id) DO UPDATE SET
  version = EXCLUDED.version,
  manifest_hash = EXCLUDED.manifest_hash,
  state = EXCLUDED.state,
  updated_at = EXCLUDED.updated_at`,
		installation.UserID, installation.PluginID, installation.Version, installation.ManifestHash,
		installation.State, installation.InstalledAt, installation.UpdatedAt,
	)
	return err
}

func (s *Store) DeletePluginInstallation(ctx context.Context, userID, pluginID string) (bool, error) {
	if s == nil || s.DB == nil {
		return false, fmt.Errorf("cloud store is unavailable")
	}
	userID, pluginID = strings.TrimSpace(userID), strings.TrimSpace(pluginID)
	if userID == "" || pluginID == "" {
		return false, fmt.Errorf("plugin installation identity is required")
	}
	tag, err := s.DB.Exec(ctx, `DELETE FROM codelocal_plugin_installations WHERE user_id=$1 AND plugin_id=$2`, userID, pluginID)
	if err != nil {
		return false, err
	}
	return tag.RowsAffected() > 0, nil
}

func (s *Store) ListPluginInstallations(ctx context.Context, userID string) ([]PluginInstallation, error) {
	if s == nil || s.DB == nil {
		return nil, fmt.Errorf("cloud store is unavailable")
	}
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return nil, fmt.Errorf("plugin installation user is required")
	}
	rows, err := s.DB.Query(ctx, `
SELECT plugin_id, version, manifest_hash, state, installed_at, updated_at
FROM codelocal_plugin_installations
WHERE user_id=$1
ORDER BY updated_at DESC, plugin_id`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []PluginInstallation{}
	for rows.Next() {
		item := PluginInstallation{UserID: userID}
		if err := rows.Scan(&item.PluginID, &item.Version, &item.ManifestHash, &item.State, &item.InstalledAt, &item.UpdatedAt); err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

func (s *Store) PluginInstallationByID(ctx context.Context, userID, pluginID string) (PluginInstallation, bool, error) {
	if s == nil || s.DB == nil {
		return PluginInstallation{}, false, fmt.Errorf("cloud store is unavailable")
	}
	userID, pluginID = strings.TrimSpace(userID), strings.TrimSpace(pluginID)
	item := PluginInstallation{UserID: userID, PluginID: pluginID}
	err := s.DB.QueryRow(ctx, `
SELECT version, manifest_hash, state, installed_at, updated_at
FROM codelocal_plugin_installations
WHERE user_id=$1 AND plugin_id=$2`, userID, pluginID).Scan(
		&item.Version, &item.ManifestHash, &item.State, &item.InstalledAt, &item.UpdatedAt,
	)
	if err == pgx.ErrNoRows {
		return PluginInstallation{}, false, nil
	}
	if err != nil {
		return PluginInstallation{}, false, err
	}
	return item, true, nil
}

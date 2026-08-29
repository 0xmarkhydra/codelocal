package cloud

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/skills"
)

const preferredSkillAffinityBoost = 0.15

type SkillRuntimeCatalog interface {
	StableSkillVersionRecords(ctx context.Context, userID string) ([]SkillVersionRecord, error)
	SkillVersionRecordByIdentity(ctx context.Context, tenantUserID, skillID, version string) (SkillVersionRecord, bool, error)
	ListSkillUserStates(ctx context.Context, userID string) ([]SkillUserState, error)
}

type SkillRuntime struct {
	catalog  SkillRuntimeCatalog
	packages skills.PackageStore
}

type SkillRuntimeSnapshot struct {
	Engine             *skills.Engine
	PreferenceAffinity map[string]float64
	Warnings           []string
}

type effectiveSkillSource struct {
	manifest skills.Manifest
	artifact *skills.Artifact
	record   *SkillVersionRecord
}

func NewSkillRuntime(catalog SkillRuntimeCatalog, packages skills.PackageStore) *SkillRuntime {
	return &SkillRuntime{catalog: catalog, packages: packages}
}

// Snapshot builds the effective tenant catalog for one request. Precedence is
// System > Personal > Community, while user state may disable any effective
// Skill or pin a version inside that Skill's authoritative namespace.
func (r *SkillRuntime) Snapshot(ctx context.Context, userID string) (SkillRuntimeSnapshot, error) {
	builtinRegistry := skills.DefaultRegistry()
	builtinArtifacts, err := skills.BuiltinArtifacts(builtinRegistry)
	if err != nil {
		return SkillRuntimeSnapshot{}, err
	}
	builtinByID := make(map[string]effectiveSkillSource, len(builtinRegistry.List()))
	artifactByID := make(map[string]skills.Artifact, len(builtinArtifacts))
	for _, artifact := range builtinArtifacts {
		artifactByID[artifact.Manifest.SkillID] = artifact
	}
	for _, manifest := range builtinRegistry.List() {
		source := effectiveSkillSource{manifest: manifest}
		if artifact, ok := artifactByID[manifest.ID]; ok {
			copy := artifact
			source.artifact = &copy
		}
		builtinByID[manifest.ID] = source
	}

	selected := make(map[string]effectiveSkillSource, len(builtinByID))
	for id, source := range builtinByID {
		selected[id] = source
	}
	warnings := []string{}
	userID = strings.TrimSpace(userID)

	var records []SkillVersionRecord
	var states []SkillUserState
	if r != nil && r.catalog != nil {
		records, err = r.catalog.StableSkillVersionRecords(ctx, userID)
		if err != nil {
			return SkillRuntimeSnapshot{}, fmt.Errorf("load stable skill catalog: %w", err)
		}
		states, err = r.catalog.ListSkillUserStates(ctx, userID)
		if err != nil {
			return SkillRuntimeSnapshot{}, fmt.Errorf("load skill user state: %w", err)
		}
	}

	// Community cannot squat an official System ID.
	for _, record := range records {
		if record.Manifest.Scope != skills.ScopeCommunity || !routableSkillRecord(record) {
			continue
		}
		if existing, exists := selected[record.Manifest.ID]; exists && existing.manifest.Scope == skills.ScopeSystem {
			warnings = append(warnings, "community skill "+record.Manifest.ID+" ignored because the ID is reserved by System")
			continue
		}
		copy := record
		selected[record.Manifest.ID] = effectiveSkillSource{manifest: record.Manifest, record: &copy}
	}
	// Personal can override Community but never an official System Skill.
	for _, record := range records {
		if record.Manifest.Scope != skills.ScopePersonal || !routableSkillRecord(record) {
			continue
		}
		if existing, exists := selected[record.Manifest.ID]; exists && existing.manifest.Scope == skills.ScopeSystem {
			warnings = append(warnings, "personal skill "+record.Manifest.ID+" ignored because the ID is reserved by System")
			continue
		}
		copy := record
		selected[record.Manifest.ID] = effectiveSkillSource{manifest: record.Manifest, record: &copy}
	}
	// Promoted System packages replace the bounded built-in fallback, enabling a
	// full durable artifact (for example UI/UX Pro) without changing call sites.
	for _, record := range records {
		if record.Manifest.Scope != skills.ScopeSystem || !routableSkillRecord(record) {
			continue
		}
		copy := record
		selected[record.Manifest.ID] = effectiveSkillSource{manifest: record.Manifest, record: &copy}
	}

	stateByID := make(map[string]SkillUserState, len(states))
	preferenceAffinity := map[string]float64{}
	for _, state := range states {
		stateByID[state.SkillID] = state
	}
	for skillID, state := range stateByID {
		source, exists := selected[skillID]
		if !exists {
			continue
		}
		switch state.Mode {
		case "disabled":
			delete(selected, skillID)
			continue
		case "prefer":
			preferenceAffinity[skillID] = preferredSkillAffinityBoost
		}
		pinned := strings.TrimSpace(state.PinnedVersion)
		if pinned == "" || pinned == source.manifest.Version {
			continue
		}
		if r == nil || r.catalog == nil {
			warnings = append(warnings, "pinned skill "+skillID+" unavailable because the Cloud catalog is not configured")
			continue
		}
		tenantUserID := ""
		if source.record != nil {
			tenantUserID = source.record.TenantUserID
		}
		pinnedRecord, ok, lookupErr := r.catalog.SkillVersionRecordByIdentity(ctx, tenantUserID, skillID, pinned)
		if lookupErr != nil {
			warnings = append(warnings, "pinned skill "+skillID+" lookup failed")
			continue
		}
		if !ok || !routableSkillRecord(pinnedRecord) || pinnedRecord.Manifest.Scope != source.manifest.Scope {
			warnings = append(warnings, "pinned skill "+skillID+"@"+pinned+" is not routable")
			continue
		}
		copy := pinnedRecord
		selected[skillID] = effectiveSkillSource{manifest: pinnedRecord.Manifest, record: &copy}
	}

	ids := make([]string, 0, len(selected))
	for id := range selected {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	manifests := make([]skills.Manifest, 0, len(ids))
	artifacts := make([]skills.Artifact, 0, len(ids))
	for _, id := range ids {
		source := selected[id]
		resolved, ok, warning := r.resolveSource(ctx, source, builtinByID[id])
		if warning != "" {
			warnings = append(warnings, warning)
		}
		if !ok {
			continue
		}
		manifests = append(manifests, resolved.manifest)
		if resolved.artifact != nil {
			artifacts = append(artifacts, *resolved.artifact)
		}
	}

	registry, err := skills.NewRegistry(manifests...)
	if err != nil {
		return SkillRuntimeSnapshot{}, err
	}
	knowledge, err := skills.NewArtifactKnowledgeStore(registry, artifacts...)
	if err != nil {
		return SkillRuntimeSnapshot{}, err
	}
	return SkillRuntimeSnapshot{
		Engine:             skills.NewEngine(registry, knowledge),
		PreferenceAffinity: preferenceAffinity,
		Warnings:           warnings,
	}, nil
}

func (r *SkillRuntime) resolveSource(ctx context.Context, source, builtinFallback effectiveSkillSource) (effectiveSkillSource, bool, string) {
	if source.record == nil {
		return source, true, ""
	}
	fallbackAllowed := source.manifest.Scope == skills.ScopeSystem && builtinFallback.manifest.ID == source.manifest.ID
	fallback := func(message string) (effectiveSkillSource, bool, string) {
		if fallbackAllowed {
			return builtinFallback, true, message + "; using bounded built-in fallback"
		}
		return effectiveSkillSource{}, false, message
	}
	if r == nil || r.packages == nil {
		return fallback("skill package store unavailable for " + source.manifest.ID)
	}
	pkg, ok, err := r.packages.Get(ctx, source.record.PackageHash)
	if err != nil {
		return fallback("skill package load failed for " + source.manifest.ID)
	}
	if !ok {
		return fallback("skill package missing for " + source.manifest.ID)
	}
	if pkg.PackageHash != source.record.PackageHash || pkg.Artifact.Manifest.ContentHash != source.record.ArtifactHash || !reflect.DeepEqual(pkg.Manifest, source.record.Manifest) {
		return fallback("skill package registry mismatch for " + source.manifest.ID)
	}
	artifact := pkg.Artifact
	return effectiveSkillSource{manifest: pkg.Manifest, artifact: &artifact, record: source.record}, true, ""
}

func routableSkillRecord(record SkillVersionRecord) bool {
	if err := record.Manifest.Validate(); err != nil {
		return false
	}
	switch record.State {
	case SkillVersionActive, SkillVersionPromoted:
		return true
	default:
		return false
	}
}

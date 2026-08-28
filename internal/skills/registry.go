package skills

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu       sync.RWMutex
	versions map[string]map[string]Manifest
	current  map[string]string
}

func NewRegistry(manifests ...Manifest) (*Registry, error) {
	r := &Registry{
		versions: make(map[string]map[string]Manifest, len(manifests)),
		current:  make(map[string]string, len(manifests)),
	}
	for _, manifest := range manifests {
		if err := r.Put(manifest); err != nil {
			return nil, err
		}
	}
	return r, nil
}

// Put stores an immutable skill version. Adding a newer version never silently
// changes the stable/current version; rollout/promotion must call
// SetCurrentVersion explicitly.
func (r *Registry) Put(manifest Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	id := strings.TrimSpace(manifest.ID)
	version := strings.TrimSpace(manifest.Version)
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.versions[id] == nil {
		r.versions[id] = map[string]Manifest{}
	}
	if _, exists := r.versions[id][version]; exists {
		return fmt.Errorf("skill %s@%s already exists", id, version)
	}
	r.versions[id][version] = manifest
	if _, exists := r.current[id]; !exists {
		r.current[id] = version
	}
	return nil
}

// SetCurrentVersion is the promotion pointer. The referenced immutable version
// must already exist in the registry.
func (r *Registry) SetCurrentVersion(id, version string) error {
	id = strings.TrimSpace(id)
	version = strings.TrimSpace(version)
	r.mu.Lock()
	defer r.mu.Unlock()
	versions := r.versions[id]
	if versions == nil {
		return fmt.Errorf("skill %s does not exist", id)
	}
	if _, ok := versions[version]; !ok {
		return fmt.Errorf("skill %s@%s does not exist", id, version)
	}
	r.current[id] = version
	return nil
}

func (r *Registry) Get(id string) (Manifest, bool) {
	id = strings.TrimSpace(id)
	r.mu.RLock()
	defer r.mu.RUnlock()
	version, ok := r.current[id]
	if !ok {
		return Manifest{}, false
	}
	manifest, ok := r.versions[id][version]
	return manifest, ok
}

func (r *Registry) GetVersion(id, version string) (Manifest, bool) {
	id = strings.TrimSpace(id)
	version = strings.TrimSpace(version)
	r.mu.RLock()
	defer r.mu.RUnlock()
	manifest, ok := r.versions[id][version]
	return manifest, ok
}

func (r *Registry) Versions(id string) []Manifest {
	id = strings.TrimSpace(id)
	r.mu.RLock()
	versions := r.versions[id]
	out := make([]Manifest, 0, len(versions))
	for _, manifest := range versions {
		out = append(out, manifest)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out
}

// List returns only the promoted/current version of every skill. This is the
// catalog that routing consumes; candidate/canary versions stay addressable by
// version but cannot leak into stable routing by accident.
func (r *Registry) List() []Manifest {
	r.mu.RLock()
	out := make([]Manifest, 0, len(r.current))
	for id, version := range r.current {
		if manifest, ok := r.versions[id][version]; ok {
			out = append(out, manifest)
		}
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func BuiltinManifests() []Manifest {
	return []Manifest{
		{
			ID:        "ui-ux-pro",
			Name:      "UI/UX Pro",
			Version:   "1.0.0",
			Publisher: "CodeLocal",
			Scope:     ScopeSystem,
			Kind:      KindKnowledge,
			Intents: []string{
				"design_ui", "refactor_ui", "audit_ux", "frontend_visual_review",
			},
			Tags: []string{
				"ui", "ux", "frontend", "design", "dashboard", "landing", "accessibility", "typography", "responsive", "visual",
			},
			Stacks:    []string{"react", "nextjs", "vue", "svelte", "flutter", "swiftui", "react-native", "html", "tailwind"},
			Quality:   0.90,
			Verified:  true,
			SourceURL: "https://github.com/nextlevelbuilder/ui-ux-pro-max-skill",
			License:   "MIT",
		},
	}
}

func DefaultRegistry() *Registry {
	registry, err := NewRegistry(BuiltinManifests()...)
	if err != nil {
		panic(err)
	}
	return registry
}

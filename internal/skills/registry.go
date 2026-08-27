package skills

import (
	"fmt"
	"sort"
	"strings"
	"sync"
)

type Registry struct {
	mu     sync.RWMutex
	skills map[string]Manifest
}

func NewRegistry(manifests ...Manifest) (*Registry, error) {
	r := &Registry{skills: make(map[string]Manifest, len(manifests))}
	for _, manifest := range manifests {
		if err := r.Put(manifest); err != nil {
			return nil, err
		}
	}
	return r, nil
}

func (r *Registry) Put(manifest Manifest) error {
	if err := manifest.Validate(); err != nil {
		return err
	}
	id := strings.TrimSpace(manifest.ID)
	r.mu.Lock()
	defer r.mu.Unlock()
	if current, ok := r.skills[id]; ok && current.Version == manifest.Version {
		return fmt.Errorf("skill %s@%s already exists", id, manifest.Version)
	}
	r.skills[id] = manifest
	return nil
}

func (r *Registry) Get(id string) (Manifest, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	manifest, ok := r.skills[strings.TrimSpace(id)]
	return manifest, ok
}

func (r *Registry) List() []Manifest {
	r.mu.RLock()
	out := make([]Manifest, 0, len(r.skills))
	for _, manifest := range r.skills {
		out = append(out, manifest)
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

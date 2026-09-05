package plugins

import (
	"fmt"
	"sort"
)

type CatalogEntry struct {
	Manifest Manifest `json:"manifest"`
	Featured bool     `json:"featured,omitempty"`
}

func BuiltinCatalog() []CatalogEntry {
	return []CatalogEntry{
		builtinCatalogEntry("github", "GitHub", "Work with repositories, issues, pull requests and code review through an MCP connection.", []string{"Developer Tools", "Collaboration"}, []Capability{CapabilityExternalRead, CapabilityExternalWrite, CapabilityExternalDelete, CapabilityNetwork}, true),
		builtinCatalogEntry("notion", "Notion", "Search workspace knowledge and create or update pages from CodeLocal chat.", []string{"Productivity", "Knowledge"}, []Capability{CapabilityExternalRead, CapabilityExternalWrite, CapabilityNetwork}, true),
		builtinCatalogEntry("linear", "Linear", "Read projects and issues, then create or update engineering work from chat.", []string{"Developer Tools", "Project Management"}, []Capability{CapabilityExternalRead, CapabilityExternalWrite, CapabilityNetwork}, true),
		builtinCatalogEntry("slack", "Slack", "Find conversations and send messages through an approval-aware MCP connection.", []string{"Communication", "Collaboration"}, []Capability{CapabilityExternalRead, CapabilityExternalWrite, CapabilityNetwork}, false),
	}
}

func builtinCatalogEntry(id, name, description string, categories []string, capabilities []Capability, featured bool) CatalogEntry {
	return CatalogEntry{
		Featured: featured,
		Manifest: Manifest{
			SchemaVersion: SchemaVersion,
			ID:            id,
			Name:          name,
			Version:       "0.1.0",
			Publisher:     Publisher{ID: "codelocal", Name: "CodeLocal", Verified: true},
			Description:   description,
			Categories:    append([]string(nil), categories...),
			Scope:         ScopeSystem,
			Distribution:  DistributionInternal,
			Components: []Component{{
				Kind: ComponentAppTemplate,
				ID:   id + "-app",
				AppTemplate: &AppTemplateDefinition{
					Transport:    TransportMCPHTTP,
					Auth:         AuthDefinition{Kind: AuthOAuth2},
					Capabilities: append([]Capability(nil), capabilities...),
					Fields: []AppTemplateField{{
						Key:         "endpoint",
						Label:       "MCP endpoint",
						Required:    true,
						Description: fmt.Sprintf("HTTPS MCP endpoint for the %s integration.", name),
					}},
				},
			}},
		},
	}
}

func FindBuiltin(pluginID string) (CatalogEntry, bool) {
	for _, entry := range BuiltinCatalog() {
		if entry.Manifest.ID == pluginID {
			return entry, true
		}
	}
	return CatalogEntry{}, false
}

func ManifestCapabilities(manifest Manifest) []Capability {
	seen := map[Capability]struct{}{}
	for _, component := range manifest.Components {
		if component.App != nil {
			for _, capability := range component.App.Capabilities {
				seen[capability] = struct{}{}
			}
		}
		if component.AppTemplate != nil {
			for _, capability := range component.AppTemplate.Capabilities {
				seen[capability] = struct{}{}
			}
		}
	}
	out := make([]Capability, 0, len(seen))
	for capability := range seen {
		out = append(out, capability)
	}
	sort.Slice(out, func(i, j int) bool { return out[i] < out[j] })
	return out
}

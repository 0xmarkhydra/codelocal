package projectbrain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
)

const (
	ParserMarkdownInstructions = "markdown_instructions"
	ParserMarkdownRules        = "markdown_frontmatter_rules"
	ParserJSONProjectMetadata  = "json_project_metadata"
)

type AdapterManifest struct {
	ID             string   `json:"id"`
	Version        string   `json:"version"`
	Provider       string   `json:"provider"`
	SourceType     string   `json:"sourceType"`
	Patterns       []string `json:"patterns"`
	Classification string   `json:"classification"`
	ParserKind     string   `json:"parserKind"`
	Signer         string   `json:"signer,omitempty"`
	Signature      string   `json:"signature,omitempty"`
	Digest         string   `json:"digest"`
	BuiltIn        bool     `json:"builtIn,omitempty"`
}

type AdapterTrust struct {
	Signer            string
	SignatureVerified bool
}

type AdapterRegistry struct {
	mu       sync.RWMutex
	adapters map[string]AdapterManifest
}

func allowedDeclarativeParser(kind string) bool {
	switch kind {
	case ParserMarkdownInstructions, ParserMarkdownRules, ParserJSONProjectMetadata:
		return true
	default:
		return false
	}
}

func normalizeAdapterManifest(input AdapterManifest) AdapterManifest {
	input.ID = strings.ToLower(strings.TrimSpace(input.ID))
	input.Version = strings.TrimSpace(input.Version)
	input.Provider = strings.ToLower(strings.TrimSpace(input.Provider))
	input.SourceType = strings.ToLower(strings.TrimSpace(input.SourceType))
	input.Classification = strings.ToLower(strings.TrimSpace(input.Classification))
	input.ParserKind = strings.ToLower(strings.TrimSpace(input.ParserKind))
	input.Signer = strings.TrimSpace(input.Signer)
	input.Signature = strings.TrimSpace(input.Signature)
	input.Patterns = uniqueStrings(input.Patterns)
	sort.Strings(input.Patterns)
	return input
}

func AdapterManifestDigest(input AdapterManifest) string {
	input = normalizeAdapterManifest(input)
	input.Digest = ""
	input.Signature = ""
	payload, _ := json.Marshal(struct {
		ID             string   `json:"id"`
		Version        string   `json:"version"`
		Provider       string   `json:"provider"`
		SourceType     string   `json:"sourceType"`
		Patterns       []string `json:"patterns"`
		Classification string   `json:"classification"`
		ParserKind     string   `json:"parserKind"`
		Signer         string   `json:"signer,omitempty"`
		BuiltIn        bool     `json:"builtIn,omitempty"`
	}{input.ID, input.Version, input.Provider, input.SourceType, input.Patterns, input.Classification, input.ParserKind, input.Signer, input.BuiltIn})
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func validateAdapterManifest(input AdapterManifest, trust AdapterTrust) (AdapterManifest, error) {
	input = normalizeAdapterManifest(input)
	if input.ID == "" || input.Version == "" || input.Provider == "" || input.SourceType == "" || len(input.Patterns) == 0 || !allowedDeclarativeParser(input.ParserKind) {
		return AdapterManifest{}, errors.New("invalid declarative adapter manifest")
	}
	if input.Classification == "" {
		input.Classification = "private_project"
	}
	if input.Classification != "public_project" && input.Classification != "team_project" && input.Classification != "private_project" && input.Classification != "local_private" && input.Classification != "sensitive" {
		return AdapterManifest{}, errors.New("invalid adapter classification")
	}
	expected := AdapterManifestDigest(input)
	if input.Digest == "" {
		input.Digest = expected
	}
	if input.Digest != expected {
		return AdapterManifest{}, errors.New("adapter manifest digest mismatch")
	}
	if !input.BuiltIn {
		if !trust.SignatureVerified || strings.TrimSpace(trust.Signer) == "" || trust.Signer != input.Signer || input.Signature == "" {
			return AdapterManifest{}, errors.New("external adapter manifest is not verified")
		}
	}
	return input, nil
}

func NewAdapterRegistry() *AdapterRegistry {
	registry := &AdapterRegistry{adapters: map[string]AdapterManifest{}}
	for _, manifest := range BuiltInAdapterManifests() {
		validated, err := validateAdapterManifest(manifest, AdapterTrust{})
		if err == nil {
			registry.adapters[validated.ID] = validated
		}
	}
	return registry
}

func (r *AdapterRegistry) Register(manifest AdapterManifest, trust AdapterTrust) error {
	if r == nil {
		return errors.New("adapter registry unavailable")
	}
	validated, err := validateAdapterManifest(manifest, trust)
	if err != nil {
		return err
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.adapters == nil {
		r.adapters = map[string]AdapterManifest{}
	}
	r.adapters[validated.ID] = validated
	return nil
}

func (r *AdapterRegistry) List() []AdapterManifest {
	if r == nil {
		return nil
	}
	r.mu.RLock()
	out := make([]AdapterManifest, 0, len(r.adapters))
	for _, manifest := range r.adapters {
		out = append(out, manifest)
	}
	r.mu.RUnlock()
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out
}

func builtInAdapter(id, provider, sourceType, parserKind, classification string, patterns ...string) AdapterManifest {
	manifest := AdapterManifest{ID: id, Version: "1", Provider: provider, SourceType: sourceType, ParserKind: parserKind, Classification: classification, Patterns: patterns, BuiltIn: true}
	manifest.Digest = AdapterManifestDigest(manifest)
	return manifest
}

func BuiltInAdapterManifests() []AdapterManifest {
	return []AdapterManifest{
		builtInAdapter("agents-md", "agents", "instructions", ParserMarkdownInstructions, "private_project", "AGENTS.md", "**/AGENTS.md"),
		builtInAdapter("claude-md", "claude", "instructions", ParserMarkdownInstructions, "private_project", "CLAUDE.md", "**/CLAUDE.md"),
		builtInAdapter("claude-local-md", "claude", "instructions", ParserMarkdownInstructions, "local_private", "CLAUDE.local.md", "**/CLAUDE.local.md"),
		builtInAdapter("cursor-rules", "cursor", "rule", ParserMarkdownRules, "private_project", ".cursor/rules/**/*.mdc", ".cursor/rules/**/*.md"),
		builtInAdapter("copilot-root", "github-copilot", "instructions", ParserMarkdownInstructions, "private_project", ".github/copilot-instructions.md"),
		builtInAdapter("copilot-instructions", "github-copilot", "instructions", ParserMarkdownRules, "private_project", ".github/instructions/**/*.instructions.md"),
		builtInAdapter("codelocal-project", "codelocal", "project_metadata", ParserJSONProjectMetadata, "local_private", ".codelocal/project.json"),
	}
}

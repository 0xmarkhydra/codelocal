package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

const SkillArtifactFormatVersion = 1

type ArtifactSource struct {
	URL      string `json:"url,omitempty"`
	Ref      string `json:"ref,omitempty"`
	TreeHash string `json:"treeHash,omitempty"`
	License  string `json:"license,omitempty"`
}

type ArtifactManifest struct {
	FormatVersion int            `json:"formatVersion"`
	SkillID       string         `json:"skillId"`
	SkillVersion  string         `json:"skillVersion"`
	Source        ArtifactSource `json:"source"`
	ChunkCount    int            `json:"chunkCount"`
	ContentHash   string         `json:"contentHash"`
}

type Artifact struct {
	Manifest ArtifactManifest `json:"manifest"`
	Chunks   []KnowledgeChunk `json:"chunks"`
}

// BuildArtifact produces a portable immutable Skill payload. Ingestion happens
// outside user workspaces; Cloud and desktop can cache this artifact by
// skill/version/content hash without cloning the upstream repository per user.
func BuildArtifact(manifest Manifest, chunks []KnowledgeChunk) (Artifact, error) {
	if err := manifest.Validate(); err != nil {
		return Artifact{}, err
	}
	if manifest.Scope == ScopeSystem || manifest.Scope == ScopeCommunity {
		if strings.TrimSpace(manifest.SourceURL) == "" || strings.TrimSpace(manifest.SourceRef) == "" || strings.TrimSpace(manifest.SourceHash) == "" || strings.TrimSpace(manifest.License) == "" {
			return Artifact{}, fmt.Errorf("shared skill %s@%s requires pinned source provenance", manifest.ID, manifest.Version)
		}
	}
	normalized, err := normalizeArtifactChunks(manifest, chunks)
	if err != nil {
		return Artifact{}, err
	}
	hash, err := artifactContentHash(normalized)
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{
		Manifest: ArtifactManifest{
			FormatVersion: SkillArtifactFormatVersion,
			SkillID:       manifest.ID,
			SkillVersion:  manifest.Version,
			Source: ArtifactSource{
				URL: manifest.SourceURL, Ref: manifest.SourceRef,
				TreeHash: manifest.SourceHash, License: manifest.License,
			},
			ChunkCount:  len(normalized),
			ContentHash: hash,
		},
		Chunks: normalized,
	}, nil
}

func ValidateArtifact(artifact Artifact, registry *Registry) error {
	m := artifact.Manifest
	if m.FormatVersion != SkillArtifactFormatVersion {
		return fmt.Errorf("unsupported skill artifact format %d", m.FormatVersion)
	}
	if registry == nil {
		return fmt.Errorf("skill registry is required")
	}
	manifest, ok := registry.GetVersion(m.SkillID, m.SkillVersion)
	if !ok {
		return fmt.Errorf("skill %s@%s is not registered", m.SkillID, m.SkillVersion)
	}
	if m.ChunkCount != len(artifact.Chunks) {
		return fmt.Errorf("artifact chunk count mismatch: manifest=%d actual=%d", m.ChunkCount, len(artifact.Chunks))
	}
	normalized, err := normalizeArtifactChunks(manifest, artifact.Chunks)
	if err != nil {
		return err
	}
	hash, err := artifactContentHash(normalized)
	if err != nil {
		return err
	}
	if hash != strings.TrimSpace(m.ContentHash) {
		return fmt.Errorf("artifact content hash mismatch")
	}
	if manifest.Scope == ScopeSystem || manifest.Scope == ScopeCommunity {
		if m.Source.URL != manifest.SourceURL || m.Source.Ref != manifest.SourceRef || m.Source.TreeHash != manifest.SourceHash || m.Source.License != manifest.License {
			return fmt.Errorf("artifact provenance does not match registered skill version")
		}
	}
	return nil
}

func normalizeArtifactChunks(manifest Manifest, chunks []KnowledgeChunk) ([]KnowledgeChunk, error) {
	out := make([]KnowledgeChunk, 0, len(chunks))
	seen := map[string]struct{}{}
	for _, input := range chunks {
		chunk := input
		chunk.ID = strings.TrimSpace(chunk.ID)
		chunk.SkillID = strings.TrimSpace(chunk.SkillID)
		chunk.SkillVersion = strings.TrimSpace(chunk.SkillVersion)
		chunk.Domain = strings.TrimSpace(chunk.Domain)
		chunk.Title = strings.TrimSpace(chunk.Title)
		chunk.Content = strings.TrimSpace(chunk.Content)
		chunk.Source = strings.TrimSpace(chunk.Source)
		chunk.SourceRef = strings.TrimSpace(chunk.SourceRef)
		chunk.SourceHash = strings.TrimSpace(chunk.SourceHash)
		if chunk.ID == "" || chunk.Content == "" {
			return nil, fmt.Errorf("skill artifact chunks require id and content")
		}
		if _, duplicate := seen[chunk.ID]; duplicate {
			return nil, fmt.Errorf("duplicate skill artifact chunk %q", chunk.ID)
		}
		seen[chunk.ID] = struct{}{}
		if chunk.SkillID != manifest.ID || chunk.SkillVersion != manifest.Version {
			return nil, fmt.Errorf("chunk %s belongs to %s@%s, expected %s@%s", chunk.ID, chunk.SkillID, chunk.SkillVersion, manifest.ID, manifest.Version)
		}
		if manifest.Scope == ScopeSystem || manifest.Scope == ScopeCommunity {
			if chunk.Source != manifest.SourceURL || chunk.SourceRef != manifest.SourceRef || chunk.SourceHash != manifest.SourceHash {
				return nil, fmt.Errorf("chunk %s source provenance mismatch", chunk.ID)
			}
		}
		chunk.Tags = normalizedStrings(chunk.Tags)
		out = append(out, chunk)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].ID < out[j].ID })
	return out, nil
}

func normalizedStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	sort.Strings(out)
	return out
}

func artifactContentHash(chunks []KnowledgeChunk) (string, error) {
	payload, err := json.Marshal(chunks)
	if err != nil {
		return "", err
	}
	sum := sha256.Sum256(payload)
	return "sha256:" + hex.EncodeToString(sum[:]), nil
}

// ArtifactKnowledgeStore lets Cloud/Desktop hydrate one or more validated
// immutable artifacts into the same retrieval interface used by Planner.
type ArtifactKnowledgeStore struct {
	memory *MemoryKnowledgeStore
}

func NewArtifactKnowledgeStore(registry *Registry, artifacts ...Artifact) (*ArtifactKnowledgeStore, error) {
	chunks := []KnowledgeChunk{}
	seen := map[string]struct{}{}
	for _, artifact := range artifacts {
		if err := ValidateArtifact(artifact, registry); err != nil {
			return nil, err
		}
		key := artifact.Manifest.SkillID + "@" + artifact.Manifest.SkillVersion
		if _, duplicate := seen[key]; duplicate {
			return nil, fmt.Errorf("duplicate skill artifact %s", key)
		}
		seen[key] = struct{}{}
		chunks = append(chunks, artifact.Chunks...)
	}
	return &ArtifactKnowledgeStore{memory: NewMemoryKnowledgeStore(chunks)}, nil
}

func (s *ArtifactKnowledgeStore) Search(task TaskContext, selections []Selection, limit int) []KnowledgeMatch {
	if s == nil || s.memory == nil {
		return nil
	}
	return s.memory.Search(task, selections, limit)
}

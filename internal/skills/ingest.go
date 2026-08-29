package skills

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strings"
	"unicode/utf8"
)

type SourceDocument struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

type IngestPolicy struct {
	AllowedExtensions    []string
	ExcludedPathSegments []string
	MaxChunkBytes        int
	MaxDocumentBytes     int
	MaxDocuments         int
}

func DefaultKnowledgeIngestPolicy() IngestPolicy {
	return IngestPolicy{
		AllowedExtensions:    []string{".md", ".csv", ".json", ".yaml", ".yml", ".txt"},
		ExcludedPathSegments: []string{".git", "bin", "build", "dist", "node_modules", "scripts", "vendor"},
		MaxChunkBytes:        6 * 1024,
		MaxDocumentBytes:     512 * 1024,
		MaxDocuments:         5000,
	}
}

// BuildArtifactFromDocuments converts trusted source snapshots into a portable,
// immutable knowledge artifact. It never executes source files: only explicitly
// allowed text formats become bounded KnowledgeChunks.
func BuildArtifactFromDocuments(manifest Manifest, documents []SourceDocument, policy IngestPolicy) (Artifact, error) {
	if err := manifest.Validate(); err != nil {
		return Artifact{}, err
	}
	policy = normalizedIngestPolicy(policy)
	if len(documents) > policy.MaxDocuments {
		return Artifact{}, fmt.Errorf("skill ingestion document limit exceeded: %d > %d", len(documents), policy.MaxDocuments)
	}

	type normalizedDocument struct {
		path    string
		content string
	}
	normalized := make([]normalizedDocument, 0, len(documents))
	seen := make(map[string]struct{}, len(documents))
	for _, document := range documents {
		normalizedPath, err := normalizeSourceDocumentPath(document.Path)
		if err != nil {
			return Artifact{}, err
		}
		if _, duplicate := seen[normalizedPath]; duplicate {
			return Artifact{}, fmt.Errorf("duplicate skill source document %q", normalizedPath)
		}
		seen[normalizedPath] = struct{}{}
		if excludedSourcePath(normalizedPath, policy.ExcludedPathSegments) || !allowedSourceExtension(normalizedPath, policy.AllowedExtensions) {
			continue
		}
		content := normalizeSourceContent(document.Content)
		if content == "" {
			continue
		}
		if len(content) > policy.MaxDocumentBytes {
			return Artifact{}, fmt.Errorf("skill source document %q exceeds %d bytes", normalizedPath, policy.MaxDocumentBytes)
		}
		normalized = append(normalized, normalizedDocument{path: normalizedPath, content: content})
	}
	if len(normalized) == 0 {
		return Artifact{}, fmt.Errorf("skill ingestion produced no knowledge documents")
	}
	sort.Slice(normalized, func(i, j int) bool { return normalized[i].path < normalized[j].path })

	chunks := make([]KnowledgeChunk, 0, len(normalized))
	for _, document := range normalized {
		parts := chunkSourceDocument(document.path, document.content, policy.MaxChunkBytes)
		for index, part := range parts {
			chunks = append(chunks, KnowledgeChunk{
				ID:           stableSourceChunkID(document.path, index),
				SkillID:      manifest.ID,
				SkillVersion: manifest.Version,
				Domain:       sourceDocumentDomain(document.path),
				Title:        sourceChunkTitle(document.path, part, index, len(parts)),
				Content:      part,
				Tags:         sourceDocumentTags(document.path),
				Source:       manifest.SourceURL,
				SourceRef:    manifest.SourceRef,
				SourceHash:   manifest.SourceHash,
			})
		}
	}
	return BuildArtifact(manifest, chunks)
}

func normalizedIngestPolicy(policy IngestPolicy) IngestPolicy {
	defaults := DefaultKnowledgeIngestPolicy()
	if len(policy.AllowedExtensions) == 0 {
		policy.AllowedExtensions = defaults.AllowedExtensions
	}
	if len(policy.ExcludedPathSegments) == 0 {
		policy.ExcludedPathSegments = defaults.ExcludedPathSegments
	}
	if policy.MaxChunkBytes <= 0 {
		policy.MaxChunkBytes = defaults.MaxChunkBytes
	}
	if policy.MaxDocumentBytes <= 0 {
		policy.MaxDocumentBytes = defaults.MaxDocumentBytes
	}
	if policy.MaxDocuments <= 0 {
		policy.MaxDocuments = defaults.MaxDocuments
	}
	policy.AllowedExtensions = normalizedExtensions(policy.AllowedExtensions)
	policy.ExcludedPathSegments = normalizedStrings(policy.ExcludedPathSegments)
	return policy
}

func normalizedExtensions(values []string) []string {
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.ToLower(strings.TrimSpace(value))
		if value == "" {
			continue
		}
		if !strings.HasPrefix(value, ".") {
			value = "." + value
		}
		out = append(out, value)
	}
	return normalizedStrings(out)
}

func normalizeSourceDocumentPath(raw string) (string, error) {
	raw = strings.TrimSpace(strings.ReplaceAll(raw, "\\", "/"))
	if raw == "" || strings.HasPrefix(raw, "/") {
		return "", fmt.Errorf("invalid skill source path %q", raw)
	}
	cleaned := path.Clean(raw)
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, "../") {
		return "", fmt.Errorf("skill source path escapes root: %q", raw)
	}
	return cleaned, nil
}

func excludedSourcePath(documentPath string, excluded []string) bool {
	blocked := make(map[string]struct{}, len(excluded))
	for _, value := range excluded {
		blocked[strings.ToLower(strings.TrimSpace(value))] = struct{}{}
	}
	for _, segment := range strings.Split(documentPath, "/") {
		if _, found := blocked[strings.ToLower(segment)]; found {
			return true
		}
	}
	return false
}

func allowedSourceExtension(documentPath string, allowed []string) bool {
	extension := strings.ToLower(path.Ext(documentPath))
	for _, candidate := range allowed {
		if extension == candidate {
			return true
		}
	}
	return false
}

func normalizeSourceContent(content string) string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	content = strings.ReplaceAll(content, "\r", "\n")
	return strings.TrimSpace(content)
}

func chunkSourceDocument(documentPath, content string, maxBytes int) []string {
	if strings.EqualFold(path.Ext(documentPath), ".csv") {
		return chunkCSV(content, maxBytes)
	}
	return chunkText(content, maxBytes)
}

func chunkCSV(content string, maxBytes int) []string {
	rows := strings.Split(content, "\n")
	if len(rows) <= 1 || len(rows[0]) >= maxBytes {
		return chunkText(content, maxBytes)
	}
	header := rows[0]
	chunks := []string{}
	current := header
	for _, row := range rows[1:] {
		row = strings.TrimSpace(row)
		if row == "" {
			continue
		}
		candidate := current + "\n" + row
		if len(candidate) <= maxBytes {
			current = candidate
			continue
		}
		if current != header {
			chunks = append(chunks, current)
		}
		candidate = header + "\n" + row
		if len(candidate) <= maxBytes {
			current = candidate
			continue
		}
		chunks = append(chunks, chunkText(candidate, maxBytes)...)
		current = header
	}
	if current != header || len(chunks) == 0 {
		chunks = append(chunks, current)
	}
	return chunks
}

func chunkText(content string, maxBytes int) []string {
	if maxBytes <= 0 || len(content) <= maxBytes {
		return []string{content}
	}
	blocks := strings.Split(content, "\n\n")
	chunks := []string{}
	current := ""
	flush := func() {
		if strings.TrimSpace(current) != "" {
			chunks = append(chunks, strings.TrimSpace(current))
		}
		current = ""
	}
	for _, block := range blocks {
		block = strings.TrimSpace(block)
		if block == "" {
			continue
		}
		if len(block) > maxBytes {
			flush()
			chunks = append(chunks, splitUTF8ByBytes(block, maxBytes)...)
			continue
		}
		candidate := block
		if current != "" {
			candidate = current + "\n\n" + block
		}
		if len(candidate) > maxBytes {
			flush()
			current = block
		} else {
			current = candidate
		}
	}
	flush()
	return chunks
}

func splitUTF8ByBytes(content string, maxBytes int) []string {
	if maxBytes <= 0 || len(content) <= maxBytes {
		return []string{content}
	}
	chunks := []string{}
	for len(content) > maxBytes {
		cut := maxBytes
		for cut > 0 && !utf8.RuneStart(content[cut]) {
			cut--
		}
		if cut == 0 {
			_, size := utf8.DecodeRuneInString(content)
			cut = size
		}
		piece := strings.TrimSpace(content[:cut])
		if piece != "" {
			chunks = append(chunks, piece)
		}
		content = strings.TrimSpace(content[cut:])
	}
	if content != "" {
		chunks = append(chunks, content)
	}
	return chunks
}

func stableSourceChunkID(documentPath string, index int) string {
	sum := sha256.Sum256([]byte(fmt.Sprintf("%s\x00%d", documentPath, index)))
	return "doc:" + hex.EncodeToString(sum[:12])
}

func sourceDocumentDomain(documentPath string) string {
	segments := strings.Split(documentPath, "/")
	for index := len(segments) - 2; index >= 0; index-- {
		segment := strings.Trim(strings.ToLower(segments[index]), ".-_ ")
		if segment != "" && segment != "skills" && segment != "references" && segment != "data" {
			return segment
		}
	}
	return "general"
}

func sourceDocumentTags(documentPath string) []string {
	base := strings.TrimSuffix(path.Base(documentPath), path.Ext(documentPath))
	parts := strings.FieldsFunc(strings.ToLower(base), func(r rune) bool {
		return r == '-' || r == '_' || r == ' ' || r == '.'
	})
	return normalizedStrings(parts)
}

func sourceChunkTitle(documentPath, content string, index, total int) string {
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "#") {
			title := strings.TrimSpace(strings.TrimLeft(line, "#"))
			if title != "" {
				return title
			}
		}
	}
	title := strings.TrimSuffix(path.Base(documentPath), path.Ext(documentPath))
	if total > 1 {
		return fmt.Sprintf("%s (%d/%d)", title, index+1, total)
	}
	return title
}

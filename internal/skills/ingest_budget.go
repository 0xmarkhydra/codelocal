package skills

import "fmt"

const DefaultSkillpackTotalBytes = 32 * 1024 * 1024
const DefaultSkillpackDocumentBytes = 4 * 1024 * 1024

// BuildArtifactFromDocumentsBudgeted adds a total normalized-text budget on top
// of the existing per-document ingestion policy. This lets curated datasets be
// larger than interactive user uploads without allowing an unbounded source
// snapshot to consume memory during packaging.
func BuildArtifactFromDocumentsBudgeted(manifest Manifest, documents []SourceDocument, policy IngestPolicy, maxTotalBytes int) (Artifact, error) {
	policy = normalizedIngestPolicy(policy)
	if maxTotalBytes <= 0 {
		maxTotalBytes = DefaultSkillpackTotalBytes
	}
	total := 0
	for _, document := range documents {
		normalizedPath, err := normalizeSourceDocumentPath(document.Path)
		if err != nil {
			return Artifact{}, err
		}
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
		total += len(content)
		if total > maxTotalBytes {
			return Artifact{}, fmt.Errorf("skill ingestion total text budget exceeded: %d > %d bytes", total, maxTotalBytes)
		}
	}
	return BuildArtifactFromDocuments(manifest, documents, policy)
}

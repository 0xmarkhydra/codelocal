package projectbrain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
)

type Source struct {
	Path                      string `json:"path"`
	Provider                  string `json:"provider"`
	SourceType                string `json:"sourceType"`
	ScopePath                 string `json:"scopePath"`
	Classification            string `json:"classification"`
	ContentHash               string `json:"contentHash"`
	ParserFingerprint         string `json:"parserFingerprint"`
	AdapterVersion            string `json:"adapterVersion"`
	ParserVersion             string `json:"parserVersion"`
	SemanticNormalizerVersion string `json:"semanticNormalizerVersion"`
	Size                      int64  `json:"size"`
	BaseRevisionID            string `json:"baseRevisionId,omitempty"`
}

type Manifest struct {
	RootHash string   `json:"rootHash"`
	Sources  []Source `json:"sources"`
}

func cleanPath(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" {
		return ""
	}
	value = path.Clean(value)
	if value == "." || value == ".." || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "/") {
		return ""
	}
	return value
}

func stringValue(value any) string {
	if value == nil {
		return ""
	}
	return strings.TrimSpace(fmt.Sprint(value))
}

func int64Value(value any) int64 {
	switch typed := value.(type) {
	case int:
		return int64(typed)
	case int64:
		return typed
	case float64:
		return int64(typed)
	case jsonNumber:
		parsed, _ := strconv.ParseInt(string(typed), 10, 64)
		return parsed
	default:
		parsed, _ := strconv.ParseInt(strings.TrimSpace(fmt.Sprint(value)), 10, 64)
		return parsed
	}
}

type jsonNumber string

func sourceFromMap(value map[string]any) (Source, bool) {
	source := Source{
		Path:                      cleanPath(stringValue(value["path"])),
		Provider:                  strings.ToLower(stringValue(value["provider"])),
		SourceType:                strings.ToLower(stringValue(value["sourceType"])),
		ScopePath:                 stringValue(value["scopePath"]),
		Classification:            strings.ToLower(stringValue(value["classification"])),
		ContentHash:               strings.ToLower(stringValue(value["contentHash"])),
		ParserFingerprint:         strings.ToLower(stringValue(value["parserFingerprint"])),
		AdapterVersion:            stringValue(value["adapterVersion"]),
		ParserVersion:             stringValue(value["parserVersion"]),
		SemanticNormalizerVersion: stringValue(value["semanticNormalizerVersion"]),
		Size:                      int64Value(value["size"]),
	}
	if source.Path == "" || source.Provider == "" || source.SourceType == "" || source.ContentHash == "" || source.ParserFingerprint == "" || source.AdapterVersion == "" || source.ParserVersion == "" || source.SemanticNormalizerVersion == "" {
		return Source{}, false
	}
	if source.ScopePath == "" {
		source.ScopePath = "."
	}
	if source.Classification == "" {
		source.Classification = "private_project"
	}
	return source, true
}

func sourceKey(source Source) string {
	return strings.Join([]string{source.Path, source.Provider, source.SourceType}, "\x00")
}

func SourceFingerprint(source Source) string {
	parts := []string{
		source.Path,
		source.Provider,
		source.SourceType,
		source.ScopePath,
		source.Classification,
		source.ContentHash,
		source.ParserFingerprint,
		source.AdapterVersion,
		source.ParserVersion,
		source.SemanticNormalizerVersion,
		strconv.FormatInt(source.Size, 10),
	}
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return hex.EncodeToString(sum[:])
}

func NewManifest(sources []Source) Manifest {
	unique := make(map[string]Source, len(sources))
	for _, source := range sources {
		if source.Path == "" || source.ContentHash == "" {
			continue
		}
		unique[sourceKey(source)] = source
	}
	ordered := make([]Source, 0, len(unique))
	for _, source := range unique {
		ordered = append(ordered, source)
	}
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].Path != ordered[j].Path {
			return ordered[i].Path < ordered[j].Path
		}
		if ordered[i].Provider != ordered[j].Provider {
			return ordered[i].Provider < ordered[j].Provider
		}
		return ordered[i].SourceType < ordered[j].SourceType
	})
	h := sha256.New()
	for _, source := range ordered {
		_, _ = h.Write([]byte(SourceFingerprint(source)))
		_, _ = h.Write([]byte{0})
	}
	return Manifest{RootHash: hex.EncodeToString(h.Sum(nil)), Sources: ordered}
}

func FromProjectMap(projectMap map[string]any) Manifest {
	items := []Source{}
	switch raw := projectMap["knowledgeSources"].(type) {
	case []any:
		for _, value := range raw {
			if entry, ok := value.(map[string]any); ok {
				if source, valid := sourceFromMap(entry); valid {
					items = append(items, source)
				}
			}
		}
	case []map[string]any:
		for _, entry := range raw {
			if source, valid := sourceFromMap(entry); valid {
				items = append(items, source)
			}
		}
	}
	return NewManifest(items)
}

func CloudSafeSource(source Source) bool {
	switch strings.ToLower(strings.TrimSpace(source.Classification)) {
	case "local_private", "sensitive":
		return false
	default:
		return true
	}
}

func CloudSafeManifest(manifest Manifest) Manifest {
	sources := make([]Source, 0, len(manifest.Sources))
	for _, source := range manifest.Sources {
		if CloudSafeSource(source) {
			sources = append(sources, source)
		}
	}
	return NewManifest(sources)
}

func WithBaseRevisions(manifest Manifest, revisions map[string]string) Manifest {
	if len(revisions) == 0 {
		return manifest
	}
	out := manifest
	out.Sources = append([]Source(nil), manifest.Sources...)
	for index := range out.Sources {
		out.Sources[index].BaseRevisionID = strings.TrimSpace(revisions[sourceKey(out.Sources[index])])
	}
	return out
}

func SourceIdentityKey(source Source) string { return sourceKey(source) }

func RepositoryScopeForPath(sourcePath string, repositories []projectidentity.Repository) (string, string) {
	sourcePath = cleanPath(sourcePath)
	if sourcePath == "" {
		return "", ""
	}
	bestID := ""
	bestRoot := "."
	bestDepth := -1
	for _, repository := range repositories {
		rel := cleanPath(repository.RelativePath)
		if strings.TrimSpace(repository.RelativePath) == "." {
			rel = "."
		}
		if rel == "" {
			continue
		}
		if strings.TrimSpace(repository.ID) == "" {
			continue
		}
		matches := rel == "." || sourcePath == rel || strings.HasPrefix(sourcePath, rel+"/")
		if !matches {
			continue
		}
		depth := 0
		if rel != "." {
			depth = strings.Count(rel, "/") + 1
		}
		if depth > bestDepth {
			bestDepth = depth
			bestID = repository.ID
			bestRoot = rel
		}
	}
	if bestID == "" {
		return "", sourcePath
	}
	canonicalPath := sourcePath
	if bestRoot != "." {
		canonicalPath = strings.TrimPrefix(sourcePath, bestRoot+"/")
		if sourcePath == bestRoot {
			canonicalPath = "."
		}
	}
	return bestID, canonicalPath
}

func RepositoryIDForPath(sourcePath string, repositories []projectidentity.Repository) string {
	repositoryID, _ := RepositoryScopeForPath(sourcePath, repositories)
	return repositoryID
}

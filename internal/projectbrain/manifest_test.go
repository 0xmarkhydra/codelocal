package projectbrain

import (
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
)

func TestManifestIsStableAndIgnoresBaseRevision(t *testing.T) {
	projectMap := map[string]any{
		"knowledgeSources": []any{
			map[string]any{"path": "backend/AGENTS.md", "provider": "agents", "sourceType": "instructions", "scopePath": "backend", "classification": "private_project", "contentHash": "bbbb", "parserFingerprint": "pppp", "adapterVersion": "1", "parserVersion": "1", "semanticNormalizerVersion": "1", "size": float64(12)},
			map[string]any{"path": "AGENTS.md", "provider": "agents", "sourceType": "instructions", "scopePath": ".", "classification": "private_project", "contentHash": "aaaa", "parserFingerprint": "pppp", "adapterVersion": "1", "parserVersion": "1", "semanticNormalizerVersion": "1", "size": float64(10)},
		},
	}
	manifest := FromProjectMap(projectMap)
	if len(manifest.Sources) != 2 {
		t.Fatalf("sources=%d", len(manifest.Sources))
	}
	if manifest.Sources[0].Path != "AGENTS.md" || manifest.Sources[1].Path != "backend/AGENTS.md" {
		t.Fatalf("unstable ordering: %#v", manifest.Sources)
	}
	if len(manifest.RootHash) != 64 {
		t.Fatalf("root hash=%q", manifest.RootHash)
	}
	withBase := WithBaseRevisions(manifest, map[string]string{SourceIdentityKey(manifest.Sources[0]): "krev_base"})
	if withBase.RootHash != manifest.RootHash || withBase.Sources[0].BaseRevisionID != "krev_base" {
		t.Fatalf("base revision changed semantic manifest: before=%#v after=%#v", manifest, withBase)
	}
}

func TestCloudSafeManifestExcludesLocalPrivateAndSensitive(t *testing.T) {
	base := Source{Provider: "agents", SourceType: "instructions", ScopePath: ".", ContentHash: "hash", ParserFingerprint: "parser", AdapterVersion: "1", ParserVersion: "1", SemanticNormalizerVersion: "1", Size: 10}
	project := base
	project.Path = "AGENTS.md"
	project.Classification = "private_project"
	quality := base
	quality.Path = ".codelocal/quality.json"
	quality.Provider = "codelocal"
	quality.SourceType = "quality_policy"
	quality.Classification = "private_project"
	local := base
	local.Path = "CLAUDE.local.md"
	local.Classification = "local_private"
	sensitive := base
	sensitive.Path = "secret.rules.md"
	sensitive.Classification = "sensitive"
	manifest := CloudSafeManifest(NewManifest([]Source{project, quality, local, sensitive}))
	if len(manifest.Sources) != 2 || manifest.Sources[0].Path != ".codelocal/quality.json" || manifest.Sources[1].Path != "AGENTS.md" {
		t.Fatalf("cloud-safe sources=%#v", manifest.Sources)
	}
}

func TestManifestDedupesAndRejectsUnsafePaths(t *testing.T) {
	source := Source{Path: "AGENTS.md", Provider: "agents", SourceType: "instructions", ScopePath: ".", Classification: "private_project", ContentHash: "hash", ParserFingerprint: "parser", AdapterVersion: "1", ParserVersion: "1", SemanticNormalizerVersion: "1", Size: 10}
	manifest := NewManifest([]Source{source, source, {Path: "../AGENTS.md", Provider: "agents", SourceType: "instructions", ContentHash: "bad", ParserFingerprint: "parser", AdapterVersion: "1", ParserVersion: "1", SemanticNormalizerVersion: "1"}})
	if len(manifest.Sources) != 2 {
		// NewManifest assumes already-normalized Sources; FromProjectMap owns path validation.
		// Keep this assertion explicit so callers do not mistake it for a trust boundary.
		t.Fatalf("NewManifest should only dedupe exact identities, sources=%d", len(manifest.Sources))
	}
	fromMap := FromProjectMap(map[string]any{"knowledgeSources": []any{
		map[string]any{"path": "AGENTS.md", "provider": "agents", "sourceType": "instructions", "scopePath": ".", "classification": "private_project", "contentHash": "hash", "parserFingerprint": "parser", "adapterVersion": "1", "parserVersion": "1", "semanticNormalizerVersion": "1", "size": 10},
		map[string]any{"path": "../AGENTS.md", "provider": "agents", "sourceType": "instructions", "scopePath": ".", "classification": "private_project", "contentHash": "bad", "parserFingerprint": "parser", "adapterVersion": "1", "parserVersion": "1", "semanticNormalizerVersion": "1", "size": 10},
	}})
	if len(fromMap.Sources) != 1 || fromMap.Sources[0].Path != "AGENTS.md" {
		t.Fatalf("unsafe map entry survived: %#v", fromMap.Sources)
	}
}

func TestRepositoryIDForPathUsesDeepestRepository(t *testing.T) {
	repositories := []projectidentity.Repository{
		{ID: "root", RelativePath: "."},
		{ID: "backend", RelativePath: "services/backend"},
		{ID: "nested", RelativePath: "services/backend/plugins/auth"},
	}
	cases := map[string]string{
		"AGENTS.md":                                    "root",
		"services/backend/CLAUDE.md":                   "backend",
		"services/backend/plugins/auth/AGENTS.md":      "nested",
		"services/backend/plugins/auth/deep/CLAUDE.md": "nested",
		"services/frontend/.cursor/rules/react.mdc":    "root",
	}
	for sourcePath, want := range cases {
		if got := RepositoryIDForPath(sourcePath, repositories); got != want {
			t.Fatalf("RepositoryIDForPath(%q)=%q want=%q", sourcePath, got, want)
		}
	}
	if got := RepositoryIDForPath("../escape", repositories); got != "" {
		t.Fatalf("unsafe path mapped to repo: %s", got)
	}
	repositoryID, canonicalPath := RepositoryScopeForPath("services/backend/plugins/auth/AGENTS.md", repositories)
	if repositoryID != "nested" || canonicalPath != "AGENTS.md" {
		t.Fatalf("repository scoped canonical path = (%q,%q)", repositoryID, canonicalPath)
	}
	repositoryID, canonicalPath = RepositoryScopeForPath("services/backend/CLAUDE.md", repositories)
	if repositoryID != "backend" || canonicalPath != "CLAUDE.md" {
		t.Fatalf("backend canonical path = (%q,%q)", repositoryID, canonicalPath)
	}
}

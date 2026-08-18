package project

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/security"
)

type projectScanState struct {
	engine           *Engine
	languages        map[string]int
	manifests        []string
	sourceFiles      int
	files            int
	roots            map[string]struct{}
	modules          map[string]struct{}
	entrypoints      []string
	knowledgeSources []knowledgeSourceCandidate
	knowledgeSeen    map[string]struct{}
}

func newProjectScanState(engine *Engine) *projectScanState {
	return &projectScanState{
		engine: engine, languages: map[string]int{}, roots: map[string]struct{}{".": {}},
		modules: map[string]struct{}{}, knowledgeSeen: map[string]struct{}{},
	}
}

func (s *projectScanState) addKnowledgeSource(path, rel string) {
	candidate, ok := knowledgeSourceForPath(rel)
	if !ok {
		return
	}
	key := candidate.Provider + "\x00" + candidate.SourceType + "\x00" + candidate.Path
	if _, exists := s.knowledgeSeen[key]; exists {
		return
	}
	hydrated, ok := hydrateKnowledgeSource(path, candidate)
	if !ok {
		return
	}
	s.knowledgeSeen[key] = struct{}{}
	s.knowledgeSources = append(s.knowledgeSources, hydrated)
}

func (s *projectScanState) visit(path string, d os.DirEntry, walkErr error) error {
	if walkErr != nil {
		return nil
	}
	rel := s.engine.FS.Rel(path)
	if rel == "." {
		return nil
	}
	if security.IsSensitivePath(rel) || s.engine.FS.Ignored(rel, d.IsDir()) {
		if d.IsDir() {
			return filepath.SkipDir
		}
		return nil
	}
	if d.IsDir() {
		return nil
	}
	s.addKnowledgeSource(path, rel)
	s.files++
	s.addManifest(rel, d.Name())
	s.addSource(rel, d.Name())
	return nil
}

func (s *projectScanState) addManifest(rel, name string) {
	if _, ok := manifestNames[name]; !ok {
		return
	}
	s.manifests = append(s.manifests, rel)
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir == "." {
		dir = "."
	}
	s.roots[dir] = struct{}{}
}

func (s *projectScanState) addSource(rel, name string) {
	lang := sourceExt[strings.ToLower(filepath.Ext(name))]
	if lang == "" {
		return
	}
	s.languages[lang]++
	s.sourceFiles++
	dir := filepath.ToSlash(filepath.Dir(rel))
	if dir != "." {
		s.modules[dir] = struct{}{}
	}
	base := strings.ToLower(name)
	if base == "main.go" || base == "main.dart" || base == "main.py" || base == "index.ts" || base == "index.js" || base == "main.rs" {
		s.entrypoints = append(s.entrypoints, rel)
	}
}

func (s *projectScanState) addPortableKnowledge() {
	root := s.engine.FS.Root
	s.addKnowledgeSource(filepath.Join(root, ".codelocal", "project.json"), ".codelocal/project.json")
	s.addKnowledgeSource(filepath.Join(root, ".codelocal", "quality.json"), ".codelocal/quality.json")
}

func (s *projectScanState) knowledgeMaps() []map[string]any {
	sort.Slice(s.knowledgeSources, func(i, j int) bool {
		a, b := s.knowledgeSources[i], s.knowledgeSources[j]
		if a.Path != b.Path {
			return a.Path < b.Path
		}
		if a.Provider != b.Provider {
			return a.Provider < b.Provider
		}
		return a.SourceType < b.SourceType
	})
	if len(s.knowledgeSources) > maxKnowledgeSources {
		s.knowledgeSources = s.knowledgeSources[:maxKnowledgeSources]
	}
	out := make([]map[string]any, 0, len(s.knowledgeSources))
	for _, candidate := range s.knowledgeSources {
		out = append(out, s.engine.annotatePathMap(knowledgeSourceMap(candidate)))
	}
	return out
}

func (s *projectScanState) languageList() []string {
	out := make([]string, 0, len(s.languages))
	for language := range s.languages {
		out = append(out, language)
	}
	sort.Strings(out)
	return out
}

func projectCommands(root string) map[string][]string {
	commands := map[string][]string{"build": {}, "test": {}, "typecheck": {}, "lint": {}}
	if exists(filepath.Join(root, "package.json")) {
		commands = packageCommands(root, commands)
	}
	if exists(filepath.Join(root, "go.mod")) {
		commands["build"] = append(commands["build"], "go build ./...")
		commands["test"] = append(commands["test"], "go test ./...")
	}
	if exists(filepath.Join(root, "Cargo.toml")) {
		commands["build"] = append(commands["build"], "cargo build")
		commands["test"] = append(commands["test"], "cargo test")
	}
	if exists(filepath.Join(root, "pubspec.yaml")) {
		commands["build"] = append(commands["build"], "flutter analyze")
		commands["test"] = append(commands["test"], "flutter test")
	}
	return commands
}

func (s *projectScanState) result() map[string]any {
	knowledgeMaps := s.knowledgeMaps()
	languages := s.languageList()
	sort.Strings(s.manifests)
	sort.Strings(s.entrypoints)
	modules := keys(s.modules)
	commands := projectCommands(s.engine.FS.Root)
	return map[string]any{
		"generatedAt": time.Now().UnixMilli(), "rootName": filepath.Base(s.engine.FS.Root), "languages": languages,
		"frameworks": detectFrameworks(s.engine.FS.Root, s.manifests), "workspaceRoots": keys(s.roots), "entrypoints": s.entrypoints,
		"sourceRoots": sourceRoots(modules), "testRoots": testRoots(modules), "manifests": s.manifests, "knowledgeSources": knowledgeMaps,
		"repositories": s.engine.repositorySummaries(s.manifests, modules, s.entrypoints), "buildCommands": commands["build"], "testCommands": commands["test"],
		"lintCommands": commands["lint"], "typecheckCommands": commands["typecheck"], "modules": modules, "packageManager": packageManager(s.engine.FS.Root),
		"intelligence": map[string]any{"builtAt": time.Now().UnixMilli(), "dirty": false, "files": s.files, "sourceFiles": s.sourceFiles, "knowledgeSources": len(knowledgeMaps), "repositories": len(s.engine.Repositories.All()), "languages": languages},
	}
}

func (e *Engine) scan() (map[string]any, error) {
	state := newProjectScanState(e)
	if err := filepath.WalkDir(e.FS.Root, state.visit); err != nil {
		return nil, err
	}
	state.addPortableKnowledge()
	return state.result(), nil
}

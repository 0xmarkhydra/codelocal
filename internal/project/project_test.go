package project

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
)

func TestContextForTaskPrefersSymbolsAndCentersSnippets(t *testing.T) {
	root := t.TempDir()
	lines := make([]string, 0, 130)
	for i := 1; i <= 90; i++ {
		lines = append(lines, fmt.Sprintf("// filler line %d", i))
	}
	lines = append(lines,
		"export function saveAvatar() {",
		"  return 'saved'",
		"}",
	)
	for i := 94; i <= 125; i++ {
		lines = append(lines, fmt.Sprintf("// trailing line %d", i))
	}
	if err := os.WriteFile(filepath.Join(root, "profile.ts"), []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "settings.ts"), []byte("import { saveAvatar } from './profile'\nexport function onSave() { return saveAvatar() }\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(fs)
	defer engine.Close()

	packet, err := engine.ContextForTask(context.Background(), "fix saveAvatar settings flow", 10)
	if err != nil {
		t.Fatal(err)
	}

	symbols, _ := packet["symbols"].([]map[string]any)
	if len(symbols) == 0 {
		t.Fatalf("expected semantic/structural symbols, packet=%v", packet)
	}

	ranked, _ := packet["rankedFiles"].([]map[string]any)
	foundProfile := false
	for _, item := range ranked {
		if item["path"] == "profile.ts" {
			foundProfile = true
			if score, _ := item["score"].(int); score < 8 {
				t.Fatalf("expected symbol-weighted score for profile.ts, got %v", item["score"])
			}
		}
	}
	if !foundProfile {
		t.Fatalf("expected profile.ts in ranked files: %v", ranked)
	}

	snippets, _ := packet["snippets"].([]map[string]any)
	foundCentered := false
	for _, snippet := range snippets {
		if snippet["path"] != "profile.ts" {
			continue
		}
		content, _ := snippet["content"].(string)
		startLine, _ := snippet["startLine"].(int)
		if strings.Contains(content, "function saveAvatar") && startLine > 1 {
			foundCentered = true
		}
	}
	if !foundCentered {
		t.Fatalf("expected a symbol-centered profile.ts snippet, got %v", snippets)
	}

	edges, _ := packet["graphEdges"].([]map[string]any)
	foundEdge := false
	for _, edge := range edges {
		if edge["from"] == "settings.ts" && edge["to"] == "profile.ts" {
			foundEdge = true
		}
	}
	if !foundEdge {
		t.Fatalf("expected resolved settings.ts -> profile.ts graph edge, got %v", edges)
	}
}

func initNestedProjectRepo(t *testing.T, root, relative string) string {
	t.Helper()
	dir := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("git", "init", "-q")
	cmd.Dir = dir
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git init %s: %v\n%s", relative, err, out)
	}
	return dir
}

func TestMultiRepoProjectIntelligencePreservesRepositoryOwnership(t *testing.T) {
	root := t.TempDir()
	web := initNestedProjectRepo(t, root, "web")
	auth := initNestedProjectRepo(t, root, "backend/auth")
	payment := initNestedProjectRepo(t, root, "backend/payment")
	writeKnowledgeFixture(t, web, "package.json", `{"scripts":{"test":"node --test"}}`)
	writeKnowledgeFixture(t, web, "src/index.ts", "import { authenticate } from '../../backend/auth/api'\nexport function loginClient() { return authenticate() }\n")
	writeKnowledgeFixture(t, auth, "package.json", `{"scripts":{"test":"node --test"}}`)
	writeKnowledgeFixture(t, auth, "api.ts", "export function authenticate() { return true }\n")
	writeKnowledgeFixture(t, payment, "service.ts", "export function settleInvoice() { return true }\n")

	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(fs)
	defer engine.Close()

	projectMap, err := engine.Map(true)
	if err != nil {
		t.Fatal(err)
	}
	repositories, ok := projectMap["repositories"].([]any)
	if !ok || len(repositories) != 3 {
		t.Fatalf("expected three repository summaries, got %#v", projectMap["repositories"])
	}
	paths := map[string]bool{}
	for _, raw := range repositories {
		item, _ := raw.(map[string]any)
		paths[fmt.Sprint(item["path"])] = true
	}
	if !paths["web"] || !paths["backend/auth"] || !paths["backend/payment"] {
		t.Fatalf("repository paths missing: %#v", repositories)
	}

	symbols, err := engine.Symbols("authenticate", 20)
	if err != nil || len(symbols) == 0 {
		t.Fatalf("expected auth symbol: symbols=%#v err=%v", symbols, err)
	}
	if symbols[0]["repositoryPath"] != "backend/auth" || symbols[0]["repositoryId"] == "" {
		t.Fatalf("symbol repository provenance missing: %#v", symbols[0])
	}

	edges, err := engine.ImportGraph(50)
	if err != nil {
		t.Fatal(err)
	}
	foundCrossRepo := false
	for _, edge := range edges {
		if edge["from"] == "web/src/index.ts" && edge["to"] == "backend/auth/api.ts" {
			foundCrossRepo = edge["crossRepository"] == true && edge["fromRepositoryPath"] == "web" && edge["toRepositoryPath"] == "backend/auth"
		}
	}
	if !foundCrossRepo {
		t.Fatalf("expected repository-aware cross-repo import edge: %#v", edges)
	}

	packet, err := engine.ContextForTask(context.Background(), "fix loginClient flow", 12)
	if err != nil {
		t.Fatal(err)
	}
	ranked, _ := packet["rankedFiles"].([]map[string]any)
	foundRankedRepo := false
	for _, item := range ranked {
		if item["path"] == "backend/auth/api.ts" && item["repositoryPath"] == "backend/auth" && item["repositoryId"] != "" {
			foundRankedRepo = true
		}
	}
	if !foundRankedRepo {
		t.Fatalf("ranked file repository provenance missing: %#v", ranked)
	}
	for _, item := range ranked {
		if item["repositoryPath"] == "backend/payment" {
			t.Fatalf("unrelated payment repository leaked into focused ranked context: %#v", ranked)
		}
	}
	route, ok := packet["repositoryRoute"].(map[string]any)
	if !ok || route["mode"] != "focused" {
		t.Fatalf("expected focused repository routing, got %#v", packet["repositoryRoute"])
	}
	selected, _ := route["selectedRepositories"].([]map[string]any)
	selectedPaths := map[string]bool{}
	authHasGraphReason := false
	for _, item := range selected {
		path := fmt.Sprint(item["repositoryPath"])
		selectedPaths[path] = true
		if maxChars, _ := item["maxChars"].(int); maxChars <= 0 {
			t.Fatalf("focused repository must receive a positive context budget: %#v", item)
		}
		if path == "backend/auth" {
			for _, reason := range item["reasons"].([]string) {
				if reason == "cross-repository-graph" {
					authHasGraphReason = true
				}
			}
		}
	}
	if !selectedPaths["web"] || !selectedPaths["backend/auth"] || selectedPaths["backend/payment"] {
		t.Fatalf("unexpected repository route: %#v", selected)
	}
	if !authHasGraphReason {
		t.Fatalf("auth repository should be expanded from cross-repository graph evidence: %#v", selected)
	}
	budgetInfo, _ := packet["contextBudget"].(map[string]any)
	if budgetInfo["repositoryFocused"] != true {
		t.Fatalf("context budget must report focused routing: %#v", budgetInfo)
	}
}

func TestDiagnosticsAggregatesGitDiffCheckAcrossRepositories(t *testing.T) {
	root := t.TempDir()
	web := initNestedProjectRepo(t, root, "web")
	auth := initNestedProjectRepo(t, root, "backend/auth")
	writeKnowledgeFixture(t, web, "app.ts", "export const app = true\n")
	writeKnowledgeFixture(t, auth, "auth.go", "package auth\n")
	for _, dir := range []string{web, auth} {
		cmd := exec.Command("git", "add", ".")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git add: %v\n%s", err, out)
		}
		cmd = exec.Command("git", "-c", "user.name=CodeLocal", "-c", "user.email=test@codelocal.invalid", "commit", "-qm", "initial")
		cmd.Dir = dir
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git commit: %v\n%s", err, out)
		}
	}
	writeKnowledgeFixture(t, auth, "auth.go", "package auth  \n")

	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(fs)
	defer engine.Close()
	result, err := engine.Diagnostics(context.Background(), "", 50)
	if err != nil {
		t.Fatal(err)
	}
	diagnostics, _ := result["diagnostics"].([]map[string]any)
	foundAuth := false
	for _, item := range diagnostics {
		if item["provider"] == "git-diff-check" && item["repositoryPath"] == "backend/auth" {
			foundAuth = true
		}
		if item["repositoryPath"] == "web" {
			t.Fatalf("clean web repository should not produce diff-check warning: %#v", diagnostics)
		}
	}
	if !foundAuth {
		t.Fatalf("expected repo-aware auth diff-check warning: %#v", diagnostics)
	}
}

func TestContextTermsSupportsVietnameseAndCapsSearchFanout(t *testing.T) {
	got := contextTerms("tối ưu hiệu năng CodeLocal và đọc file context_for_task hiệu năng cache redis postgres queue worker")
	want := []string{"hiệu", "năng", "codelocal", "đọc", "file", "context_for_task", "cache", "redis"}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("context terms = %v, want %v", got, want)
	}
}

func writeKnowledgeFixture(t *testing.T, root, rel, content string) {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func knowledgeMaps(t *testing.T, projectMap map[string]any) []map[string]any {
	t.Helper()
	raw, ok := projectMap["knowledgeSources"].([]any)
	if !ok {
		t.Fatalf("knowledgeSources has unexpected type: %T", projectMap["knowledgeSources"])
	}
	out := make([]map[string]any, 0, len(raw))
	for _, item := range raw {
		entry, ok := item.(map[string]any)
		if !ok {
			t.Fatalf("knowledge source has unexpected type: %T", item)
		}
		out = append(out, entry)
	}
	return out
}

func knowledgeSnapshot(items []map[string]any) []string {
	out := make([]string, 0, len(items))
	for _, item := range items {
		out = append(out, fmt.Sprintf("%v|%v|%v|%v|%v|%v|%v", item["path"], item["provider"], item["sourceType"], item["scopePath"], item["classification"], item["contentHash"], item["parserFingerprint"]))
	}
	return out
}

func TestProjectMapDiscoversCanonicalKnowledgeSourcesWithoutSecondScanner(t *testing.T) {
	root := t.TempDir()
	fixtures := map[string]string{
		"AGENTS.md":                                            "root instructions",
		"backend/AGENTS.md":                                    "backend instructions",
		"backend/CLAUDE.md":                                    "claude backend",
		"CLAUDE.local.md":                                      "local claude preference",
		".cursor/rules/backend.mdc":                            "---\ndescription: backend\n---\nUse services.",
		".cursor/rules/ignored.mdc":                            "ignored cursor rule",
		".github/copilot-instructions.md":                      "copilot root",
		".github/instructions/go.instructions.md":              "go instructions",
		"packages/api/.cursor/rules/api.mdc":                   "---\nglobs:\n  - src/**/*.go\n---\nUse API boundaries.",
		"packages/api/.github/copilot-instructions.md":         "copilot api root",
		"packages/api/.github/instructions/go.instructions.md": "---\napplyTo:\n  - src/**/*.go\n---\nUse API Go instructions.",
		"ignored/AGENTS.md":                                    "ignored agents",
		".codelocal/project.json":                              `{"projectId":"project-test"}`,
		".codelocal/quality.json":                              `{"schemaVersion":1,"languages":{"go":{"maxFunctionLines":10}}}`,
		".codelocal/secrets.json":                              `{"token":"must-not-be-discovered"}`,
		".claude/settings.local.json":                          `{"dangerouslyAllow":"not-a-knowledge-source"}`,
		"web/node_modules/recharts/AGENTS.md":                  "dependency instructions must not leak",
		"web/.next/generated/index.ts":                         "export const generated = true",
	}
	for rel, content := range fixtures {
		writeKnowledgeFixture(t, root, rel, content)
	}
	writeKnowledgeFixture(t, root, ".gitignore", "ignored/\n.cursor/rules/ignored.mdc\n")
	outside := t.TempDir()
	writeKnowledgeFixture(t, outside, "AGENTS.md", "outside workspace")
	_ = os.MkdirAll(filepath.Join(root, "symlinked"), 0o755)
	symlinkCreated := os.Symlink(filepath.Join(outside, "AGENTS.md"), filepath.Join(root, "symlinked", "AGENTS.md")) == nil

	fs, err := localfs.New(root)
	if err != nil {
		t.Fatal(err)
	}
	engine := New(fs)
	defer engine.Close()

	firstMap, err := engine.Map(true)
	if err != nil {
		t.Fatal(err)
	}
	items := knowledgeMaps(t, firstMap)
	wantPaths := []string{
		".codelocal/project.json",
		".codelocal/quality.json",
		".cursor/rules/backend.mdc",
		".github/copilot-instructions.md",
		".github/instructions/go.instructions.md",
		"AGENTS.md",
		"CLAUDE.local.md",
		"backend/AGENTS.md",
		"backend/CLAUDE.md",
		"packages/api/.cursor/rules/api.mdc",
		"packages/api/.github/copilot-instructions.md",
		"packages/api/.github/instructions/go.instructions.md",
	}
	if len(items) != len(wantPaths) {
		t.Fatalf("knowledge source count=%d want=%d items=%v", len(items), len(wantPaths), items)
	}
	for i, item := range items {
		if got := fmt.Sprint(item["path"]); got != wantPaths[i] {
			t.Fatalf("knowledge path[%d]=%q want=%q", i, got, wantPaths[i])
		}
		if item["content"] != nil {
			t.Fatalf("raw content leaked into project map for %s", gotPath(item))
		}
		if len(fmt.Sprint(item["contentHash"])) != 64 || len(fmt.Sprint(item["parserFingerprint"])) != 64 {
			t.Fatalf("missing stable hashes for %s: %v", gotPath(item), item)
		}
	}

	byPath := map[string]map[string]any{}
	for _, item := range items {
		byPath[gotPath(item)] = item
	}
	if byPath["backend/AGENTS.md"]["scopePath"] != "backend" {
		t.Fatalf("nested AGENTS scope=%v want backend", byPath["backend/AGENTS.md"]["scopePath"])
	}
	for _, nestedConfig := range []string{
		"packages/api/.cursor/rules/api.mdc",
		"packages/api/.github/copilot-instructions.md",
		"packages/api/.github/instructions/go.instructions.md",
	} {
		if byPath[nestedConfig]["scopePath"] != "packages/api" {
			t.Fatalf("nested repository config %s scope=%v want packages/api", nestedConfig, byPath[nestedConfig]["scopePath"])
		}
	}
	if byPath["CLAUDE.local.md"]["classification"] != "local_private" {
		t.Fatalf("CLAUDE.local classification=%v", byPath["CLAUDE.local.md"]["classification"])
	}
	if byPath[".codelocal/project.json"]["sourceType"] != "project_metadata" || byPath[".codelocal/project.json"]["classification"] != "local_private" {
		t.Fatalf("unexpected CodeLocal marker classification: %v", byPath[".codelocal/project.json"])
	}
	if byPath[".codelocal/quality.json"]["sourceType"] != "quality_policy" || byPath[".codelocal/quality.json"]["classification"] != "private_project" {
		t.Fatalf("unexpected CodeLocal quality policy classification: %v", byPath[".codelocal/quality.json"])
	}
	excludedPaths := []string{"ignored/AGENTS.md", ".cursor/rules/ignored.mdc", ".codelocal/secrets.json", ".claude/settings.local.json", "web/node_modules/recharts/AGENTS.md"}
	if symlinkCreated {
		excludedPaths = append(excludedPaths, "symlinked/AGENTS.md")
	}
	for _, excluded := range excludedPaths {
		if _, exists := byPath[excluded]; exists {
			t.Fatalf("unsafe/ignored source discovered: %s", excluded)
		}
	}

	secondMap, err := engine.Map(false)
	if err != nil {
		t.Fatal(err)
	}
	if fmt.Sprint(knowledgeSnapshot(items)) != fmt.Sprint(knowledgeSnapshot(knowledgeMaps(t, secondMap))) {
		t.Fatalf("knowledge discovery is not stable across cached maps")
	}

	oldHash := fmt.Sprint(byPath["AGENTS.md"]["contentHash"])
	writeKnowledgeFixture(t, root, "AGENTS.md", "root instructions changed")
	engine.Invalidate()
	changedMap, err := engine.Map(true)
	if err != nil {
		t.Fatal(err)
	}
	changedByPath := map[string]map[string]any{}
	for _, item := range knowledgeMaps(t, changedMap) {
		changedByPath[gotPath(item)] = item
	}
	if newHash := fmt.Sprint(changedByPath["AGENTS.md"]["contentHash"]); newHash == oldHash {
		t.Fatalf("content hash did not change after source edit: %s", newHash)
	}
}

func gotPath(item map[string]any) string {
	return fmt.Sprint(item["path"])
}

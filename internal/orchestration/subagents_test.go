package orchestration

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validSubagentMD = `---
name: explorer
description: Khao sat nhanh codebase, chi doc khong sua.
mode: subagent
role: investigator
engineProfile: balanced
temperature: 0.2
tools:
  - read_*
  - search_*
  - lsp_*
permission:
  edit_*: deny
  terminal_*: deny
tokenBudget: 24000
readOnly: true
---
Ban la subagent khao sat. Chi tra AgentReport gon.
`

func TestBuiltinSubagentDefinitionsSnapshot(t *testing.T) {
	builtins := BuiltinSubagentDefinitions()
	if len(builtins) != 7 {
		t.Fatalf("expected 7 builtins, got %d", len(builtins))
	}
	names := map[string]bool{}
	for _, def := range builtins {
		names[def.Name] = true
		if def.TokenBudget <= 0 || def.MaxDepth <= 0 || def.MaxConcurrent <= 0 {
			t.Fatalf("builtin %s missing bounds", def.Name)
		}
		if def.EngineProfile == "" || def.Description == "" {
			t.Fatalf("builtin %s missing profile/description", def.Name)
		}
	}
	for _, want := range []string{"quick", "investigator", "implementer", "tester", "reviewer", "security", "deep"} {
		if !names[want] {
			t.Fatalf("missing builtin %s", want)
		}
	}
	for _, def := range builtins {
		if def.Role != SpecialistInvestigator {
			continue
		}
		for _, tool := range []string{"read.file", "search.text", "lsp.definition", "git.diff", "terminal.run"} {
			matched := false
			for _, pattern := range def.Tools {
				matched = matched || SubagentPatternMatches(pattern, tool)
			}
			if !matched {
				t.Fatalf("builtin investigator patterns %v do not admit %s", def.Tools, tool)
			}
		}
	}
}

func TestParseSubagentContentValid(t *testing.T) {
	def, err := ParseSubagentContent("explorer.md", validSubagentMD, SubagentSourceProject)
	if err != nil {
		t.Fatal(err)
	}
	if def.Name != "explorer" || def.Role != SpecialistInvestigator || !def.ReadOnly {
		t.Fatalf("unexpected parsed def: %+v", def)
	}
	if def.TokenBudget != 24000 || !def.HasTemp || def.Temperature != 0.2 {
		t.Fatalf("unexpected budgets: %+v", def)
	}
	if ResolveToolPermission(def, "edit.replace") != "deny" {
		t.Fatalf("deny must win, got %q", ResolveToolPermission(def, "edit.replace"))
	}
}

func TestParseSubagentRejectsBadNamesAndGlobs(t *testing.T) {
	mutate := func(old, next string) string { return strings.Replace(old, next, next, 1) }
	_ = mutate
	badName := strings.Replace(validSubagentMD, "name: explorer", "name: Bad_Name!", 1)
	if _, err := ParseSubagentContent("x.md", badName, SubagentSourceGlobal); err == nil {
		t.Fatalf("bad name accepted")
	}
	badGlob := strings.Replace(validSubagentMD, "  - read_*", "  - \"*\"", 1)
	if _, err := ParseSubagentContent("x.md", badGlob, SubagentSourceGlobal); err == nil {
		t.Fatalf("bad glob accepted")
	}
	badMode := strings.Replace(validSubagentMD, "mode: subagent", "mode: boss", 1)
	if _, err := ParseSubagentContent("x.md", badMode, SubagentSourceGlobal); err == nil {
		t.Fatalf("bad mode accepted")
	}
	unknownField := strings.Replace(validSubagentMD, "mode: subagent", "mode: subagent\nmodle: typo", 1)
	if _, err := ParseSubagentContent("x.md", unknownField, SubagentSourceGlobal); err == nil {
		t.Fatal("unknown frontmatter field accepted")
	}
	duplicatePermission := strings.Replace(validSubagentMD, "  edit_*: deny", "  edit_*: deny\n  edit_*: allow", 1)
	if _, err := ParseSubagentContent("x.md", duplicatePermission, SubagentSourceGlobal); err == nil {
		t.Fatal("duplicate permission key accepted")
	}
	badTemp := strings.Replace(validSubagentMD, "temperature: 0.2", "temperature: 2.5", 1)
	if _, err := ParseSubagentContent("x.md", badTemp, SubagentSourceGlobal); err == nil {
		t.Fatalf("bad temperature accepted")
	}
	noFront := "no frontmatter here"
	if _, err := ParseSubagentContent("x.md", noFront, SubagentSourceGlobal); err == nil {
		t.Fatalf("missing frontmatter accepted")
	}
	big := validSubagentMD + strings.Repeat("x", MaxSubagentPromptBytes+1)
	if _, err := ParseSubagentContent("x.md", big, SubagentSourceGlobal); err == nil {
		t.Fatalf("oversize prompt accepted")
	}
}

func TestParseSubagentRejectsSecrets(t *testing.T) {
	withSecret := validSubagentMD + "\nkey: sk-abcdefghij1234567890\n"
	if _, err := ParseSubagentContent("x.md", withSecret, SubagentSourceGlobal); err == nil {
		t.Fatalf("secret prompt accepted")
	}
}

func TestProjectLocalNarrowOnly(t *testing.T) {
	// Widen tokenBudget beyond investigator (24000) -> reject for project-local.
	wide := strings.Replace(validSubagentMD, "tokenBudget: 24000", "tokenBudget: 32000", 1)
	if _, err := ParseSubagentContent("explorer.md", wide, SubagentSourceProject); err == nil {
		t.Fatalf("project widen tokenBudget accepted")
	}
	// Widen tools to edit_* on readOnly investigator -> reject.
	wideTool := strings.Replace(validSubagentMD, "  - lsp_*", "  - lsp_*\n  - edit_*", 1)
	if _, err := ParseSubagentContent("explorer.md", wideTool, SubagentSourceProject); err == nil {
		t.Fatalf("project widen tools accepted")
	}
	// Widen readOnly false on investigator -> reject.
	wideRO := strings.Replace(validSubagentMD, "readOnly: true", "readOnly: false", 1)
	if _, err := ParseSubagentContent("explorer.md", wideRO, SubagentSourceProject); err == nil {
		t.Fatalf("project widen readOnly accepted")
	}
	// Narrow is fine: smaller budget.
	narrow := strings.Replace(validSubagentMD, "tokenBudget: 24000", "tokenBudget: 8000", 1)
	if _, err := ParseSubagentContent("explorer.md", narrow, SubagentSourceProject); err != nil {
		t.Fatalf("project narrow rejected: %v", err)
	}
	// Global layer may widen within absolute system caps.
	if _, err := ParseSubagentContent("explorer.md", wide, SubagentSourceGlobal); err != nil {
		t.Fatalf("global bounded widen should be allowed: %v", err)
	}
	tooWide := strings.Replace(validSubagentMD, "tokenBudget: 24000", "tokenBudget: 999999", 1)
	if _, err := ParseSubagentContent("explorer.md", tooWide, SubagentSourceGlobal); err == nil {
		t.Fatal("global token budget above system cap accepted")
	}
}

func TestResolveToolPermissionDenyWins(t *testing.T) {
	def, err := ParseSubagentContent("x.md", `---
name: mixed
description: Mixed permission probe.
mode: subagent
role: implementer
tools:
  - read_*
  - edit_*
permission:
  edit_*: allow
  edit_replace: deny
---
Prompt.
`, SubagentSourceGlobal)
	if err != nil {
		t.Fatal(err)
	}
	if got := ResolveToolPermission(def, "edit_replace"); got != "deny" {
		t.Fatalf("deny must win, got %q", got)
	}
	if got := ResolveToolPermission(def, "edit.write"); got != "allow" {
		t.Fatalf("allow expected, got %q", got)
	}
	if got := ResolveToolPermission(def, "read.file"); got != "" {
		t.Fatalf("no rule expected, got %q", got)
	}
}

func TestValidateAgentReportBounds(t *testing.T) {
	valid := AgentReport{BriefID: "b1", Status: "done", Summary: "ok", FilesTouched: []string{"a.go"}, Verification: "go test ./... pass"}
	if err := ValidateAgentReport(valid); err != nil {
		t.Fatal(err)
	}
	badStatus := valid
	badStatus.Status = "maybe"
	if err := ValidateAgentReport(badStatus); err == nil {
		t.Fatalf("bad status accepted")
	}
	tooMany := valid
	for i := 0; i < 21; i++ {
		tooMany.FilesTouched = append(tooMany.FilesTouched, "f.go")
	}
	if err := ValidateAgentReport(tooMany); err == nil {
		t.Fatalf("too many files accepted")
	}
	tooLong := valid
	tooLong.Summary = strings.Repeat("word ", 501)
	if err := ValidateAgentReport(tooLong); err == nil {
		t.Fatalf("too long summary accepted")
	}
}

func TestLoadSubagentRegistryPrecedence(t *testing.T) {
	globalDir := t.TempDir()
	projectDir := t.TempDir()
	globalMD := strings.Replace(validSubagentMD, "tokenBudget: 24000", "tokenBudget: 20000", 1)
	projectMD := strings.Replace(validSubagentMD, "tokenBudget: 24000", "tokenBudget: 8000", 1)
	if err := os.WriteFile(filepath.Join(globalDir, "explorer.md"), []byte(globalMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "explorer.md"), []byte(projectMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "custom.md"), []byte(strings.Replace(validSubagentMD, "name: explorer", "name: custom", 1)), 0o644); err != nil {
		t.Fatal(err)
	}
	registry, err := LoadSubagentRegistry(projectDir, globalDir)
	if err != nil {
		t.Fatal(err)
	}
	explorer, ok := registry.Get("explorer")
	if !ok || explorer.TokenBudget != 8000 || explorer.Source != SubagentSourceProject {
		t.Fatalf("project must win: %+v ok=%v", explorer, ok)
	}
	if _, ok := registry.Get("custom"); !ok {
		t.Fatalf("custom missing")
	}
	if _, ok := registry.Get("quick"); !ok {
		t.Fatalf("builtin quick missing")
	}
	// Invalid file fails loud.
	if err := os.WriteFile(filepath.Join(projectDir, "bad.md"), []byte("no frontmatter"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSubagentRegistry(projectDir, globalDir); err == nil {
		t.Fatalf("invalid file silently accepted")
	}
}

func TestLoadSubagentRegistryMissingDirs(t *testing.T) {
	registry, err := LoadSubagentRegistry(filepath.Join(t.TempDir(), "nope"), filepath.Join(t.TempDir(), "nope2"))
	if err != nil {
		t.Fatal(err)
	}
	if len(registry.List()) != 7 {
		t.Fatalf("expected 7 builtins, got %d", len(registry.List()))
	}
}

func TestLoadSubagentRegistrySkipsSymlinksAndRejectsOversizeFiles(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(t.TempDir(), "external.md")
	if err := os.WriteFile(target, []byte(validSubagentMD), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "linked.md")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	registry, err := LoadSubagentRegistry(dir, "")
	if err != nil {
		t.Fatal(err)
	}
	if def, ok := registry.Get("investigator"); !ok || def.Source != SubagentSourceBuiltin {
		t.Fatalf("symlink must not override builtin: %+v ok=%v", def, ok)
	}
	if err := os.WriteFile(filepath.Join(dir, "huge.md"), []byte(strings.Repeat("x", MaxSubagentDefinitionBytes+1)), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadSubagentRegistry(dir, ""); err == nil {
		t.Fatal("oversize definition file accepted")
	}
}

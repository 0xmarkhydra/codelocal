package project

import (
	"context"
	"fmt"
	"os"
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

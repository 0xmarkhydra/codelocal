package localfs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadWriteEditAndSensitivePolicy(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "ignored"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "ignored", "x.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	written, err := fs.Write("hello.txt", "hello\nworld\n", "")
	if err != nil {
		t.Fatal(err)
	}
	hash, _ := written["hash"].(string)
	if hash == "" {
		t.Fatal("write should return hash")
	}

	read, err := fs.Read("hello.txt", 2, 2)
	if err != nil {
		t.Fatal(err)
	}
	if read["content"] != "world" {
		t.Fatalf("unexpected range content: %#v", read["content"])
	}

	edited, err := fs.ExactEdit("hello.txt", "world", "Go", false, hash)
	if err != nil {
		t.Fatal(err)
	}
	if edited["replacements"] != 1 {
		t.Fatalf("unexpected edit result: %#v", edited)
	}

	listed, err := fs.List(".", 3, false)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := listed["entries"].([]map[string]any)
	for _, entry := range entries {
		if entry["path"] == "ignored/" || entry["path"] == "ignored/x.txt" {
			t.Fatal("gitignored path leaked into normal list")
		}
	}

	if _, err := fs.Write(".env", "SECRET=x", ""); err == nil {
		t.Fatal("sensitive path write should be blocked")
	}
}

func TestReadManyPreservesInputOrderWithParallelWorkers(t *testing.T) {
	root := t.TempDir()
	paths := make([]string, 12)
	for index := range paths {
		paths[index] = fmt.Sprintf("file-%02d.txt", index)
		if err := os.WriteFile(filepath.Join(root, paths[index]), []byte(fmt.Sprintf("content-%02d", index)), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	fs, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	fs.readConcurrency = 4

	result, err := fs.ReadMany(paths)
	if err != nil {
		t.Fatal(err)
	}
	files, _ := result["files"].([]map[string]any)
	if len(files) != len(paths) {
		t.Fatalf("files = %d, want %d", len(files), len(paths))
	}
	for index, file := range files {
		if file["path"] != paths[index] || file["content"] != fmt.Sprintf("content-%02d", index) {
			t.Fatalf("result %d lost input ordering: %#v", index, file)
		}
	}
}

func TestReadManyRejectsOversizedBatchDuringPreflight(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "large.txt"), []byte("12345"), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	fs.maxBatchBytes = 4
	if _, err := fs.ReadMany([]string{"large.txt"}); err == nil || !strings.Contains(err.Error(), "batch exceeds") {
		t.Fatalf("expected batch limit error, got %v", err)
	}
}

func TestSearchStopsAtRequestedResultLimit(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "many.txt"), []byte(strings.Repeat("needle\n", 300)), 0o644); err != nil {
		t.Fatal(err)
	}
	fs, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	result, err := fs.Search("needle", ".", 5, true, false)
	if err != nil {
		t.Fatal(err)
	}
	matches, _ := result["matches"].([]string)
	if len(matches) != 5 || result["truncated"] != true {
		t.Fatalf("unexpected bounded search result: %#v", result)
	}
}

func TestCodeLocalStateIsIgnoredAndGitIgnoreIsRepaired(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".codelocal", "worktrees", "nested"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codelocal", "worktrees", "nested", "copy.go"), []byte("package nested // private-needle\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codelocal", "project.json"), []byte(`{"projectId":"portable-project"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, ".codelocal", "rules"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, ".codelocal", "rules", "backend.md"), []byte("portable-rule\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	// Simulate the legacy blanket ignore written by older CodeLocal releases.
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(".codelocal/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	fs, err := New(root)
	if err != nil {
		t.Fatal(err)
	}
	if fs.Ignored(".codelocal/project.json", false) || fs.Ignored(".codelocal/rules/backend.md", false) {
		t.Fatal("portable CodeLocal project identity/rules must remain indexable")
	}
	if !fs.Ignored(".codelocal/worktrees", true) || !fs.Ignored(".codelocal/worktrees/nested/copy.go", false) {
		t.Fatal("CodeLocal runtime worktrees must remain hard-ignored from project indexing")
	}
	listed, err := fs.List(".", 4, true)
	if err != nil {
		t.Fatal(err)
	}
	entries, _ := listed["entries"].([]map[string]any)
	portableSeen := false
	for _, entry := range entries {
		path, _ := entry["path"].(string)
		if strings.HasPrefix(path, ".codelocal/worktrees") {
			t.Fatalf("runtime worktree leaked into list even with includeIgnored: %q", path)
		}
		if path == ".codelocal/rules/backend.md" {
			portableSeen = true
		}
	}
	if !portableSeen {
		t.Fatal("portable CodeLocal rules should remain visible to project indexing")
	}
	ignoreData, err := os.ReadFile(filepath.Join(root, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(ignoreData), ".codelocal/\n") || !strings.Contains(string(ignoreData), ".codelocal/worktrees/\n") {
		t.Fatalf(".gitignore was not migrated to runtime-only state: %q", string(ignoreData))
	}
	result, err := fs.Search("portable-rule", ".", 20, true, false)
	if err != nil {
		t.Fatal(err)
	}
	if matches, _ := result["matches"].([]string); len(matches) != 1 {
		t.Fatalf("portable CodeLocal rules should be searchable: %#v", matches)
	}
	result, err = fs.Search("private-needle", ".", 20, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if matches, _ := result["matches"].([]string); len(matches) != 0 {
		t.Fatalf("CodeLocal internal worktree leaked into search: %#v", matches)
	}
}

func BenchmarkReadMany(b *testing.B) {
	root := b.TempDir()
	paths := make([]string, 12)
	content := []byte(strings.Repeat("abcdefghijklmnopqrstuvwxyz01234\n", 16*1024))
	for index := range paths {
		paths[index] = fmt.Sprintf("bench-%02d.txt", index)
		if err := os.WriteFile(filepath.Join(root, paths[index]), content, 0o644); err != nil {
			b.Fatal(err)
		}
	}
	fs, err := New(root)
	if err != nil {
		b.Fatal(err)
	}
	fs.maxBatchBytes = int64(len(content) * len(paths) * 2)

	for _, workers := range []int{1, 6} {
		b.Run(fmt.Sprintf("workers-%d", workers), func(b *testing.B) {
			fs.readConcurrency = workers
			b.SetBytes(int64(len(content) * len(paths)))
			b.ResetTimer()
			for range b.N {
				if _, err := fs.ReadMany(paths); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

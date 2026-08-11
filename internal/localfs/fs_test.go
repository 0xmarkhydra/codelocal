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

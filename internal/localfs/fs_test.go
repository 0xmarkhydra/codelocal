package localfs

import (
	"os"
	"path/filepath"
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

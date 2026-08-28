package main

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/skills"
)

func TestRunBuildsPortablePackage(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.json")
	sourceRoot := filepath.Join(root, "source")
	if err := os.MkdirAll(filepath.Join(sourceRoot, "references"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(sourceRoot, "scripts"), 0o755); err != nil {
		t.Fatal(err)
	}

	manifest := skills.Manifest{
		ID:        "personal-review",
		Name:      "Personal Review",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     skills.ScopePersonal,
		Kind:      skills.KindKnowledge,
		Quality:   0.5,
	}
	manifestJSON, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(manifestPath, manifestJSON, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "SKILL.md"), []byte("# Review\nPrefer concrete fixes."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "references", "testing.md"), []byte("# Testing\nRun focused tests first."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "scripts", "unsafe.js"), []byte("console.log('do not ingest')"), 0o644); err != nil {
		t.Fatal(err)
	}

	var output bytes.Buffer
	if err := run([]string{"-manifest", manifestPath, "-root", sourceRoot, "-pretty=false"}, &output); err != nil {
		t.Fatal(err)
	}
	var pkg skills.Package
	if err := json.Unmarshal(output.Bytes(), &pkg); err != nil {
		t.Fatal(err)
	}
	if pkg.Manifest.ID != manifest.ID || pkg.Artifact.Manifest.SkillID != manifest.ID {
		t.Fatalf("unexpected package identity: %#v", pkg)
	}
	if len(pkg.Artifact.Chunks) != 2 {
		t.Fatalf("expected two safe knowledge chunks, got %d", len(pkg.Artifact.Chunks))
	}
	if err := skills.ValidatePackageForImport(pkg, skills.UserImportPolicy()); err != nil {
		t.Fatalf("built package should pass user import policy: %v", err)
	}
}

func TestRunWritesDeterministicPackage(t *testing.T) {
	root := t.TempDir()
	manifestPath := filepath.Join(root, "manifest.json")
	sourceRoot := filepath.Join(root, "source")
	outputA := filepath.Join(root, "a.skill.json")
	outputB := filepath.Join(root, "b.skill.json")
	if err := os.MkdirAll(sourceRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := skills.Manifest{
		ID:        "deterministic",
		Name:      "Deterministic",
		Version:   "1.0.0",
		Publisher: "user",
		Scope:     skills.ScopePersonal,
		Kind:      skills.KindKnowledge,
		Quality:   0.5,
	}
	payload, _ := json.Marshal(manifest)
	if err := os.WriteFile(manifestPath, payload, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sourceRoot, "knowledge.md"), []byte("# Stable\nSame input, same output."), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-manifest", manifestPath, "-root", sourceRoot, "-out", outputA}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	if err := run([]string{"-manifest", manifestPath, "-root", sourceRoot, "-out", outputB}, &bytes.Buffer{}); err != nil {
		t.Fatal(err)
	}
	a, err := os.ReadFile(outputA)
	if err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(outputB)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(a, b) {
		t.Fatal("skillpack output must be deterministic")
	}
}

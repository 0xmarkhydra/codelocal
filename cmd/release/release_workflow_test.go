package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func releaseWorkflowText(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", ".github", "workflows", "release-npm.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(raw)
}

func TestReleaseWorkflowPublishesOnlyFromImmutableVersionedTag(t *testing.T) {
	workflow := releaseWorkflowText(t)
	for _, required := range []string{
		"release_tag:",
		"github.ref == 'refs/heads/main'",
		"npm version \"$VERSION\" --no-git-tag-version --ignore-scripts",
		"git add package.json package-lock.json internal/version/version.go",
		"git commit -m \"release: codelocal v$VERSION\"",
		"git tag \"$TAG\" \"$release_sha\"",
		"git push origin \"refs/tags/$TAG\"",
		"gh workflow run release-npm.yml",
		"--ref \"$TAG\"",
		"if [[ \"$GITHUB_REF_TYPE\" != \"tag\" || \"$GITHUB_REF_NAME\" != \"$REQUESTED_TAG\" ]]",
		"tag_sha=\"$(git rev-list -n 1 \"$TAG\")\"",
		"if [[ \"$tag_sha\" != \"$head_sha\" ]]",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release workflow missing provenance invariant %q", required)
		}
	}
}

func TestReleaseWorkflowDoesNotMutateVersionInPublishStage(t *testing.T) {
	workflow := releaseWorkflowText(t)
	index := strings.Index(workflow, "\n  publish:\n")
	if index < 0 {
		t.Fatal("publish job not found")
	}
	publish := workflow[index:]
	for _, forbidden := range []string{
		"npm version \"$VERSION\" --no-git-tag-version",
		"runtime.replace(pattern",
		"git commit -m \"release:",
		"git tag \"$TAG\"",
	} {
		if strings.Contains(publish, forbidden) {
			t.Fatalf("publish stage must consume immutable source, found mutation %q", forbidden)
		}
	}
}

func TestReleaseWorkflowChecksPackageLockAndRuntimeVersion(t *testing.T) {
	workflow := releaseWorkflowText(t)
	for _, required := range []string{
		"require('./package-lock.json').version",
		"require('./package-lock.json').packages[''].version",
		"internal/version/version.go",
		"Release source mismatch:",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release workflow must verify all source version carriers: missing %q", required)
		}
	}
}

func TestReleaseWorkflowPackagesNativeMacOSComputerWorkers(t *testing.T) {
	workflow := releaseWorkflowText(t)
	for _, required := range []string{
		"native-macos:",
		"runner: macos-15",
		"runner: macos-15-intel",
		"swift_arch: arm64",
		"swift_arch: x86_64",
		"goarch: arm64",
		"goarch: amd64",
		"swiftc -O -target \"$SWIFT_ARCH-apple-macos14.0\" cmd/computernative/main.swift",
		"computer-native-darwin-$GOARCH",
		"actions/upload-artifact@v4",
		"needs: native-macos",
		"actions/download-artifact@v4",
		"pattern: codelocal-native-*",
		".release/npm/bin/helpers",
		"computer-native-darwin-arm64 computer-native-darwin-amd64",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release workflow must package native macOS Computer workers: missing %q", required)
		}
	}
}

func TestReleaseWorkflowKeepsNativeMacOSWorkerOptionalAtRuntime(t *testing.T) {
	workflow := releaseWorkflowText(t)
	publishIndex := strings.Index(workflow, "\n  publish:\n")
	if publishIndex < 0 {
		t.Fatal("publish job not found")
	}
	if strings.Contains(workflow, "CODELOCAL_COMPUTER_NATIVE_DAEMON=") {
		t.Fatal("release workflow must not require a development-only explicit native daemon path")
	}
	if !strings.Contains(workflow[publishIndex:], "npm run release:prepare") {
		t.Fatal("publish stage must keep the cross-platform Go package build as the compatibility base")
	}
}

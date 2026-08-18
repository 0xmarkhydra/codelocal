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
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

func TestReleaseWorkflowPublishesOnlyFromImmutableVersionedTag(t *testing.T) {
	workflow := releaseWorkflowText(t)
	for _, required := range []string{
		"release_tag:",
		"workflow_run:",
		"workflows: ['CodeLocal CI']",
		"github.event.workflow_run.conclusion == 'success'",
		"github.event.workflow_run.head_branch == 'main'",
		"ref: ${{ github.event.workflow_run.head_sha }}",
		"CI source mismatch:",
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

func TestReleaseWorkflowCannotPrepareBeforeMainCIPasses(t *testing.T) {
	workflow := releaseWorkflowText(t)
	if strings.Contains(workflow, "branches: [main]") {
		t.Fatal("release preparation must not trigger directly from a main push before CI concludes")
	}
	for _, required := range []string{
		"github.event_name == 'workflow_run'",
		"github.event.workflow_run.conclusion == 'success'",
		"github.event.workflow_run.head_branch == 'main'",
		"This manual entry point never reserves a new release from main.",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release workflow missing CI-gated prepare invariant %q", required)
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
		"codelocal-native-unsigned-${{ matrix.goarch }}",
		"sign-native-macos:",
		"needs: native-macos",
		"runs-on: macos-15",
		"APPLE_CERTIFICATE_P12_BASE64",
		"APPLE_CERTIFICATE_PASSWORD",
		"security create-keychain",
		"security import",
		"security set-key-partition-list",
		"security find-identity -v -p codesigning",
		"Remove temporary signing keychain",
		"pattern: codelocal-native-unsigned-*",
		"name: codelocal-native-signed",
		"needs: sign-native-macos",
		".release/npm/bin/helpers",
		"computer-native-darwin-arm64 computer-native-darwin-amd64",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release workflow must package native macOS Computer workers: missing %q", required)
		}
	}
}

func TestReleaseWorkflowSupportsDeveloperIDSigningAndNotarization(t *testing.T) {
	workflow := releaseWorkflowText(t)
	signIndex := strings.Index(workflow, "\n  sign-native-macos:\n")
	publishIndex := strings.Index(workflow, "\n  publish:\n")
	if signIndex < 0 || publishIndex < 0 || publishIndex <= signIndex {
		t.Fatal("dedicated native signing job must run before publish")
	}
	signingJob := workflow[signIndex:publishIndex]

	for _, required := range []string{
		"runs-on: macos-15",
		"APPLE_CERTIFICATE_P12_BASE64",
		"APPLE_CERTIFICATE_PASSWORD",
		"security create-keychain",
		"security import",
		"security set-key-partition-list",
		"security find-identity -v -p codesigning",
		"APPLE_DEVELOPER_ID_APPLICATION",
		"APPLE_DEVELOPER_ID_APPLICATION is required",
		"APPLE_ID",
		"APPLE_TEAM_ID",
		"APPLE_APP_SPECIFIC_PASSWORD",
		"codesign --force --timestamp --options runtime",
		"codesign --verify --strict",
		"Authority=Developer ID Application",
		"xcrun notarytool submit",
		"--apple-id",
		"--team-id",
		"--password",
		"--wait",
		"Remove temporary signing keychain",
		"security delete-keychain",
	} {
		if !strings.Contains(signingJob, required) {
			t.Fatalf("release workflow must support GitHub-hosted Apple signing/notarization: missing %q", required)
		}
	}
	for _, forbidden := range []string{
		"runs-on: [codelocal-signing]",
		"APPLE_NOTARY_KEYCHAIN_PROFILE",
		"actions/checkout@v6",
	} {
		if strings.Contains(signingJob, forbidden) {
			t.Fatalf("GitHub-hosted signing job must not depend on self-hosted signing state or source checkout: found %q", forbidden)
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

func TestReleaseWorkflowDryRunsFinalTarballAfterNativeWorkersAreInjected(t *testing.T) {
	workflow := releaseWorkflowText(t)
	publishIndex := strings.Index(workflow, "\n  publish:\n")
	if publishIndex < 0 {
		t.Fatal("publish job not found")
	}
	publish := workflow[publishIndex:]
	download := strings.Index(publish, "Download native macOS Computer workers")
	finalPack := strings.Index(publish, "npm pack --dry-run --json ./.release/npm")
	if download < 0 || finalPack < 0 || finalPack <= download {
		t.Fatal("publish stage must dry-run the final npm tarball after native workers are downloaded")
	}
	for _, required := range []string{
		"bin/helpers/computer-native-darwin-arm64",
		"bin/helpers/computer-native-darwin-amd64",
		"Final npm tarball is missing",
	} {
		if !strings.Contains(publish, required) {
			t.Fatalf("final tarball verification missing %q", required)
		}
	}
}

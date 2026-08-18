package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func releaseEmailWorkflowText(t *testing.T) string {
	t.Helper()
	path := filepath.Join("..", "..", ".github", "workflows", "release-email.yml")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.ReplaceAll(string(raw), "\r\n", "\n")
}

func TestReleaseEmailWaitsForImmutableTaggedPublishRun(t *testing.T) {
	workflow := releaseEmailWorkflowText(t)
	for _, required := range []string{
		"github.event.workflow_run.conclusion == 'success'",
		"ref: ${{ github.event.workflow_run.head_sha }}",
		"uses: actions/setup-node@v6",
		"node-version: '24'",
		"git tag --points-at \"$HEAD_SHA\" --list 'codelocal-v*'",
		"echo \"notify=false\" >> \"$GITHUB_OUTPUT\"",
		"immutable tag publish run will send the notification",
		"echo \"notify=true\" >> \"$GITHUB_OUTPUT\"",
		"if: steps.release.outputs.notify == 'true'",
		"Release notification source mismatch:",
	} {
		if !strings.Contains(workflow, required) {
			t.Fatalf("release email workflow missing immutable-release invariant %q", required)
		}
	}
	if strings.Contains(workflow, "github.event.workflow_run.head_branch == 'main'") {
		t.Fatal("release email must not ignore the second-stage publish run dispatched from the immutable release tag")
	}
}

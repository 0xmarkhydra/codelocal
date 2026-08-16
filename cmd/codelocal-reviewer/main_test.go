package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPrepareFixtureCreatesIsolatedReviewerProject(t *testing.T) {
	root, err := prepareFixture()
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)

	for _, path := range []string{"README.md", "AGENTS.md", "package.json", "src/calculator.js", "test/calculator.test.js"} {
		if _, err := os.Stat(filepath.Join(root, path)); err != nil {
			t.Fatalf("fixture missing %s: %v", path, err)
		}
	}
	content, err := os.ReadFile(filepath.Join(root, "src", "calculator.js"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "return null") {
		t.Fatalf("reviewer fixture no longer contains deterministic bug: %s", content)
	}
}

func TestReviewerCredentialRequiresDeploymentSecrets(t *testing.T) {
	t.Setenv("CODELOCAL_REVIEWER_SERVER_URL", "https://codelocal.cloud")
	t.Setenv("CODELOCAL_REVIEWER_CREDENTIAL_ID", "")
	t.Setenv("CODELOCAL_REVIEWER_CREDENTIAL_SECRET", "")
	if _, err := reviewerCredential(); err == nil {
		t.Fatal("reviewer credential unexpectedly succeeded without secrets")
	}
}

func TestReviewerCredentialUsesDedicatedDefaults(t *testing.T) {
	t.Setenv("CODELOCAL_REVIEWER_SERVER_URL", "https://codelocal.cloud")
	t.Setenv("CODELOCAL_REVIEWER_CREDENTIAL_ID", "credential-id")
	t.Setenv("CODELOCAL_REVIEWER_CREDENTIAL_SECRET", "credential-secret")
	t.Setenv("CODELOCAL_REVIEWER_DEVICE_ID", "")
	t.Setenv("CODELOCAL_REVIEWER_DEVICE_NAME", "")
	credential, err := reviewerCredential()
	if err != nil {
		t.Fatal(err)
	}
	if credential.DeviceID != "codelocal-openai-reviewer" || credential.DeviceName != "CodeLocal OpenAI Reviewer Sandbox" {
		t.Fatalf("unexpected reviewer identity: %#v", credential)
	}
}

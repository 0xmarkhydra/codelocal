package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/identity"
	"github.com/0xmarkhydra/codelocal/internal/runtime"
)

//go:embed fixture/* fixture/src/* fixture/test/*
var reviewerFixture embed.FS

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func envDefault(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}

func prepareStateDir() (string, error) {
	if value := strings.TrimSpace(os.Getenv("CODELOCAL_STATE_DIR")); value != "" {
		return value, os.MkdirAll(value, 0o700)
	}
	dir, err := os.MkdirTemp("", "codelocal-reviewer-state-*")
	if err == nil {
		_ = os.Setenv("CODELOCAL_STATE_DIR", dir)
	}
	return dir, err
}

func prepareFixture() (string, error) {
	root, err := os.MkdirTemp("", "codelocal-reviewer-project-*")
	if err != nil {
		return "", err
	}
	if err := fs.WalkDir(reviewerFixture, "fixture", func(path string, entry fs.DirEntry, walkErr error) error {
		return copyFixtureEntry(root, path, entry, walkErr)
	}); err != nil {
		_ = os.RemoveAll(root)
		return "", err
	}
	initializeGit(root)
	return root, nil
}

func copyFixtureEntry(root, path string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	rel := strings.TrimPrefix(strings.TrimPrefix(path, "fixture"), "/")
	if rel == "" {
		return nil
	}
	target := filepath.Join(root, filepath.FromSlash(rel))
	if entry.IsDir() {
		return os.MkdirAll(target, 0o755)
	}
	data, err := reviewerFixture.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(target, data, 0o644)
}

func runGit(root string, args ...string) error {
	command := exec.Command("git", args...)
	command.Dir = root
	command.Stdout, command.Stderr = os.Stdout, os.Stderr
	return command.Run()
}

func initializeGit(root string) {
	if _, err := exec.LookPath("git"); err != nil {
		return
	}
	_ = runGit(root, "init", "-q")
	_ = runGit(root, "config", "user.email", "reviewer@codelocal.cloud")
	_ = runGit(root, "config", "user.name", "CodeLocal Reviewer")
	_ = runGit(root, "add", ".")
	_ = runGit(root, "commit", "-q", "-m", "chore: reviewer fixture baseline")
}

func reviewerCredential() (identity.Credential, error) {
	serverURL, err := requiredEnv("CODELOCAL_REVIEWER_SERVER_URL")
	if err != nil {
		return identity.Credential{}, err
	}
	credentialID, err := requiredEnv("CODELOCAL_REVIEWER_CREDENTIAL_ID")
	if err != nil {
		return identity.Credential{}, err
	}
	secret, err := requiredEnv("CODELOCAL_REVIEWER_CREDENTIAL_SECRET")
	if err != nil {
		return identity.Credential{}, err
	}
	return identity.Credential{CredentialID: credentialID, CredentialSecret: secret, DeviceID: envDefault("CODELOCAL_REVIEWER_DEVICE_ID", "codelocal-openai-reviewer"), DeviceName: envDefault("CODELOCAL_REVIEWER_DEVICE_NAME", "CodeLocal OpenAI Reviewer Sandbox"), ServerURL: serverURL, CreatedAt: time.Now().UnixMilli()}, nil
}

func run() error {
	credential, err := reviewerCredential()
	if err != nil {
		return err
	}
	stateDir, err := prepareStateDir()
	if err != nil {
		return err
	}
	fixture, err := prepareFixture()
	if err != nil {
		return err
	}
	defer os.RemoveAll(fixture)
	fmt.Printf("CodeLocal reviewer sandbox state: %s\n", stateDir)
	fmt.Printf("CodeLocal reviewer fixture: %s\n", fixture)

	r := runtime.New(runtime.Options{BaseURL: credential.ServerURL, Credential: credential, IdleWorkspace: 24 * time.Hour})
	name := envDefault("CODELOCAL_REVIEWER_WORKSPACE_NAME", "codelocal-reviewer-project")
	workspace, err := r.Registry.Grant(fixture, name)
	if err != nil {
		return err
	}
	fmt.Printf("Reviewer workspace ready: %s (%s)\n", workspace.WorkspaceName, workspace.WorkspaceID)

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if err := r.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
		return err
	}
	return nil
}

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "reviewer sandbox:", err)
		os.Exit(1)
	}
}

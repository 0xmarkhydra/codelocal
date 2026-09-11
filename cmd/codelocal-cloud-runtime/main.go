package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/identity"
	codelocalruntime "github.com/0xmarkhydra/codelocal/internal/runtime"
	"github.com/0xmarkhydra/codelocal/internal/workspace"
)

const defaultWorkspacePath = "/workspace"

type repositorySeed struct {
	RepositoryID string `json:"repositoryId"`
	Remote       string `json:"remote"`
	RelativePath string `json:"relativePath"`
}

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func safeSeedRemote(value string) string {
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "https" && parsed.Scheme != "http") {
		return ""
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return parsed.String()
}

func safeSeedPath(value string) string {
	value = filepath.ToSlash(strings.TrimSpace(value))
	if value == "" || value == "." {
		return "."
	}
	clean := filepath.ToSlash(filepath.Clean(value))
	if clean == "." {
		return "."
	}
	if filepath.IsAbs(clean) || clean == ".." || strings.HasPrefix(clean, "../") || len(clean) > 500 {
		return ""
	}
	return clean
}

func hydrateWorkspace(ctx context.Context, root, raw string) error {
	var seeds []repositorySeed
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &seeds); err != nil || len(seeds) == 0 {
		return errors.New("cloud runtime has no valid repository hydration sources")
	}
	if len(seeds) > 64 {
		return errors.New("cloud runtime repository hydration source limit exceeded")
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return err
	}
	for _, seed := range seeds {
		remote := safeSeedRemote(seed.Remote)
		relative := safeSeedPath(seed.RelativePath)
		if remote == "" || relative == "" {
			return fmt.Errorf("invalid cloud repository source %q", seed.RepositoryID)
		}
		target := root
		if relative != "." {
			target = filepath.Join(root, filepath.FromSlash(relative))
			if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
				return err
			}
		}
		if _, err := os.Stat(filepath.Join(target, ".git")); err == nil {
			continue
		}
		if entries, err := os.ReadDir(target); err == nil && len(entries) > 0 {
			return fmt.Errorf("cloud repository target is not empty: %s", relative)
		}
		cmd := exec.CommandContext(ctx, "git", "clone", "--depth=1", "--no-tags", "--", remote, target)
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "GIT_CONFIG_NOSYSTEM=1")
		if output, err := cmd.CombinedOutput(); err != nil {
			return fmt.Errorf("hydrate repository %q: %w: %s", seed.RepositoryID, err, strings.TrimSpace(string(output)))
		}
	}
	return nil
}

func runtimeCredential(ctx context.Context, serverURL, workspaceID, runtimeSessionID, deviceID, deviceName string) (identity.Credential, error) {
	if saved, err := identity.Load(serverURL); err != nil {
		return identity.Credential{}, err
	} else if saved != nil {
		if strings.TrimSpace(saved.DeviceID) != strings.TrimSpace(deviceID) {
			return identity.Credential{}, errors.New("restored cloud runtime credential belongs to a different managed device")
		}
		_ = os.Unsetenv("CODELOCAL_RUNTIME_BOOTSTRAP_TOKEN")
		return *saved, nil
	}

	bootstrapToken, err := requiredEnv("CODELOCAL_RUNTIME_BOOTSTRAP_TOKEN")
	if err != nil {
		return identity.Credential{}, err
	}
	credential, exchangedWorkspaceID, _, err := identity.ExchangeRuntimeBootstrap(ctx, identity.RuntimeBootstrapConfig{
		ServerURL:        serverURL,
		Token:            bootstrapToken,
		RuntimeSessionID: runtimeSessionID,
		DeviceID:         deviceID,
		DeviceName:       deviceName,
	})
	_ = os.Unsetenv("CODELOCAL_RUNTIME_BOOTSTRAP_TOKEN")
	if err != nil {
		return identity.Credential{}, err
	}
	if exchangedWorkspaceID != workspaceID {
		return identity.Credential{}, errors.New("runtime bootstrap workspace binding mismatch")
	}
	// The identity file lives under CODELOCAL_STATE_DIR, outside /workspace.
	// It is captured by OpenSandbox rootfs snapshots so a restored sandbox can
	// reconnect without replaying a consumed bootstrap token. It is never copied
	// into the user's project tree.
	if err := identity.Save(credential); err != nil {
		return identity.Credential{}, fmt.Errorf("persist managed runtime credential: %w", err)
	}
	return credential, nil
}

func run(ctx context.Context) error {
	serverURL, err := requiredEnv("CODELOCAL_CLOUD_SERVER")
	if err != nil {
		return err
	}
	workspaceID, err := requiredEnv("CODELOCAL_RUNTIME_WORKSPACE_ID")
	if err != nil {
		return err
	}
	runtimeSessionID, err := requiredEnv("CODELOCAL_RUNTIME_SESSION_ID")
	if err != nil {
		return err
	}
	deviceID, err := requiredEnv("CODELOCAL_RUNTIME_DEVICE_ID")
	if err != nil {
		return err
	}
	repositoriesJSON, err := requiredEnv("CODELOCAL_RUNTIME_REPOSITORIES_JSON")
	if err != nil {
		return err
	}
	deviceName := strings.TrimSpace(os.Getenv("CODELOCAL_RUNTIME_DEVICE_NAME"))
	if deviceName == "" {
		deviceName = "CodeLocal Cloud"
	}
	workspaceName := strings.TrimSpace(os.Getenv("CODELOCAL_RUNTIME_WORKSPACE_NAME"))
	if workspaceName == "" {
		workspaceName = "Cloud Workspace"
	}
	workspacePath := strings.TrimSpace(os.Getenv("CODELOCAL_WORKSPACE_PATH"))
	if workspacePath == "" {
		workspacePath = defaultWorkspacePath
	}
	workspacePath, err = filepath.Abs(workspacePath)
	if err != nil {
		return err
	}

	credential, err := runtimeCredential(ctx, serverURL, workspaceID, runtimeSessionID, deviceID, deviceName)
	if err != nil {
		return err
	}
	if err := hydrateWorkspace(ctx, workspacePath, repositoriesJSON); err != nil {
		return err
	}
	_ = os.Unsetenv("CODELOCAL_RUNTIME_REPOSITORIES_JSON")

	if _, err := workspace.New().GrantManaged(workspaceID, workspacePath, workspaceName); err != nil {
		return fmt.Errorf("authorize managed cloud workspace: %w", err)
	}

	ready := make(chan struct{}, 1)
	machine := codelocalruntime.New(codelocalruntime.Options{
		BaseURL:       serverURL,
		Credential:    credential,
		IdleWorkspace: 20 * time.Minute,
		OnReady: func() {
			select {
			case ready <- struct{}{}:
			default:
			}
		},
	})
	defer machine.Stop()

	runErr := make(chan error, 1)
	go func() { runErr <- machine.Run(ctx) }()
	select {
	case <-ctx.Done():
		machine.Stop()
		return nil
	case err := <-runErr:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	case <-ready:
		slog.Info("CodeLocal cloud runtime ready", "workspaceId", workspaceID, "deviceId", deviceID)
	}

	select {
	case <-ctx.Done():
		machine.Stop()
		return nil
	case err := <-runErr:
		if errors.Is(err, context.Canceled) {
			return nil
		}
		return err
	}
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	if err := run(ctx); err != nil {
		slog.Error("CodeLocal cloud runtime failed", "error", err)
		os.Exit(1)
	}
}

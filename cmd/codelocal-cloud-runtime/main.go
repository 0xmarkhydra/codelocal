package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
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

func requiredEnv(name string) (string, error) {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return "", fmt.Errorf("%s is required", name)
	}
	return value, nil
}

func run(ctx context.Context) error {
	serverURL, err := requiredEnv("CODELOCAL_CLOUD_SERVER")
	if err != nil {
		return err
	}
	bootstrapToken, err := requiredEnv("CODELOCAL_RUNTIME_BOOTSTRAP_TOKEN")
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

	credential, workspaceID, _, err := identity.ExchangeRuntimeBootstrap(ctx, identity.RuntimeBootstrapConfig{
		ServerURL:        serverURL,
		Token:            bootstrapToken,
		RuntimeSessionID: runtimeSessionID,
		DeviceID:         deviceID,
		DeviceName:       deviceName,
	})
	// Bootstrap credentials are one-shot. Remove the token from the process
	// environment as soon as the exchange is complete so child processes cannot
	// inherit it accidentally.
	_ = os.Unsetenv("CODELOCAL_RUNTIME_BOOTSTRAP_TOKEN")
	if err != nil {
		return err
	}

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

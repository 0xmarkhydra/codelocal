package main

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/identity"
	"github.com/0xmarkhydra/codelocal/internal/runtimecontrol"
	"github.com/0xmarkhydra/codelocal/internal/state"
)

type cleanupOptions struct {
	all bool
	yes bool
}

var errCleanupCancelled = errors.New("cleanup cancelled")

func parseCleanupOptions(command string, args []string) (cleanupOptions, error) {
	var options cleanupOptions
	for _, arg := range args {
		switch arg {
		case "--all":
			options.all = true
		case "--yes", "-y":
			options.yes = true
		default:
			return cleanupOptions{}, fmt.Errorf("unknown %s option: %s", command, arg)
		}
	}
	if !options.all {
		return cleanupOptions{}, fmt.Errorf("%s requires --all; this permanently removes local CodeLocal data", command)
	}
	return options, nil
}

func safeCleanupDir(dir string) (string, error) {
	dir = strings.TrimSpace(dir)
	if dir == "" {
		return "", errors.New("refusing to remove an empty state directory")
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolve state directory: %w", err)
	}
	abs = filepath.Clean(abs)
	if info, statErr := os.Lstat(abs); statErr == nil && info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("refusing to remove symlinked state directory: %s", abs)
	} else if statErr != nil && !os.IsNotExist(statErr) {
		return "", fmt.Errorf("inspect state directory: %w", statErr)
	}

	home, _ := os.UserHomeDir()
	cwd, _ := os.Getwd()
	protected := []string{string(filepath.Separator), filepath.Clean(home), filepath.Clean(cwd), filepath.Clean(os.TempDir())}
	if volume := filepath.VolumeName(abs); volume != "" {
		protected = append(protected, filepath.Clean(volume+string(filepath.Separator)))
	}
	for _, candidate := range protected {
		if candidate != "" && abs == candidate {
			return "", fmt.Errorf("refusing to remove protected directory: %s", abs)
		}
	}
	return abs, nil
}

func stopRuntimeForCleanup(ctx context.Context, dir string) error {
	stopCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	_, err := runtimecontrol.Send(stopCtx, dir, "shutdown")
	cancel()
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("running CodeLocal could not be stopped safely: %w", err)
	}

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		probeCtx, probeCancel := context.WithTimeout(ctx, 250*time.Millisecond)
		summary := runtimecontrol.Summary(probeCtx, dir)
		probeCancel()
		running, _ := summary["running"].(bool)
		if !running {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(75 * time.Millisecond):
		}
	}
	return errors.New("CodeLocal runtime did not stop within 5 seconds; close it and retry")
}

func purgeLocalState(ctx context.Context, dir string) (string, error) {
	safeDir, err := safeCleanupDir(dir)
	if err != nil {
		return "", err
	}
	if err := stopRuntimeForCleanup(ctx, safeDir); err != nil {
		return "", err
	}
	if err := os.RemoveAll(safeDir); err != nil {
		return "", fmt.Errorf("remove CodeLocal state: %w", err)
	}
	return safeDir, nil
}

func confirmCleanup(command, dir string, yes bool) error {
	if yes {
		return nil
	}
	if !stdinInteractive() {
		return fmt.Errorf("%s requires an interactive confirmation; pass --yes only when intentional", command)
	}
	fmt.Println()
	fmt.Printf("CodeLocal %s --all\n", command)
	fmt.Printf("  Local data: %s\n", dir)
	fmt.Println("  Removes: login, workspace grants, approvals, indexes, history, browser runtime and automation settings")
	if command == "uninstall" {
		fmt.Println("  Package: the global codelocal npm package")
	}
	fmt.Println("  macOS/Windows/Linux privacy permissions are managed by the operating system and are not changed.")
	fmt.Println()
	confirmed, err := askYesNo(bufio.NewReader(os.Stdin), "Continue?", false)
	if err != nil {
		return err
	}
	if !confirmed {
		return errCleanupCancelled
	}
	return nil
}

func resetCommand(ctx context.Context, args []string) error {
	options, err := parseCleanupOptions("reset", args)
	if err != nil {
		return err
	}
	dir, err := safeCleanupDir(state.Dir())
	if err != nil {
		return err
	}
	if err := confirmCleanup("reset", dir, options.yes); err != nil {
		if errors.Is(err, errCleanupCancelled) {
			fmt.Println("Cancelled. No CodeLocal data was removed.")
			return nil
		}
		return err
	}
	removed, err := purgeLocalState(ctx, dir)
	if err != nil {
		return err
	}
	fmt.Printf("✓ CodeLocal local data removed: %s\n", removed)
	fmt.Println("Run `codelocal` to start first-time setup again.")
	return nil
}

func uninstallNPMPackage(ctx context.Context) error {
	npm, err := exec.LookPath("npm")
	if err != nil {
		return errors.New("local data was removed, but npm was not found; run `npm uninstall -g codelocal` manually")
	}
	cmd := exec.CommandContext(ctx, npm, "uninstall", "-g", "codelocal")
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("local data was removed, but npm uninstall failed: %w", err)
	}
	return nil
}

func uninstallCommand(ctx context.Context, args []string) error {
	options, err := parseCleanupOptions("uninstall", args)
	if err != nil {
		return err
	}
	dir, err := safeCleanupDir(state.Dir())
	if err != nil {
		return err
	}
	if err := confirmCleanup("uninstall", dir, options.yes); err != nil {
		if errors.Is(err, errCleanupCancelled) {
			fmt.Println("Cancelled. CodeLocal remains installed and no data was removed.")
			return nil
		}
		return err
	}

	if credential, loadErr := identity.Load(""); loadErr == nil && credential != nil {
		if revoked, revokeErr := revokeRemoteCredential(ctx, *credential, ""); revokeErr != nil || !revoked {
			fmt.Println("! CodeLocal Cloud could not confirm device revocation. Local removal will continue; revoke the old device from the dashboard if it remains listed.")
		}
	}
	removed, err := purgeLocalState(ctx, dir)
	if err != nil {
		return err
	}
	fmt.Printf("✓ CodeLocal local data removed: %s\n", removed)
	fmt.Println("Removing the global codelocal npm package...")
	if err := uninstallNPMPackage(ctx); err != nil {
		return err
	}
	fmt.Println("✓ CodeLocal uninstalled.")
	return nil
}

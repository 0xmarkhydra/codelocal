package automation

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/state"
)

const settingsVersion = 3

type BrowserSettings struct {
	Enabled  bool `json:"enabled"`
	Prepared bool `json:"prepared,omitempty"`
}

type ComputerSettings struct {
	Enabled bool `json:"enabled"`
}

type Settings struct {
	Version     int              `json:"version"`
	CompletedAt int64            `json:"completedAt"`
	Coding      bool             `json:"coding"`
	Browser     BrowserSettings  `json:"browser"`
	Computer    ComputerSettings `json:"computer"`
}

type Environment struct {
	Platform          string   `json:"platform"`
	BrowserCLI        string   `json:"browserCli,omitempty"`
	BrowserReady      bool     `json:"browserReady"`
	ComputerSupported bool     `json:"computerSupported"`
	ComputerBackend   string   `json:"computerBackend,omitempty"`
	Notes             []string `json:"notes,omitempty"`
}

func Path() string { return filepath.Join(state.Dir(), "automation.json") }

// BrowserRuntimeDir keeps CodeLocal's managed browser inside CodeLocal state.
// This avoids deleting or depending on Playwright caches shared by unrelated
// projects and lets `codelocal reset --all` remove the complete local runtime.
func BrowserRuntimeDir() string { return filepath.Join(state.Dir(), "browser-runtime") }

func browserCommandEnv(extra ...string) []string {
	env := os.Environ()
	key := "PLAYWRIGHT_BROWSERS_PATH="
	filtered := env[:0]
	for _, entry := range env {
		if !strings.HasPrefix(entry, key) {
			filtered = append(filtered, entry)
		}
	}
	filtered = append(filtered, key+BrowserRuntimeDir())
	return append(filtered, extra...)
}

func Default() Settings {
	return Settings{
		Version: settingsVersion,
		Coding:  true,
		Browser: BrowserSettings{Enabled: true},
		// Full computer control is intentionally opt-in because it can view the
		// screen and synthesize pointer/keyboard input.
		Computer: ComputerSettings{Enabled: false},
	}
}

func Load() (*Settings, error) {
	var settings Settings
	if err := state.ReadJSON(Path(), &settings); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	originalVersion := settings.Version
	if settings.Version <= 0 {
		settings.Version = 1
	}
	// Coding is the core CodeLocal capability. Older/pre-release settings that
	// did not contain the field must not accidentally disable the runtime.
	settings.Coding = true
	// Versions before 3 used Playwright's shared machine cache. Force one
	// idempotent preparation so enabled installs move into CodeLocal-owned state.
	if originalVersion < 3 && settings.Browser.Enabled {
		settings.Browser.Prepared = false
	}
	return &settings, nil
}

func Save(settings Settings) error {
	settings.Version = settingsVersion
	settings.Coding = true
	if settings.CompletedAt == 0 {
		settings.CompletedAt = time.Now().UnixMilli()
	}
	return state.WriteJSONAtomic(Path(), settings)
}

func Detect() Environment {
	env := Environment{Platform: runtime.GOOS, Notes: []string{}}
	env.BrowserCLI = BrowserCLIPath()
	env.BrowserReady = env.BrowserCLI != ""

	switch runtime.GOOS {
	case "darwin":
		env.ComputerSupported = true
		env.ComputerBackend = "macos-accessibility+screencapturekit"
		env.Notes = append(env.Notes, "Computer Use requires macOS Accessibility and Screen Recording permissions.")
	case "windows":
		env.ComputerSupported = true
		env.ComputerBackend = "windows-uia+graphics-capture"
		env.Notes = append(env.Notes, "Windows secure desktop and UAC prompts remain outside Computer Use control.")
	case "linux":
		if strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY")) != "" {
			env.ComputerSupported = commandExists("gdbus") || commandExists("busctl")
			env.ComputerBackend = "wayland-xdg-desktop-portal"
			env.Notes = append(env.Notes, "Wayland Computer Use depends on the desktop portal/compositor capabilities granted by the user.")
		} else if strings.TrimSpace(os.Getenv("DISPLAY")) != "" {
			env.ComputerSupported = true
			env.ComputerBackend = "x11+at-spi"
		} else {
			env.ComputerBackend = "linux-headless"
			env.Notes = append(env.Notes, "No graphical Linux session was detected.")
		}
	}
	return env
}

// EnsureBrowserRuntime installs Playwright's managed browser only after the
// user opted into Browser Automation. npm supplies the CLI with CodeLocal, so
// the user never needs a second install command.
func EnsureBrowserRuntime(ctx context.Context, progress io.Writer) error {
	cli := BrowserCLIPath()
	if cli == "" {
		return errors.New("bundled Playwright CLI was not found")
	}
	if err := state.EnsurePrivateDir(BrowserRuntimeDir()); err != nil {
		return fmt.Errorf("prepare CodeLocal browser directory: %w", err)
	}
	// The managed automation runtime defaults to Chromium. Installing only that
	// engine avoids downloading unused Firefox and WebKit bundles on first run.
	cmd := exec.CommandContext(ctx, cli, "install-browser", "chromium")
	cmd.Env = browserCommandEnv("CI=1")
	var captured bytes.Buffer
	if progress != nil {
		writer := io.MultiWriter(progress, &captured)
		cmd.Stdout = writer
		cmd.Stderr = writer
	} else {
		cmd.Stdout = &captured
		cmd.Stderr = &captured
	}
	err := cmd.Run()
	if err != nil {
		message := strings.TrimSpace(captured.String())
		if message == "" {
			message = err.Error()
		}
		return fmt.Errorf("install Playwright browser: %s", message)
	}
	return nil
}

func BrowserCLIPath() string {
	candidates := []string{}
	if configured := strings.TrimSpace(os.Getenv("CODELOCAL_PLAYWRIGHT_CLI")); configured != "" {
		candidates = append(candidates, configured)
	}
	if root := strings.TrimSpace(os.Getenv("CODELOCAL_PACKAGE_ROOT")); root != "" {
		name := "playwright-cli"
		if runtime.GOOS == "windows" {
			name += ".cmd"
		}
		candidates = append(candidates, filepath.Join(root, "node_modules", ".bin", name))
	}
	for _, candidate := range candidates {
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if path, err := exec.LookPath("playwright-cli"); err == nil {
		return path
	}
	return ""
}

func commandExists(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}

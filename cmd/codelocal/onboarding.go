package main

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/automation"
)

// The no-argument `codelocal` command is the runtime entrypoint. Run the
// one-time capability setup before main reaches runRuntime, so the runtime is
// never acquired/started while the user is still deciding permissions.
func init() {
	if len(os.Args) != 1 {
		return
	}
	if _, _, err := ensureFirstRunSetup(); err != nil {
		fmt.Fprintf(os.Stderr, "CodeLocal setup failed: %v\n", err)
		os.Exit(1)
	}
}

func stdinInteractive() bool {
	info, err := os.Stdin.Stat()
	return err == nil && info.Mode()&os.ModeCharDevice != 0
}

func askYesNo(reader *bufio.Reader, label string, def bool) (bool, error) {
	suffix := "[y/N]"
	if def {
		suffix = "[Y/n]"
	}
	for {
		fmt.Printf("%s %s ", label, suffix)
		line, err := reader.ReadString('\n')
		if err != nil && len(line) == 0 {
			return def, err
		}
		switch strings.ToLower(strings.TrimSpace(line)) {
		case "":
			return def, nil
		case "y", "yes":
			return true, nil
		case "n", "no":
			return false, nil
		default:
			fmt.Println("Please answer y or n.")
		}
	}
}

func prepareBrowser(settings *automation.Settings, environment automation.Environment, verbose bool) {
	if settings == nil || !settings.Browser.Enabled || settings.Browser.Prepared || !environment.BrowserReady {
		return
	}
	if verbose {
		fmt.Println("Preparing Browser Automation...")
		fmt.Println("  CodeLocal is installing one managed Chromium browser for website automation.")
		fmt.Println("  The first download may take several minutes, depending on your network.")
		fmt.Println("  Progress from Playwright will appear below. Press Ctrl+C to cancel;")
		fmt.Println("  rerun codelocal and choose n for Browser Automation to skip it.")
		fmt.Println()
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	var progress *os.File
	if verbose {
		progress = os.Stdout
	}
	done := make(chan error, 1)
	startedAt := time.Now()
	go func() {
		done <- automation.EnsureBrowserRuntime(ctx, progress)
	}()
	ticker := time.NewTicker(15 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case err := <-done:
			if err == nil {
				settings.Browser.Prepared = true
				if verbose {
					fmt.Println("✓ Browser Automation ready")
				}
				return
			}
			if verbose {
				fmt.Printf("! Browser runtime preparation failed: %v\n", err)
				fmt.Println("  CodeLocal will retry automatically next time; Coding can still start now.")
			}
			return
		case <-ticker.C:
			if verbose {
				fmt.Printf("  Still preparing Browser Automation... %s elapsed\n", time.Since(startedAt).Round(time.Second))
			}
		}
	}
}

func ensureFirstRunSetup() (automation.Settings, automation.Environment, error) {
	existing, err := automation.Load()
	if err != nil {
		return automation.Settings{}, automation.Environment{}, err
	}
	environment := automation.Detect()
	interactive := stdinInteractive()
	if existing != nil {
		beforePrepared := existing.Browser.Prepared
		prepareBrowser(existing, environment, interactive)
		if existing.Browser.Prepared != beforePrepared {
			if err := automation.Save(*existing); err != nil {
				return automation.Settings{}, environment, err
			}
		}
		return *existing, environment, nil
	}

	settings := automation.Default()
	if !interactive {
		// A service manager, CI job, SSH pipe or background launcher cannot give
		// meaningful first-run consent. Do not persist choices and do not trigger
		// a browser download. Start Coding-only for this process; the next normal
		// interactive `codelocal` run will still show the setup screen.
		settings.Browser.Enabled = false
		settings.Computer.Enabled = false
		fmt.Println("· Interactive CodeLocal setup has not been completed yet; starting Coding only for this session.")
		fmt.Println("  Run `codelocal` in a terminal later to choose Browser and Computer permissions.")
		return settings, environment, nil
	}
	return promptAutomationSetup(settings, environment, true)
}

func promptAutomationSetup(settings automation.Settings, environment automation.Environment, startRuntime bool) (automation.Settings, automation.Environment, error) {
	fmt.Println()
	if startRuntime {
		fmt.Println("CodeLocal first-time setup")
	} else {
		fmt.Println("CodeLocal capability setup")
	}
	fmt.Println()
	fmt.Println("Choose what connected MCP clients may use on this computer. Coding is always available inside workspaces you explicitly authorize.")
	fmt.Println()
	fmt.Println("✓ Coding")
	fmt.Println("  Read/edit authorized workspaces, Git and guarded terminal tools")
	fmt.Println("  Preview safety: terminal commands run on this host after policy/approval checks; CodeLocal does not provide an OS-level sandbox yet.")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Browser Automation")
	fmt.Println("  Let compatible AI clients open, inspect, click, type and screenshot websites in an isolated browser.")
	fmt.Println("  Downloads one managed Chromium browser on first use; Coding works without it.")
	var err error
	settings.Browser.Enabled, err = askYesNo(reader, "Enable Browser Automation?", settings.Browser.Enabled)
	if err != nil {
		return automation.Settings{}, environment, err
	}
	fmt.Println()

	if environment.ComputerSupported {
		fmt.Println("Computer Use")
		fmt.Println("  Let compatible AI clients inspect and control desktop apps using screen, pointer and keyboard tools.")
		fmt.Println("  No browser download is needed; macOS/Windows/Linux permissions and action approvals still apply.")
		settings.Computer.Enabled, err = askYesNo(reader, "Enable Computer Use?", settings.Computer.Enabled)
		if err != nil {
			return automation.Settings{}, environment, err
		}
	} else {
		settings.Computer.Enabled = false
		fmt.Println("Computer Use")
		fmt.Println("  Not available in this graphical session, so it stays disabled.")
	}
	fmt.Println()

	if settings.Browser.Enabled {
		if !environment.BrowserReady {
			fmt.Println("! Bundled Playwright CLI was not detected. Coding will still start; update or repair CodeLocal to enable Browser Automation.")
		} else {
			prepareBrowser(&settings, environment, true)
		}
	}
	if settings.Computer.Enabled {
		fmt.Printf("✓ Computer Use preference enabled for %s\n", environment.ComputerBackend)
		for _, note := range environment.Notes {
			fmt.Printf("  %s\n", note)
		}
	}

	if err := automation.Save(settings); err != nil {
		return automation.Settings{}, environment, err
	}
	fmt.Println()
	fmt.Println("✓ Setup saved locally.")
	if startRuntime {
		fmt.Println("Starting CodeLocal runtime...")
	} else {
		fmt.Println("Restart CodeLocal so active workspace sessions advertise the updated capabilities.")
	}
	fmt.Println()
	return settings, environment, nil
}

func rerunAutomationSetup() error {
	if !stdinInteractive() {
		return fmt.Errorf("codelocal setup requires an interactive terminal")
	}
	settings := automation.Default()
	if existing, err := automation.Load(); err != nil {
		return err
	} else if existing != nil {
		settings = *existing
	}
	_, _, err := promptAutomationSetup(settings, automation.Detect(), false)
	return err
}

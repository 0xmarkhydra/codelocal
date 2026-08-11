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
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()
	if err := automation.EnsureBrowserRuntime(ctx); err != nil {
		if verbose {
			fmt.Printf("! Browser runtime preparation failed: %v\n", err)
			fmt.Println("  CodeLocal will retry automatically next time; Coding can still start now.")
		}
		return
	}
	settings.Browser.Prepared = true
	if verbose {
		fmt.Println("✓ Browser Automation ready")
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

	fmt.Println()
	fmt.Println("CodeLocal first-time setup")
	fmt.Println()
	fmt.Println("Choose what ChatGPT may use on this computer. Coding is always available inside workspaces you explicitly authorize.")
	fmt.Println()
	fmt.Println("✓ Coding")
	fmt.Println("  Read/edit authorized workspaces, Git and guarded terminal tools")
	fmt.Println()

	reader := bufio.NewReader(os.Stdin)
	fmt.Println("Browser Automation")
	fmt.Println("  Inspect and test websites using CodeLocal's bundled Playwright runtime.")
	settings.Browser.Enabled, err = askYesNo(reader, "Enable Browser Automation?", true)
	if err != nil {
		return automation.Settings{}, environment, err
	}
	fmt.Println()

	if environment.ComputerSupported {
		fmt.Println("Computer Use")
		fmt.Println("  Allow desktop UI access. Screen, pointer and keyboard actions remain separately policy-controlled.")
		settings.Computer.Enabled, err = askYesNo(reader, "Enable Computer Use?", false)
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
	fmt.Println("Starting CodeLocal runtime...")
	fmt.Println()
	return settings, environment, nil
}

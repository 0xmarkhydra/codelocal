package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/automation"
)

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

func ensureFirstRunSetup() (automation.Settings, automation.Environment, error) {
	existing, err := automation.Load()
	if err != nil {
		return automation.Settings{}, automation.Environment{}, err
	}
	environment := automation.Detect()
	if existing != nil {
		return *existing, environment, nil
	}

	settings := automation.Default()
	fmt.Println()
	fmt.Println("CodeLocal first-time setup")
	fmt.Println()
	fmt.Println("CodeLocal lets ChatGPT use capabilities on this computer. You can change local permissions later without reinstalling CodeLocal.")
	fmt.Println()
	fmt.Println("✓ Coding")
	fmt.Println("  Authorized workspaces, Git and guarded terminal tools")

	if stdinInteractive() {
		reader := bufio.NewReader(os.Stdin)
		settings.Browser.Enabled, err = askYesNo(reader, "Enable Browser Automation?", true)
		if err != nil {
			return automation.Settings{}, environment, err
		}
		fmt.Println("  Lets ChatGPT inspect and test websites with CodeLocal's bundled Playwright runtime.")
		settings.Computer.Enabled, err = askYesNo(reader, "Enable Computer Use?", false)
		if err != nil {
			return automation.Settings{}, environment, err
		}
		fmt.Println("  Lets ChatGPT view desktop UI and, when supported, request mouse/keyboard actions.")
	} else {
		// Never block CI, remote shells or service managers waiting for stdin.
		// Use the safest useful defaults: coding + browser, no full desktop control.
		fmt.Println("· Non-interactive session detected; using safe defaults (Browser on, Computer Use off).")
	}

	if settings.Browser.Enabled && !environment.BrowserReady {
		fmt.Println("! Bundled Playwright CLI was not detected. CodeLocal will still start, but Browser Automation will remain unavailable until the package is repaired or updated.")
	}
	if settings.Computer.Enabled {
		if environment.ComputerSupported {
			fmt.Printf("✓ Computer Use backend detected: %s\n", environment.ComputerBackend)
			for _, note := range environment.Notes {
				fmt.Printf("  %s\n", note)
			}
		} else {
			fmt.Println("! Computer Use is enabled in preferences, but this graphical session does not currently expose a supported backend.")
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

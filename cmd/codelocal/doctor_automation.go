package main

import (
	"fmt"
	"os"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/automation"
)

// Keep `codelocal doctor` as the single diagnostics entrypoint. This init runs
// only for that explicit subcommand, before the existing project/tool doctor,
// so users do not need separate browser/computer setup commands.
func init() {
	if len(os.Args) < 2 || os.Args[1] != "doctor" {
		return
	}
	printAutomationDoctor()
}

func yesNo(value bool) string {
	if value {
		return "yes"
	}
	return "no"
}

func capabilityBool(value map[string]any, key string) bool {
	result, _ := value[key].(bool)
	return result
}

func capabilityString(value map[string]any, key string) string {
	result, _ := value[key].(string)
	return result
}

func printAutomationDoctor() {
	settings, err := automation.Load()
	environment := automation.Detect()
	browserEnabled := false
	browserPrepared := false
	computerEnabled := false
	if err == nil && settings != nil {
		browserEnabled = settings.Browser.Enabled
		browserPrepared = settings.Browser.Prepared
		computerEnabled = settings.Computer.Enabled
	}
	browserCLI := automation.BrowserCLIPath()
	computer := automation.ComputerCapabilities()

	fmt.Println("Automation readiness")
	fmt.Printf("  platform: %s\n", environment.Platform)
	if err != nil {
		fmt.Printf("  settings: error (%v)\n", err)
	} else if settings == nil {
		fmt.Println("  settings: first-run setup pending")
	} else {
		fmt.Println("  settings: configured")
	}
	fmt.Printf("  browser: enabled=%s prepared=%s cli=%s\n", yesNo(browserEnabled), yesNo(browserPrepared), yesNo(browserCLI != ""))
	fmt.Printf("    runtime: %s\n", automation.BrowserRuntimeDir())
	if browserEnabled && (!browserPrepared || browserCLI == "") {
		fmt.Println("    ! Browser Automation is not ready yet. Start `codelocal` interactively to retry preparation.")
	}
	backend := capabilityString(computer, "backend")
	if strings.TrimSpace(backend) == "" {
		backend = environment.ComputerBackend
	}
	fmt.Printf("  computer: enabled=%s available=%s helper=%s backend=%s\n", yesNo(computerEnabled), yesNo(capabilityBool(computer, "available")), yesNo(capabilityBool(computer, "helperReady")), backend)
	fmt.Printf("    windows=%s capture=%s uiTree=%s pointer=%s keyboard=%s clipboard=%s background=%s\n",
		yesNo(capabilityBool(computer, "windowList")),
		yesNo(capabilityBool(computer, "screenCapture")),
		yesNo(capabilityBool(computer, "uiTree")),
		yesNo(capabilityBool(computer, "pointer")),
		yesNo(capabilityBool(computer, "keyboard")),
		yesNo(capabilityBool(computer, "clipboard")),
		yesNo(capabilityBool(computer, "backgroundControl")),
	)
	for _, note := range environment.Notes {
		fmt.Printf("    · %s\n", note)
	}
	fmt.Println()
}

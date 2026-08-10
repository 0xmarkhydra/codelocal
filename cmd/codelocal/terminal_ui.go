package main

import (
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiGray    = "\x1b[90m"
	ansiGreen   = "\x1b[32m"
	ansiMagenta = "\x1b[35m"
)

func terminalColor(code, value string) string {
	if os.Getenv("NO_COLOR") != "" || os.Getenv("TERM") == "dumb" {
		return value
	}
	return code + value + ansiReset
}

func terminalRuntimeHeader() {
	width := 48
	line := strings.Repeat("─", width-2)
	row := func(value string) string {
		plain := []rune(value)
		padding := width - 4 - len(plain)
		if padding < 0 {
			padding = 0
		}
		return "│  " + value + strings.Repeat(" ", padding) + "│"
	}
	fmt.Println()
	fmt.Println(terminalColor(ansiMagenta, "╭"+line+"╮"))
	fmt.Println(terminalColor(ansiMagenta, row("◆ CodeLocal")))
	fmt.Println(terminalColor(ansiGray, row("Local runtime for ChatGPT")))
	fmt.Println(terminalColor(ansiMagenta, "╰"+line+"╯"))
	fmt.Println()
	fmt.Printf("  %s %-12s %s\n", terminalColor(ansiGreen, "●"), terminalColor(ansiBold, "Runtime"), "Starting…")
	fmt.Printf("  %s %-12s %s\n", terminalColor(ansiGreen, "●"), terminalColor(ansiBold, "Cloud"), "Connecting…")
	fmt.Printf("  %s %-12s %s\n\n", terminalColor(ansiGreen, "●"), terminalColor(ansiBold, "Status"), "Waiting for ChatGPT tool calls…")
}

func init() {
	// The bare `codelocal` command is the long-running runtime. Keep its terminal
	// readable: tool calls are printed by internal/runtime, while generic INFO
	// heartbeat/connectivity messages stay hidden unless they are warnings/errors.
	if len(os.Args) == 1 {
		slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})))
		terminalRuntimeHeader()
	}
}

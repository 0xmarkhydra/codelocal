package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const (
	ansiReset   = "\x1b[0m"
	ansiBold    = "\x1b[1m"
	ansiGray    = "\x1b[90m"
	ansiCyan    = "\x1b[36m"
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
		padding := width - 4 - len([]rune(value))
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
	fmt.Printf("  %s %-12s %s\n\n", terminalColor(ansiGray, "◌"), terminalColor(ansiBold, "Status"), "Connecting to CodeLocal Cloud…")
}

func terminalRuntimeReady(workspaces int64) {
	fmt.Printf("  %s %-12s %s\n", terminalColor(ansiGreen, "●"), terminalColor(ansiBold, "Runtime"), "Online")
	fmt.Printf("  %s %-12s %s\n", terminalColor(ansiGreen, "●"), terminalColor(ansiBold, "Cloud"), "Connected")
	fmt.Printf("  %s %-12s %d authorized\n", terminalColor(ansiCyan, "◇"), terminalColor(ansiBold, "Workspaces"), workspaces)
	fmt.Printf("  %s %-12s %s\n\n", terminalColor(ansiGray, "◌"), terminalColor(ansiBold, "Status"), "Waiting for ChatGPT…")
}

type terminalRuntimeHandler struct {
	fallback slog.Handler
}

func (h terminalRuntimeHandler) Enabled(_ context.Context, level slog.Level) bool {
	return level >= slog.LevelInfo
}

func (h terminalRuntimeHandler) Handle(ctx context.Context, record slog.Record) error {
	if record.Level == slog.LevelInfo && record.Message == "CodeLocal runtime connected" {
		var workspaces int64
		record.Attrs(func(attr slog.Attr) bool {
			if attr.Key == "workspaces" {
				workspaces = attr.Value.Int64()
			}
			return true
		})
		terminalRuntimeReady(workspaces)
		return nil
	}
	if record.Level < slog.LevelWarn {
		return nil
	}
	return h.fallback.Handle(ctx, record)
}

func (h terminalRuntimeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return terminalRuntimeHandler{fallback: h.fallback.WithAttrs(attrs)}
}

func (h terminalRuntimeHandler) WithGroup(name string) slog.Handler {
	return terminalRuntimeHandler{fallback: h.fallback.WithGroup(name)}
}

func init() {
	if len(os.Args) == 1 {
		fallback := slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelWarn})
		slog.SetDefault(slog.New(terminalRuntimeHandler{fallback: fallback}))
		terminalRuntimeHeader()
	}
}

package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/cloudserver"
	"github.com/0xmarkhydra/codelocal/internal/mcpgateway"
)

func main() {
	// Railway classifies stderr as errors. Keep structured INFO/WARN traffic on
	// stdout so healthy requests do not paint the deployment log red.
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo})))

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	migrationMode, err := cloud.StoreMigrationMode()
	if err != nil {
		slog.Error("CodeLocal migration mode is invalid", "error", err)
		os.Exit(1)
	}
	if migrationMode == cloud.StoreMigrationOnly {
		if err := cloud.MigrateOnly(ctx); err != nil {
			slog.Error("CodeLocal migration failed", "error", err)
			os.Exit(1)
		}
		slog.Info("CodeLocal migration completed")
		return
	}
	server, err := cloudserver.New(ctx)
	if err != nil {
		slog.Error("CodeLocal Cloud initialization failed", "error", err)
		os.Exit(1)
	}
	server.RegisterRuntimeRealtime()
	server.RegisterReleaseEmail()
	// Keep old per-thread ChatGPT MCP schemas functional after the compact tool
	// migration without re-exposing the legacy granular tools in tools/list.
	server.HTTP.Handler = mcpgateway.LegacyToolCallCompatibility(server.HTTP.Handler)
	errCh := make(chan error, 1)
	go func() { errCh <- server.ListenAndServe() }()
	select {
	case err := <-errCh:
		if err != nil && !errors.Is(err, context.Canceled) {
			slog.Error("CodeLocal Cloud stopped", "error", err)
			os.Exit(1)
		}
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			slog.Error("CodeLocal Cloud shutdown failed", "error", err)
		}
	}
}

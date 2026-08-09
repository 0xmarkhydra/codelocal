// Dev client bootstrap. Patch the Chokidar singleton before client-v2 loads so
// existing watcher consumers transparently use a native recursive backend.
// @parcel/watcher is optional; bounded Chokidar remains the safety fallback.

const { log } = await import("./log.js");
const fallbackDepth = Math.max(0, Number(process.env.CODELOCAL_WATCH_DEPTH ?? 2) || 0);
const chokidarModule = await import("chokidar");
const chokidar = chokidarModule.default;
const { installNativeWatcherAdapter } = await import("./native-watcher.js");

const watcher = installNativeWatcherAdapter(chokidar, {
  fallbackDepth,
  log(level, event, detail = {}) {
    log(level, event, detail);
  },
});

log("info", "workspace.watcher_configured", {
  backend: watcher.preferredBackend,
  recursive: true,
  fallbackBackend: "chokidar-bounded",
  fallbackDepth: watcher.fallbackDepth,
});

try {
  const { syncCloudMcpBeforeClient } = await import("./cloud-client-sync.js");
  const sync = await syncCloudMcpBeforeClient();
  log("info", "mcp.cloud_sync", sync as Record<string, unknown>);
} catch (error) {
  // Cloud config sync must never prevent the local coding runtime from starting.
  // Existing local MCP config remains usable and the user can reconnect later.
  log("warn", "mcp.cloud_sync_failed", { error: error instanceof Error ? error.message : String(error) });
}

await import("./client-v2.js");

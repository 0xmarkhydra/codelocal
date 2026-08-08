// Dev client bootstrap. Patch the Chokidar singleton before client-v2 loads so
// existing watcher consumers transparently use a native recursive backend.
// @parcel/watcher is optional; bounded Chokidar remains the safety fallback.

const fallbackDepth = Math.max(0, Number(process.env.CODELOCAL_WATCH_DEPTH ?? 2) || 0);
const chokidarModule = await import("chokidar");
const chokidar = chokidarModule.default;
const { installNativeWatcherAdapter } = await import("./native-watcher.js");

const watcher = installNativeWatcherAdapter(chokidar, {
  fallbackDepth,
  log(level, event, detail = {}) {
    const payload = { ts: new Date().toISOString(), level, event, ...detail };
    if (level === "error") console.error(JSON.stringify(payload));
    else if (level === "warn") console.warn(JSON.stringify(payload));
    else console.log(JSON.stringify(payload));
  },
});

console.log(JSON.stringify({
  ts: new Date().toISOString(),
  level: "info",
  event: "workspace.watcher_configured",
  backend: watcher.preferredBackend,
  recursive: true,
  fallbackBackend: "chokidar-bounded",
  fallbackDepth: watcher.fallbackDepth,
}));

try {
  const { syncCloudMcpBeforeClient } = await import("./cloud-client-sync.js");
  const sync = await syncCloudMcpBeforeClient();
  console.log(JSON.stringify({ ts: new Date().toISOString(), level: "info", event: "mcp.cloud_sync", ...sync }));
} catch (error) {
  // Cloud config sync must never prevent the local coding runtime from starting.
  // Existing local MCP config remains usable and the user can reconnect later.
  console.warn(JSON.stringify({ ts: new Date().toISOString(), level: "warn", event: "mcp.cloud_sync_failed", error: error instanceof Error ? error.message : String(error) }));
}

await import("./client-v2.js");

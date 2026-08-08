// Production client bootstrap. Configure the filesystem watcher before loading client-v2.
// macOS can exhaust fs.watch handles on large workspaces; polling avoids EMFILE crashes.

if (process.platform === "darwin" && process.env.CHOKIDAR_USEPOLLING == null) {
  process.env.CHOKIDAR_USEPOLLING = "1";
  process.env.CHOKIDAR_INTERVAL ??= "1000";
}

const chokidarModule = await import("chokidar");
const chokidar = chokidarModule.default;
const originalWatch = chokidar.watch.bind(chokidar) as any;

(chokidar as any).watch = (...args: any[]) => {
  const watcher = originalWatch(...args);
  watcher.on("error", (error: NodeJS.ErrnoException) => {
    const payload = {
      ts: new Date().toISOString(),
      level: "error",
      event: "workspace.watcher_error",
      code: error?.code ?? null,
      message: error?.message ?? String(error),
      backend: process.env.CHOKIDAR_USEPOLLING === "1" ? "polling" : "native",
    };
    console.error(JSON.stringify(payload));
  });
  return watcher;
};

if (process.platform === "darwin") {
  console.log(JSON.stringify({
    ts: new Date().toISOString(),
    level: "info",
    event: "workspace.watcher_configured",
    backend: process.env.CHOKIDAR_USEPOLLING === "1" ? "polling" : "native",
    intervalMs: process.env.CHOKIDAR_USEPOLLING === "1" ? Number(process.env.CHOKIDAR_INTERVAL ?? 1000) : null,
  }));
}

await import("./client-v2.js");

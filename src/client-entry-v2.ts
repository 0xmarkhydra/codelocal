// Production client bootstrap. Configure the filesystem watcher before loading client-v2.
// Large workspaces can exhaust fs.watch handles or heap when watched recursively.
// On macOS we default to polling and a shallow watch depth; CodeLocal-owned edits
// invalidate semantic/context caches directly, while external deep changes are picked
// up on the next explicit refresh/project-map request.

if (process.platform === "darwin" && process.env.CHOKIDAR_USEPOLLING == null) {
  process.env.CHOKIDAR_USEPOLLING = "1";
  process.env.CHOKIDAR_INTERVAL ??= "1500";
}

const watchDepth = Math.max(0, Number(process.env.CODELOCAL_WATCH_DEPTH ?? (process.platform === "darwin" ? 1 : 3)) || 0);
const chokidarModule = await import("chokidar");
const chokidar = chokidarModule.default;
const originalWatch = chokidar.watch.bind(chokidar) as any;

(chokidar as any).watch = (paths: any, options: any = {}) => {
  const watcher = originalWatch(paths, {
    ...options,
    depth: options.depth ?? watchDepth,
    followSymlinks: false,
  });
  watcher.on("error", (error: NodeJS.ErrnoException) => {
    const payload = {
      ts: new Date().toISOString(),
      level: "error",
      event: "workspace.watcher_error",
      code: error?.code ?? null,
      message: error?.message ?? String(error),
      backend: process.env.CHOKIDAR_USEPOLLING === "1" ? "polling" : "native",
      depth: watchDepth,
    };
    console.error(JSON.stringify(payload));
  });
  return watcher;
};

console.log(JSON.stringify({
  ts: new Date().toISOString(),
  level: "info",
  event: "workspace.watcher_configured",
  backend: process.env.CHOKIDAR_USEPOLLING === "1" ? "polling" : "native",
  intervalMs: process.env.CHOKIDAR_USEPOLLING === "1" ? Number(process.env.CHOKIDAR_INTERVAL ?? 1500) : null,
  depth: watchDepth,
}));

await import("./client-v2.js");

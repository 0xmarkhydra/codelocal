import { EventEmitter } from "node:events";
import path from "node:path";

const COMMON_IGNORES = [
  ".git/**",
  "node_modules/**",
  ".next/**",
  "dist/**",
  "build/**",
  "target/**",
  ".venv/**",
  "venv/**",
  "coverage/**",
  ".cache/**",
  ".turbo/**",
  ".dart_tool/**",
  ".gradle/**",
  "Pods/**",
  "DerivedData/**",
];

type NativeSubscription = { unsubscribe(): Promise<void> | void };
type FallbackWatcher = { close?: () => Promise<void> | void; on(event: string, listener: (...args: any[]) => void): any };
type CompatibleWatcher = EventEmitter & {
  close: () => Promise<void>;
  add: (...args: any[]) => CompatibleWatcher;
  unwatch: (...args: any[]) => Promise<void>;
};

type InstallOptions = {
  fallbackDepth?: number;
  log?: (level: "info" | "warn" | "error", event: string, detail?: Record<string, unknown>) => void;
};

function backendForPlatform() {
  if (process.platform === "darwin") return "fs-events";
  if (process.platform === "linux") return "inotify";
  if (process.platform === "win32") return "windows";
  return undefined;
}

function isPlainDirectoryTarget(targets: unknown): targets is string {
  return typeof targets === "string" && !/[*!?{}[\]]/.test(targets);
}

function ignoredByOption(ignored: unknown, absolutePath: string) {
  if (!ignored) return false;
  if (typeof ignored === "function") {
    try { return !!ignored(absolutePath); } catch { return false; }
  }
  const values = Array.isArray(ignored) ? ignored : [ignored];
  const normalized = absolutePath.replace(/\\/g, "/");
  return values.some((value) => typeof value === "string" && normalized.includes(value.replace(/\*+/g, "")));
}

function mapEvent(type: string) {
  if (type === "create") return "add";
  if (type === "delete") return "unlink";
  return "change";
}

/**
 * Patch Chokidar's singleton `watch` entry point before client-v2 is imported.
 * CodeLocal keeps the existing watcher consumer API, while large workspaces use
 * @parcel/watcher native recursive backends (FSEvents/inotify/Windows) whenever
 * the dependency is available. A shallow Chokidar fallback remains.
 */
export function installNativeWatcherAdapter(chokidar: any, options: InstallOptions = {}) {
  const originalWatch = chokidar.watch.bind(chokidar) as (targets: unknown, options?: Record<string, unknown>) => FallbackWatcher;
  const fallbackDepth = Math.max(0, options.fallbackDepth ?? 2);
  const emitLog = options.log ?? ((level, event, detail = {}) => {
    const line = JSON.stringify({ ts: new Date().toISOString(), level, event, ...detail });
    if (level === "error") console.error(line);
    else if (level === "warn") console.warn(line);
    else console.log(line);
  });

  chokidar.watch = (targets: unknown, watchOptions: Record<string, any> = {}) => {
    if (!isPlainDirectoryTarget(targets)) {
      return originalWatch(targets, { ...watchOptions, depth: watchOptions.depth ?? fallbackDepth, followSymlinks: false });
    }

    const root = path.resolve(targets);
    const emitter = new EventEmitter() as CompatibleWatcher;
    let subscription: NativeSubscription | null = null;
    let fallback: FallbackWatcher | null = null;
    let closed = false;

    const close = async () => {
      if (closed) return;
      closed = true;
      try { await subscription?.unsubscribe(); } catch {}
      try { await fallback?.close?.(); } catch {}
      emitter.removeAllListeners();
    };
    emitter.close = close;
    emitter.unwatch = close;
    emitter.add = () => emitter;

    const startFallback = (cause: unknown) => {
      if (closed) return;
      emitLog("warn", "workspace.watcher_fallback", {
        backend: "chokidar-bounded",
        depth: fallbackDepth,
        reason: cause instanceof Error ? cause.message : String(cause),
      });
      fallback = originalWatch(root, {
        ...watchOptions,
        depth: watchOptions.depth ?? fallbackDepth,
        followSymlinks: false,
      });
      fallback.on("all", (event: string, changed: string) => emitter.emit("all", event, changed));
      fallback.on("error", (error: unknown) => emitter.emit("error", error));
      fallback.on("ready", () => emitter.emit("ready"));
    };

    queueMicrotask(async () => {
      try {
        const dynamicImport = new Function("m", "return import(m)") as (module: string) => Promise<any>;
        const parcel = await dynamicImport("@parcel/watcher");
        if (closed) return;
        const backend = backendForPlatform();
        subscription = await parcel.subscribe(
          root,
          (error: unknown, events: Array<{ type: string; path: string }> = []) => {
            if (closed) return;
            if (error) {
              emitter.emit("error", error);
              return;
            }
            for (const item of events) {
              const absolute = path.resolve(item.path);
              if (ignoredByOption(watchOptions.ignored, absolute)) continue;
              const event = mapEvent(item.type);
              emitter.emit("all", event, absolute);
              emitter.emit(event, absolute);
            }
          },
          { ignore: COMMON_IGNORES, ...(backend ? { backend } : {}) },
        );
        emitLog("info", "workspace.watcher_ready", { backend: backend ?? "native-default", recursive: true, root });
        emitter.emit("ready");
      } catch (error) {
        startFallback(error);
      }
    });

    return emitter;
  };

  return { preferredBackend: backendForPlatform() ?? "native-default", fallbackDepth };
}

import { access } from "node:fs/promises";
import os from "node:os";
import path from "node:path";
import { spawn } from "node:child_process";
import { constants } from "node:fs";

export type SandboxMode = "native" | "best-effort" | "policy-only" | "none";
export type NetworkMode = "deny" | "approval" | "allow";

export type SandboxInfo = {
  platform: NodeJS.Platform;
  backend: string;
  mode: SandboxMode;
  available: boolean;
  networkMode: NetworkMode;
  notes: string[];
};

async function executableExists(command: string) {
  const paths = (process.env.PATH ?? "").split(path.delimiter);
  const candidates = process.platform === "win32" ? [command, `${command}.exe`, `${command}.cmd`] : [command];
  for (const dir of paths) {
    for (const candidate of candidates) {
      try {
        await access(path.join(dir, candidate), constants.X_OK);
        return true;
      } catch {}
    }
  }
  return false;
}

function existingSecretDirs() {
  const home = os.homedir();
  return [".ssh", ".aws", ".gnupg", ".gcloud", ".azure"].map((x) => path.join(home, x));
}

function escapeSbpl(value: string) {
  return value.replace(/\\/g, "\\\\").replace(/\"/g, "\\\"");
}

function extraWritableDeveloperPaths() {
  const raw = process.env.CODELOCAL_EXTRA_WRITABLE_PATHS?.trim();
  if (!raw) return [];
  return raw
    .split(path.delimiter)
    .map((value) => value.trim())
    .filter(Boolean)
    .map((value) => path.resolve(value));
}

async function writableDeveloperPaths() {
  const home = os.homedir();
  const candidates = [
    "/tmp",
    "/private/tmp",
    // Dart/Flutter state. Flutter writes telemetry/session state here before a
    // build begins (for example dart-flutter-telemetry-session.json).
    path.join(home, ".dart-tool"),
    path.join(home, ".dartServer"),
    path.join(home, ".flutter"),
    path.join(home, ".pub-cache"),
    path.join(home, "Library", "Caches"),
    path.join(home, "Library", "Developer", "Xcode", "DerivedData"),
    path.join(home, "Library", "Developer", "Xcode", "Archives"),
    path.join(home, "Library", "Developer", "CoreSimulator"),
    path.join(home, "Library", "Logs", "CoreSimulator"),
    path.join(home, "Library", "CocoaPods"),
    path.join(home, "develop", "flutter", "bin", "cache"),
    ...extraWritableDeveloperPaths(),
  ];
  const flutterRoot = process.env.FLUTTER_ROOT?.trim();
  if (flutterRoot) candidates.push(path.join(flutterRoot, "bin", "cache"));

  // Sandbox rules may safely name paths that do not exist yet; that is needed
  // for first-run caches. Keep the list narrow instead of granting all of $HOME.
  return [...new Set(candidates.map((candidate) => path.resolve(candidate)))];
}

export class SandboxManager {
  constructor(private workspace: string, private networkMode: NetworkMode = "approval") {}

  async info(): Promise<SandboxInfo> {
    if (process.env.CODELOCAL_SANDBOX === "off") {
      return { platform: process.platform, backend: "disabled", mode: "none", available: false, networkMode: this.networkMode, notes: ["disabled by CODELOCAL_SANDBOX=off"] };
    }
    if (process.platform === "linux" && await executableExists("bwrap")) {
      return { platform: process.platform, backend: "bubblewrap", mode: "native", available: true, networkMode: this.networkMode, notes: ["workspace is writable; host root is read-only; common credential directories are masked"] };
    }
    if (process.platform === "darwin" && await executableExists("sandbox-exec")) {
      return { platform: process.platform, backend: "sandbox-exec", mode: "best-effort", available: true, networkMode: this.networkMode, notes: ["sandbox-exec is best-effort; workspace plus narrowly scoped developer caches/state are writable so Flutter/Xcode tooling can run"] };
    }
    if (process.platform === "win32") {
      return { platform: process.platform, backend: "windows-policy", mode: "policy-only", available: false, networkMode: this.networkMode, notes: ["native Windows restricted-token helper is not bundled yet; workspace and command policy still apply"] };
    }
    return { platform: process.platform, backend: "policy-only", mode: "policy-only", available: false, networkMode: this.networkMode, notes: ["no native sandbox backend detected"] };
  }

  async wrapShell(command: string, cwd: string) {
    const info = await this.info();
    if (info.backend === "bubblewrap") {
      const args = [
        "--die-with-parent",
        "--new-session",
        "--ro-bind", "/", "/",
        "--bind", this.workspace, this.workspace,
        "--chdir", cwd,
        "--proc", "/proc",
        "--dev", "/dev",
        "--tmpfs", "/tmp",
      ];
      for (const secret of existingSecretDirs()) {
        try {
          await access(secret);
          args.push("--tmpfs", secret);
        } catch {}
      }
      if (this.networkMode === "deny") args.push("--unshare-net");
      args.push("--", process.env.SHELL || "/bin/sh", "-lc", command);
      return { command: "bwrap", args, cwd: this.workspace, sandbox: info };
    }

    if (info.backend === "sandbox-exec") {
      const rules = [
        "(version 1)",
        "(deny default)",
        "(allow process*)",
        "(allow signal)",
        "(allow sysctl-read)",
        "(allow file-read*)",
        `(allow file-write* (subpath \"${escapeSbpl(this.workspace)}\"))`,
        // Shells and developer tools routinely redirect probes to /dev/null.
        // Without this explicit rule Flutter reports misleading secondary errors
        // such as \"Unable to find git in your PATH\".
        "(allow file-write* (literal \"/dev/null\"))",
      ];
      for (const writable of await writableDeveloperPaths()) {
        rules.push(`(allow file-write* (subpath \"${escapeSbpl(writable)}\"))`);
      }
      if (this.networkMode !== "deny") rules.push("(allow network*)");
      const profile = rules.join(" ");
      return { command: "sandbox-exec", args: ["-p", profile, process.env.SHELL || "/bin/sh", "-lc", command], cwd, sandbox: info };
    }

    const shell = process.platform === "win32" ? (process.env.ComSpec || "cmd.exe") : (process.env.SHELL || "/bin/sh");
    const args = process.platform === "win32" ? ["/d", "/s", "/c", command] : ["-lc", command];
    return { command: shell, args, cwd, sandbox: info };
  }

  async smokeTest() {
    const info = await this.info();
    if (!info.available) return { ...info, smokeTest: "not-run" as const };
    // Verify stdout, /dev/null and Dart state because Flutter depends on all of
    // them before the application build itself starts.
    const dartState = path.join(os.homedir(), ".dart-tool", ".codelocal-sandbox-probe");
    const wrapped = await this.wrapShell(`printf codelocal-sandbox-ok && printf probe >/dev/null && mkdir -p \"${dartState.replace(/\/\.codelocal-sandbox-probe$/, "")}\" && printf probe >\"${dartState}\" && rm -f \"${dartState}\"`, this.workspace);
    return await new Promise<Record<string, unknown>>((resolve) => {
      const child = spawn(wrapped.command, wrapped.args, { cwd: wrapped.cwd, stdio: ["ignore", "pipe", "pipe"], env: process.env });
      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (d) => { stdout += d.toString(); });
      child.stderr.on("data", (d) => { stderr += d.toString(); });
      child.on("error", (error) => resolve({ ...info, smokeTest: "failed", error: error.message }));
      child.on("close", (code) => resolve({ ...info, smokeTest: code === 0 && stdout.includes("codelocal-sandbox-ok") ? "passed" : "failed", exitCode: code, stderr: stderr.slice(-1000) }));
    });
  }
}

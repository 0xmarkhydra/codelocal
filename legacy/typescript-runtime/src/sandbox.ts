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

export function isMacDeveloperToolCommand(command: string) {
  const segments = command
    .replace(/\s+/g, " ")
    .split(/&&|\|\||;|\|/)
    .map((segment) => segment.trim())
    .filter(Boolean);

  return segments.some((segment) => {
    const withoutEnv = segment.replace(/^(?:env\s+)?(?:[A-Za-z_][A-Za-z0-9_]*=(?:"[^"]*"|'[^']*'|\S+)\s+)*/i, "");
    if (/^(?:fvm\s+flutter|bundle\s+exec\s+pod)(?:\s|$)/i.test(withoutEnv)) return true;
    const first = withoutEnv.match(/^(?:"([^"]+)"|'([^']+)'|(\S+))/)?.slice(1).find(Boolean) ?? "";
    const base = path.basename(first);
    return /^(flutter|dart|xcodebuild|xcrun|pod)$/i.test(base);
  });
}

function macDeveloperHostModeEnabled() {
  return process.env.CODELOCAL_MACOS_DEVTOOLS_HOST !== "0";
}

async function writableDeveloperPaths() {
  const home = os.homedir();
  const candidates = [
    "/tmp",
    "/private/tmp",
    os.tmpdir(),
    process.env.TMPDIR ?? "",
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
    // A source checkout of Flutter is partly self-updating: the tool may write
    // version/cache/artifact state outside bin/cache. Keep this scoped to the
    // Flutter SDK tree rather than opening the whole home directory.
    path.join(home, "develop", "flutter"),
    ...extraWritableDeveloperPaths(),
  ].filter(Boolean);
  const flutterRoot = process.env.FLUTTER_ROOT?.trim();
  if (flutterRoot) candidates.push(flutterRoot);

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
      return {
        platform: process.platform,
        backend: "sandbox-exec",
        mode: "best-effort",
        available: true,
        networkMode: this.networkMode,
        notes: [
          "normal commands use sandbox-exec with scoped writable developer paths",
          "approved Flutter/Xcode/CocoaPods commands use macOS developer host mode because Simulator/Xcode require system IPC and user temp services that sandbox-exec cannot reliably proxy",
        ],
      };
    }
    if (process.platform === "win32") {
      return { platform: process.platform, backend: "windows-policy", mode: "policy-only", available: false, networkMode: this.networkMode, notes: ["native Windows restricted-token helper is not bundled yet; workspace and command policy still apply"] };
    }
    return { platform: process.platform, backend: "policy-only", mode: "policy-only", available: false, networkMode: this.networkMode, notes: ["no native sandbox backend detected"] };
  }

  async wrapShell(command: string, cwd: string) {
    const info = await this.info();

    // Flutter/Xcode/Simulator on macOS require access to per-user temporary
    // directories, CoreSimulator services, Xcode helpers and SDK state. Running
    // them under sandbox-exec produces misleading EPERM errors even when the
    // project and SDK permissions are correct. These commands are still gated
    // by CodeLocal's command policy + local approval before ProcessManager gets
    // here; only the OS sandbox is bypassed for the selected developer toolchain.
    if (
      process.platform === "darwin" &&
      this.networkMode !== "deny" &&
      macDeveloperHostModeEnabled() &&
      isMacDeveloperToolCommand(command)
    ) {
      const shell = process.env.SHELL || "/bin/zsh";
      return {
        command: shell,
        args: ["-lc", command],
        cwd,
        sandbox: {
          ...info,
          backend: "macos-developer-host",
          mode: "policy-only" as const,
          available: false,
          notes: [
            "OS sandbox intentionally bypassed for an approved macOS developer-toolchain command",
            "CodeLocal command policy, approval, workspace cwd validation and audit remain active",
          ],
        },
      };
    }

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
    const dartState = path.join(os.homedir(), ".dart-tool", ".codelocal-sandbox-probe");
    const tempState = path.join(os.tmpdir(), `.codelocal-temp-probe-${process.pid}`);
    const wrapped = await this.wrapShell(
      `printf codelocal-sandbox-ok && printf probe >/dev/null && mkdir -p \"${path.dirname(dartState)}\" && printf probe >\"${dartState}\" && rm -f \"${dartState}\" && printf probe >\"${tempState}\" && rm -f \"${tempState}\"`,
      this.workspace,
    );
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

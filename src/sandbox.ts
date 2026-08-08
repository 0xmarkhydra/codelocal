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
      return { platform: process.platform, backend: "sandbox-exec", mode: "best-effort", available: true, networkMode: this.networkMode, notes: ["sandbox-exec is used when present; availability and long-term support vary by macOS version"] };
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
      const escapedWorkspace = this.workspace.replace(/\\/g, "\\\\").replace(/\"/g, "\\\"");
      const rules = [
        "(version 1)",
        "(deny default)",
        "(allow process*)",
        "(allow sysctl-read)",
        "(allow file-read*)",
        `(allow file-write* (subpath \"${escapedWorkspace}\"))`,
        "(allow file-write* (subpath \"/tmp\"))",
      ];
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
    const wrapped = await this.wrapShell("printf codelocal-sandbox-ok", this.workspace);
    return await new Promise<Record<string, unknown>>((resolve) => {
      const child = spawn(wrapped.command, wrapped.args, { cwd: wrapped.cwd, stdio: ["ignore", "pipe", "pipe"] });
      let stdout = "";
      let stderr = "";
      child.stdout.on("data", (d) => { stdout += d.toString(); });
      child.stderr.on("data", (d) => { stderr += d.toString(); });
      child.on("error", (error) => resolve({ ...info, smokeTest: "failed", error: error.message }));
      child.on("close", (code) => resolve({ ...info, smokeTest: code === 0 && stdout.includes("codelocal-sandbox-ok") ? "passed" : "failed", exitCode: code, stderr: stderr.slice(-1000) }));
    });
  }
}

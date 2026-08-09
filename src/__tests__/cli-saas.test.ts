import test from "node:test";
import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, realpath, rm, stat } from "node:fs/promises";
import os from "node:os";
import path from "node:path";

function runCli(cli: string, args: string[], cwd: string, stateDir: string) {
  return new Promise<{ code: number | null; stdout: string; stderr: string }>((resolve, reject) => {
    const child = spawn(process.execPath, [cli, ...args], {
      cwd,
      env: {
        ...process.env,
        CODELOCAL_STATE_DIR: stateDir,
        CODELOCAL_SERVER: "http://127.0.0.1:9",
      },
      stdio: ["ignore", "pipe", "pipe"],
    });
    let stdout = "";
    let stderr = "";
    child.stdout.setEncoding("utf8");
    child.stderr.setEncoding("utf8");
    child.stdout.on("data", (chunk) => { stdout += chunk; });
    child.stderr.on("data", (chunk) => { stderr += chunk; });
    const timer = setTimeout(() => {
      child.kill("SIGKILL");
      reject(new Error(`CLI timed out: ${args.join(" ")}`));
    }, 3_000);
    child.once("error", (error) => { clearTimeout(timer); reject(error); });
    child.once("close", (code) => { clearTimeout(timer); resolve({ code, stdout, stderr }); });
  });
}

test("codelocal . authorizes locally and exits without connecting to Cloud", async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "codelocal-cli-add-"));
  try {
    const project = path.join(temp, "Project A");
    const stateDir = path.join(temp, "state");
    await mkdir(project, { recursive: true });
    const cli = path.resolve("dist", "cli-saas.js");
    const result = spawnSync(process.execPath, [cli, "."], {
      cwd: project,
      encoding: "utf8",
      timeout: 3_000,
      env: {
        ...process.env,
        CODELOCAL_STATE_DIR: stateDir,
        CODELOCAL_SERVER: "http://127.0.0.1:9",
      },
    });

    assert.equal(result.error, undefined, result.error?.message);
    assert.equal(result.status, 0, `${result.stdout}\n${result.stderr}`);
    assert.match(result.stdout, /Added Project A/);
    assert.match(result.stdout, /Run `codelocal` when you want to connect ChatGPT/);

    const registry = JSON.parse(await readFile(path.join(stateDir, "workspaces.json"), "utf8")) as { workspaces?: Array<{ localPath?: string }> };
    assert.equal(registry.workspaces?.length, 1);
    assert.equal(registry.workspaces?.[0]?.localPath, await realpath(project));
    assert.equal(await stat(path.join(stateDir, "credential.json")).then(() => true).catch(() => false), false);
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
});

test("concurrent codelocal . commands preserve every authorized workspace", async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "codelocal-cli-race-"));
  try {
    const projectA = path.join(temp, "Project A");
    const projectB = path.join(temp, "Project B");
    const stateDir = path.join(temp, "state");
    await mkdir(projectA, { recursive: true });
    await mkdir(projectB, { recursive: true });
    const cli = path.resolve("dist", "cli-saas.js");

    const [first, second] = await Promise.all([
      runCli(cli, ["."], projectA, stateDir),
      runCli(cli, ["."], projectB, stateDir),
    ]);
    assert.equal(first.code, 0, `${first.stdout}\n${first.stderr}`);
    assert.equal(second.code, 0, `${second.stdout}\n${second.stderr}`);

    const registry = JSON.parse(await readFile(path.join(stateDir, "workspaces.json"), "utf8")) as { workspaces?: Array<{ workspaceName?: string }> };
    assert.equal(registry.workspaces?.length, 2);
    assert.deepEqual(new Set(registry.workspaces?.map((workspace) => workspace.workspaceName)), new Set(["Project A", "Project B"]));
    assert.equal(await stat(path.join(stateDir, "credential.json")).then(() => true).catch(() => false), false);
  } finally {
    await rm(temp, { recursive: true, force: true });
  }
});

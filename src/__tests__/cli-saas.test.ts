import test from "node:test";
import assert from "node:assert/strict";
import { spawn, spawnSync } from "node:child_process";
import { mkdir, mkdtemp, readFile, realpath, rm, stat } from "node:fs/promises";
import http from "node:http";
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

async function waitFor(predicate: () => Promise<boolean>, timeoutMs = 3_000) {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    if (await predicate()) return;
    await new Promise((resolve) => setTimeout(resolve, 25));
  }
  throw new Error("Timed out waiting for condition.");
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

test("machine runtime exposes singleton IPC while first-use pairing is still in progress", async () => {
  const temp = await mkdtemp(path.join(os.tmpdir(), "codelocal-cli-pairing-"));
  const stateDir = path.join(temp, "state");
  const cli = path.resolve("dist", "cli-saas.js");
  const server = http.createServer((req, res) => {
    if (req.method === "POST" && req.url === "/pair/start") {
      res.setHeader("content-type", "application/json");
      res.end(JSON.stringify({
        pairingId: "pairing-test",
        code: "123456",
        expiresAt: Date.now() + 30_000,
        approveUrl: "http://127.0.0.1/pair/approve?pairingId=pairing-test",
      }));
      return;
    }
    if (req.method === "POST" && req.url === "/pair/claim") {
      res.statusCode = 409;
      res.end("pending");
      return;
    }
    res.statusCode = 404;
    res.end("not-found");
  });

  await new Promise<void>((resolve) => server.listen(0, "127.0.0.1", resolve));
  const address = server.address();
  assert.ok(address && typeof address === "object");
  const baseUrl = `http://127.0.0.1:${address.port}`;
  const env = {
    ...process.env,
    CODELOCAL_STATE_DIR: stateDir,
    CODELOCAL_SERVER: baseUrl,
    CODELOCAL_NO_BROWSER: "1",
  };
  const runtime = spawn(process.execPath, [cli], { cwd: temp, env, stdio: "ignore" });

  try {
    await waitFor(async () => stat(path.join(stateDir, "runtime.lock")).then(() => true).catch(() => false));
    let status: any = null;
    await waitFor(async () => {
      const result = spawnSync(process.execPath, [cli, "status"], { cwd: temp, env, encoding: "utf8", timeout: 2_000 });
      if (result.status !== 0) return false;
      try { status = JSON.parse(result.stdout); } catch { return false; }
      return status?.runtime?.responsive === true;
    });
    assert.equal(status.runtime.running, true);
    assert.equal(status.runtime.detail?.phase, "pairing");

    const second = spawnSync(process.execPath, [cli], { cwd: temp, env, encoding: "utf8", timeout: 2_000 });
    assert.equal(second.status, 0, `${second.stdout}\n${second.stderr}`);
    assert.match(second.stdout, /already running on this machine/i);
    assert.match(second.stdout, /Status: pairing/i);
  } finally {
    runtime.kill("SIGTERM");
    await new Promise<void>((resolve) => runtime.once("close", () => resolve()));
    await new Promise<void>((resolve) => server.close(() => resolve()));
    await rm(temp, { recursive: true, force: true });
  }

});

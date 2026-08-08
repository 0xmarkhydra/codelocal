#!/usr/bin/env node
import path from "node:path";
import os from "node:os";
import { promises as fs } from "node:fs";
import { createInterface } from "node:readline/promises";
import { spawn } from "node:child_process";
import { defaultDeviceIdentity, deleteLocalCredential, loadLocalCredential, saveLocalCredential } from "./identity.js";

function usage() {
  console.log(`CodeLocal CLI\n\nCommands:\n  codelocal doctor <project>\n  codelocal pair <https://gateway>\n  codelocal start <project> [wss://gateway/client]\n  codelocal status\n  codelocal rotate <https://gateway>\n  codelocal revoke <https://gateway>\n  codelocal login <https://gateway>\n`);
}

function httpToWs(base: string) {
  const url = new URL(base);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  url.pathname = "/client";
  url.search = "";
  url.hash = "";
  return url.toString();
}

function normalizeBase(value: string) {
  const url = new URL(value);
  url.pathname = "";
  url.search = "";
  url.hash = "";
  return url.toString().replace(/\/$/, "");
}

async function commandExists(command: string) {
  const dirs = (process.env.PATH ?? "").split(path.delimiter);
  const suffixes = process.platform === "win32" ? ["", ".exe", ".cmd", ".bat"] : [""];
  for (const dir of dirs) {
    for (const suffix of suffixes) {
      try { await fs.access(path.join(dir, `${command}${suffix}`)); return true; } catch {}
    }
  }
  return false;
}

async function doctor(projectArg: string) {
  const project = await fs.realpath(path.resolve(projectArg || "."));
  const checks: Array<{ name: string; ok: boolean; detail?: string }> = [];
  checks.push({ name: "project root", ok: true, detail: project });
  for (const command of ["git", "rg", "node", "npm"]) checks.push({ name: command, ok: await commandExists(command) });
  for (const command of ["pyright-langserver", "rust-analyzer", "gopls", "clangd", "jdtls", "lua-language-server", "sourcekit-lsp", "zls"]) {
    checks.push({ name: `semantic:${command}`, ok: await commandExists(command), detail: "optional; text fallback remains available" });
  }
  const markers = ["package.json", "pyproject.toml", "Cargo.toml", "go.mod", "pom.xml", "build.gradle", "CMakeLists.txt", "Package.swift", "pubspec.yaml"];
  const detected: string[] = [];
  for (const marker of markers) if (await fs.stat(path.join(project, marker)).then(() => true).catch(() => false)) detected.push(marker);
  console.log(`\nCodeLocal doctor\nProject: ${project}\nDetected: ${detected.join(", ") || "generic workspace"}\n`);
  for (const check of checks) console.log(`${check.ok ? "✓" : "·"} ${check.name}${check.detail ? ` — ${check.detail}` : ""}`);
  console.log("\nRequired core tools missing:", checks.filter((c) => ["git", "node", "npm"].includes(c.name) && !c.ok).map((c) => c.name).join(", ") || "none");
  console.log("Semantic servers are optional because CodeLocal falls back to TypeScript AST/text search.\n");
}

async function pair(serverArg: string) {
  const base = normalizeBase(serverArg);
  const identity = defaultDeviceIdentity();
  const response = await fetch(`${base}/pair/start`, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify(identity) });
  if (!response.ok) throw new Error(`Pair start failed: ${response.status} ${await response.text()}`);
  const pairing = await response.json() as { pairingId: string; code: string; expiresAt: number; approveUrl: string };
  console.log(`\nPairing code: ${pairing.code}\nApprove URL: ${pairing.approveUrl}\n`);
  console.log("Open the URL, enter this pairing code and your CodeLocal passphrase. Waiting for approval...");
  while (Date.now() < pairing.expiresAt) {
    await new Promise((resolve) => setTimeout(resolve, 2000));
    const claim = await fetch(`${base}/pair/claim`, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ pairingId: pairing.pairingId, code: pairing.code }) });
    if (claim.ok) {
      const credential = await claim.json() as { credentialId: string; credentialSecret: string; deviceId: string; deviceName: string };
      const wsUrl = httpToWs(base);
      await saveLocalCredential({ ...credential, serverUrl: wsUrl });
      console.log(`\n✓ Device paired. Credential saved privately for ${wsUrl}.`);
      return;
    }
  }
  throw new Error("Pairing expired before approval.");
}

async function start(projectArg: string, serverArg?: string) {
  const project = await fs.realpath(path.resolve(projectArg || "."));
  const credential = await loadLocalCredential();
  const server = serverArg ? (serverArg.startsWith("ws") ? serverArg : httpToWs(normalizeBase(serverArg))) : process.env.SERVER_URL ?? credential?.serverUrl;
  if (!server) throw new Error("No gateway configured. Run `codelocal pair https://gateway` or pass a websocket URL.");
  process.env.PROJECT_ROOT = project;
  process.env.SERVER_URL = server;
  process.env.CODELOCAL_ALLOW_SHELL ??= "1";
  process.env.CODELOCAL_APPROVAL_MODE ??= "prompt";
  await import("./client-v2.js");
}

async function promptSecret(label: string) {
  const rl = createInterface({ input: process.stdin, output: process.stdout });
  try { return (await rl.question(`${label}: `)).trim(); } finally { rl.close(); }
}

async function rotate(serverArg: string) {
  const base = normalizeBase(serverArg);
  const ws = httpToWs(base);
  const credential = await loadLocalCredential(ws);
  if (!credential) throw new Error("No matching local credential.");
  const password = await promptSecret("CodeLocal passphrase");
  const response = await fetch(`${base}/devices/rotate`, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ credentialId: credential.credentialId, password }) });
  if (!response.ok) throw new Error(`Rotate failed: ${response.status}`);
  const rotated = await response.json() as { credentialId: string; credentialSecret: string };
  await saveLocalCredential({ ...credential, credentialSecret: rotated.credentialSecret, serverUrl: ws, createdAt: credential.createdAt });
  console.log("✓ Device credential rotated.");
}

async function revoke(serverArg: string) {
  const base = normalizeBase(serverArg);
  const ws = httpToWs(base);
  const credential = await loadLocalCredential(ws);
  if (!credential) throw new Error("No matching local credential.");
  const password = await promptSecret("CodeLocal passphrase");
  const response = await fetch(`${base}/devices/revoke`, { method: "POST", headers: { "content-type": "application/json" }, body: JSON.stringify({ credentialId: credential.credentialId, password }) });
  if (!response.ok) throw new Error(`Revoke failed: ${response.status}`);
  await deleteLocalCredential();
  console.log("✓ Device revoked and local credential removed.");
}

async function status() {
  const credential = await loadLocalCredential();
  console.log(JSON.stringify({
    device: defaultDeviceIdentity(),
    paired: !!credential,
    credential: credential ? { credentialId: credential.credentialId, deviceId: credential.deviceId, deviceName: credential.deviceName, serverUrl: credential.serverUrl, createdAt: credential.createdAt, credentialSecret: "[REDACTED]" } : null,
    stateDir: process.env.CODELOCAL_STATE_DIR ?? path.join(os.homedir(), ".codelocal"),
  }, null, 2));
}

async function login(serverArg: string) {
  const base = normalizeBase(serverArg);
  console.log(`Gateway: ${base}\nMCP endpoint: ${base}/mcp\nOAuth authorization happens when you connect this MCP endpoint from ChatGPT.`);
}

const [, , command, ...args] = process.argv;
try {
  if (!command || command === "help" || command === "--help" || command === "-h") usage();
  else if (command === "doctor") await doctor(args[0] ?? ".");
  else if (command === "pair") await pair(args[0] ?? process.env.CODELOCAL_SERVER ?? "");
  else if (command === "start") await start(args[0] ?? ".", args[1]);
  else if (command === "status") await status();
  else if (command === "rotate") await rotate(args[0] ?? process.env.CODELOCAL_SERVER ?? "");
  else if (command === "revoke") await revoke(args[0] ?? process.env.CODELOCAL_SERVER ?? "");
  else if (command === "login") await login(args[0] ?? process.env.CODELOCAL_SERVER ?? "");
  else { usage(); process.exitCode = 1; }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}

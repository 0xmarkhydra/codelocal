#!/usr/bin/env node
import path from "node:path";
import os from "node:os";
import { promises as fs } from "node:fs";
import { createInterface } from "node:readline/promises";
import { defaultDeviceIdentity, deleteLocalCredential, loadLocalCredential, saveLocalCredential } from "./identity.js";
import { McpHub, parseEnvReference, parseHeaderEnvReference, type McpHeaderReference } from "./mcp-hub.js";

function usage() {
  console.log(`CodeLocal CLI

Usage:
  codelocal .
  codelocal <project-path>

Core commands:
  codelocal doctor <project>
  codelocal pair <https://gateway>
  codelocal start <project> [wss://gateway/client]
  codelocal status
  codelocal rotate <https://gateway>
  codelocal revoke <https://gateway>
  codelocal login <https://gateway>

MCP Hub:
  codelocal mcp add <name> -- <command> [args...]
  codelocal mcp add <name> --stdio <command> [--arg <arg> ...]
  codelocal mcp add <name> --url <https://server/mcp>
  codelocal mcp list [--json]
  codelocal mcp info <name>
  codelocal mcp probe <name>
  codelocal mcp search <query> [--server <name>]
  codelocal mcp remove <name> [--global|--workspace]

MCP add options:
  --global                 Install for every workspace (default: current workspace)
  --workspace              Install only for the current workspace
  --cwd <path>             Working directory for stdio MCP
  --env KEY[=SOURCE_ENV]   Pass an environment variable by reference; secret is not stored
  --header-env H=ENV       HTTP header value from an environment variable
  --bearer-env ENV         Authorization: Bearer <value from ENV>
  --no-probe               Save config without connecting/listing tools
`);
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
  await import("./client-entry-v2.js");
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

function optionValues(args: string[], option: string) {
  const values: string[] = [];
  for (let i = 0; i < args.length; i++) {
    if (args[i] !== option) continue;
    const value = args[i + 1];
    if (!value || value.startsWith("--")) throw new Error(`${option} requires a value.`);
    values.push(value);
    i++;
  }
  return values;
}

function optionValue(args: string[], option: string) {
  const values = optionValues(args, option);
  if (values.length > 1) throw new Error(`${option} may only be specified once.`);
  return values[0];
}

async function mcpCommand(args: string[]) {
  const [actionRaw, nameOrQuery, ...rest] = args;
  const action = actionRaw === "install" ? "add" : actionRaw;
  const hub = new McpHub(process.cwd());
  if (!action || action === "help" || action === "--help" || action === "-h") {
    usage();
    return;
  }
  if (action === "list") {
    const servers = await hub.listServers();
    if (rest.includes("--json") || nameOrQuery === "--json") console.log(JSON.stringify({ servers }, null, 2));
    else if (!servers.length) console.log("No MCP servers installed for this workspace.");
    else {
      console.log("\nCodeLocal MCP servers\n");
      for (const server of servers) console.log(`${server.enabled ? "✓" : "·"} ${server.name}  ${server.transport}  ${server.scope}  tools:${server.toolsCached}${server.connected ? "  connected" : ""}`);
      console.log("");
    }
    return;
  }
  if (action === "info") {
    if (!nameOrQuery) throw new Error("Usage: codelocal mcp info <name>");
    console.log(JSON.stringify(await hub.serverInfo(nameOrQuery), null, 2));
    return;
  }
  if (action === "probe") {
    if (!nameOrQuery) throw new Error("Usage: codelocal mcp probe <name>");
    const result = await hub.probe(nameOrQuery);
    console.log(`✓ ${nameOrQuery}: connected, ${result.toolCount} tools discovered.`);
    for (const tool of result.tools.slice(0, 20)) console.log(`  - ${tool.name}${tool.description ? ` — ${tool.description.slice(0, 100)}` : ""}`);
    if (result.truncated || result.toolCount > 20) console.log(`  … ${result.toolCount - Math.min(20, result.toolCount)} more`);
    await hub.shutdown();
    return;
  }
  if (action === "search") {
    const queryParts = [nameOrQuery, ...rest.filter((value, index) => value !== "--server" && (index === 0 || rest[index - 1] !== "--server"))].filter((value): value is string => !!value && !value.startsWith("--"));
    const server = optionValue(rest, "--server");
    const query = queryParts.join(" ").trim();
    if (!query) throw new Error("Usage: codelocal mcp search <query> [--server <name>]");
    console.log(JSON.stringify(await hub.searchTools(query, { server }), null, 2));
    await hub.shutdown();
    return;
  }
  if (action === "remove" || action === "uninstall") {
    if (!nameOrQuery) throw new Error("Usage: codelocal mcp remove <name> [--global|--workspace]");
    if (rest.includes("--global") && rest.includes("--workspace")) throw new Error("Choose only one of --global or --workspace.");
    const scope = rest.includes("--global") ? "global" as const : rest.includes("--workspace") ? "workspace" as const : undefined;
    const result = await hub.removeServer(nameOrQuery, scope);
    console.log(result.removed ? `✓ Removed ${nameOrQuery} (${result.removed} config${result.removed === 1 ? "" : "s"}).` : `No matching MCP config found for ${nameOrQuery}.`);
    return;
  }
  if (action !== "add") throw new Error(`Unknown MCP action: ${action}`);
  if (!nameOrQuery) throw new Error("Usage: codelocal mcp add <name> -- <command> [args...] OR --url <url>");

  const separator = rest.indexOf("--");
  const commandTail = separator >= 0 ? rest.slice(separator + 1) : [];
  const options = separator >= 0 ? rest.slice(0, separator) : rest;
  if (options.includes("--global") && options.includes("--workspace")) throw new Error("Choose only one of --global or --workspace.");
  const scope = options.includes("--global") ? "global" as const : "workspace" as const;
  const stdioOption = optionValue(options, "--stdio");
  const remoteUrl = optionValue(options, "--url");
  if (commandTail.length && stdioOption) throw new Error("Use either `-- <command>` or --stdio, not both.");
  if (remoteUrl && (commandTail.length || stdioOption)) throw new Error("Use either a stdio command or --url, not both.");

  const env = Object.fromEntries(optionValues(options, "--env").map(parseEnvReference));
  const headers: Record<string, McpHeaderReference> = Object.fromEntries(optionValues(options, "--header-env").map(parseHeaderEnvReference));
  const bearerEnv = optionValue(options, "--bearer-env");
  if (bearerEnv) headers.Authorization = { source: bearerEnv, prefix: "Bearer " };
  const cwd = optionValue(options, "--cwd");
  const command = commandTail[0] ?? stdioOption;
  const commandArgs = commandTail.length ? commandTail.slice(1) : optionValues(options, "--arg");

  if (!command && !remoteUrl) throw new Error("MCP add requires `-- <command> [args...]`, --stdio <command>, or --url <url>.");
  const server = await hub.addServer({
    name: nameOrQuery,
    enabled: true,
    scope,
    workspaceRoot: scope === "workspace" ? process.cwd() : undefined,
    transport: remoteUrl ? "http" : "stdio",
    command,
    args: command ? commandArgs : undefined,
    cwd,
    env: Object.keys(env).length ? env : undefined,
    url: remoteUrl,
    headers: Object.keys(headers).length ? headers : undefined,
  });
  console.log(`✓ Installed MCP ${server.name} (${server.transport}, ${server.scope}).`);
  if (!options.includes("--no-probe")) {
    try {
      const result = await hub.probe(server.name);
      console.log(`✓ Probe passed: ${result.toolCount} tools cached for smart routing.`);
    } catch (error) {
      console.warn(`! Installed, but probe failed: ${error instanceof Error ? error.message : String(error)}`);
      console.warn(`  Fix the MCP configuration/environment, then run: codelocal mcp probe ${server.name}`);
    }
  }
  await hub.shutdown();
}

async function looksLikeProjectPath(value: string) {
  if (!value || value.startsWith("-")) return false;
  if (value === "." || value === ".." || value.startsWith("./") || value.startsWith("../") || path.isAbsolute(value)) return true;
  return fs.stat(path.resolve(value)).then((stat) => stat.isDirectory()).catch(() => false);
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
  else if (command === "mcp") await mcpCommand(args);
  else if (await looksLikeProjectPath(command)) await start(command, args[0]);
  else { usage(); process.exitCode = 1; }
} catch (error) {
  console.error(error instanceof Error ? error.message : String(error));
  process.exitCode = 1;
}

import express, { type Request, type Response } from "express";
import http from "node:http";
import { randomUUID, timingSafeEqual } from "node:crypto";
import { WebSocketServer, type WebSocket } from "ws";
import { McpServer } from "@modelcontextprotocol/sdk/server/mcp.js";
import { StreamableHTTPServerTransport } from "@modelcontextprotocol/sdk/server/streamableHttp.js";
import { isInitializeRequest } from "@modelcontextprotocol/sdk/types.js";
import { z } from "zod";
import { oauthRouter, requireMcpAuth } from "./oauth.js";
import { log, summarizeToolArgs } from "./log.js";
import { PROTOCOL_VERSION, MIN_PROTOCOL_VERSION, isSideEffectingTool, protocolCompatible } from "./protocol.js";
import { CloudDeviceStore } from "./cloud-device-store.js";
import { audit } from "./audit.js";
import { cloudStore } from "./cloud-store.js";
import { runtimeActivationStore } from "./runtime-activation-store.js";
import { webAuthRouter, getWebIdentity, requireWebUser, verifyCsrf } from "./saas-auth.js";
import { createDashboardRouter } from "./dashboard.js";
import { authPage, escapeHtml, landingPage } from "./web-ui.js";
import { bridgeMcpToolResult } from "./mcp-bridge.js";
import { createWorkspaceRoutingState, selectWorkspaceForSession, workspaceKeyForSession, type WorkspaceRoutingState } from "./mcp-session-routing.js";
import { clientReleaseManifest, evaluateClientUpdate, renderClientUpdateNotice, type ClientUpdateNotice } from "./client-update.js";
import { VERSION } from "./version.js";
import { rateLimit } from "./rate-limit.js";
const PORT = Number(process.env.PORT ?? 3333);
const HOST = process.env.HOST ?? "0.0.0.0";
const DEVICE_TOKEN = process.env.DEVICE_TOKEN ?? "";
const TOOL_TIMEOUT_MS = Number(process.env.TOOL_TIMEOUT_MS ?? 180_000);
const HEARTBEAT_MS = Number(process.env.CODELOCAL_HEARTBEAT_MS ?? 20_000);
const STALE_MS = Number(process.env.CODELOCAL_STALE_MS ?? 70_000);
const WORKSPACE_ACTIVATION_TIMEOUT_MS = Number(process.env.CODELOCAL_WORKSPACE_ACTIVATION_TIMEOUT_MS ?? 30_000);
const ALLOW_LEGACY_DEVICE_TOKEN = process.env.ALLOW_LEGACY_DEVICE_TOKEN === "1" && !!DEVICE_TOKEN;
const CLIENT_RELEASE = clientReleaseManifest();
const deviceStore = new CloudDeviceStore();

await cloudStore.init();

type ClientCapabilities = {
  filesystem?: boolean; git?: boolean; shell?: boolean; pty?: boolean; sandbox?: string; semanticProviders?: string[]; idempotency?: boolean; cancellation?: boolean; approvals?: boolean; terminalChatApproval?: boolean; terminalHistory?: boolean; mcpHub?: boolean;
};

type ClientRecord = {
  key: string;
  userId: string;
  deviceId: string;
  deviceName: string;
  workspaceId: string;
  workspaceName: string;
  projectRoot?: string;
  credentialId?: string;
  protocolVersion: number;
  clientVersion?: string;
  capabilities: ClientCapabilities;
  ws: WebSocket;
  connectedAt: number;
  lastSeenAt: number;
};

type Pending = {
  resolve: (value: unknown) => void;
  reject: (error?: unknown) => void;
  timer: NodeJS.Timeout;
  tool: string;
  startedAt: number;
  clientKey: string;
  userId: string;
  abortCleanup?: () => void;
};

type TransportRecord = { transport: StreamableHTTPServerTransport; userId: string; routing: WorkspaceRoutingState };

const clients = new Map<string, ClientRecord>();
const socketKeys = new Map<WebSocket, string>();
const pending = new Map<string, Pending>();
const transports: Record<string, TransportRecord> = {};

function clientKey(userId: string, deviceId: string, workspaceId: string) { return `${userId}::${deviceId}::${workspaceId}`; }
function textResult(value: unknown, notice?: ClientUpdateNotice | null) {
  const content: Array<{ type: "text"; text: string }> = [];
  if (notice) content.push({ type: "text", text: renderClientUpdateNotice(notice) });
  content.push({ type: "text", text: typeof value === "string" ? value : JSON.stringify(value, null, 2) });
  return { content };
}
function prependNotice(result: any, notice?: ClientUpdateNotice | null) {
  if (!notice) return result;
  return { ...result, content: [{ type: "text" as const, text: renderClientUpdateNotice(notice) }, ...(Array.isArray(result?.content) ? result.content : [])] };
}
function clientUpdateStatus(clientVersion?: string) {
  const notice = evaluateClientUpdate(clientVersion, CLIENT_RELEASE);
  return notice ? { level: notice.level, installedVersion: notice.installedVersion, latestVersion: notice.latestVersion, minimumVersion: notice.minimumVersion, updateCommand: notice.updateCommand, restartCommand: notice.restartCommand } : null;
}
function availableClients(userId: string) { return [...clients.values()].filter((client) => client.userId === userId && client.ws.readyState === client.ws.OPEN); }
function equalSecret(a: string, b: string) { const aa = Buffer.from(a); const bb = Buffer.from(b); return aa.length === bb.length && timingSafeEqual(aa, bb); }
function sleep(ms: number) { return new Promise((resolve) => setTimeout(resolve, ms)); }
function splitWorkspaceRoutingArgs(args: unknown) {
  if (!args || typeof args !== "object" || Array.isArray(args)) return { workspaceKey: null as string | null, forwardedArgs: args };
  const record = args as Record<string, unknown>;
  const rawWorkspaceKey = record.workspaceKey;
  const workspaceKey = typeof rawWorkspaceKey === "string" && rawWorkspaceKey.trim() ? rawWorkspaceKey.trim() : null;
  if (!Object.prototype.hasOwnProperty.call(record, "workspaceKey")) return { workspaceKey, forwardedArgs: args };
  const { workspaceKey: _workspaceKey, ...forwardedArgs } = record;
  return { workspaceKey, forwardedArgs };
}

async function workspaceCatalog(userId: string) {
  const workspaces = await cloudStore.listWorkspaceRecords(userId);
  const deviceIds = [...new Set(workspaces.map((workspace) => workspace.deviceId))];
  const runtimeStates = new Map(await Promise.all(deviceIds.map(async (deviceId) => {
    const runtimeOnline = await runtimeActivationStore.isOnline(userId, deviceId).catch(() => false);
    const authorizedIds = runtimeOnline ? await runtimeActivationStore.authorizedIds(userId, deviceId).catch(() => null) : null;
    return [deviceId, { runtimeOnline, authorizedIds: new Set(authorizedIds ?? []) }] as const;
  })));
  const output = [] as Array<Record<string, unknown>>;
  for (const workspace of workspaces) {
    const key = clientKey(userId, workspace.deviceId, workspace.workspaceId);
    const active = clients.get(key);
    const storedClientVersion = typeof workspace.capabilities?.clientVersion === "string" ? workspace.capabilities.clientVersion : undefined;
    const currentClientVersion = active?.clientVersion ?? storedClientVersion;
    const runtimeState = runtimeStates.get(workspace.deviceId) ?? { runtimeOnline: false, authorizedIds: new Set<string>() };
    const authorizedNow = runtimeState.runtimeOnline ? runtimeState.authorizedIds.has(workspace.workspaceId) : null;
    if (!active && runtimeState.runtimeOnline && authorizedNow === false) continue;
    output.push({
      key,
      deviceId: workspace.deviceId,
      deviceName: active?.deviceName ?? workspace.deviceId,
      workspaceId: workspace.workspaceId,
      workspaceName: workspace.workspaceName,
      status: active ? "active" : runtimeState.runtimeOnline ? "sleeping" : "device_offline",
      runtimeOnline: runtimeState.runtimeOnline,
      authorized: active ? true : authorizedNow,
      clientVersion: currentClientVersion ?? null,
      update: clientUpdateStatus(currentClientVersion),
      projectRoot: active?.projectRoot ?? null,
      capabilities: active?.capabilities ?? workspace.capabilities ?? {},
      lastSeenAt: active?.lastSeenAt ?? workspace.lastSeenAt,
    });
  }
  return output;
}

function resolveClient(userId: string, selectedKey?: string | null) {
  if (selectedKey) {
    const client = clients.get(selectedKey);
    if (!client || client.userId !== userId || client.ws.readyState !== client.ws.OPEN) throw new Error(`Selected workspace is offline or unavailable: ${selectedKey}`);
    return client;
  }
  const online = availableClients(userId);
  if (online.length === 1) return online[0];
  if (!online.length) throw new Error("No active workspace. Call list_workspaces and select_workspace; the CodeLocal machine runtime can activate a sleeping workspace.");
  throw new Error("Multiple workspaces are active. Call list_workspaces then select_workspace. Pass the returned workspaceKey on subsequent project, Git, terminal and MCP calls for explicit thread-safe routing.");
}

async function activateWorkspace(userId: string, key: string) {
  const already = clients.get(key);
  if (already && already.userId === userId && already.ws.readyState === already.ws.OPEN) return already;
  const catalog = await workspaceCatalog(userId);
  const workspace = catalog.find((item) => item.key === key) as any;
  if (!workspace) throw new Error(`Workspace is not available or no longer authorized: ${key}`);
  if (!workspace.runtimeOnline) throw new Error(`The device for ${workspace.workspaceName} is offline. Run \`codelocal\` on that device; no project directory is required.`);
  if (workspace.authorized !== true) throw new Error(`Workspace is not authorized by the local CodeLocal runtime: ${workspace.workspaceName}`);

  const requestId = randomUUID();
  await runtimeActivationStore.request(userId, String(workspace.deviceId), { workspaceId: String(workspace.workspaceId), requestId, requestedAt: Date.now() });
  await cloudStore.audit(userId, "workspace.activation_requested", { requestId }, String(workspace.deviceId), String(workspace.workspaceId)).catch(() => undefined);
  const deadline = Date.now() + WORKSPACE_ACTIVATION_TIMEOUT_MS;
  while (Date.now() < deadline) {
    const client = clients.get(key);
    if (client && client.userId === userId && client.ws.readyState === client.ws.OPEN) {
      await cloudStore.audit(userId, "workspace.activated", { requestId }, client.deviceId, client.workspaceId).catch(() => undefined);
      return client;
    }
    await sleep(200);
  }
  throw new Error(`Workspace activation timed out after ${WORKSPACE_ACTIVATION_TIMEOUT_MS}ms. Keep \`codelocal\` running on ${workspace.deviceId} and try again.`);
}

function isTerminalExecutionTool(tool: string) {
  return tool === "run_command" || tool === "exec_start" || tool === "pty_start";
}

async function callClient(userId: string, tool: string, args: unknown, selectedKey: string | null, options: { signal?: AbortSignal; sessionId?: string } = {}) {
  const client = selectedKey ? await activateWorkspace(userId, selectedKey) : resolveClient(userId, selectedKey);
  const requestId = randomUUID();
  const startedAt = Date.now();
  const idempotencyKey = isSideEffectingTool(tool) ? requestId : undefined;
  log("info", "tool.dispatch", { requestId, tool, userId, mcpSessionId: options.sessionId ?? null, selectedWorkspaceKey: selectedKey, clientKey: client.key, args: summarizeToolArgs(tool, args), pending: pending.size });

  const result = new Promise<unknown>((resolve, reject) => {
    const timer = setTimeout(() => {
      pending.delete(requestId);
      if (client.capabilities.cancellation) client.ws.send(JSON.stringify({ type: "tool_cancel", protocolVersion: PROTOCOL_VERSION, requestId, reason: "gateway timeout" }));
      reject(new Error(`Client tool call timed out after ${TOOL_TIMEOUT_MS}ms.`));
    }, TOOL_TIMEOUT_MS);
    const entry: Pending = { resolve, reject, timer, tool, startedAt, clientKey: client.key, userId };
    if (options.signal) {
      const onAbort = () => {
        if (!pending.has(requestId)) return;
        client.ws.send(JSON.stringify({ type: "tool_cancel", protocolVersion: PROTOCOL_VERSION, requestId, reason: "MCP request cancelled" }));
        clearTimeout(timer); pending.delete(requestId); reject(new Error("Tool request cancelled."));
      };
      options.signal.addEventListener("abort", onAbort, { once: true });
      entry.abortCleanup = () => options.signal?.removeEventListener("abort", onAbort);
    }
    pending.set(requestId, entry);
  });

  client.ws.send(JSON.stringify({ type: "tool_call", protocolVersion: PROTOCOL_VERSION, requestId, id: requestId, sessionId: options.sessionId, workspaceKey: client.key, tool, args, idempotencyKey, deadline: Date.now() + TOOL_TIMEOUT_MS }));
  return { result: await result, client };
}

function createMcpServer(userId: string, routing: WorkspaceRoutingState) {
  const server = new McpServer({ name: "codelocal", version: VERSION });
  const shownUpdateNotices = new Set<string>();
  const LOCAL_RESULT = Symbol("codelocal-local-result");
  const claimUpdateNotice = (client?: ClientRecord | null) => {
    if (!client) return null;
    const notice = evaluateClientUpdate(client.clientVersion, CLIENT_RELEASE);
    if (!notice) return null;
    const key = `${client.key}:${notice.key}`;
    if (shownUpdateNotices.has(key)) return null;
    shownUpdateNotices.add(key);
    return notice;
  };
  const nextUpdateClient = () => availableClients(userId).find((client) => {
    const notice = evaluateClientUpdate(client.clientVersion, CLIENT_RELEASE);
    return notice && !shownUpdateNotices.has(`${client.key}:${notice.key}`);
  });
  const localResult = (value: unknown, client?: ClientRecord | null) => ({ [LOCAL_RESULT]: true, value, client });
  const workspaceRoutingSchema = z.string().min(1).optional().describe("Exact workspace key returned by select_workspace. Pass it to keep routing explicit across multiple active projects, ChatGPT threads or fresh MCP sessions.");
  const localTool = (name: string, title: string, description: string, inputSchema: Record<string, any>, handler: (args: any, extra: any) => unknown | Promise<unknown>) => {
    server.registerTool(name, { title, description, inputSchema }, async (args: any, extra: any) => {
      const handled: any = await handler(args, extra);
      if (handled?.[LOCAL_RESULT]) return textResult(handled.value, claimUpdateNotice(handled.client));
      return textResult(handled);
    });
  };
  const remote = (name: string, title: string, description: string, schema: Record<string, any>) => {
    const inputSchema = Object.prototype.hasOwnProperty.call(schema, "workspaceKey") ? schema : { ...schema, workspaceKey: workspaceRoutingSchema };
    server.registerTool(name, { title, description: `${description} When multiple workspaces are active, pass workspaceKey returned by select_workspace so this call stays bound to the intended project even across separate ChatGPT threads or MCP sessions.`, inputSchema }, async (args: any, extra: any) => {
      const { workspaceKey, forwardedArgs } = splitWorkspaceRoutingArgs(args);
      const selectedWorkspaceKey = workspaceKeyForSession(routing, workspaceKey);
      const dispatched = await callClient(userId, name, forwardedArgs, selectedWorkspaceKey, { signal: extra?.signal, sessionId: extra?.sessionId });
      const notice = claimUpdateNotice(dispatched.client);
      if (name === "mcp_call") {
        const bridged = bridgeMcpToolResult(dispatched.result);
        return (bridged ? prependNotice(bridged, notice) : textResult(dispatched.result, notice)) as any;
      }
      return textResult(dispatched.result, notice);
    });
  };

  localTool("list_devices", "List active devices", "List active CodeLocal workspace clients belonging to this account.", {}, async () => {
    const grouped = new Map<string, any[]>();
    for (const client of availableClients(userId)) {
      const list = grouped.get(client.deviceId) ?? [];
      list.push({ workspaceId: client.workspaceId, workspaceName: client.workspaceName, key: client.key, projectRoot: client.projectRoot ?? null, protocolVersion: client.protocolVersion, clientVersion: client.clientVersion ?? null, update: clientUpdateStatus(client.clientVersion), capabilities: client.capabilities, lastSeenAt: client.lastSeenAt });
      grouped.set(client.deviceId, list);
    }
    return localResult({ devices: [...grouped.entries()].map(([deviceId, workspaces]) => ({ deviceId, workspaces })) }, nextUpdateClient());
  });
  localTool("list_device_identities", "List paired devices", "List this account's paired device identities without secrets.", {}, () => deviceStore.listDevices(userId));
  localTool("revoke_device", "Revoke device", "Revoke one of this account's paired device credentials.", { credentialId: z.string().min(1) }, async (args) => ({ revoked: await revokeAndDisconnect(userId, args.credentialId) }));
  localTool("rename_device", "Rename device", "Rename one of this account's durable paired device identities.", { credentialId: z.string().min(1), deviceName: z.string().min(1).max(120) }, async (args) => ({ renamed: await deviceStore.rename(userId, args.credentialId, args.deviceName) }));
  localTool("list_workspaces", "List authorized workspaces", "List workspaces previously granted on CodeLocal devices. Sleeping workspaces can be activated without the user changing terminal directories. Each returned key can be used as workspaceKey for explicit thread-safe routing.", {}, async () => localResult({ selectedWorkspace: routing.selectedWorkspaceKey, workspaces: await workspaceCatalog(userId) }, nextUpdateClient()));
  localTool("select_workspace", "Select and activate workspace", "Select a workspace for this ChatGPT MCP session. Returns workspaceKey; pass it to subsequent project, Git, terminal and MCP calls when multiple projects are active or when the client starts a fresh MCP session.", { key: z.string().min(1) }, async (args, extra) => {
    const client = await activateWorkspace(userId, String(args.key));
    selectWorkspaceForSession(routing, client.key);
    log("info", "mcp.workspace_selected", { mcpSessionId: extra?.sessionId ?? null, userId, workspaceKey: client.key });
    return localResult({ selected: client.key, workspaceKey: client.key, deviceId: client.deviceId, workspaceId: client.workspaceId, workspaceName: client.workspaceName, clientVersion: client.clientVersion ?? null, update: clientUpdateStatus(client.clientVersion), status: "active" }, client);
  });
  localTool("workspace_info", "Workspace info", "Show a selected CodeLocal workspace. Pass workspaceKey for explicit routing if the current MCP session does not retain selection.", { workspaceKey: workspaceRoutingSchema }, async (args) => {
    const explicitKey = typeof args.workspaceKey === "string" && args.workspaceKey.trim() ? args.workspaceKey.trim() : null;
    const targetKey = workspaceKeyForSession(routing, explicitKey);
    const client = targetKey ? await activateWorkspace(userId, targetKey) : resolveClient(userId, null);
    return localResult({ selected: client.key, workspaceKey: client.key, deviceId: client.deviceId, deviceName: client.deviceName, workspaceId: client.workspaceId, workspaceName: client.workspaceName, projectRoot: client.projectRoot ?? null, protocolVersion: client.protocolVersion, clientVersion: client.clientVersion ?? null, update: clientUpdateStatus(client.clientVersion), capabilities: client.capabilities, lastSeenAt: client.lastSeenAt }, client);
  });

  remote("project_info", "Project info", "Inspect workspace capabilities, project map, semantic providers, host execution policy, approval memory and instructions. Call first after selecting a workspace. For coding/debug/refactor tasks, follow it with context_for_task before broad file listing, text search, or multi-file reads.", {});
  remote("project_map", "Project map", "Return cached compact project structure, languages, frameworks, commands and roots.", { force: z.boolean().default(false) });
  remote("context_for_task", "Context for task", "Primary semantic-first retrieval step for coding, debugging, review and refactor work. Call this with the user's concrete task before broad repository scans. It returns ranked files, semantic/LSP symbols, dependency graph neighbors, diagnostics and bounded source snippets; use that packet first, then request exact definitions/references or targeted ranges only as needed.", { taskHint: z.string().min(1), limit: z.number().int().min(1).max(100).default(30) });
  remote("read_instructions", "Read instructions", "Read scoped AGENTS.md and supported coding instructions.", { path: z.string().default(".") });
  remote("list_files", "List files", "Gitignore-aware project listing. Sensitive paths remain blocked. For coding tasks, prefer context_for_task first and use listing only when structural discovery is still needed.", { path: z.string().default("."), maxDepth: z.number().int().min(0).max(20).default(4), includeIgnored: z.boolean().default(false) });
  remote("file_info", "File metadata", "Read metadata/hash without source content.", { path: z.string().min(1) });
  remote("read_file", "Read file", "Read a targeted UTF-8 file. For coding tasks, choose the file from context_for_task or semantic navigation rather than guessing paths from a broad scan.", { path: z.string().min(1) });
  remote("read_file_range", "Read file range", "Read a targeted line range. Prefer ranges identified by context_for_task, definitions/references, symbols or diagnostics.", { path: z.string().min(1), startLine: z.number().int().min(1), endLine: z.number().int().min(1) });
  remote("read_files", "Read files", "Batch read targeted files. Avoid broad multi-file dumping; for coding tasks use context_for_task first and batch-read only the ranked files that still need more context.", { paths: z.array(z.string().min(1)).min(1).max(50) });
  remote("search_code", "Search code", "Literal-text fallback for unknown strings, config keys, logs and exact text. For coding/debug/refactor discovery, call context_for_task and semantic definition/reference tools first; do not use repository-wide grep as the default context strategy.", { query: z.string().min(1), path: z.string().default("."), maxResults: z.number().int().min(1).max(1000).default(200), fixedStrings: z.boolean().default(false), includeIgnored: z.boolean().default(false) });
  remote("inspect_dependency", "Inspect dependency", "Inspect dependency metadata for Node/Python/Rust/Go.", { name: z.string().min(1), ecosystem: z.enum(["auto", "node", "python", "rust", "go"]).default("auto") });
  remote("read_dependency", "Read dependency", "Read a targeted installed Node dependency file.", { name: z.string().min(1), path: z.string().default("package.json"), startLine: z.number().int().min(1).optional(), endLine: z.number().int().min(1).optional(), ecosystem: z.string().optional() });
  remote("search_dependency", "Search dependency", "Search inside one installed Node dependency.", { name: z.string().min(1), query: z.string().min(1), maxResults: z.number().int().min(1).max(500).default(100), fixedStrings: z.boolean().default(false) });

  remote("semantic_info", "Semantic providers", "Show installed semantic providers and fallback mode.", {});
  remote("workspace_symbols", "Workspace symbols", "Find symbols across a polyglot workspace.", { query: z.string().default(""), limit: z.number().int().min(1).max(1000).default(200) });
  remote("find_symbol", "Find symbol", "Compatibility alias for workspace symbol lookup.", { query: z.string().default(""), limit: z.number().int().min(1).max(1000).default(200) });
  remote("document_symbols", "Document symbols", "Find structural symbols in a file using LSP/TypeScript/fallback.", { path: z.string().min(1), limit: z.number().int().min(1).max(1000).default(500) });
  const semanticPositionSchema = { path: z.string().optional(), line: z.number().int().min(1).optional(), column: z.number().int().min(1).optional(), name: z.string().optional(), query: z.string().optional(), limit: z.number().int().min(1).max(2000).optional() };
  remote("find_definition", "Find definition", "Find definitions using exact LSP position when supplied, otherwise name/fallback search.", semanticPositionSchema);
  remote("find_references", "Find references", "Find references using exact LSP position when supplied, otherwise name/fallback search.", semanticPositionSchema);
  remote("find_implementations", "Find implementations", "Find implementations using the file position semantic provider.", { path: z.string().min(1), line: z.number().int().min(1), column: z.number().int().min(1), limit: z.number().int().min(1).max(1000).default(200) });
  remote("get_hover", "Get hover", "Get type/signature/hover information at a file position.", { path: z.string().min(1), line: z.number().int().min(1), column: z.number().int().min(1) });
  remote("get_diagnostics", "Get diagnostics", "Get diagnostics from TypeScript or the file's LSP provider.", { path: z.string().optional(), limit: z.number().int().min(1).max(2000).default(500) });
  remote("get_callers", "Get callers", "Get TypeScript/JavaScript callers where semantic call graph is available.", { name: z.string().min(1), limit: z.number().int().min(1).max(1000).default(300) });
  remote("get_callees", "Get callees", "Get TypeScript/JavaScript callees where semantic call graph is available.", { name: z.string().min(1), limit: z.number().int().min(1).max(1000).default(300) });
  remote("get_import_graph", "Get import graph", "Get current lightweight import graph.", { limit: z.number().int().min(1).max(10000).default(2000) });

  remote("write_file", "Write file", "Create/overwrite a file with optional stale-hash protection.", { path: z.string().min(1), content: z.string(), expectedHash: z.string().optional() });
  remote("edit_file", "Exact edit", "Compatibility exact-text edit with optional hash protection.", { path: z.string().min(1), oldText: z.string().min(1), newText: z.string(), replaceAll: z.boolean().default(false), expectedHash: z.string().optional() });
  remote("apply_patch", "Apply patch", "Validate and apply a unified Git patch.", { patch: z.string().min(1) });
  remote("apply_edits", "Apply structured edits", "Transactionally validate and apply multiple hash-safe file/range edits with rollback on failure.", { files: z.array(z.object({ path: z.string().min(1), expectedHash: z.string().optional(), edits: z.array(z.object({ startOffset: z.number().int().min(0).optional(), endOffset: z.number().int().min(0).optional(), startLine: z.number().int().min(1).optional(), startColumn: z.number().int().min(1).optional(), endLine: z.number().int().min(1).optional(), endColumn: z.number().int().min(1).optional(), replacement: z.string() })).min(1) })).min(1).max(100) });
  remote("format_changed_files", "Format changed files", "Use installed project/language formatters without downloading tools.", { paths: z.array(z.string().min(1)).min(1).max(200) });
  remote("snapshot_diagnostics", "Snapshot diagnostics", "Create a before-change diagnostic baseline.", { paths: z.array(z.string()).default([]) });
  remote("verify_changes", "Verify changes", "Return diagnostics regression, recommended checks and Git diff after edits.", { paths: z.array(z.string()).default([]), baselineId: z.string().optional() });

  remote("git_status", "Git status", "Read branch and working tree status.", {});
  remote("git_diff", "Git diff", "Read working/staged diff.", { cached: z.boolean().default(false), path: z.string().optional() });
  remote("git_log", "Git log", "Read recent Git history.", { limit: z.number().int().min(1).max(100).default(20), path: z.string().optional() });
  remote("git_show", "Git show", "Read a commit/ref summary.", { ref: z.string().default("HEAD") });
  remote("git_blame", "Git blame", "Read blame for a file/range.", { path: z.string().min(1), startLine: z.number().int().min(1).optional(), endLine: z.number().int().min(1).optional() });
  remote("git_file_history", "Git file history", "Read follow-renames file history.", { path: z.string().min(1), limit: z.number().int().min(1).max(100).default(30) });
  remote("git_stage", "Stage files", "Git stage operation. If approval_required is returned, ask the user in ChatGPT and retry with approvalToken.", { paths: z.array(z.string().min(1)).min(1).max(200), approvalToken: z.string().optional() });
  remote("git_unstage", "Unstage files", "Git unstage operation. If approval_required is returned, ask the user in ChatGPT and retry with approvalToken.", { paths: z.array(z.string().min(1)).min(1).max(200), approvalToken: z.string().optional() });
  remote("git_commit", "Commit staged changes", "Commit staged changes on the host. The first reviewed approval is confirmed in ChatGPT and then remembered locally for this workspace; critical Git actions are never remembered.", { message: z.string().min(1).max(5000), expectedPaths: z.array(z.string()).optional(), approvalToken: z.string().optional() });
  remote("git_push", "Push commits", "Non-force push on the host. Explicit remote+branch approvals can be remembered locally and are bound to the current remote URL; force push remains blocked by this tool.", { remote: z.string().optional(), branch: z.string().optional(), force: z.boolean().default(false), approvalToken: z.string().optional() });

  remote("sandbox_info", "Execution security", "Compatibility tool that reports the active host-policy execution model. OS sandboxing is disabled.", {});
  remote("sandbox_smoke_test", "Execution security smoke test", "Compatibility check for host-policy execution; no OS sandbox is started.", {});
  remote("terminal_preflight", "Check terminal command risk", "Call this before running a terminal command. Deterministic local policy blocks workspace escapes and credentials, reuses remembered structured approvals, and requires fresh ChatGPT confirmation for critical actions.", { command: z.string().min(1), cwd: z.string().default(".") });
  remote("terminal_history", "Terminal history", "Query the local redacted audit history of terminal commands CodeLocal actually executed in this workspace.", { query: z.string().default(""), limit: z.number().int().min(1).max(500).default(50), event: z.enum(["started", "finished", "all"]).default("started") });
  remote("run_command", "Run command", "Run a host command after terminal_preflight. Safe or remembered actions run immediately; reviewed actions require ChatGPT approval, while critical actions always require fresh approval.", { command: z.string().min(1), cwd: z.string().default("."), approvalToken: z.string().optional(), yieldMs: z.number().int().min(0).max(10000).default(1000), timeoutMs: z.number().int().min(0).max(3_600_000).default(0) });
  remote("exec_start", "Start process", "Start a guarded non-PTY process. Call terminal_preflight first; reviewed commands require the one-time approvalToken after explicit confirmation in ChatGPT.", { command: z.string().min(1), cwd: z.string().default("."), approvalToken: z.string().optional(), timeoutMs: z.number().int().min(0).max(3_600_000).default(0) });
  remote("exec_poll", "Poll process", "Read incremental stdout/stderr.", { processId: z.string().min(1), stdoutCursor: z.number().int().min(0).optional(), stderrCursor: z.number().int().min(0).optional() });
  remote("exec_write", "Write process stdin", "Write to a running process stdin.", { processId: z.string().min(1), input: z.string() });
  remote("exec_signal", "Signal process", "Send a supported signal to a process.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGINT", "SIGKILL"]).default("SIGTERM") });
  remote("exec_cancel", "Cancel process", "Cancel a running process.", { processId: z.string().min(1), reason: z.string().optional() });
  remote("exec_kill", "Kill process", "Terminate a process.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGKILL"]).default("SIGTERM") });
  remote("pty_start", "Start PTY", "Start a guarded PTY. Call terminal_preflight first; reviewed commands require the one-time approvalToken after explicit confirmation in ChatGPT.", { command: z.string().min(1), cwd: z.string().default("."), approvalToken: z.string().optional(), timeoutMs: z.number().int().min(0).max(3_600_000).default(0) });
  remote("pty_poll", "Poll PTY", "Read incremental PTY output.", { processId: z.string().min(1), stdoutCursor: z.number().int().min(0).optional(), stderrCursor: z.number().int().min(0).optional() });
  remote("pty_write", "Write PTY", "Write input to PTY/process.", { processId: z.string().min(1), input: z.string() });
  remote("pty_resize", "Resize PTY", "Resize a true PTY.", { processId: z.string().min(1), cols: z.number().int().min(10).max(500), rows: z.number().int().min(5).max(300) });
  remote("pty_signal", "Signal PTY", "Signal PTY/process.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGINT", "SIGKILL"]).default("SIGTERM") });
  remote("pty_kill", "Kill PTY", "Terminate PTY/process.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGKILL"]).default("SIGTERM") });
  remote("process_list", "List processes", "List CodeLocal-started processes.", {});
  remote("process_poll", "Poll process compatibility", "Compatibility poll tool.", { processId: z.string().min(1), cursor: z.number().int().min(0).optional() });
  remote("process_write", "Write process compatibility", "Compatibility stdin tool.", { processId: z.string().min(1), input: z.string() });
  remote("process_kill", "Kill process compatibility", "Compatibility termination tool.", { processId: z.string().min(1), signal: z.enum(["SIGTERM", "SIGKILL"]).default("SIGTERM") });

  remote("approval_list", "List remembered approvals", "List approval memory stored locally for the selected workspace. No approval data is stored in CodeLocal Cloud.", {});
  remote("approval_revoke", "Revoke remembered approval", "Forget one locally remembered approval by ID or structured action key. This can only reduce future permissions.", { id: z.string().optional(), actionKey: z.string().optional() });
  remote("approval_reset", "Reset remembered approvals", "Forget every locally remembered approval for the selected workspace. This can only reduce future permissions.", {});

  remote("mcp_list", "List installed MCPs", "List MCP extensions installed in the selected local CodeLocal workspace without exposing every extension tool to ChatGPT.", {});
  remote("mcp_search_tools", "Search installed MCP tools", "Search the local MCP extension catalog and return only the most relevant tools.", { query: z.string().default(""), limit: z.number().int().min(1).max(50).default(8), server: z.string().optional(), refresh: z.boolean().default(false) });
  remote("mcp_tool_info", "Inspect MCP tool", "Get one installed MCP tool's exact schema before calling it.", { server: z.string().min(1), tool: z.string().min(1) });
  remote("mcp_call", "Call installed MCP tool", "Call one tool from an installed MCP extension. Local policy is authoritative; approval happens in ChatGPT and never in the local terminal. Retry with approvalToken after confirmation.", { server: z.string().min(1), tool: z.string().min(1), arguments: z.record(z.unknown()).default({}), approvalToken: z.string().optional() });
  return server;
}

async function revokeAndDisconnect(userId: string, credentialId: string) {
  const identities = await deviceStore.listDevices(userId);
  const identity = identities.find((item) => item.credentialId === credentialId);
  const revoked = await deviceStore.revoke(userId, credentialId);
  if (!revoked) return false;
  for (const client of clients.values()) if (client.userId === userId && client.credentialId === credentialId) client.ws.close(4403, "device revoked");
  if (identity) {
    await runtimeActivationStore.clearPresence(userId, identity.deviceId).catch(() => undefined);
    await cloudStore.reconcileWorkspacesForDevice(userId, identity.deviceId, []).catch(() => undefined);
  }
  await cloudStore.audit(userId, "device.revoked", { credentialId });
  return true;
}

async function requestWorkspaceRevocation(userId: string, deviceId: string, workspaceId: string) {
  if (!(await runtimeActivationStore.isOnline(userId, deviceId))) throw new Error("That CodeLocal machine runtime is offline.");
  const requestId = randomUUID();
  await runtimeActivationStore.requestRevocation(userId, deviceId, { workspaceId, requestId, requestedAt: Date.now() });
  const deadline = Date.now() + 7_000;
  while (Date.now() < deadline) {
    const workspaces = await cloudStore.listWorkspaceRecords(userId);
    if (!workspaces.some((workspace) => workspace.deviceId === deviceId && workspace.workspaceId === workspaceId)) {
      await cloudStore.audit(userId, "workspace.revoked", { requestId }, deviceId, workspaceId).catch(() => undefined);
      return;
    }
    await sleep(180);
  }
}

function deviceAuthFromRequest(req: Request) {
  const credentialId = String(req.headers["x-codelocal-credential-id"] ?? "");
  const authorization = String(req.headers.authorization ?? "");
  const secret = authorization.startsWith("Device ") ? authorization.slice(7) : "";
  return { credentialId, secret };
}

const app = express();
app.disable("x-powered-by");
app.use(express.json({ limit: "12mb" }));
app.use((req, res, next) => {
  const startedAt = Date.now();
  res.on("finish", () => { if (req.path !== "/health") log("info", "http.request", { method: req.method, path: req.path, status: res.statusCode, durationMs: Date.now() - startedAt, mcpSessionId: req.headers["mcp-session-id"] ?? null }); });
  next();
});
app.use(webAuthRouter);
app.use(oauthRouter);
app.use(createDashboardRouter({
  onDeviceRevoked: async (userId, credentialId) => { await revokeAndDisconnect(userId, credentialId); },
  onWorkspaceRemoveRequested: async (userId, deviceId, workspaceId) => { await requestWorkspaceRevocation(userId, deviceId, workspaceId); },
  isDeviceOnline: async (userId, deviceId) => runtimeActivationStore.isOnline(userId, deviceId),
}));

const pairStartIpRateLimit = rateLimit({ scope: "pair-start-ip", limit: 40, windowSeconds: 10 * 60 });
const pairStartDeviceRateLimit = rateLimit({
  scope: "pair-start-device",
  limit: 12,
  windowSeconds: 10 * 60,
  subject: (req) => String(req.body?.deviceId ?? "unknown").slice(0, 200) || "unknown",
});
const pairClaimIpRateLimit = rateLimit({ scope: "pair-claim-ip", limit: 300, windowSeconds: 60 });
const pairClaimRequestRateLimit = rateLimit({
  scope: "pair-claim-request",
  limit: 120,
  windowSeconds: 60,
  subject: (req) => String(req.body?.pairingId ?? "unknown").slice(0, 120) || "unknown",
});

app.get("/", async (req, res) => {
  const base = (process.env.PUBLIC_BASE_URL ?? `${req.protocol}://${req.get("host")}`).replace(/\/$/, "");
  res.type("html").send(landingPage({ endpoint: `${base}/mcp`, signedIn: Boolean(await getWebIdentity(req)) }));
});

app.post("/pair/start", pairStartIpRateLimit, pairStartDeviceRateLimit, async (req, res) => {
  const deviceId = String(req.body?.deviceId ?? "").trim();
  const deviceName = String(req.body?.deviceName ?? deviceId).trim().slice(0, 120);
  if (!deviceId || deviceId.length > 200) { res.status(400).json({ error: "deviceId_required" }); return; }
  const pairing = await deviceStore.startPairing(deviceId, deviceName);
  const base = (process.env.PUBLIC_BASE_URL ?? `${req.protocol}://${req.get("host")}`).replace(/\/$/, "");
  res.json({ pairingId: pairing.pairingId, code: pairing.code, expiresAt: pairing.expiresAt, approveUrl: `${base}/pair/approve?pairingId=${encodeURIComponent(pairing.pairingId)}` });
});
app.get("/pair/approve", async (req, res) => {
  const pairingId = String(req.query.pairingId ?? "");
  const pairing = await deviceStore.getPairing(pairingId);
  if (!pairing || pairing.expiresAt <= Date.now() || pairing.claimedAt) { res.status(404).send("Pairing request not found or expired."); return; }
  const me = await getWebIdentity(req);
  if (!me) { res.redirect(302, `/login?next=${encodeURIComponent(req.originalUrl)}`); return; }
  res.type("html").send(authPage({
    title: "Approve device",
    subtitle: `Pair ${pairing.deviceName} with ${me.user.email}.`,
    body: `<div class="card" style="box-shadow:none;padding:16px;margin-bottom:16px"><div class="label">Device requesting access</div><div class="title" style="margin-top:6px">${escapeHtml(pairing.deviceName)}</div><div class="row-meta" style="margin-top:6px">Device ID: ${escapeHtml(pairing.deviceId)}</div></div><form class="form" method="post" action="/pair/approve"><input type="hidden" name="csrf" value="${escapeHtml(me.csrf)}"><input type="hidden" name="pairingId" value="${escapeHtml(pairingId)}"><input type="hidden" name="code" value="${escapeHtml(pairing.code)}"><button class="btn primary" type="submit">Approve device</button><div class="hint">Only approve if you just started CodeLocal on this device.</div></form>`,
  }));
});
app.post("/pair/approve", express.urlencoded({ extended: false }), requireWebUser, async (req, res) => {
  if (!verifyCsrf(req)) { res.status(403).send("Invalid security token."); return; }
  const me = res.locals.webIdentity;
  const pairing = await deviceStore.approvePairing(String(req.body?.pairingId ?? ""), String(req.body?.code ?? ""), me.user.id);
  if (!pairing) { res.status(400).send("Invalid or expired pairing request/code."); return; }
  await cloudStore.audit(me.user.id, "device.pairing_approved", { deviceId: pairing.deviceId, deviceName: pairing.deviceName }, pairing.deviceId);
  res.type("html").send(authPage({ title: "Device approved", subtitle: "Return to your terminal. CodeLocal will claim its device credential automatically.", body: `<a class="btn primary" href="/dashboard/devices">View devices</a>` }));
});
app.post("/pair/claim", pairClaimIpRateLimit, pairClaimRequestRateLimit, async (req, res) => {
  const credential = await deviceStore.claimPairing(String(req.body?.pairingId ?? ""), String(req.body?.code ?? ""));
  if (!credential) { res.status(400).json({ error: "pairing_not_approved_or_expired" }); return; }
  await cloudStore.audit(credential.userId, "device.paired", { credentialId: credential.credentialId, deviceName: credential.deviceName }, credential.deviceId);
  const { userId: _userId, ...publicCredential } = credential;
  res.json(publicCredential);
});

app.post("/api/client/auth/check", async (req, res) => {
  const { credentialId, secret } = deviceAuthFromRequest(req);
  const device = credentialId && secret ? await deviceStore.authenticate(credentialId, secret) : null;
  if (!device) { res.status(401).json({ error: "device_auth_failed" }); return; }
  res.json({ ok: true, deviceId: device.deviceId, now: Date.now() });
});

app.post("/api/client/workspaces/sync", async (req, res) => {
  const { credentialId, secret } = deviceAuthFromRequest(req);
  const device = credentialId && secret ? await deviceStore.authenticate(credentialId, secret) : null;
  if (!device) { res.status(401).json({ error: "device_auth_failed" }); return; }
  const source = Array.isArray(req.body?.workspaces) ? req.body.workspaces.slice(0, 500) : [];
  const syncedClientVersion = typeof req.body?.clientVersion === "string" && req.body.clientVersion.trim() ? req.body.clientVersion.trim().slice(0, 80) : undefined;
  let synced = 0;
  const authorizedWorkspaceIds: string[] = [];
  for (const item of source) {
    const workspaceId = String(item?.workspaceId ?? "").trim();
    const workspaceName = String(item?.workspaceName ?? workspaceId).trim().slice(0, 120);
    if (!/^[A-Za-z0-9._-]{1,80}$/.test(workspaceId) || !workspaceName) continue;
    authorizedWorkspaceIds.push(workspaceId);
    const key = clientKey(device.userId, device.deviceId, workspaceId);
    const active = clients.get(key);
    if (!active) {
      await cloudStore.upsertWorkspace({ userId: device.userId, deviceId: device.deviceId, workspaceId, workspaceName, protocolVersion: PROTOCOL_VERSION, capabilities: { authorized: true, sleeping: true, ...(syncedClientVersion ? { clientVersion: syncedClientVersion } : {}) } });
      await cloudStore.clearPresence(device.userId, device.deviceId, workspaceId);
    }
    synced++;
  }
  const removed = await cloudStore.reconcileWorkspacesForDevice(device.userId, device.deviceId, authorizedWorkspaceIds);
  await runtimeActivationStore.heartbeat(device.userId, device.deviceId, authorizedWorkspaceIds, 45);
  await cloudStore.audit(device.userId, "runtime.workspaces_synced", { count: synced, removed: removed.length }, device.deviceId).catch(() => undefined);
  res.json({ synced, removed: removed.length, syncedAt: Date.now() });
});

app.post("/api/client/runtime/poll", async (req, res) => {
  const { credentialId, secret } = deviceAuthFromRequest(req);
  const device = credentialId && secret ? await deviceStore.authenticate(credentialId, secret) : null;
  if (!device) { res.status(401).json({ error: "device_auth_failed" }); return; }
  const workspaceIds = Array.isArray(req.body?.workspaceIds) ? [...new Set(req.body.workspaceIds.map(String).filter((value: string) => /^[A-Za-z0-9._-]{1,80}$/.test(value)))].slice(0, 500) : [];
  const requestedWaitMs = Number(req.body?.waitMs ?? 0);
  const waitMs = Number.isFinite(requestedWaitMs) ? Math.max(0, Math.min(30_000, Math.floor(requestedWaitMs))) : 0;
  const presenceTtlSeconds = Math.max(20, Math.ceil((waitMs + 15_000) / 1000));
  await runtimeActivationStore.heartbeat(device.userId, device.deviceId, workspaceIds, presenceTtlSeconds);
  const next = await runtimeActivationStore.waitForNext(device.userId, device.deviceId, waitMs);
  const revocation = next.revocation;
  const activation = revocation ? null : next.activation;
  if (activation && !(await runtimeActivationStore.isAuthorized(device.userId, device.deviceId, activation.workspaceId))) {
    await cloudStore.audit(device.userId, "workspace.activation_rejected", { requestId: activation.requestId, reason: "not-authorized" }, device.deviceId, activation.workspaceId).catch(() => undefined);
    res.json({ activation: null, revocation: null, now: Date.now() });
    return;
  }
  res.json({ activation: activation ?? null, revocation: revocation ?? null, now: Date.now() });
});

app.get("/health", (_req, res) => res.json({ ok: true, version: VERSION, protocolVersion: PROTOCOL_VERSION, cloud: true, onlineWorkspaces: clients.size, pendingToolCalls: pending.size }));
app.get("/api/status", async (req, res) => {
  const me = await getWebIdentity(req);
  if (!me) { res.status(401).json({ error: "unauthorized" }); return; }
  res.json({ user: { id: me.user.id, email: me.user.email }, workspaces: await cloudStore.listWorkspaces(me.user.id), devices: (await deviceStore.listDevices(me.user.id)).map((d) => ({ ...d })) });
});

app.use("/mcp", requireMcpAuth);
app.post("/mcp", async (req: Request, res: Response) => {
  try {
    const userId = String(res.locals.oauth?.sub ?? "");
    if (!userId) { res.status(401).json({ error: "unauthorized" }); return; }
    const sessionId = req.headers["mcp-session-id"] as string | undefined;
    let transport: StreamableHTTPServerTransport;
    if (sessionId && transports[sessionId]) {
      const record = transports[sessionId];
      if (record.userId !== userId) { res.status(403).json({ error: "session_user_mismatch" }); return; }
      transport = record.transport;
      log("debug", "mcp.session_request", { mcpSessionId: sessionId, userId, selectedWorkspaceKey: record.routing.selectedWorkspaceKey, method: req.body?.method ?? null });
    } else if (!sessionId && isInitializeRequest(req.body)) {
      const routing = createWorkspaceRoutingState();
      transport = new StreamableHTTPServerTransport({
        sessionIdGenerator: () => randomUUID(), enableJsonResponse: true,
        onsessioninitialized: (id) => { transports[id] = { transport, userId, routing }; log("info", "mcp.session_open", { mcpSessionId: id, userId, selectedWorkspaceKey: routing.selectedWorkspaceKey }); },
      });
      transport.onclose = () => { if (transport.sessionId) { const record = transports[transport.sessionId]; delete transports[transport.sessionId]; log("info", "mcp.session_close", { mcpSessionId: transport.sessionId, userId, selectedWorkspaceKey: record?.routing.selectedWorkspaceKey ?? null }); } };
      await createMcpServer(userId, routing).connect(transport);
    } else { res.status(400).json({ jsonrpc: "2.0", error: { code: -32000, message: "Invalid or missing MCP session." }, id: null }); return; }
    await transport.handleRequest(req, res, req.body);
  } catch (error) {
    log("error", "mcp.request_failed", { error });
    if (!res.headersSent) res.status(500).json({ jsonrpc: "2.0", error: { code: -32603, message: "Internal error" }, id: null });
  }
});
app.get("/mcp", async (req: Request, res: Response) => {
  const userId = String(res.locals.oauth?.sub ?? "");
  const sessionId = req.headers["mcp-session-id"] as string | undefined;
  const record = sessionId ? transports[sessionId] : undefined;
  if (!record || record.userId !== userId) { res.status(400).send("Invalid MCP session"); return; }
  await record.transport.handleRequest(req, res);
});
app.delete("/mcp", async (req: Request, res: Response) => {
  const userId = String(res.locals.oauth?.sub ?? "");
  const sessionId = req.headers["mcp-session-id"] as string | undefined;
  const record = sessionId ? transports[sessionId] : undefined;
  if (!record || record.userId !== userId) { res.status(400).send("Invalid MCP session"); return; }
  await record.transport.handleRequest(req, res);
});

const httpServer = http.createServer(app);
const wss = new WebSocketServer({ server: httpServer, path: "/client" });
wss.on("connection", (ws) => {
  let authenticated = false;
  const authTimer = setTimeout(() => { if (!authenticated) ws.close(4401, "registration timeout"); }, 10_000);
  ws.on("message", async (raw) => {
    let msg: any;
    try { msg = JSON.parse(raw.toString()); } catch { ws.close(4400, "invalid json"); return; }
    if (!authenticated) {
      if (msg.type !== "register") { ws.close(4401, "register first"); return; }
      const version = typeof msg.protocolVersion === "number" ? msg.protocolVersion : 1;
      if (!protocolCompatible(version)) { ws.close(4406, `CLIENT_UPGRADE_REQUIRED:${MIN_PROTOCOL_VERSION}-${PROTOCOL_VERSION}`); return; }
      let device: any = null;
      if (msg.credentialId && msg.credentialSecret) device = await deviceStore.authenticate(String(msg.credentialId), String(msg.credentialSecret));
      else if (ALLOW_LEGACY_DEVICE_TOKEN && msg.token && equalSecret(String(msg.token), DEVICE_TOKEN)) {
        ws.close(4403, "Legacy DEVICE_TOKEN cannot establish a multi-tenant SaaS identity. Pair this device through the website."); return;
      }
      if (!device?.userId) { ws.close(4403, "AUTH_FAILED"); return; }
      const deviceId = String(msg.deviceId || device.deviceId || "device");
      if (deviceId !== device.deviceId) { ws.close(4403, "DEVICE_ID_MISMATCH"); return; }
      const workspaceId = String(msg.workspaceId || "workspace");
      const key = clientKey(device.userId, deviceId, workspaceId);
      const existing = clients.get(key);
      if (existing && existing.ws !== ws) existing.ws.close(4001, "replaced by newer connection");
      const clientVersion = typeof msg.clientVersion === "string" && msg.clientVersion.trim() ? msg.clientVersion.trim().slice(0, 80) : undefined;
      const record: ClientRecord = {
        key, userId: device.userId, deviceId, deviceName: String(msg.deviceName ?? device.deviceName ?? deviceId), workspaceId,
        workspaceName: String(msg.workspaceName ?? workspaceId), projectRoot: typeof msg.projectRoot === "string" ? msg.projectRoot : undefined,
        credentialId: device.credentialId, protocolVersion: version, clientVersion, capabilities: msg.capabilities ?? {}, ws, connectedAt: Date.now(), lastSeenAt: Date.now(),
      };
      clients.set(key, record); socketKeys.set(ws, key); authenticated = true; clearTimeout(authTimer);
      const storedCapabilities = { ...record.capabilities, ...(record.clientVersion ? { clientVersion: record.clientVersion } : {}) };
      await cloudStore.upsertWorkspace({ userId: record.userId, deviceId: record.deviceId, workspaceId: record.workspaceId, workspaceName: record.workspaceName, protocolVersion: record.protocolVersion, capabilities: storedCapabilities });
      ws.send(JSON.stringify({ type: "registered", protocolVersion: PROTOCOL_VERSION, serverCapabilities: { cancellation: true, idempotency: true, pairing: true, multiWorkspace: true, lazyWorkspaceActivation: true, multiTenant: true, terminalChatApproval: true, clientUpdateNotices: true } }));
      log("info", "client.authenticated", { clientKey: key, userId: record.userId, protocolVersion: version, clientVersion: record.clientVersion ?? null, capabilities: record.capabilities });
      await cloudStore.audit(record.userId, "client.connected", { protocolVersion: version, clientVersion: record.clientVersion ?? null }, record.deviceId, record.workspaceId).catch(() => undefined);
      return;
    }
    const key = socketKeys.get(ws);
    const record = key ? clients.get(key) : undefined;
    if (record) record.lastSeenAt = Date.now();
    if (msg.type === "pong") {
      if (record) await cloudStore.touchWorkspace(record.userId, record.deviceId, record.workspaceId).catch(() => undefined);
      return;
    }
    if (msg.type !== "tool_result") return;
    const requestId = String(msg.requestId ?? msg.id ?? "");
    const wait = pending.get(requestId);
    if (!wait || !record || wait.userId !== record.userId) return;
    pending.delete(requestId); clearTimeout(wait.timer); wait.abortCleanup?.();
    const durationMs = Date.now() - wait.startedAt;
    if (msg.ok) {
      wait.resolve(msg.result);
      log("info", "tool.complete", { requestId, tool: wait.tool, clientKey: wait.clientKey, durationMs });
      if (isTerminalExecutionTool(wait.tool)) {
        await audit({ event: "gateway.terminal.executed", requestId, workspaceKey: wait.clientKey, tool: wait.tool });
        await cloudStore.audit(record.userId, "terminal.executed", { requestId, tool: wait.tool }, record.deviceId, record.workspaceId).catch(() => undefined);
      }
    }
    else { const message = String(msg.errorMessage ?? msg.error ?? "Client tool failed"); const error = new Error(message); (error as any).code = msg.errorCode; wait.reject(error); log("error", "tool.failed", { requestId, tool: wait.tool, clientKey: wait.clientKey, durationMs, errorCode: msg.errorCode, error: message }); }
  });
  ws.on("close", () => {
    clearTimeout(authTimer);
    const key = socketKeys.get(ws); socketKeys.delete(ws);
    const record = key ? clients.get(key) : undefined;
    if (record?.ws === ws) {
      clients.delete(key!);
      void cloudStore.clearPresence(record.userId, record.deviceId, record.workspaceId).catch(() => undefined);
      void cloudStore.audit(record.userId, "client.disconnected", {}, record.deviceId, record.workspaceId).catch(() => undefined);
    }
    for (const [requestId, wait] of pending) {
      if (wait.clientKey !== key) continue;
      clearTimeout(wait.timer); wait.abortCleanup?.(); pending.delete(requestId); wait.reject(new Error("Client disconnected during tool call."));
    }
    if (key) log("warn", "client.disconnected", { clientKey: key });
  });
  ws.on("error", (error) => log("error", "client.socket_error", { error }));
});

const heartbeat = setInterval(() => {
  const now = Date.now();
  for (const client of clients.values()) {
    if (now - client.lastSeenAt > STALE_MS) { client.ws.terminate(); continue; }
    if (client.ws.readyState === client.ws.OPEN) client.ws.send(JSON.stringify({ type: "ping", protocolVersion: PROTOCOL_VERSION, ts: now }));
  }
}, HEARTBEAT_MS);
heartbeat.unref?.();

const shutdown = async () => {
  clearInterval(heartbeat);
  for (const client of clients.values()) client.ws.close(1001, "server shutdown");
  await Promise.allSettled([runtimeActivationStore.close(), cloudStore.close()]);
};
process.once("SIGTERM", () => { void shutdown().finally(() => process.exit(0)); });
process.once("SIGINT", () => { void shutdown().finally(() => process.exit(0)); });

httpServer.listen(PORT, HOST, () => log("info", "server.started", { host: HOST, port: PORT, version: VERSION, protocolVersion: PROTOCOL_VERSION, cloud: true, lazyWorkspaceActivation: true, legacyDeviceTokenEnabled: ALLOW_LEGACY_DEVICE_TOKEN }));

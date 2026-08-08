import { EventEmitter } from "node:events";
import { promises as fs } from "node:fs";
import os from "node:os";
import path from "node:path";
import { spawn } from "node:child_process";
import WebSocket, { type RawData } from "ws";

export type BrowserBackend = "chromium-cdp";

export type BrowserTarget = {
  targetId: string;
  type: string;
  title: string;
  url: string;
  attached?: boolean;
};

export type BrowserAxNode = {
  nodeId: string;
  backendDOMNodeId?: number;
  role: string | null;
  name: string | null;
  description: string | null;
  ignored: boolean;
  interactive: boolean;
  disabled?: boolean;
  focused?: boolean;
  value?: string | number | boolean | null;
};

export type BrowserScreenshot = {
  __mcpImage: { mimeType: "image/png"; data: string };
  targetId: string;
  url: string;
  title: string;
};

export type ChromiumCandidate = {
  profileBase: string;
  browserFamily: string;
  port: number;
  browserWsUrl: string | null;
  discovery: "json-version" | "devtools-active-port" | "permission-blocked" | "unreachable";
  error?: string;
};

type Pending = {
  resolve: (value: any) => void;
  reject: (error: Error) => void;
  timer: NodeJS.Timeout;
};

const INTERNAL_URL_PREFIXES = [
  "chrome://",
  "chrome-untrusted://",
  "devtools://",
  "chrome-extension://",
  "edge://",
  "brave://",
  "about:",
];

const INTERACTIVE_ROLES = new Set([
  "button",
  "checkbox",
  "combobox",
  "gridcell",
  "link",
  "listbox",
  "menuitem",
  "menuitemcheckbox",
  "menuitemradio",
  "option",
  "radio",
  "searchbox",
  "slider",
  "spinbutton",
  "switch",
  "tab",
  "textbox",
  "treeitem",
]);

const MAC_PROFILE_BASES = [
  "Library/Application Support/Google/Chrome",
  "Library/Application Support/Google/Chrome Canary",
  "Library/Application Support/Microsoft Edge",
  "Library/Application Support/Microsoft Edge Beta",
  "Library/Application Support/Microsoft Edge Dev",
  "Library/Application Support/BraveSoftware/Brave-Browser",
  "Library/Application Support/Arc/User Data",
  "Library/Application Support/Dia/User Data",
  "Library/Application Support/Comet",
];

const LINUX_PROFILE_BASES = [
  ".config/google-chrome",
  ".config/chromium",
  ".config/chromium-browser",
  ".config/microsoft-edge",
  ".config/microsoft-edge-beta",
  ".config/microsoft-edge-dev",
  ".config/BraveSoftware/Brave-Browser",
  ".var/app/org.chromium.Chromium/config/chromium",
  ".var/app/com.google.Chrome/config/google-chrome",
  ".var/app/com.brave.Browser/config/BraveSoftware/Brave-Browser",
  ".var/app/com.microsoft.Edge/config/microsoft-edge",
];

const WINDOWS_PROFILE_BASES = [
  "Google/Chrome/User Data",
  "Google/Chrome SxS/User Data",
  "Google/Chrome Beta/User Data",
  "Google/Chrome Dev/User Data",
  "Chromium/User Data",
  "Microsoft/Edge/User Data",
  "Microsoft/Edge Beta/User Data",
  "Microsoft/Edge Dev/User Data",
  "Microsoft/Edge SxS/User Data",
  "BraveSoftware/Brave-Browser/User Data",
];

function browserFamilyFromPath(profileBase: string) {
  const lower = profileBase.toLowerCase();
  if (lower.includes("brave")) return "brave";
  if (lower.includes("edge")) return "edge";
  if (lower.includes("arc")) return "arc";
  if (lower.includes("dia")) return "dia";
  if (lower.includes("comet")) return "comet";
  if (lower.includes("chromium")) return "chromium";
  return "chrome";
}

export function chromiumProfileDirs(platform = process.platform, home = os.homedir(), localAppData = process.env.LOCALAPPDATA) {
  if (platform === "win32") {
    const base = localAppData || path.join(home, "AppData", "Local");
    return WINDOWS_PROFILE_BASES.map((item) => path.join(base, ...item.split("/")));
  }
  const items = platform === "darwin" ? MAC_PROFILE_BASES : LINUX_PROFILE_BASES;
  return items.map((item) => path.join(home, ...item.split("/")));
}

export function isBrowserInternalUrl(url: string) {
  return INTERNAL_URL_PREFIXES.some((prefix) => url.startsWith(prefix));
}

export function isLocalDevUrl(value: string) {
  try {
    const url = new URL(value);
    return ["localhost", "127.0.0.1", "::1", "0.0.0.0"].includes(url.hostname);
  } catch {
    return false;
  }
}

function validateNavigableUrl(value: string) {
  const url = new URL(value);
  if (!['http:', 'https:'].includes(url.protocol)) throw new Error(`Unsupported browser URL scheme: ${url.protocol}`);
  return url.toString();
}

async function fetchJson(url: string, timeoutMs = 1200) {
  const controller = new AbortController();
  const timer = setTimeout(() => controller.abort(), timeoutMs);
  try {
    const response = await fetch(url, { signal: controller.signal });
    return { response, body: await response.text() };
  } finally {
    clearTimeout(timer);
  }
}

async function resolveCandidate(profileBase: string): Promise<ChromiumCandidate | null> {
  let lines: string[];
  try {
    lines = (await fs.readFile(path.join(profileBase, "DevToolsActivePort"), "utf8")).split(/\r?\n/);
  } catch {
    return null;
  }

  const port = Number(lines[0]?.trim());
  const wsPath = lines[1]?.trim() ?? "";
  if (!Number.isInteger(port) || port <= 0) return null;
  const browserFamily = browserFamilyFromPath(profileBase);

  try {
    const { response, body } = await fetchJson(`http://127.0.0.1:${port}/json/version`);
    if (response.status === 403) {
      return { profileBase, browserFamily, port, browserWsUrl: null, discovery: "permission-blocked", error: "Chrome remote-debugging attach is waiting for user approval." };
    }
    if (response.ok) {
      const parsed = JSON.parse(body);
      if (typeof parsed.webSocketDebuggerUrl === "string") {
        return { profileBase, browserFamily, port, browserWsUrl: parsed.webSocketDebuggerUrl, discovery: "json-version" };
      }
    }
    if (response.status === 404 && wsPath) {
      return { profileBase, browserFamily, port, browserWsUrl: `ws://127.0.0.1:${port}${wsPath}`, discovery: "devtools-active-port" };
    }
    return { profileBase, browserFamily, port, browserWsUrl: wsPath ? `ws://127.0.0.1:${port}${wsPath}` : null, discovery: "unreachable", error: `DevTools discovery returned HTTP ${response.status}.` };
  } catch (error) {
    return { profileBase, browserFamily, port, browserWsUrl: wsPath ? `ws://127.0.0.1:${port}${wsPath}` : null, discovery: "unreachable", error: error instanceof Error ? error.message : String(error) };
  }
}

export async function discoverChromiumCandidates() {
  const out: ChromiumCandidate[] = [];
  for (const profileBase of chromiumProfileDirs()) {
    const candidate = await resolveCandidate(profileBase);
    if (candidate) out.push(candidate);
  }
  return out;
}

class CdpConnection extends EventEmitter {
  private ws: WebSocket | null = null;
  private sequence = 1;
  private pending = new Map<number, Pending>();

  constructor(public readonly url: string) {
    super();
  }

  async connect(timeoutMs = 45_000) {
    if (this.ws?.readyState === WebSocket.OPEN) return;
    const ws = new WebSocket(this.url, { handshakeTimeout: timeoutMs });
    this.ws = ws;
    ws.on("message", (raw: RawData) => this.onMessage(raw));
    ws.on("close", () => this.failAll(new Error("Browser CDP connection closed.")));
    ws.on("error", (error) => this.failAll(error instanceof Error ? error : new Error(String(error))));
    await new Promise<void>((resolve, reject) => {
      const timer = setTimeout(() => reject(new Error(`Browser CDP connection timed out after ${timeoutMs}ms.`)), timeoutMs + 1000);
      const cleanup = () => clearTimeout(timer);
      ws.once("open", () => { cleanup(); resolve(); });
      ws.once("error", (error) => { cleanup(); reject(error); });
    });
  }

  private onMessage(raw: RawData) {
    let message: any;
    try { message = JSON.parse(raw.toString()); } catch { return; }
    if (typeof message?.id === "number" && this.pending.has(message.id)) {
      const pending = this.pending.get(message.id)!;
      this.pending.delete(message.id);
      clearTimeout(pending.timer);
      if (message.error) pending.reject(new Error(message.error.message ?? "CDP request failed."));
      else pending.resolve(message.result);
      return;
    }
    if (message?.method) this.emit("event", message);
  }

  private failAll(error: Error) {
    for (const [id, pending] of this.pending) {
      clearTimeout(pending.timer);
      pending.reject(error);
      this.pending.delete(id);
    }
  }

  async request(method: string, params: Record<string, unknown> = {}, sessionId?: string, timeoutMs = 15_000): Promise<any> {
    await this.connect();
    if (!this.ws || this.ws.readyState !== WebSocket.OPEN) throw new Error("Browser CDP connection is not open.");
    const id = this.sequence++;
    const promise = new Promise<any>((resolve, reject) => {
      const timer = setTimeout(() => {
        this.pending.delete(id);
        reject(new Error(`CDP request timed out: ${method}`));
      }, timeoutMs);
      this.pending.set(id, { resolve, reject, timer });
    });
    this.ws.send(JSON.stringify({ id, method, params, ...(sessionId ? { sessionId } : {}) }));
    return promise;
  }

  close() {
    this.failAll(new Error("Browser CDP connection closed."));
    try { this.ws?.close(); } catch {}
    this.ws = null;
  }
}

function axValue(node: any, key: string) {
  return node?.[key]?.value ?? null;
}

function axProperty(node: any, name: string) {
  const item = Array.isArray(node?.properties) ? node.properties.find((property: any) => property?.name === name) : null;
  return item?.value?.value;
}

function normalizeAxNode(node: any): BrowserAxNode {
  const role = axValue(node, "role");
  const focusable = axProperty(node, "focusable") === true;
  const editable = axProperty(node, "editable") === true;
  return {
    nodeId: String(node.nodeId ?? ""),
    backendDOMNodeId: typeof node.backendDOMNodeId === "number" ? node.backendDOMNodeId : undefined,
    role: typeof role === "string" ? role : null,
    name: typeof axValue(node, "name") === "string" ? axValue(node, "name") : null,
    description: typeof axValue(node, "description") === "string" ? axValue(node, "description") : null,
    ignored: !!node.ignored,
    interactive: INTERACTIVE_ROLES.has(String(role ?? "").toLowerCase()) || focusable || editable,
    disabled: axProperty(node, "disabled") === true,
    focused: axProperty(node, "focused") === true,
    value: axValue(node, "value"),
  };
}

function quadCenter(quad: number[]) {
  if (!Array.isArray(quad) || quad.length < 8) throw new Error("Element does not have a usable box model.");
  const xs = quad.filter((_, index) => index % 2 === 0);
  const ys = quad.filter((_, index) => index % 2 === 1);
  return { x: xs.reduce((sum, value) => sum + value, 0) / xs.length, y: ys.reduce((sum, value) => sum + value, 0) / ys.length };
}

async function systemOpen(url: string) {
  const command = process.platform === "darwin" ? "/usr/bin/open" : process.platform === "win32" ? "cmd" : "xdg-open";
  const args = process.platform === "win32" ? ["/c", "start", "", url] : [url];
  return new Promise<void>((resolve, reject) => {
    const child = spawn(command, args, { detached: process.platform !== "win32", stdio: "ignore" });
    child.once("error", reject);
    child.once("spawn", () => { child.unref(); resolve(); });
  });
}

export class ChromiumBrowserControl {
  private connection: CdpConnection | null = null;
  private candidate: ChromiumCandidate | null = null;
  private currentTargetId: string | null = null;
  private sessions = new Map<string, string>();

  async status() {
    const candidates = await discoverChromiumCandidates();
    return {
      backend: "chromium-cdp" as const,
      connected: !!this.connection,
      currentTargetId: this.currentTargetId,
      candidate: this.candidate ? { browserFamily: this.candidate.browserFamily, port: this.candidate.port, discovery: this.candidate.discovery, profileBase: this.candidate.profileBase } : null,
      discovered: candidates.map((item) => ({ browserFamily: item.browserFamily, port: item.port, discovery: item.discovery, profileBase: item.profileBase, connectable: !!item.browserWsUrl })),
      setupRequired: !candidates.some((item) => !!item.browserWsUrl),
      setupHint: "For a running Chromium-family default browser, enable chrome://inspect/#remote-debugging and approve the attach popup when Chrome asks. CodeLocal keeps one CDP websocket open instead of reconnecting for every MCP call.",
    };
  }

  async connect() {
    if (this.connection) return this.status();
    const candidates = await discoverChromiumCandidates();
    const connectable = candidates.find((item) => item.browserWsUrl);
    if (!connectable?.browserWsUrl) {
      const blocked = candidates.find((item) => item.discovery === "permission-blocked");
      if (blocked) throw new Error("Browser attach blocked pending local Chrome approval. Click Allow in Chrome, then retry.");
      throw new Error("No attachable Chromium browser found. Enable chrome://inspect/#remote-debugging in the browser instance you want CodeLocal to control, then retry.");
    }
    const connection = new CdpConnection(connectable.browserWsUrl);
    await connection.connect();
    this.connection = connection;
    this.candidate = connectable;
    const tabs = await this.tabs();
    const first = tabs.find((tab) => tab.type === "page" && !isBrowserInternalUrl(tab.url));
    this.currentTargetId = first?.targetId ?? null;
    return this.status();
  }

  async openDefault(url: string) {
    const targetUrl = validateNavigableUrl(url);
    await systemOpen(targetUrl);
    await new Promise((resolve) => setTimeout(resolve, 500));
    await this.connect();
    const tabs = await this.tabs();
    const desired = tabs.find((tab) => tab.url === targetUrl) ?? tabs.find((tab) => {
      try { return new URL(tab.url).origin === new URL(targetUrl).origin; } catch { return false; }
    });
    if (desired) this.currentTargetId = desired.targetId;
    return { opened: targetUrl, currentTargetId: this.currentTargetId, tabs };
  }

  async tabs(): Promise<BrowserTarget[]> {
    if (!this.connection) await this.connect();
    const result = await this.connection!.request("Target.getTargets", { filter: [{ type: "page", exclude: false }] });
    return (result.targetInfos ?? []).map((target: any) => ({
      targetId: String(target.targetId),
      type: String(target.type ?? ""),
      title: String(target.title ?? ""),
      url: String(target.url ?? ""),
      attached: !!target.attached,
    }));
  }

  async selectTab(targetId: string) {
    const tabs = await this.tabs();
    const target = tabs.find((item) => item.targetId === targetId);
    if (!target) throw new Error(`Browser target not found: ${targetId}`);
    this.currentTargetId = targetId;
    await this.connection!.request("Target.activateTarget", { targetId });
    return target;
  }

  private async target() {
    const tabs = await this.tabs();
    const current = this.currentTargetId ? tabs.find((item) => item.targetId === this.currentTargetId) : null;
    const target = current ?? tabs.find((item) => item.type === "page" && !isBrowserInternalUrl(item.url));
    if (!target) throw new Error("No real browser tab is available.");
    this.currentTargetId = target.targetId;
    return target;
  }

  private async session(targetId: string) {
    const existing = this.sessions.get(targetId);
    if (existing) return existing;
    if (!this.connection) await this.connect();
    const result = await this.connection!.request("Target.attachToTarget", { targetId, flatten: true });
    const sessionId = String(result.sessionId ?? "");
    if (!sessionId) throw new Error("Chrome did not return a CDP session id.");
    this.sessions.set(targetId, sessionId);
    return sessionId;
  }

  async snapshot(options: { interactiveOnly?: boolean; limit?: number } = {}) {
    const target = await this.target();
    const sessionId = await this.session(target.targetId);
    await this.connection!.request("Accessibility.enable", {}, sessionId);
    const result = await this.connection!.request("Accessibility.getFullAXTree", {}, sessionId, 20_000);
    const interactiveOnly = options.interactiveOnly !== false;
    const limit = Math.max(1, Math.min(options.limit ?? 300, 2000));
    const nodes = (result.nodes ?? []).map(normalizeAxNode).filter((node: BrowserAxNode) => !node.ignored && (!interactiveOnly || node.interactive)).slice(0, limit);
    return { target, interactiveOnly, nodes, truncated: (result.nodes?.length ?? 0) > nodes.length };
  }

  async screenshot(): Promise<BrowserScreenshot> {
    const target = await this.target();
    const sessionId = await this.session(target.targetId);
    await this.connection!.request("Page.enable", {}, sessionId);
    const result = await this.connection!.request("Page.captureScreenshot", { format: "png", fromSurface: true, captureBeyondViewport: false }, sessionId, 20_000);
    if (typeof result.data !== "string") throw new Error("Chrome did not return screenshot data.");
    return { __mcpImage: { mimeType: "image/png", data: result.data }, targetId: target.targetId, url: target.url, title: target.title };
  }

  async click(input: { backendDOMNodeId?: number; x?: number; y?: number }) {
    const target = await this.target();
    const sessionId = await this.session(target.targetId);
    let x = input.x;
    let y = input.y;
    if (input.backendDOMNodeId != null) {
      const box = await this.connection!.request("DOM.getBoxModel", { backendNodeId: input.backendDOMNodeId }, sessionId);
      const center = quadCenter(box.model?.content ?? box.model?.border ?? []);
      x = center.x;
      y = center.y;
    }
    if (!Number.isFinite(x) || !Number.isFinite(y)) throw new Error("browser_click requires backendDOMNodeId or numeric x/y coordinates.");
    await this.connection!.request("Input.dispatchMouseEvent", { type: "mouseMoved", x, y }, sessionId);
    await this.connection!.request("Input.dispatchMouseEvent", { type: "mousePressed", x, y, button: "left", clickCount: 1 }, sessionId);
    await this.connection!.request("Input.dispatchMouseEvent", { type: "mouseReleased", x, y, button: "left", clickCount: 1 }, sessionId);
    return { targetId: target.targetId, x, y };
  }

  async typeText(input: { text: string; backendDOMNodeId?: number }) {
    const target = await this.target();
    const sessionId = await this.session(target.targetId);
    if (input.backendDOMNodeId != null) await this.connection!.request("DOM.focus", { backendNodeId: input.backendDOMNodeId }, sessionId);
    await this.connection!.request("Input.insertText", { text: input.text }, sessionId);
    return { targetId: target.targetId, characters: [...input.text].length, focusedBackendDOMNodeId: input.backendDOMNodeId ?? null };
  }

  async navigate(url: string) {
    const targetUrl = validateNavigableUrl(url);
    const target = await this.target();
    const sessionId = await this.session(target.targetId);
    await this.connection!.request("Page.enable", {}, sessionId);
    const result = await this.connection!.request("Page.navigate", { url: targetUrl }, sessionId, 20_000);
    return { targetId: target.targetId, url: targetUrl, loaderId: result.loaderId ?? null, errorText: result.errorText ?? null };
  }

  close() {
    this.sessions.clear();
    this.currentTargetId = null;
    this.connection?.close();
    this.connection = null;
    this.candidate = null;
  }
}

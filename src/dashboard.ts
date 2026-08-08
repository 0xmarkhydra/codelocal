import express from "express";
import { cloudStore } from "./cloud-store.js";
import { requireWebUser, type WebIdentity, verifyCsrf } from "./saas-auth.js";
import { dashboardPage, escapeHtml, formatTime } from "./web-ui.js";

const PUBLIC_BASE_URL = (process.env.PUBLIC_BASE_URL ?? "").replace(/\/$/, "");

type DashboardHooks = {
  onDeviceRevoked?: (userId: string, credentialId: string) => void | Promise<void>;
  onMcpChanged?: (userId: string, workspaceId?: string) => void | Promise<void>;
};

function identity(res: express.Response) {
  return res.locals.webIdentity as WebIdentity;
}

function shell(res: express.Response, options: Parameters<typeof dashboardPage>[0]) {
  res.type("html").send(dashboardPage(options));
}

function parseArgs(value: string) {
  if (!value.trim()) return [];
  return value.split(/\r?\n/).map((line) => line.trim()).filter(Boolean);
}

function parseSecrets(value: string) {
  return [...new Set(value.split(/[\s,]+/).map((item) => item.trim()).filter(Boolean))].filter((name) => /^[A-Za-z_][A-Za-z0-9_]*$/.test(name)).slice(0, 30);
}

function validMcpName(value: string) {
  return /^[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}$/.test(value);
}

function validateRemoteUrl(value: string) {
  const url = new URL(value);
  const host = url.hostname.replace(/^\[/, "").replace(/\]$/, "");
  const local = ["localhost", "127.0.0.1", "::1"].includes(host);
  if (url.protocol !== "https:" && !(url.protocol === "http:" && local)) throw new Error("Remote MCP URLs must use HTTPS (HTTP is allowed only for localhost)." );
  return url.toString();
}

function csrfGuard(req: express.Request, res: express.Response, next: express.NextFunction) {
  if (!verifyCsrf(req)) { res.status(403).send("Invalid security token. Reload the page and try again."); return; }
  next();
}

export function createDashboardRouter(hooks: DashboardHooks = {}) {
  const router = express.Router();
  router.use(express.urlencoded({ extended: false, limit: "128kb" }));
  router.use("/dashboard", requireWebUser);

  router.get("/dashboard", async (_req, res) => {
    const me = identity(res);
    const [devices, workspaces, mcps, audit] = await Promise.all([
      cloudStore.listDevices(me.user.id),
      cloudStore.listWorkspaces(me.user.id),
      cloudStore.allMcpInstallations(me.user.id),
      cloudStore.recentAudit(me.user.id, 8),
    ]);
    const online = workspaces.filter((workspace) => workspace.online).length;
    shell(res, {
      title: "Overview", active: "overview", email: me.user.email, csrf: me.csrf,
      subtitle: "Your CodeLocal account connects ChatGPT to the development environments you explicitly pair.",
      actions: `<a class="btn primary" href="/dashboard/connect">Connect ChatGPT</a>`,
      body: `<div class="grid">
        <div class="card span3"><div class="label">Devices</div><div class="metric">${devices.filter((d) => !d.revokedAt).length}</div></div>
        <div class="card span3"><div class="label">Online workspaces</div><div class="metric">${online}</div></div>
        <div class="card span3"><div class="label">MCP extensions</div><div class="metric">${mcps.filter((m) => m.enabled).length}</div></div>
        <div class="card span3"><div class="label">Account</div><div class="metric" style="font-size:17px;margin-top:15px">Development</div></div>
        <div class="card span8"><div class="title">Recent workspaces</div><div class="label">Only metadata lives in CodeLocal Cloud. Source code remains on your machine.</div><div class="divider"></div>${workspaces.slice(0, 5).map((w) => `<div class="row"><div class="row-main"><div class="row-title">${escapeHtml(w.workspaceName)}</div><div class="row-meta">${escapeHtml(w.deviceId)} · ${escapeHtml(w.projectRoot ?? "Local workspace")} · ${formatTime(w.lastSeenAt)}</div></div><span class="badge ${w.online ? "green" : ""}">${w.online ? "Online" : "Offline"}</span></div>`).join("") || `<div class="empty">Run <code>codelocal .</code> in a project to register your first workspace.</div>`}</div>
        <div class="card span4"><div class="title">Recent security activity</div><div class="divider"></div>${audit.map((item) => `<div class="row"><div class="row-main"><div class="row-title">${escapeHtml(item.event)}</div><div class="row-meta">${formatTime(item.createdAt)}</div></div></div>`).join("") || `<div class="empty">No activity yet.</div>`}</div>
      </div>`,
    });
  });

  router.get("/dashboard/devices", async (_req, res) => {
    const me = identity(res);
    const devices = await cloudStore.listDevices(me.user.id);
    shell(res, {
      title: "Devices", active: "devices", email: me.user.email, csrf: me.csrf,
      subtitle: "A device credential authorizes one local CodeLocal runtime. Revoke anything you no longer recognize.",
      body: `<div class="card">${devices.map((d) => `<div class="row"><div class="row-main"><div class="row-title">${escapeHtml(d.deviceName)}</div><div class="row-meta mono">${escapeHtml(d.deviceId)} · paired ${formatTime(d.createdAt)} · last seen ${formatTime(d.lastSeenAt)}</div></div><div class="actions"><span class="badge ${d.revokedAt ? "red" : "green"}">${d.revokedAt ? "Revoked" : "Active"}</span>${d.revokedAt ? "" : `<form method="post" action="/dashboard/devices/${encodeURIComponent(d.credentialId)}/revoke"><input type="hidden" name="csrf" value="${escapeHtml(me.csrf)}"><button class="btn danger small" type="submit">Revoke</button></form>`}</div></div>`).join("") || `<div class="empty">No paired devices yet.</div>`}</div>`,
    });
  });

  router.post("/dashboard/devices/:credentialId/revoke", csrfGuard, async (req, res) => {
    const me = identity(res);
    const credentialId = String(req.params.credentialId ?? "");
    if (await cloudStore.revokeDevice(me.user.id, credentialId)) {
      await cloudStore.audit(me.user.id, "device.revoked", { credentialId });
      await hooks.onDeviceRevoked?.(me.user.id, credentialId);
    }
    res.redirect(303, "/dashboard/devices");
  });

  router.get("/dashboard/workspaces", async (_req, res) => {
    const me = identity(res);
    const workspaces = await cloudStore.listWorkspaces(me.user.id);
    shell(res, {
      title: "Workspaces", active: "workspaces", email: me.user.email, csrf: me.csrf,
      subtitle: "Workspaces are local project roots currently or previously connected through your devices.",
      body: `<div class="card">${workspaces.map((w) => `<div class="row"><div class="row-main"><div class="row-title">${escapeHtml(w.workspaceName)}</div><div class="row-meta"><span class="mono">${escapeHtml(w.workspaceId)}</span> · ${escapeHtml(w.projectRoot ?? "Local path hidden")}</div><div class="row-meta">Device ${escapeHtml(w.deviceId)} · last seen ${formatTime(w.lastSeenAt)}</div></div><span class="badge ${w.online ? "green" : ""}">${w.online ? "Online" : "Offline"}</span></div>`).join("") || `<div class="empty">No workspace has connected yet.</div>`}</div>`,
    });
  });

  router.get("/dashboard/mcp", async (req, res) => {
    const me = identity(res);
    const [installations, workspaces] = await Promise.all([cloudStore.allMcpInstallations(me.user.id), cloudStore.listWorkspaces(me.user.id)]);
    const notice = typeof req.query.ok === "string" ? `<div class="alert success">${escapeHtml(req.query.ok)}</div><div style="height:16px"></div>` : "";
    const error = typeof req.query.error === "string" ? `<div class="alert">${escapeHtml(req.query.error)}</div><div style="height:16px"></div>` : "";
    shell(res, {
      title: "MCP Extensions", active: "mcp", email: me.user.email, csrf: me.csrf,
      subtitle: "Install capabilities once in CodeLocal. ChatGPT still sees only the stable MCP Hub router tools instead of every extension tool.",
      body: `${notice}${error}<div class="grid"><div class="card span7"><div class="title">Installed</div><div class="label">Secrets are never entered here. The cloud stores only required environment-variable names.</div><div class="divider"></div>${installations.map((m) => `<div class="row"><div class="row-main"><div class="row-title">${escapeHtml(m.name)}</div><div class="row-meta">${escapeHtml(m.transport)} · ${escapeHtml(m.scope)}${m.workspaceId ? ` · ${escapeHtml(m.workspaceId)}` : ""} · secrets: ${escapeHtml(m.requiredSecrets.join(", ") || "none")}</div></div><div class="actions"><span class="badge blue">${m.enabled ? "Enabled" : "Disabled"}</span><form method="post" action="/dashboard/mcp/${encodeURIComponent(m.id)}/remove"><input type="hidden" name="csrf" value="${escapeHtml(me.csrf)}"><button class="btn danger small" type="submit">Remove</button></form></div></div>`).join("") || `<div class="empty">No MCP extensions installed yet.</div>`}</div>
      <div class="card span5"><div class="title">Install MCP</div><div class="label">MVP supports stdio and Streamable HTTP. Commands are spawned locally with <span class="mono">shell:false</span>.</div><div class="divider"></div><form class="form" method="post" action="/dashboard/mcp/install"><input type="hidden" name="csrf" value="${escapeHtml(me.csrf)}">
        <div class="field"><label>Name</label><input class="input" name="name" placeholder="github" pattern="[a-zA-Z0-9][a-zA-Z0-9._-]{0,63}" required></div>
        <div class="field"><label>Transport</label><select class="select" name="transport"><option value="stdio">stdio (local process)</option><option value="http">Streamable HTTP</option></select></div>
        <div class="field"><label>Scope</label><select class="select" name="scope"><option value="global">All my workspaces</option><option value="workspace">One workspace</option></select></div>
        <div class="field"><label>Workspace for workspace scope</label><select class="select" name="workspaceId"><option value="">Select later</option>${workspaces.map((w) => `<option value="${escapeHtml(w.workspaceId)}">${escapeHtml(w.workspaceName)} · ${escapeHtml(w.workspaceId)}</option>`).join("")}</select></div>
        <div class="field"><label>Command (stdio)</label><input class="input mono" name="command" placeholder="npx"></div>
        <div class="field"><label>Arguments — one per line (stdio)</label><textarea class="textarea mono" name="args" placeholder="-y&#10;@modelcontextprotocol/server-example"></textarea></div>
        <div class="field"><label>URL (HTTP)</label><input class="input mono" name="url" placeholder="https://example.com/mcp"></div>
        <div class="field"><label>Required local secrets</label><input class="input mono" name="requiredSecrets" placeholder="GITHUB_TOKEN, DATABASE_URL"><div class="hint">Only these variable names are stored in Cloud. Values stay on the user's machine.</div></div>
        <button class="btn primary" type="submit">Install extension</button></form></div></div>`,
    });
  });

  router.post("/dashboard/mcp/install", csrfGuard, async (req, res) => {
    const me = identity(res);
    try {
      const name = String(req.body?.name ?? "").trim();
      const transport = String(req.body?.transport ?? "stdio") as "stdio" | "http";
      const scope = String(req.body?.scope ?? "global") as "global" | "workspace";
      const workspaceId = String(req.body?.workspaceId ?? "").trim() || undefined;
      if (!validMcpName(name)) throw new Error("Invalid MCP name.");
      if (!(["stdio", "http"] as string[]).includes(transport)) throw new Error("Invalid transport.");
      if (!(["global", "workspace"] as string[]).includes(scope)) throw new Error("Invalid scope.");
      if (scope === "workspace" && !workspaceId) throw new Error("Choose a workspace for workspace-scoped MCPs.");
      const requiredSecrets = parseSecrets(String(req.body?.requiredSecrets ?? ""));
      let config: Record<string, unknown>;
      if (transport === "stdio") {
        const command = String(req.body?.command ?? "").trim();
        if (!command || /[\r\n\0]/.test(command)) throw new Error("stdio MCP requires one executable command.");
        config = { command, args: parseArgs(String(req.body?.args ?? "")) };
      } else {
        const url = validateRemoteUrl(String(req.body?.url ?? "").trim());
        config = { url };
      }
      const installed = await cloudStore.upsertMcpInstallation({ userId: me.user.id, name, enabled: true, scope, workspaceId, transport, config, requiredSecrets });
      await cloudStore.audit(me.user.id, "mcp.installed", { id: installed.id, name, scope, workspaceId, transport });
      await hooks.onMcpChanged?.(me.user.id, workspaceId);
      res.redirect(303, `/dashboard/mcp?ok=${encodeURIComponent(`${name} installed. Restart codelocal . to sync this extension safely.`)}`);
    } catch (error) {
      res.redirect(303, `/dashboard/mcp?error=${encodeURIComponent(error instanceof Error ? error.message : "Unable to install MCP.")}`);
    }
  });

  router.post("/dashboard/mcp/:id/remove", csrfGuard, async (req, res) => {
    const me = identity(res);
    const id = String(req.params.id ?? "");
    const current = (await cloudStore.allMcpInstallations(me.user.id)).find((item) => item.id === id);
    if (await cloudStore.removeMcpInstallation(me.user.id, id)) {
      await cloudStore.audit(me.user.id, "mcp.removed", { id, name: current?.name });
      await hooks.onMcpChanged?.(me.user.id, current?.workspaceId);
    }
    res.redirect(303, "/dashboard/mcp");
  });

  router.get("/dashboard/connect", async (_req, res) => {
    const me = identity(res);
    const endpoint = `${PUBLIC_BASE_URL}/mcp`;
    shell(res, {
      title: "Connect ChatGPT", active: "connect", email: me.user.email, csrf: me.csrf,
      subtitle: "Connect CodeLocal once. Your account controls which paired devices and workspaces ChatGPT can reach.",
      body: `<div class="grid"><div class="card span7"><div class="title">ChatGPT MCP endpoint</div><div class="divider"></div><div class="code-block">${escapeHtml(endpoint)}</div><div style="height:14px"></div><div class="label">When ChatGPT opens OAuth, sign in with this CodeLocal account and approve access. The OAuth token is bound to your user ID.</div></div><div class="card span5"><div class="title">Then connect a project</div><div class="divider"></div><div class="code-block">npm install -g codelocal

cd ~/your-project
codelocal .</div><div style="height:14px"></div><div class="label">On a new machine, CodeLocal opens a pairing URL. Sign in here, approve the device, then the workspace becomes available to ChatGPT.</div></div></div>`,
    });
  });

  router.get("/dashboard/security", async (_req, res) => {
    const me = identity(res);
    const audit = await cloudStore.recentAudit(me.user.id, 100);
    shell(res, {
      title: "Security", active: "security", email: me.user.email, csrf: me.csrf,
      subtitle: "Account-scoped audit metadata. Source code, command output and MCP secret values are not stored in these records.",
      body: `<div class="card"><div class="title">Activity</div><div class="divider"></div>${audit.map((item) => `<div class="row"><div class="row-main"><div class="row-title">${escapeHtml(item.event)}</div><div class="row-meta">${formatTime(item.createdAt)}${item.deviceId ? ` · device ${escapeHtml(item.deviceId)}` : ""}${item.workspaceId ? ` · workspace ${escapeHtml(item.workspaceId)}` : ""}</div></div></div>`).join("") || `<div class="empty">No audit events yet.</div>`}</div>`,
    });
  });

  return router;
}

import express from "express";
import { fileURLToPath } from "node:url";
import { cloudStore, type CloudWorkspace } from "./cloud-store.js";
import { requireWebUser, type WebIdentity, verifyCsrf } from "./saas-auth.js";
import { dashboardPage, escapeHtml, formatTime, uiIcons } from "./web-ui.js";

const PUBLIC_BASE_URL = (process.env.PUBLIC_BASE_URL ?? "").replace(/\/$/, "");
const brandAsset = (name: string) => fileURLToPath(new URL(`../assets/${name}`, import.meta.url));

type DashboardHooks = {
  onDeviceRevoked?: (userId: string, credentialId: string) => void | Promise<void>;
  onWorkspaceRemoveRequested?: (userId: string, deviceId: string, workspaceId: string) => void | Promise<void>;
  isDeviceOnline?: (userId: string, deviceId: string) => boolean | Promise<boolean>;
};

function identity(res: express.Response) {
  return res.locals.webIdentity as WebIdentity;
}

function shell(res: express.Response, options: Parameters<typeof dashboardPage>[0]) {
  res.type("html").send(dashboardPage(options));
}

function csrfGuard(req: express.Request, res: express.Response, next: express.NextFunction) {
  if (!verifyCsrf(req)) { res.status(403).send("Invalid security token. Reload the page and try again."); return; }
  next();
}

function flash(req: express.Request) {
  const ok = typeof req.query.ok === "string" ? `<div class="alert success">${escapeHtml(req.query.ok)}</div>` : "";
  const error = typeof req.query.error === "string" ? `<div class="alert">${escapeHtml(req.query.error)}</div>` : "";
  return `${ok}${error}`;
}

function workspaceState(workspace: CloudWorkspace & { online: boolean }, runtimeOnline: boolean) {
  if (workspace.online) return { label: "Active", badge: "green" };
  if (runtimeOnline) return { label: "Sleeping", badge: "blue" };
  return { label: "Device offline", badge: "muted" };
}

async function onlineDeviceMap(userId: string, deviceIds: string[], hooks: DashboardHooks) {
  const unique = [...new Set(deviceIds)];
  const values = await Promise.all(unique.map(async (deviceId) => [deviceId, await hooks.isDeviceOnline?.(userId, deviceId) === true] as const));
  return new Map(values);
}

export function createDashboardRouter(hooks: DashboardHooks = {}) {
  const router = express.Router();
  router.get("/assets/codelocal-icon.png", (_req, res) => {
    res.setHeader("Cache-Control", "public, max-age=604800, immutable");
    res.sendFile(brandAsset("codelocal-icon.png"));
  });
  router.get("/assets/chatgpt-plugin-icon.png", (_req, res) => {
    res.setHeader("Cache-Control", "public, max-age=604800, immutable");
    res.sendFile(brandAsset("chatgpt-plugin-icon.png"));
  });
  router.get("/assets/apple-touch-icon.png", (_req, res) => {
    res.setHeader("Cache-Control", "public, max-age=604800, immutable");
    res.sendFile(brandAsset("apple-touch-icon.png"));
  });
  router.get("/favicon.ico", (_req, res) => {
    res.setHeader("Cache-Control", "public, max-age=604800, immutable");
    res.sendFile(brandAsset("favicon.ico"));
  });
  router.use(express.urlencoded({ extended: false, limit: "128kb" }));
  router.use("/dashboard", requireWebUser);

  router.get("/dashboard", async (_req, res) => {
    const me = identity(res);
    const [devices, workspaces] = await Promise.all([
      cloudStore.listDevices(me.user.id),
      cloudStore.listWorkspaces(me.user.id),
    ]);
    const deviceOnline = await onlineDeviceMap(me.user.id, devices.filter((device) => !device.revokedAt).map((device) => device.deviceId), hooks);
    const activeWorkspaces = workspaces.filter((workspace) => workspace.online).length;
    const sleepingWorkspaces = workspaces.filter((workspace) => !workspace.online && deviceOnline.get(workspace.deviceId)).length;
    const onlineDevices = [...deviceOnline.values()].filter(Boolean).length;
    shell(res, {
      title: "Overview", active: "overview", email: me.user.email, csrf: me.csrf,
      subtitle: "A private control plane for the local machines and project folders you explicitly authorize.",
      actions: `<a class="btn primary" href="/dashboard/connect"><span class="btn-icon">↗</span>Connect ChatGPT</a>`,
      body: `<div class="grid">
        <div class="card metric-card span4"><div class="metric-label">Machine runtimes</div><div class="metric">${onlineDevices}</div><div class="metric-sub">${devices.filter((d) => !d.revokedAt).length} paired device${devices.filter((d) => !d.revokedAt).length === 1 ? "" : "s"}</div></div>
        <div class="card metric-card span4"><div class="metric-label">Active workspaces</div><div class="metric">${activeWorkspaces}</div><div class="metric-sub">Loaded for a ChatGPT session</div></div>
        <div class="card metric-card span4"><div class="metric-label">Sleeping workspaces</div><div class="metric">${sleepingWorkspaces}</div><div class="metric-sub">Authorized, zero heavy runtime</div></div>
        <div class="card span12"><div class="section-head"><div><div class="title">Workspaces</div><div class="label">Only folders granted by you are visible here.</div></div><a class="btn small" href="/dashboard/workspaces">View all</a></div><div class="divider"></div><div class="list">${workspaces.slice(0, 5).map((workspace) => {
          const state = workspaceState(workspace, deviceOnline.get(workspace.deviceId) === true);
          return `<div class="row"><div class="entity"><div class="entity-icon">${uiIcons.folder}</div><div class="entity-copy"><div class="row-title"><span class="row-title-text">${escapeHtml(workspace.workspaceName)}</span></div><div class="row-meta">${escapeHtml(workspace.deviceId)} · ${formatTime(workspace.lastSeenAt)}</div></div></div><span class="badge ${state.badge}">${state.label}</span></div>`;
        }).join("") || `<div class="empty"><div class="empty-icon">${uiIcons.folder}</div>No workspace yet.<br><span class="muted">Run <code>codelocal .</code> once inside a project.</span></div>`}</div></div>
      </div>`,
    });
  });

  router.get("/dashboard/devices", async (req, res) => {
    const me = identity(res);
    const devices = await cloudStore.listDevices(me.user.id);
    const deviceOnline = await onlineDeviceMap(me.user.id, devices.filter((device) => !device.revokedAt).map((device) => device.deviceId), hooks);
    shell(res, {
      title: "Devices", active: "devices", email: me.user.email, csrf: me.csrf,
      subtitle: "A paired device credential lets one CodeLocal machine runtime connect to your account. Revoke anything you no longer trust.",
      body: `${flash(req)}<div class="card"><div class="section-head"><div><div class="title">Paired machines</div><div class="label">Online means the lightweight <code>codelocal</code> runtime is reachable now.</div></div></div><div class="divider"></div><div class="list">${devices.map((device) => {
        const runtimeOnline = !device.revokedAt && deviceOnline.get(device.deviceId) === true;
        const badge = device.revokedAt ? "red" : runtimeOnline ? "green" : "muted";
        const status = device.revokedAt ? "Revoked" : runtimeOnline ? "Online" : "Offline";
        return `<div class="row"><div class="entity"><div class="entity-icon">◉</div><div class="entity-copy"><div class="row-title"><span class="row-title-text">${escapeHtml(device.deviceName)}</span></div><div class="row-meta mono">${escapeHtml(device.deviceId)}</div><div class="row-meta">Paired ${formatTime(device.createdAt)} · last seen ${formatTime(device.lastSeenAt)}</div></div></div><div class="actions"><span class="badge ${badge}">${status}</span>${device.revokedAt ? "" : `<form method="post" action="/dashboard/devices/${encodeURIComponent(device.credentialId)}/revoke"><input type="hidden" name="csrf" value="${escapeHtml(me.csrf)}"><button class="btn danger small" type="submit" data-confirm data-confirm-tone="danger" data-confirm-title="Revoke this device?" data-confirm-message="This immediately disconnects the machine and prevents its credential from accessing CodeLocal Cloud. Local project files are not deleted." data-confirm-label="Revoke device">Revoke</button></form>`}</div></div>`;
      }).join("") || `<div class="empty"><div class="empty-icon">◉</div>No paired devices yet.</div>`}</div></div>`,
    });
  });

  router.post("/dashboard/devices/:credentialId/revoke", csrfGuard, async (req, res) => {
    const me = identity(res);
    const credentialId = String(req.params.credentialId ?? "");
    if (hooks.onDeviceRevoked) await hooks.onDeviceRevoked(me.user.id, credentialId);
    else if (await cloudStore.revokeDevice(me.user.id, credentialId)) await cloudStore.audit(me.user.id, "device.revoked", { credentialId });
    res.redirect(303, "/dashboard/devices?ok=Device%20access%20revoked.");
  });

  router.get("/dashboard/workspaces", async (req, res) => {
    const me = identity(res);
    const workspaces = await cloudStore.listWorkspaces(me.user.id);
    const deviceOnline = await onlineDeviceMap(me.user.id, workspaces.map((workspace) => workspace.deviceId), hooks);
    shell(res, {
      title: "Workspaces", active: "workspaces", email: me.user.email, csrf: me.csrf,
      subtitle: "A workspace is a local project folder you granted once. Sleeping workspaces consume no heavy project runtime until ChatGPT selects them.",
      body: `${flash(req)}<div class="card"><div class="section-head"><div><div class="title">Authorized folders</div><div class="label">Remove access without deleting or modifying the project folder itself.</div></div><div class="badge blue">${workspaces.length} authorized</div></div><div class="divider"></div><div class="list">${workspaces.map((workspace) => {
        const runtimeOnline = deviceOnline.get(workspace.deviceId) === true;
        const state = workspaceState(workspace, runtimeOnline);
        const removeButton = runtimeOnline
          ? `<form method="post" action="/dashboard/workspaces/${encodeURIComponent(workspace.deviceId)}/${encodeURIComponent(workspace.workspaceId)}/remove"><input type="hidden" name="csrf" value="${escapeHtml(me.csrf)}"><button class="btn danger small" type="submit" data-confirm data-confirm-tone="danger" data-confirm-title="Remove ${escapeHtml(workspace.workspaceName)}?" data-confirm-message="CodeLocal will revoke this folder from the local machine. The project and every file inside it stay untouched. You can authorize it again later with codelocal ." data-confirm-label="Remove workspace">Remove</button></form>`
          : `<button class="btn danger small" type="button" disabled title="Start codelocal on this device to remove local authorization">Remove</button>`;
        return `<div class="row"><div class="entity"><div class="entity-icon">${uiIcons.folder}</div><div class="entity-copy"><div class="row-title"><span class="row-title-text">${escapeHtml(workspace.workspaceName)}</span></div><div class="row-meta mono">${escapeHtml(workspace.workspaceId)}</div><div class="row-meta">Device ${escapeHtml(workspace.deviceId)} · last seen ${formatTime(workspace.lastSeenAt)}</div></div></div><div class="actions"><span class="badge ${state.badge}">${state.label}</span>${removeButton}</div></div>`;
      }).join("") || `<div class="empty"><div class="empty-icon">${uiIcons.folder}</div>No authorized workspace has synced yet.<br><span class="muted">Open a project on your machine and run <code>codelocal .</code> once.</span></div>`}</div></div>`,
    });
  });

  router.post("/dashboard/workspaces/:deviceId/:workspaceId/remove", csrfGuard, async (req, res) => {
    const me = identity(res);
    const deviceId = String(req.params.deviceId ?? "");
    const workspaceId = String(req.params.workspaceId ?? "");
    try {
      const workspaces = await cloudStore.listWorkspaces(me.user.id);
      const workspace = workspaces.find((item) => item.deviceId === deviceId && item.workspaceId === workspaceId);
      if (!workspace) throw new Error("Workspace not found.");
      if (await hooks.isDeviceOnline?.(me.user.id, deviceId) !== true) throw new Error("That device is offline. Start codelocal on it before removing local authorization.");
      await hooks.onWorkspaceRemoveRequested?.(me.user.id, deviceId, workspaceId);
      await cloudStore.audit(me.user.id, "workspace.revocation_requested", { workspaceName: workspace.workspaceName }, deviceId, workspaceId);
      res.redirect(303, `/dashboard/workspaces?ok=${encodeURIComponent(`${workspace.workspaceName} removal requested. The local runtime will revoke it now.`)}`);
    } catch (error) {
      res.redirect(303, `/dashboard/workspaces?error=${encodeURIComponent(error instanceof Error ? error.message : "Unable to remove workspace.")}`);
    }
  });

  router.get("/dashboard/connect", async (_req, res) => {
    const me = identity(res);
    const endpoint = `${PUBLIC_BASE_URL}/mcp`;
    shell(res, {
      title: "Connect ChatGPT", active: "connect", email: me.user.email, csrf: me.csrf,
      subtitle: "Connect the CodeLocal Cloud MCP once. Workspace choice remains scoped to each ChatGPT MCP session.",
      actions: `<a class="btn" href="/#setup">View full setup guide</a>`,
      body: `<div class="grid"><div class="card span7 glow"><div class="section-head"><div><div class="title">ChatGPT MCP endpoint</div><div class="label">Use this remote MCP URL when adding CodeLocal to ChatGPT.</div></div></div><div class="divider"></div><div class="copy-row"><div class="code-block" id="mcp-endpoint">${escapeHtml(endpoint)}</div><button class="btn" type="button" data-copy-target="#mcp-endpoint">Copy</button></div><div class="divider"></div><div class="label">Authentication: <strong>OAuth</strong>. Sign in with this CodeLocal account and approve the connection when ChatGPT opens the browser.</div></div><div class="card span5"><div class="section-head"><div><div class="title">ChatGPT plugin icon</div><div class="label">256 × 256 PNG · under 10 KB.</div></div><img src="/assets/chatgpt-plugin-icon.png" alt="" style="width:56px;height:56px;border-radius:16px"></div><div class="divider"></div><a class="btn primary" href="/assets/chatgpt-plugin-icon.png" download="codelocal-chatgpt-plugin-icon.png">Download icon</a><div style="height:12px"></div><div class="label">The public setup guide includes every field to enter in ChatGPT and the complete machine pairing flow.</div></div></div>`,
    });
  });

  return router;
}

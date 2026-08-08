import { promises as fs } from "node:fs";
import path from "node:path";
import { loadLocalCredential } from "./identity.js";
import { syncCloudMcpInstallations, type CloudMcpSyncEntry } from "./mcp-cloud-sync.js";

function syncUrl(serverUrl: string) {
  const url = new URL(serverUrl);
  if (url.protocol === "wss:") url.protocol = "https:";
  else if (url.protocol === "ws:") url.protocol = "http:";
  url.pathname = "/api/client/mcp-sync";
  url.search = "";
  url.hash = "";
  return url.toString();
}

export async function syncCloudMcpBeforeClient() {
  const serverUrl = process.env.SERVER_URL;
  const projectArg = process.env.PROJECT_ROOT;
  if (!serverUrl || !projectArg) return { skipped: true, reason: "missing SERVER_URL or PROJECT_ROOT" };
  const credential = await loadLocalCredential(serverUrl);
  if (!credential) return { skipped: true, reason: "device is not paired yet" };
  const workspaceRoot = await fs.realpath(path.resolve(projectArg));
  const workspaceId = process.env.CODELOCAL_WORKSPACE_ID ?? path.basename(workspaceRoot);
  const response = await fetch(syncUrl(serverUrl), {
    method: "POST",
    headers: {
      "content-type": "application/json",
      "x-codelocal-credential-id": credential.credentialId,
      authorization: `Device ${credential.credentialSecret}`,
    },
    body: JSON.stringify({ workspaceId }),
    signal: AbortSignal.timeout(12_000),
  });
  if (!response.ok) throw new Error(`Cloud MCP sync failed with HTTP ${response.status}.`);
  const payload = await response.json() as { installations?: CloudMcpSyncEntry[] };
  const installations = Array.isArray(payload.installations) ? payload.installations : [];
  const result = await syncCloudMcpInstallations(workspaceRoot, installations);
  return { skipped: false, ...result };
}

(() => {
  "use strict";

  const MESSAGE_TYPE = "codelocal:penpot-mcp-token";
  const ALLOWED_PARENT_ORIGINS = new Set([
    "https://codelocal.cloud",
    "http://localhost:3000",
    "http://localhost:3333",
  ]);

  if (window.parent === window) return;

  let parentOrigin = "";
  try {
    parentOrigin = new URL(document.referrer).origin;
  } catch {
    return;
  }
  if (!ALLOWED_PARENT_ORIGINS.has(parentOrigin)) return;

  let provisioning = false;
  let deliveredToken = "";

  async function rpc(command, body) {
    const response = await fetch(`/api/rpc/command/${command}`, {
      method: "POST",
      credentials: "include",
      cache: "no-store",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/json",
      },
      body: JSON.stringify(body),
    });
    if (!response.ok) return null;
    return response.json().catch(() => null);
  }

  function tokenFromURL(value) {
    if (typeof value !== "string" || !value) return "";
    try {
      return new URL(value, window.location.origin).searchParams.get("userToken")?.trim() || "";
    } catch {
      return "";
    }
  }

  function extractToken(value) {
    if (!value || typeof value !== "object" || Array.isArray(value)) return "";
    const record = value;
    for (const key of ["token", "accessToken", "access-token", "mcpToken", "mcp-token", "currentMcpToken"]) {
      const candidate = record[key];
      if (typeof candidate === "string" && candidate.trim().length >= 20) return candidate.trim();
    }
    for (const key of ["url", "serverUrl", "server-url", "mcpUrl", "mcp-url"]) {
      const candidate = tokenFromURL(record[key]);
      if (candidate) return candidate;
    }
    for (const key of ["result", "data"]) {
      const candidate = extractToken(record[key]);
      if (candidate) return candidate;
    }
    return "";
  }

  async function provision() {
    if (provisioning) return;
    provisioning = true;
    try {
      let token = extractToken(await rpc("get-current-mcp-token", {}));
      if (!token) {
        token = extractToken(await rpc("create-access-token", {
          name: "CodeLocal MCP",
          type: "mcp",
        }));
      }
      if (!token || token === deliveredToken) return;
      deliveredToken = token;
      window.parent.postMessage({ type: MESSAGE_TYPE, token }, parentOrigin);
    } finally {
      provisioning = false;
    }
  }

  void provision();
  window.setTimeout(() => void provision(), 2500);
  window.addEventListener("focus", () => void provision());
})();

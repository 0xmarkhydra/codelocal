const webBase = requiredURL("CODELOCAL_WEB_URL");
const backendBase = requiredURL("CODELOCAL_BACKEND_PUBLIC_URL");
const probeToken = required("CODELOCAL_EDGE_PROBE_TOKEN");
const probeUA = "CodeLocal-Edge-Probe/1";

function required(name) {
  const value = String(process.env[name] || "").trim();
  if (!value) throw new Error(`${name} is required`);
  return value;
}

function requiredURL(name) {
  const raw = required(name).replace(/\/+$/, "");
  const value = new URL(raw);
  if (!/^https?:$/.test(value.protocol)) throw new Error(`${name} must use http or https`);
  return value.toString().replace(/\/$/, "");
}

async function request(url, options = {}) {
  const controller = new AbortController();
  const timeout = setTimeout(() => controller.abort(), 15_000);
  try {
    return await fetch(url, { ...options, signal: controller.signal, redirect: "follow" });
  } finally {
    clearTimeout(timeout);
  }
}

async function json(url, options = {}) {
  const response = await request(url, options);
  const text = await response.text();
  if (!response.ok) throw new Error(`${url} returned ${response.status}: ${text.slice(0, 200)}`);
  try {
    return JSON.parse(text);
  } catch {
    throw new Error(`${url} did not return JSON`);
  }
}

async function probe(base) {
  return json(`${base}/internal/web-edge-probe`, {
    headers: {
      Authorization: `Bearer ${probeToken}`,
      "User-Agent": probeUA,
    },
  });
}

function assert(condition, message) {
  if (!condition) throw new Error(message);
}

console.log("[edge-smoke] checking Next health route");
const health = await json(`${webBase}/healthz`);
assert(health?.ok === true && health?.service === "codelocal-web", "Next health route returned an unexpected payload");

console.log("[edge-smoke] checking unauthenticated API proxy");
const account = await request(`${webBase}/api/v1/account`, { headers: { "User-Agent": probeUA } });
assert(account.status === 401, `proxied /api/v1/account returned ${account.status}, want 401`);
assert((account.headers.get("cache-control") || "").toLowerCase().includes("no-store"), "proxied API response is missing no-store cache policy");

console.log("[edge-smoke] comparing direct Go and Next-proxied security signals");
const direct = await probe(backendBase);
const proxied = await probe(webBase);
assert(direct.clientIpDigest === proxied.clientIpDigest, "client IP signal changed between direct Go and Next-proxied paths");
assert(direct.userAgentDigest === proxied.userAgentDigest, "browser User-Agent changed while proxying through Next");
assert(proxied.forwardedProto === "https", `proxied forwarded proto is ${JSON.stringify(proxied.forwardedProto)}, want https`);
assert(proxied.railwayEdgePresent === true, "Railway edge marker was not preserved through Next");
assert(proxied.railwayRequestIdPresent === true, "Railway request ID was not preserved through Next");

console.log("[edge-smoke] PASS — Next canary preserves Go security signals");

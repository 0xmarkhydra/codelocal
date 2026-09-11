import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";

const source = readFileSync(new URL("./codelocal-mcp-bootstrap.js", import.meta.url), "utf8");
const securityHeaders = readFileSync(new URL("./nginx-security-headers.conf", import.meta.url), "utf8");
const frontendDockerfiles = [
  readFileSync(new URL("./Dockerfile", import.meta.url), "utf8"),
  readFileSync(new URL("../docker/penpot-frontend.Dockerfile", import.meta.url), "utf8"),
];
const token = "test-account-token-not-a-real-credential";
const flush = () => new Promise((resolve) => setImmediate(resolve));

assert.match(
  securityHeaders,
  /Content-Security-Policy "frame-ancestors https:\/\/design\.codelocal\.cloud https:\/\/codelocal\.cloud"/,
  "Penpot plugin iframe and CodeLocal dashboard must remain the only frame ancestors",
);
for (const dockerfile of frontendDockerfiles) {
  assert.match(
    dockerfile,
    /\[ "\$\(grep -o 'hidden:d' .*plugin\.js \| wc -l\)" -eq 1 \]/,
    "frontend build must fail when the pinned MCP plugin bundle changes",
  );
  assert.match(
    dockerfile,
    /sed -i 's\/hidden:d\/hidden:!1\/' .*plugin\.js/,
    "integrated MCP plugin UI must load so its WebSocket bridge can start",
  );
  assert.match(
    dockerfile,
    /plugin-codelocal-v1\.js/,
    "patched MCP plugin must use a versioned URL that bypasses stale caches",
  );
  assert.match(
    dockerfile,
    /plugins\/mcp-codelocal-v1/,
    "integrated MCP iframe document must use a versioned path",
  );
  assert.match(
    dockerfile,
    /shared\.js\?version=2\.17\.0-1784702688-codelocal2/,
    "shared bundle URL must change when its integrated plugin path changes",
  );
}
assert.match(
  readFileSync(new URL("./nginx-mcp-locations.conf.template", import.meta.url), "utf8"),
  /location \/mcp\/stream \{\s+access_log off;/,
  "credential-bearing MCP query forms must never enter access logs",
);

function boot(responses, referrer = "https://codelocal.cloud/dashboard/design") {
  const calls = [];
  const messages = [];
  const events = {};
  const document = { referrer, visibilityState: "visible", addEventListener: (name, fn) => { events[name] = fn; } };
  const window = {
    parent: { postMessage: (message, origin) => messages.push({ message, origin }) },
    location: { origin: "https://design.codelocal.cloud" },
    addEventListener: (name, fn) => { events[name] = fn; },
    setTimeout: (fn) => { events.timeout = fn; },
    setInterval: (fn) => { events.interval = fn; },
  };
  runInNewContext(source, {
    window, document, URL,
    fetch: async (path, options) => {
      calls.push({ path, options });
      const response = responses.shift();
      if (response instanceof Error) throw response;
      assert.ok(response, "unexpected RPC call");
      return { ok: response.ok !== false, json: async () => response.value };
    },
  });
  return { calls, messages, events, document };
}

for (const response of [{ ok: false }, new Error("network"), { get value() { throw new Error("invalid JSON"); } }]) {
  const state = boot([response]);
  await flush();
  assert.equal(state.calls.length, 1, "failed lookup must not create/rotate a key");
  assert.equal(state.messages.length, 0);
}

const state = boot([{ ok: false }, { value: { token } }, { value: { token } }]);
await flush();
state.events.hashchange();
await flush();
assert.equal(state.messages.length, 1, "SSO navigation retries provisioning");
assert.equal(state.messages[0].origin, "https://codelocal.cloud");
assert.equal(state.messages[0].message.token, token);
state.events.interval();
await flush();
assert.equal(state.messages.length, 1, "same token is not delivered twice");
state.document.visibilityState = "hidden";
state.events.interval();
await flush();
assert.equal(state.calls.length, 3, "hidden documents do not poll");

const fresh = boot([{ value: null }, { value: { token } }]);
await flush();
assert.equal(fresh.calls.length, 2);
assert.equal(fresh.calls[1].path, "/api/rpc/command/create-access-token");
assert.equal(JSON.parse(fresh.calls[1].options.body).type, "mcp");
assert.equal(fresh.messages.length, 1);

const untrusted = boot([], "https://untrusted.example/");
await flush();
assert.equal(untrusted.calls.length, 0, "untrusted parent cannot acquire a key");
assert.equal(untrusted.messages.length, 0);
console.log("PASS: Penpot bootstrap errors, SSO retry, creation, deduplication, origin and visibility");

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";

const source = readFileSync(new URL("./codelocal-mcp-bootstrap.js", import.meta.url), "utf8");
const securityHeaders = readFileSync(new URL("./nginx-security-headers.conf", import.meta.url), "utf8");
const token = "test-account-token-not-a-real-credential";
const flush = () => new Promise((resolve) => setImmediate(resolve));

assert.match(
  securityHeaders,
  /Content-Security-Policy "frame-ancestors 'self' https:\/\/codelocal\.cloud"/,
  "same-origin MCP plugin iframe and CodeLocal dashboard must remain the only frame ancestors",
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

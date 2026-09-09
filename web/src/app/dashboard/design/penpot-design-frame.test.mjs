import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { runInNewContext } from "node:vm";
import ts from "typescript";

// Exercise the component's effects with controlled network completion, without a browser or credentials.
const source = readFileSync(new URL("./penpot-design-frame.tsx", import.meta.url), "utf8");
const code = ts.transpileModule(source, {
  compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX },
}).outputText;
const refs = [];
const effects = [];
const listeners = new Map();
const exports = {};
let refIndex = 0;
let calls = [];
let inFlight = 0;
let peak = 0;
let hold;
const items = Array.from({ length: 29 }, (_, i) => ({
  deviceId: "device", workspaceId: `workspace-${i}`, status: "active", runtimeOnline: true,
}));
items.push({ deviceId: "offline", workspaceId: "offline", status: "offline", runtimeOnline: false });
runInNewContext(code, {
  exports, URL, AbortController,
  window: {
    addEventListener: (event, listener) => listeners.set(event, listener),
    removeEventListener: (event) => listeners.delete(event),
  },
  fetch: async (_, options) => {
    calls.push(options);
    peak = Math.max(peak, ++inFlight);
    try {
      if (hold) await new Promise((resolve) => { hold = resolve; });
      return { ok: true, status: 200 };
    } finally {
      inFlight--;
    }
  },
  require: (name) => {
    if (name === "react") return {
      useRef: (initial) => refs[refIndex++] ?? (refs[refIndex - 1] = { current: initial }),
      useState: () => [0, () => {}],
      useMemo: (fn) => fn(),
      useEffect: (fn) => effects.push(fn),
    };
    if (name === "react/jsx-runtime") return { jsx: (_, props) => props };
    if (name === "@/lib/i18n/provider") return {
      useTranslations: () => ({ t: (message) => message }),
    };
    if (name === "../use-dashboard-resource") return {
      useDashboardResource: (url) => ({
        state: { kind: "ready", value: url.endsWith("/account") ? { csrf: "test-csrf" } : { items } },
        retry: () => {},
      }),
    };
    return { default: {} };
  },
});
function render() {
  refIndex = 0;
  effects.length = 0;
  exports.PenpotDesignFrame({ designUrl: "https://design.codelocal.cloud/" });
  return effects.map((effect) => effect());
}
const flush = () => new Promise((resolve) => setImmediate(resolve));
let cleanups = render();
const frame = {};
refs[0].current = { contentWindow: frame };
const message = (token, origin = "https://design.codelocal.cloud", source = frame) =>
  listeners.get("message")({ origin, source, data: { type: "codelocal:penpot-mcp-token", token } });
message("test-account-token-A", "https://untrusted.example");
message("test-account-token-A", "https://design.codelocal.cloud", {});
assert.equal(refs[1].current, "");
message("test-account-token-A");
cleanups.forEach((cleanup) => cleanup?.());
cleanups = render();
await flush();
assert.equal(calls.length, 29, "workspaces beyond index 10 receive credentials");
assert.equal(peak, 1, "connections are sequential");
assert.ok(calls.every((call) => call.headers["X-CSRF-Token"] === "test-csrf"));

message("test-account-token-B");
cleanups.forEach((cleanup) => cleanup?.());
calls = [];
hold = true;
cleanups = render();
assert.equal(calls.length, 1);
message("test-account-token-C");
hold();
hold = undefined;
await flush();
assert.equal(calls.length, 1, "old-token queue stops after rotation");
assert.equal(refs[2].current.size, 0, "old-token success cannot mark new-token connection ready");
cleanups.forEach((cleanup) => cleanup?.());
cleanups = render();
await flush();
assert.equal(calls.length, 30);
assert.ok(calls.slice(1).every((call) => JSON.parse(call.body).bearerToken === "test-account-token-C"));
cleanups.forEach((cleanup) => cleanup?.());
console.log("PASS: 29 workspaces, bounded concurrency, CSRF, origin/source checks, token rotation");

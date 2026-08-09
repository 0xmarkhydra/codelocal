import test from "node:test";
import assert from "node:assert/strict";
import { authPage, dashboardPage } from "../web-ui.js";

test("dashboard CSS defines responsive grid spans", () => {
  const html = dashboardPage({
    title: "Layout smoke",
    active: "overview",
    email: "dev@example.com",
    csrf: "csrf",
    body: `<div class="grid"><div class="card span7">left</div><div class="card span5">right</div></div>`,
  });

  assert.match(html, /\.span5\{grid-column:span 5\}/);
  assert.match(html, /\.span7\{grid-column:span 7\}/);
  assert.match(html, /grid-template-columns:repeat\(12,minmax\(0,1fr\)\)/);
  assert.match(html, /@media\(max-width:760px\)/);
});

test("dashboard protects narrow layouts and long developer values", () => {
  const html = dashboardPage({
    title: "Overflow smoke",
    active: "connect",
    email: "very-long-account-name@example.com",
    csrf: "csrf",
    body: `<div class="code-block">https://example.com/${"a".repeat(500)}</div>`,
  });

  assert.match(html, /\.main\{min-width:0/);
  assert.match(html, /\.card\{min-width:0/);
  assert.match(html, /max-width:100%/);
  assert.match(html, /overflow-wrap:anywhere/);
});

test("dashboard includes shared confirm and copy interactions", () => {
  const html = dashboardPage({
    title: "Interactions",
    active: "workspaces",
    email: "dev@example.com",
    csrf: "csrf",
    body: `<form><button data-confirm data-confirm-title="Remove?">Remove</button></form><div id="copy-me">value</div><button data-copy-target="#copy-me">Copy</button>`,
  });

  assert.match(html, /id="confirm-dialog"/);
  assert.match(html, /data-confirm/);
  assert.match(html, /navigator\.clipboard\.writeText/);
  assert.match(html, /requestSubmit\(\)/);
});

test("dashboard nav exposes only implemented product areas", () => {
  const html = dashboardPage({
    title: "Navigation",
    active: "overview",
    email: "dev@example.com",
    csrf: "csrf",
    body: "",
  });

  for (const visible of ["Overview", "Workspaces", "Devices", "Connect ChatGPT", "Security"]) assert.match(html, new RegExp(visible));
  for (const hidden of ["MCP Extensions", "Billing", "Permissions", "Browser", "Simulator"]) assert.doesNotMatch(html, new RegExp(`>${hidden}<`));
});

test("auth page keeps responsive overflow protection", () => {
  const html = authPage({
    title: "Create account",
    subtitle: "Responsive auth",
    body: `<form class="form"><input class="input" value="${"x".repeat(200)}"></form>`,
  });

  assert.match(html, /html,body\{max-width:100%;overflow-x:hidden\}/);
  assert.match(html, /\.input,\.select,\.textarea\{width:100%;min-width:0;max-width:100%/);
  assert.match(html, /\.auth\{width:min\(430px,100%\)\}/);
});

import test from "node:test";
import assert from "node:assert/strict";
import { authPage, dashboardPage, landingPage } from "../web-ui.js";

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

  for (const visible of ["Overview", "Workspaces", "Devices", "MCP Connections", "Security"]) assert.match(html, new RegExp(visible));
  for (const hidden of ["MCP Extensions", "Billing", "Permissions", "Browser", "Simulator"]) assert.doesNotMatch(html, new RegExp(`>${hidden}<`));
});

test("dashboard ships the CodeLocal icon set and accessible vector navigation", () => {
  const html = dashboardPage({
    title: "Brand smoke",
    active: "overview",
    email: "dev@example.com",
    csrf: "csrf",
    body: "",
  });

  assert.match(html, /href="\/favicon\.ico"/);
  assert.match(html, /href="\/assets\/apple-touch-icon\.png"/);
  assert.match(html, /src="\/assets\/codelocal-icon\.png"/);
  assert.match(html, /class="nav-icon"><svg/);
  assert.match(html, /prefers-color-scheme:dark/);
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

test("public landing page presents the universal MCP setup flow", () => {
  const html = landingPage({ endpoint: "https://codelocal.cloud/mcp", signedIn: false });

  for (const step of [
    "Connect an MCP-compatible AI client",
    "Install CodeLocal on your computer",
    "Authorize a project folder",
    "Start the machine runtime",
    "Start working from your AI client",
  ]) assert.match(html, new RegExp(step));
  assert.match(html, /npm install -g codelocal/);
  assert.doesNotMatch(html, /npm install -g codelocal@beta/);
  assert.match(html, /codelocal \./);
  assert.match(html, /local-only action and does not connect to CodeLocal Cloud/);
  assert.match(html, /reuses the existing machine runtime instead of starting a duplicate/);
  assert.match(html, /https:\/\/codelocal\.cloud\/mcp/);
  assert.match(html, /Authentication<\/div><div class="plugin-spec-value">OAuth/);
  assert.match(html, /href="\/assets\/chatgpt-plugin-icon\.png"/);
  assert.match(html, /256 × 256 px/);
  assert.match(html, /Under 10 KB/);
  assert.match(html, /ChatGPT \/ Codex/);
  assert.match(html, /Other compatible clients/);
  assert.match(html, /<title>CodeLocal — Universal MCP \+ Project Brain for AI Coding<\/title>/);
  assert.match(html, /property="og:title" content="CodeLocal — Universal MCP \+ Project Brain for AI Coding"/);
  assert.match(html, /property="og:description" content="Connect MCP-compatible AI clients and coding agents to one durable Project Brain/);
  assert.match(html, /property="og:image" content="https:\/\/codelocal\.cloud\/assets\/codelocal-icon\.png"/);
  assert.match(html, /name="twitter:card" content="summary"/);
  assert.match(html, /rel="canonical" href="https:\/\/codelocal\.cloud\/"/);
  assert.match(html, /One MCP layer for the AI tools you use\./);
  assert.match(html, /Your AI client reasons\. CodeLocal provides the shared MCP layer\./);
  assert.match(html, /No separate CodeLocal model key\. No per-token billing from CodeLocal\./);
  assert.match(html, /No separate MCP call quota from CodeLocal\./);
  assert.match(html, /limits and billing of the client or provider you choose/);
});

import test from "node:test";
import assert from "node:assert/strict";
import { authPage, dashboardPage } from "../web-ui.js";

test("dashboard CSS defines the grid spans used by SaaS pages", () => {
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
  assert.match(html, /\.span3,\.span4,\.span5,\.span6,\.span7,\.span8\{grid-column:span 12\}/);
});

test("dashboard CSS contains overflow guards for narrow screens and long developer values", () => {
  const html = dashboardPage({
    title: "Overflow smoke",
    active: "connect",
    email: "very-long-account-name@example.com",
    csrf: "csrf",
    body: `<div class="code-block">https://example.com/${"a".repeat(500)}</div>`,
  });

  assert.match(html, /\.main\{min-width:0/);
  assert.match(html, /\.card\{min-width:0/);
  assert.match(html, /\.code-block\{max-width:100%/);
  assert.match(html, /overflow-wrap:anywhere/);
  assert.match(html, /@media\(max-width:640px\)/);
});

test("auth page keeps the same responsive overflow protection", () => {
  const html = authPage({
    title: "Create account",
    subtitle: "Responsive auth",
    body: `<form class="form"><input class="input" value="${"x".repeat(200)}"></form>`,
  });

  assert.match(html, /html,body\{max-width:100%;overflow-x:hidden\}/);
  assert.match(html, /\.input,\.select,\.textarea\{min-width:0;width:100%;max-width:100%/);
  assert.match(html, /\.auth\{width:min\(440px,100%\);min-width:0\}/);
});

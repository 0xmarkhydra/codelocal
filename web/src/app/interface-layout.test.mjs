import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import postcss from "postcss";

const read = (path) => readFileSync(new URL(path, import.meta.url), "utf8");
const css = (path) => postcss.parse(read(path));
function declarations(path, selector, property) {
  const values = [];
  css(path).walkRules(selector, (rule) => {
    rule.walkDecls(property, (decl) => values.push(decl.value));
  });
  return values;
}

// Source-level guards only; these do not replace browser layout checks.
function luminance(hex) {
  const channels = hex.match(/[a-f\d]{2}/gi).map((channel) => {
    const value = parseInt(channel, 16) / 255;
    return value <= 0.04045 ? value / 12.92 : ((value + 0.055) / 1.055) ** 2.4;
  });
  return channels.reduce((sum, value, index) => sum + value * [0.2126, 0.7152, 0.0722][index], 0);
}
function contrast(foreground, background) {
  const values = [luminance(foreground), luminance(background)].sort((a, b) => b - a);
  return (values[0] + 0.05) / (values[1] + 0.05);
}
const tokens = new Map();
css("./globals.css").walkDecls(/^--/, (decl) => tokens.set(decl.prop, decl.value));
function token(name) {
  const value = tokens.get(name);
  assert.ok(value, `Missing token ${name}`);
  return value.startsWith("var(") ? token(value.slice(4, -1)) : value;
}
for (const foreground of ["--text", "--text-secondary", "--text-tertiary"]) {
  for (const background of ["--canvas", "--surface", "--surface-raised"]) {
    assert.ok(contrast(token(foreground), token(background)) >= 4.5, `${foreground} on ${background}`);
  }
}
for (const background of ["--accent", "--accent-strong"]) {
  assert.ok(contrast("#102019", token(background)) >= 4.5, `Primary button on ${background}`);
}

assert.ok(declarations("./dashboard/neural-graph-stage.module.css", ".stage", "min-height").every((value) => value === "0"));
assert.ok(declarations("./dashboard/dashboard-controls.module.css", ".overviewState", "flex-wrap").includes("wrap"));
assert.ok(declarations("./dashboard/design/design.module.css", ".embedShell", "grid-template-rows").includes("auto minmax(0, 1fr)"));
assert.deepEqual(declarations("./dashboard/shots/shots.module.css", ".dropZone > button:hover:not(:disabled)", "background"), ["var(--accent-strong)"]);
assert.ok(!read("./dashboard/code-graph/page.tsx").includes("styles.brainContent"));
assert.ok(read("./dashboard/code-graph/code-graph-live.tsx").includes("styles.codeGraphStage"));
const connect = read("./dashboard/connect/page.tsx");
assert.ok(!connect.includes("<LiveOverview"));
assert.match(connect, /<details[^>]*\bopen\b/);
for (const stylesheet of ["./dashboard/blogs/blog-editor.module.css", "./dashboard/blogs/series-editor.module.css"]) {
  assert.ok(read(stylesheet).includes(":focus-within"));
}
const poolStyles = "../../apps/pool/src/app/pool.module.css";
assert.ok(declarations(poolStyles, ".nav", "flex-wrap").includes("wrap"));
assert.ok(!declarations(poolStyles, ".nav", "display").includes("none"));
console.log("Interface source checks passed: contrast, graph sizing, toolbar wrapping, upload focus, Pool navigation.");

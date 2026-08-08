import test from "node:test";
import assert from "node:assert/strict";
import path from "node:path";
import { chromiumProfileDirs, isBrowserInternalUrl, isLocalDevUrl } from "../browser-control.js";

test("browser control identifies local development URLs", () => {
  assert.equal(isLocalDevUrl("http://localhost:3000/settings"), true);
  assert.equal(isLocalDevUrl("https://127.0.0.1:8443"), true);
  assert.equal(isLocalDevUrl("http://[::1]:8080"), true);
  assert.equal(isLocalDevUrl("https://example.com"), false);
});

test("browser control excludes internal Chromium targets from normal pages", () => {
  assert.equal(isBrowserInternalUrl("chrome://inspect/#remote-debugging"), true);
  assert.equal(isBrowserInternalUrl("devtools://devtools/bundled/inspector.html"), true);
  assert.equal(isBrowserInternalUrl("edge://newtab/"), true);
  assert.equal(isBrowserInternalUrl("http://localhost:3000"), false);
});

test("browser control discovers platform-specific profile roots without hard-coding one Chrome profile", () => {
  const mac = chromiumProfileDirs("darwin", "/Users/demo");
  assert.ok(mac.some((item) => item.includes(path.join("Google", "Chrome"))));
  assert.ok(mac.some((item) => item.includes("Brave-Browser")));
  assert.ok(mac.some((item) => item.includes("Microsoft Edge")));

  const win = chromiumProfileDirs("win32", "C:\\Users\\demo", "C:\\Users\\demo\\AppData\\Local");
  assert.ok(win.some((item) => item.includes("Chrome")));
  assert.ok(win.some((item) => item.includes("Edge")));
});

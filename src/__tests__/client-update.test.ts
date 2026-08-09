import test from "node:test";
import assert from "node:assert/strict";
import { clientReleaseManifest, compareSemver, evaluateClientUpdate, renderClientUpdateNotice } from "../client-update.js";

test("semver comparison handles prerelease ordering", () => {
  assert.equal(compareSemver("1.5.0-beta.2", "1.5.0-beta.3"), -1);
  assert.equal(compareSemver("1.5.0-beta.10", "1.5.0-beta.3"), 1);
  assert.equal(compareSemver("1.5.0-beta.3", "1.5.0"), -1);
  assert.equal(compareSemver("1.5.0", "1.5.0-beta.3"), 1);
  assert.equal(compareSemver("v1.5.0-beta.3", "1.5.0-beta.3"), 0);
});

test("release manifest is disabled until a published latest client version is configured", () => {
  const manifest = clientReleaseManifest({});
  assert.equal(manifest.latestVersion, null);
  assert.equal(evaluateClientUpdate("1.5.0-beta.2", manifest), null);
});

test("outdated and legacy clients receive update notices", () => {
  const manifest = clientReleaseManifest({
    CODELOCAL_LATEST_CLIENT_VERSION: "1.5.0-beta.3",
    CODELOCAL_MIN_CLIENT_VERSION: "1.5.0-beta.2",
  });
  const outdated = evaluateClientUpdate("1.5.0-beta.2", manifest);
  assert.equal(outdated?.level, "recommended");
  assert.equal(outdated?.latestVersion, "1.5.0-beta.3");
  assert.match(renderClientUpdateNotice(outdated!), /npm i -g codelocal@latest/);

  const legacy = evaluateClientUpdate(undefined, manifest);
  assert.equal(legacy?.level, "recommended");
  assert.equal(legacy?.installedVersion, null);
});

test("clients below the configured minimum are marked required", () => {
  const manifest = clientReleaseManifest({
    CODELOCAL_LATEST_CLIENT_VERSION: "1.5.0-beta.4",
    CODELOCAL_MIN_CLIENT_VERSION: "1.5.0-beta.3",
  });
  assert.equal(evaluateClientUpdate("1.5.0-beta.2", manifest)?.level, "required");
  assert.equal(evaluateClientUpdate("1.5.0-beta.4", manifest), null);
  assert.equal(evaluateClientUpdate("1.6.0", manifest), null);
});

import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import ts from "typescript";

const modules = new Map();
function load(path) {
  if (modules.has(path)) return modules.get(path);
  const source = readFileSync(new URL(path, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, target: ts.ScriptTarget.ES2022 },
  });
  const compiled = { exports: {} };
  new Function("require", "module", "exports", outputText)((id) => {
    assert.ok(id.startsWith("@/lib/"));
    return load(`./${id.slice("@/lib/".length)}.ts`);
  }, compiled, compiled.exports);
  modules.set(path, compiled.exports);
  return compiled.exports;
}

const { MediaUploadError, uploadMediaAsset } = load("./media-upload.ts");
const { translate } = load("./i18n/messages.ts");
const file = new File(["image-test"], "ảnh-चित्र.png", { type: "image/png" });
const asset = {
  id: "media_fixture", ownerUserId: "user_fixture", sourceSha256: "a".repeat(64),
  sourceContentType: file.type, sourceSize: file.size, width: 32, height: 24,
  status: "processing", preserveOriginal: true, createdAt: 1, updatedAt: 1, variants: [],
};
const prepared = {
  asset, urls: {}, deduplicated: false,
  upload: { required: true, url: "https://uploads.example.test/fixture", method: "PUT",
    headers: { "content-type": ["image/png"], "x-upload-check": ["fixture"] } },
};
const originalFetch = globalThis.fetch;
let calls;
function responses(...values) {
  calls = [];
  globalThis.fetch = async (url, options) => {
    calls.push({ url, options });
    assert.ok(values.length, "Unexpected network request");
    const next = values.shift();
    if (next instanceof Error) throw next;
    return next;
  };
}
const json = (value, status = 200) => Response.json(value, { status });
async function rejects(key, status) {
  await assert.rejects(uploadMediaAsset(file, "csrf", { preserveOriginal: true }), (error) => {
    assert.ok(error instanceof MediaUploadError);
    assert.equal(error.messageKey, key);
    assert.equal(error.values.status, String(status));
    for (const locale of ["en", "vi", "zh-Hans", "hi"]) {
      const text = translate(locale, error.messageKey, error.values);
      assert.ok(text.includes(String(status)));
      assert.ok(!text.includes("{status}"));
    }
    return true;
  });
}
try {
  responses();
  await assert.rejects(uploadMediaAsset(new File(["x"], "bad.svg", { type: "image/svg+xml" }), "csrf"), /Use a JPEG, PNG, or WebP image/);
  await assert.rejects(uploadMediaAsset(new File([], "empty.png", { type: "image/png" }), "csrf"), /25 MB/);
  assert.equal(calls.length, 0);

  responses(json(null, 403));
  await rejects("Unable to prepare image upload ({status}).", 403);
  responses(json({ invalid: true }));
  await rejects("Unable to prepare image upload ({status}).", 200);
  responses(json(prepared), new Error("direct transport failed"), json(null, 413));
  await rejects("Image upload failed ({status}).", 413);
  assert.equal(calls[2].options.headers["X-CSRF-Token"], "csrf");
  assert.equal(calls[2].options.body, file);
  responses(json({ ...prepared, upload: { required: false } }), json(null, 503));
  await rejects("Image processing failed ({status}).", 503);

  responses(json(prepared), new Response(null, { status: 200 }), json({ asset: { ...asset, status: "ready" }, urls: {} }));
  assert.equal((await uploadMediaAsset(file, "csrf", { preserveOriginal: true })).status, "ready");
  assert.equal(calls.length, 3);
  const payload = JSON.parse(calls[0].options.body);
  assert.equal(payload.preserveOriginal, true);
  assert.equal(payload.size, file.size);
  assert.match(payload.sha256, /^[a-f0-9]{64}$/);
  assert.equal(calls[1].options.headers.get("x-upload-check"), "fixture");
  assert.equal(calls[1].options.body, file);
  assert.equal(calls[2].options.headers["X-CSRF-Token"], "csrf");
  responses(json({ ...prepared, asset: { ...asset, status: "ready" }, deduplicated: true }));
  assert.equal((await uploadMediaAsset(file, "csrf")).id, asset.id);
  assert.equal(calls.length, 1);
} finally {
  globalThis.fetch = originalFetch;
}
console.log("Media upload checks passed: validation, status translations, direct/proxy flow, CSRF and deduplication.");

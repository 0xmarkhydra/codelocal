import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import ts from "typescript";

async function load(path) {
  const source = readFileSync(new URL(path, import.meta.url), "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.ESNext, target: ts.ScriptTarget.ES2022 },
  });
  return import(`data:text/javascript;base64,${Buffer.from(outputText).toString("base64")}`);
}

const { locales, isLocale, resolveLocale, languageCookie } = await load("./locale.ts");
const { messages, translate, translateKnownMessage } = await load("./messages.ts");
const dictionarySource = ts.createSourceFile("messages.ts", readFileSync(new URL("./messages.ts", import.meta.url), "utf8"), ts.ScriptTarget.Latest, true);
function checkDuplicateKeys(node) {
  if (ts.isObjectLiteralExpression(node)) {
    const keys = node.properties.flatMap((property) => property.name && (ts.isStringLiteral(property.name) || ts.isIdentifier(property.name)) ? [property.name.text] : []);
    assert.equal(new Set(keys).size, keys.length, "Duplicate dictionary property");
  }
  ts.forEachChild(node, checkDuplicateKeys);
}
checkDuplicateKeys(dictionarySource);
assert.equal(resolveLocale("hi", "vi,en;q=0.9"), "hi");
assert.equal(resolveLocale("invalid", "fr;q=1,zh-CN;q=0.8,vi;q=0.9"), "vi");
assert.equal(resolveLocale(undefined, "en;q=0,hi;q=0.8"), "hi");
assert.equal(resolveLocale(undefined, "vi;q=no,en;q=0.5"), "en");
assert.equal(resolveLocale(undefined, "vi;q=2,hi;q=0.5"), "hi");
assert.equal(resolveLocale(undefined, "vi;q=,en;q=1"), "en");
assert.equal(resolveLocale(undefined, "hi-IN, en-US;q=0.5"), "hi");
assert.equal(resolveLocale(undefined, "zh-TW"), "zh-Hans");
assert.equal(resolveLocale(undefined, "de,fr"), "en");
assert.equal(resolveLocale(undefined, "en_US;q=1,vi;q=0.4"), "vi");
assert.equal(resolveLocale(undefined, "*"), "en");
assert.equal(resolveLocale(), "en");
assert.equal(isLocale("__proto__"), false);
assert.throws(() => languageCookie("vi; Domain=example.com", true));
assert.equal(languageCookie("vi", true), "codelocal-language=vi; Path=/; Max-Age=31536000; SameSite=Lax; Secure");
assert.ok(!languageCookie("vi", false).includes("Secure"));

const placeholders = (value) => [...value.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort();
for (const [key, translations] of Object.entries(messages)) {
  assert.equal(translations.length, 3, key);
  for (const translation of translations) {
    assert.ok(translation.trim(), `Empty translation: ${key}`);
    assert.deepEqual(placeholders(translation), placeholders(key), `Placeholders: ${key}`);
  }
  for (const locale of locales) assert.equal(typeof translate(locale, key), "string");
}
assert.equal(translate("en", "Overview"), "Overview");
assert.equal(translate("vi", "Overview"), "Tổng quan");
assert.equal(translate("zh-Hans", "Overview"), "概览");
assert.equal(translate("hi", "Overview"), "अवलोकन");
assert.equal(translate("en", "{count} active", { count: 1234 }), "1,234 active");
assert.equal(translate("vi", "{count} active", { count: 1234 }), "1.234 đang hoạt động");
assert.ok(translate("hi", "Run {command} in your project directory to connect your workspace.", { command: "codelocal ." }).includes("codelocal ."));
assert.equal(translate("en", "{count} stars", { count: 1 }), "1 star");
assert.equal(translate("en", "{count} stars", { count: 2 }), "2 stars");
assert.equal(translate("hi", "{count} stars", { count: 1 }), "1 सितारा");
assert.equal(translateKnownMessage("vi", "Email or password is incorrect."), "Email hoặc mật khẩu không đúng.");
assert.equal(translateKnownMessage("vi", "__proto__"), "__proto__");
assert.equal(translateKnownMessage("vi", "constructor"), "constructor");
const externalText = 'My workspace: /Users/name/CodeLocal <script>alert("test")</script> {name}';
assert.equal(translateKnownMessage("hi", externalText), externalText);
assert.equal(translateKnownMessage("vi", "{name} behavior updated.", { name: "{count}/my.skill" }), "Đã cập nhật cách dùng {count}/my.skill.");
assert.equal(translate("zh-Hans", "New value for {key}", { key: "FFMPEG_PATH" }), "FFMPEG_PATH 的新值");
// Stored message keys must render with the current language, not the mutation's old locale.
const savedNotice = { text: "Rated {name} {rating}/5.", values: { name: "CodeLocal", rating: 4 } };
assert.equal(translateKnownMessage("en", savedNotice.text, savedNotice.values), "Rated CodeLocal 4/5.");
assert.equal(translateKnownMessage("vi", savedNotice.text, savedNotice.values), "Đã đánh giá CodeLocal 4/5.");
console.log(`i18n checks passed: negotiation, cookie validation, ${Object.keys(messages).length} messages in four languages.`);

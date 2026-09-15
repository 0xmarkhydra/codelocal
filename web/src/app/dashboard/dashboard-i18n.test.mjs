import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import { fileURLToPath } from "node:url";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ts from "typescript";

const require = createRequire(import.meta.url);
let locale = "en";
const cache = new Map();
const graph = {
  state: "current", status: "resolved", view: "architecture", query: "", depth: 1,
  maxNodes: 100, truncated: false, selectedId: "n1",
  repositories: [{ path: ".", dirty: false, indexedAt: 1, fileCount: 1, symbolCount: 1 }],
  nodes: [{ id: "n1", kind: "file", name: "नमस्ते.ts", path: "src/नमस्ते.ts",
    qualifiedName: "Overview", line: 12, column: 3, confidence: 0.8, canonical: true }],
  edges: [],
  impact: { directCallers: 1, directCallees: 2, potentialCallers: 0, affectedFiles: 1,
    evidenceEdges: 4, semanticEdges: 3, averageConfidence: 0.75, risk: "high", truncated: false },
};
const workspace = {
  deviceId: "d1", workspaceId: "w1", workspaceName: "Project 原文",
  deviceName: "Device मूल", runtimeOnline: true, status: "online",
};
const indexHealth = {
  available: true, status: "stale", projectCount: 5, currentProjects: 2,
  emptyProjects: 0, staleProjects: 2, missingProjects: 1, maxLagMs: 61_000,
};
const health = {
  csrf: "a".repeat(32),
  pipeline: { available: true, status: "degraded", pendingCount: 2, processingCount: 1,
    retryingCount: 1, deadCount: 0, processedLastHour: 1200, oldestActiveAgeMs: 3_720_000 },
  graphIndex: indexHealth, semanticIndex: { ...indexHealth, available: false },
  canary: { available: true, attemptsTotal: 3, appliedCount: 1, appliedClaimsTotal: 1,
    deterministicFallbackCount: 2, readinessBlockedCount: 0, errorCount: 0, timeoutCount: 0, slowCount: 0 },
  collective: { available: true, contributionEnabled: false, suggestionsEnabled: true,
    contributionAvailable: false, suggestionsAvailable: true, minimumContributors: 10,
    recommendations: [{ taskKind: "repair", checkProfile: ["go test ./..."], fileCountBucket: "1-5",
      qualityBucket: "verified", executionTool: "terminal", meanUserSuccessRate: 0.8, contributorCount: 12 }] },
  privacy: { rawCodeShared: false, conversationShared: false, projectIdentityShared: false, localReplayTrustShared: false },
};
const shot = {
  id: "abcdefghijkl", url: "https://codelocal.example/s/abcdefghijkl",
  imageUrl: "https://codelocal.example/image.png", thumbnailUrl: "https://codelocal.example/thumb.png",
  downloadUrl: "https://codelocal.example/download.png", width: 1024, height: 768, size: 1200,
  createdAt: 1788912000000,
};
let sharesState = { kind: "ready", value: { shares: [shot] } };
const post = {
  id: "p1", title: "Overview", slug: "overview", excerpt: "原文 giữ nguyên",
  category: "Public", tags: ["नमस्ते"], status: "published", visibility: "public",
  content: [{ type: "paragraph", text: "Nội dung gốc {count}" }], updatedAt: 1788912000000,
};
const series = { id: "s1", title: "Series 原文", slug: "series", description: "मूल विवरण", status: "active", updatedAt: post.updatedAt };
let postsState = { kind: "ready", value: { posts: [post] } };
let seriesState = { kind: "ready", value: { series: [series] } };
let postState = { kind: "ready", value: { post } };
let singleSeriesState = { kind: "ready", value: { series } };
let accountState = { kind: "ready", value: { csrf: "a".repeat(32) } };
let hooks;
const router = { replace() {}, refresh() {} };

function editorHooks() {
  const slots = [];
  let cursor = 0;
  const effects = [];
  return {
    begin() { cursor = 0; },
    flush() { for (const effect of effects.splice(0)) effect(); },
    useState(initial) {
      const index = cursor++;
      if (!(index in slots)) slots[index] = typeof initial === "function" ? initial() : initial;
      return [slots[index], (value) => { slots[index] = typeof value === "function" ? value(slots[index]) : value; }];
    },
    useRef(initial) {
      const index = cursor++;
      if (!(index in slots)) slots[index] = { current: initial };
      return slots[index];
    },
    useEffect(effect) { effects.push(effect); },
    useMemo(make) { return make(); },
    useCallback(callback) { return callback; },
  };
}

// Render real components with deterministic API data, without a browser or auth session.
function load(path) {
  const url = new URL(path, import.meta.url);
  if (cache.has(url.href)) return cache.get(url.href);
  const source = readFileSync(url, "utf8");
  const { outputText } = ts.transpileModule(source, {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
  });
  const compiled = { exports: {} };
  const resolve = (id) => {
    if (id === "react") return {
      ...React,
      ...Object.fromEntries(["useState", "useRef", "useEffect", "useMemo", "useCallback"].map((name) => [name, (...args) => (hooks ?? React)[name](...args)])),
    };
    if (id.endsWith(".css")) return { __esModule: true, default: new Proxy({}, { get: (_, key) => String(key) }) };
    if (id === "@/lib/i18n/provider" || id === "@codelocal/i18n/provider") return {
      LanguageSelect: () => null,
      useTranslations: () => ({
        locale,
        t: (key, values) => dictionary.translate(locale, key, values),
        message: (text, values) => dictionary.translateKnownMessage(locale, text, values),
      }),
    };
    if (id === "@/lib/i18n/server") return {
      getLocale: async () => locale,
      getTranslations: async () => (key, values) => dictionary.translate(locale, key, values),
    };
    if (id === "@/lib/screenshot-share-server") return { getScreenshotShareForRender: async () => shot };
    if (id === "next/navigation") return { useSearchParams: () => new URLSearchParams(), useRouter: () => router };
    if (id.endsWith("/use-dashboard-resource")) return {
      useDashboardResource: (endpoint) => ({
        state: endpoint === "/api/v1/account" ? accountState
          : endpoint === "/api/v1/blog/posts" ? postsState
          : endpoint === "/api/v1/blog/series" ? seriesState
          : endpoint === "/api/v1/blog/posts/p1" ? postState
          : endpoint === "/api/v1/blog/series/s1" ? singleSeriesState
          : endpoint === "/api/v1/shots" ? sharesState : { kind: "ready", value: endpoint === "/api/v1/workspaces" ? { items: [workspace] } : endpoint === "/api/v1/knowledge/health" ? health : graph },
      }),
    };
    if (id.endsWith("/dashboard-resource-feedback")) return { DashboardResourceFeedback: () => null };
    if (id.startsWith("@/")) return load(`../../${id.slice(2)}.ts`);
    if (id.startsWith(".")) {
      const base = new URL(id, url);
      for (const extension of [".tsx", ".ts"]) {
        try { return load(`${fileURLToPath(base)}${extension}`); }
        catch (error) { if (error.code !== "ENOENT") throw error; }
      }
    }
    return require(id);
  };
  new Function("require", "module", "exports", outputText)(resolve, compiled, compiled.exports);
  cache.set(url.href, compiled.exports);
  return compiled.exports;
}

const dictionary = load("../../lib/i18n/messages.ts");
const { CodeGraphView } = load("./code-graph/code-graph-view.tsx");
const { KnowledgeGraphView } = load("./knowledge/knowledge-graph-view.tsx");
const { LiveCodeGraph } = load("./code-graph/code-graph-live.tsx");
const { NeuralGraphStage } = load("./neural-graph-stage.tsx");
const { LiveKnowledgeHealth } = load("./knowledge/knowledge-health-live.tsx");
const { ShotsHub } = load("./shots/shots-hub.tsx");
const { BlogsHub } = load("./blogs/blogs-hub.tsx");
const { BlogEditor } = load("./blogs/blog-editor.tsx");
const { SeriesEditor } = load("./blogs/series-editor.tsx");
const { ShareActions } = load("../s/[shareID]/share-actions.tsx");
const { default: SharePage, generateMetadata } = load("../s/[shareID]/page.tsx");
const render = (Component, props) => renderToStaticMarkup(React.createElement(Component, props));
const visible = (html, text) => assert.ok(html.includes(renderToStaticMarkup(React.createElement(React.Fragment, null, text))), `Missing ${locale}: ${text}`);
for (locale of ["en", "vi", "zh-Hans", "hi"]) {
  const t = (key, values) => dictionary.translate(locale, key, values);
  const code = render(CodeGraphView, { graph });
  visible(code, t("Code node details"));
  visible(code, t("Close inspector"));
  visible(code, t("Zoom in"));
  visible(code, t("{count} links", { count: 0 }));
  visible(code, "src/नमस्ते.ts");
  visible(code, "Overview");
  visible(code, "12:3");
  visible(code, new Intl.NumberFormat(locale, { style: "percent" }).format(0.8));
  const knowledge = render(KnowledgeGraphView, { graph: { nodes: [], edges: [] } });
  visible(knowledge, t("Search Brain"));
  visible(knowledge, t("No knowledge yet"));
  const live = render(LiveCodeGraph);
  visible(live, t("Latest snapshot"));
  visible(live, t("Affected files"));
  visible(live, t("High"));
  visible(live, t("Depth {count}", { count: 3 }));
  visible(live, workspace.workspaceName);
  visible(live, workspace.deviceName);
  assert.ok(!live.includes(">Live<"));
  visible(render(NeuralGraphStage, { nodes: [], edges: [], ariaLabel: "Graph" }), t("No graph data"));
  const healthHTML = render(LiveKnowledgeHealth);
  visible(healthHTML, t("Health & controls"));
  visible(healthHTML, t("Degraded"));
  visible(healthHTML, t("Stale"));
  visible(healthHTML, t("Unavailable"));
  visible(healthHTML, t("Private opt-in. You can disable contribution at any time; CodeLocal removes your contribution aggregates when you opt out."));
  visible(healthHTML, t("Rollout: {status}. A cohort requires at least {count} independent contributors.", { status: t("Available"), count: 10 }));
  visible(healthHTML, t("Raw code shared: {value}", { value: t("No") }));
  visible(healthHTML, "go test ./...");
  visible(healthHTML, new Intl.NumberFormat(locale, { style: "unit", unit: "minute", unitDisplay: "short" }).format(1));
  assert.ok(!healthHTML.includes("GO AUTHORITY"));
  sharesState = { kind: "ready", value: { shares: [shot] } };
  const shots = render(ShotsHub);
  visible(shots, t("Choose image"));
  visible(shots, t("Anyone with this link can view the shot."));
  visible(shots, t("Revoke public link"));
  visible(shots, t("{count} active", { count: 1 }));
  visible(shots, shot.url);
  assert.ok(!shots.includes("Designed for sharing"));
  sharesState = { kind: "ready", value: { shares: [] } };
  visible(render(ShotsHub), t("No public links yet."));
  sharesState = { kind: "loading" };
  const loadingShots = render(ShotsHub);
  visible(loadingShots, t("Loading your shared screenshots…"));
  assert.ok(!loadingShots.includes(t("{count} active", { count: 0 })));
  sharesState = { kind: "unauthenticated" };
  visible(render(ShotsHub), t("Sign in again to manage your links."));
  visible(render(ShareActions, { shareURL: shot.url, downloadURL: shot.downloadUrl }), t("Download"));
  const page = renderToStaticMarkup(await SharePage({ params: Promise.resolve({ shareID: shot.id }) }));
  visible(page, t("Anyone with the link can view"));
  visible(page, t("This page is excluded from search engines. The owner can revoke access at any time."));
  assert.ok(!page.includes("Shared privately"));
  const metadata = await generateMetadata({ params: Promise.resolve({ shareID: shot.id }) });
  assert.equal(metadata.title, t("Shared screenshot"));
  assert.equal(metadata.robots.index, false);
  assert.equal(metadata.robots.follow, false);
  assert.equal(metadata.alternates.canonical, shot.url);
  postsState = { kind: "ready", value: { posts: [post] } };
  seriesState = { kind: "ready", value: { series: [series] } };
  const blogs = render(BlogsHub);
  for (const key of ["Posts", "Search posts", "Published", "Create draft", "Create series"]) visible(blogs, t(key));
  for (const original of [post.title, post.excerpt, post.category, series.title, series.description]) visible(blogs, original);
  assert.ok(!blogs.includes("Learning paths"));
  postsState = { kind: "ready", value: { posts: [] } };
  seriesState = { kind: "ready", value: { series: [] } };
  visible(render(BlogsHub), t("No posts yet."));
  visible(render(BlogsHub), t("No series yet."));
  postsState = { kind: "loading" };
  seriesState = { kind: "unauthenticated" };
  const unavailable = render(BlogsHub);
  visible(unavailable, t("Loading posts…"));
  visible(unavailable, t("Sign in again to manage your series."));
  const editor = render(BlogEditor, { postID: post.id });
  for (const key of ["Back to blogs", "Post settings", "Title", "Post body", "Public", "Unlisted", "Private", "Unpublish", "Upload cover"]) visible(editor, t(key));
  assert.ok(!editor.includes("CodeLocal stores optimized durable WebP variants."));
  const seriesEditor = render(SeriesEditor, { seriesID: series.id });
  for (const key of ["Back to blogs", "Title", "Description", "Active", "Complete", "Archived", "Archive series", "Posts stay intact; their series membership is removed."]) visible(seriesEditor, t(key));
  assert.ok(!seriesEditor.includes("Build an ordered learning path."));
  postState = { kind: "loading" };
  singleSeriesState = { kind: "loading" };
  visible(render(BlogEditor, { postID: post.id }), t("Loading editor…"));
  visible(render(SeriesEditor, { seriesID: series.id }), t("Loading series…"));
  postState = { kind: "ready", value: { post } };
  singleSeriesState = { kind: "ready", value: { series } };
}

function findElement(element, predicate) {
  if (!React.isValidElement(element)) return undefined;
  if (predicate(element)) return element;
  for (const child of React.Children.toArray(element.props.children)) {
    const found = findElement(child, predicate);
    if (found) return found;
  }
}

// Exercise real editor callbacks; hook fixture does not simulate browser layout or routing.
const originalFetch = globalThis.fetch;
const originalWindow = globalThis.window;
globalThis.window = { setTimeout: () => 0, clearTimeout: () => {} };
try {
  for (const [Editor, props] of [[BlogEditor, { postID: post.id }], [SeriesEditor, { seriesID: series.id }]]) {
    hooks = editorHooks();
    locale = "en";
    const draw = () => { hooks.begin(); return Editor(props); };
    draw();
    hooks.flush();
    let tree = draw();
    const input = findElement(tree, (element) => ["input", "textarea"].includes(element.type) && element.props.value === post.title);
    const titleInput = input ?? findElement(tree, (element) => element.type === "input" && element.props.value === series.title);
    assert.ok(titleInput);
    titleInput.props.onChange({ target: { value: "Bản nháp हिन्दी 中文 {count}" } });
    locale = "vi";
    tree = draw();
    visible(renderToStaticMarkup(tree), "Bản nháp हिन्दी 中文 {count}");
    visible(renderToStaticMarkup(tree), dictionary.translate(locale, "Unsaved"));
    accountState = { kind: "unauthenticated" };
    let calls = 0;
    globalThis.fetch = async () => { calls++; throw new Error("unexpected network"); };
    tree = draw();
    let save = findElement(tree, (element) => element.type === "button" && element.props.children === dictionary.translate(locale, "Save"));
    assert.equal(save.props.disabled, true);
    save.props.onClick();
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(calls, 0);
    accountState = { kind: "ready", value: { csrf: "a".repeat(32) } };
    globalThis.fetch = async (_url, options) => {
      assert.equal(options.headers["X-CSRF-Token"], accountState.value.csrf);
      assert.equal(JSON.parse(options.body).title, "Bản nháp हिन्दी 中文 {count}");
      calls++;
      return new Response("{}", { status: 409 });
    };
    tree = draw();
    save = findElement(tree, (element) => element.type === "button" && element.props.children === dictionary.translate(locale, "Save"));
    save.props.onClick();
    await new Promise((resolve) => setImmediate(resolve));
    assert.equal(calls, 1);
    for (locale of ["vi", "zh-Hans", "hi"]) {
      const html = renderToStaticMarkup(draw());
      visible(html, "Bản nháp हिन्दी 中文 {count}");
      visible(html, dictionary.translate(locale, Editor === BlogEditor ? "That URL slug or series part is already in use." : "That series URL is already in use."));
    }
    hooks = undefined;
  }
} finally {
  hooks = undefined;
  globalThis.fetch = originalFetch;
  globalThis.window = originalWindow;
}
assert.equal(dictionary.translate("en", "{count} nodes", { count: 1 }), "1 node");
console.log("Dashboard checks passed in four languages: graphs, health, Shots, blogs, editor draft preservation and localized save failures.");

const tick = () => new Promise((resolve) => setImmediate(resolve));
const { SkillsHub } = load("./skills/skills-hub.tsx");
try {
  locale = "vi";
  hooks = editorHooks();
  const draw = () => { hooks.begin(); return SkillsHub(); };
  globalThis.fetch = async (url) => Response.json(url.endsWith("/account")
    ? { csrf: "a".repeat(32), isAdmin: false }
    : { items: [], storageConfigured: true, autoUse: true });
  draw();
  hooks.flush();
  await tick();
  let calls = 0;
  globalThis.fetch = async (_url, init) => {
    if (init?.method === "POST") {
      calls++;
      assert.equal(init.headers["X-CSRF-Token"], "a".repeat(32));
      return Response.json({});
    }
    throw new TypeError("offline");
  };
  findElement(draw(), (node) => node.type === "input" && node.props.type === "file").props.onChange({
    target: { value: "", files: [{ text: async () => "{}" }] },
  });
  await tick();
  assert.equal(calls, 1);
  for (locale of ["en", "vi", "zh-Hans", "hi"]) {
    const html = renderToStaticMarkup(draw());
    visible(html, dictionary.translate(locale, "Change saved, but Skills could not refresh. Reload before making another change."));
    assert.ok(!html.includes(dictionary.translate(locale, "Personal Skill imported.")));
    assert.equal(findElement(draw(), (node) => node.type === "button" && node.props.children === dictionary.translate(locale, "Import personal")).props.disabled, true);
  }
} finally {
  hooks = undefined;
  globalThis.fetch = originalFetch;
}
console.log("Skills: saved mutation with failed refresh shows localized warning, never false success.");

const { useUnsavedWarning } = load("./blogs/use-unsaved-warning.ts");
function UnsavedWarningProbe() {
  useUnsavedWarning(true);
}
const originalDocument = globalThis.document;
const originalElement = globalThis.Element;
const originalAnchor = globalThis.HTMLAnchorElement;
try {
  class Element {
    closest() { return this; }
  }
  class Anchor extends Element {
    constructor(href, target = "") { super(); this.href = href; this.target = target; }
    hasAttribute() { return false; }
  }
  globalThis.Element = Element;
  globalThis.HTMLAnchorElement = Anchor;
  const listeners = {};
  globalThis.window = {
    location: { href: "https://codelocal.cloud/dashboard/blogs/p1" },
    addEventListener(name, callback) { listeners[name] = callback; },
    removeEventListener() {},
    confirm() { return false; },
  };
  globalThis.document = {
    addEventListener(name, callback) { listeners[name] = callback; },
    removeEventListener() {},
  };
  hooks = editorHooks();
  hooks.begin(); UnsavedWarningProbe(); hooks.flush();
  let prevented = false;
  listeners.beforeunload({ preventDefault() { prevented = true; } });
  assert.equal(prevented, true);
  const click = (href, target = "", ctrlKey = false) => {
    let blocked = false;
    listeners.click({
      button: 0, ctrlKey, target: new Anchor(href, target),
      preventDefault() { blocked = true; }, stopPropagation() {},
    });
    return blocked;
  };
  assert.equal(click("https://codelocal.cloud/dashboard"), true);
  assert.equal(click("https://codelocal.cloud/dashboard/blogs/p1#body"), false);
  assert.equal(click("https://codelocal.cloud/dashboard", "_blank"), false);
  assert.equal(click("https://codelocal.cloud/dashboard", "", true), false);
  globalThis.window.confirm = () => true;
  assert.equal(click("https://codelocal.cloud/dashboard"), false);
} finally {
  hooks = undefined;
  globalThis.window = originalWindow;
  globalThis.document = originalDocument;
  globalThis.Element = originalElement;
  globalThis.HTMLAnchorElement = originalAnchor;
}
console.log("Unsaved drafts: page unload and same-tab links warn; anchors and new-tab links remain available.");

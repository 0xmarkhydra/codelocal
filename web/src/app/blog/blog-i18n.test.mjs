import assert from "node:assert/strict";
import { readFileSync } from "node:fs";
import { createRequire } from "node:module";
import React from "react";
import { renderToStaticMarkup } from "react-dom/server";
import ts from "typescript";

const require = createRequire(import.meta.url);
const cache = new Map();
let locale = "en";
let missing = false;
const post = {
  slug: "original", title: "Overview", excerpt: "原文 {count}", category: "Public",
  tags: ["नमस्ते"], publishedAt: "2026-09-09", readingMinutes: 1, official: true,
  featured: true, author: { name: "क्षमा", slug: "author", role: "Original role" },
  series: { slug: "series", part: 1 },
  blocks: [{ type: "paragraph", text: "Original {name}" }, { type: "code", code: "codelocal .", language: "sh" }],
};
const series = { slug: "series", title: "Series 原文", description: "मूल विवरण", status: "active" };
const server = {
  decodeBlogRouteSlug: decodeURIComponent,
  getBlogPostsForRender: async () => [post],
  getBlogSeriesForRender: async () => [series],
  getBlogCategoriesForRender: async () => [post.category],
  getBlogTagsForRender: async () => post.tags,
  getPostsByCategorySlugForRender: async () => missing ? [] : [post],
  getPostsByTagSlugForRender: async () => [post],
  getBlogPostsByAuthorForRender: async () => missing ? [] : [post],
  getBlogPostForRender: async () => missing ? undefined : post,
  getBlogSeriesPageForRender: async () => missing ? undefined : { series, posts: [post] },
  getRelatedPostsForRender: async () => [],
};

// Resolve server components recursively; client controls remain explicit test boundaries.
function load(url) {
  if (cache.has(url.href)) return cache.get(url.href);
  const { outputText } = ts.transpileModule(readFileSync(url, "utf8"), {
    compilerOptions: { module: ts.ModuleKind.CommonJS, jsx: ts.JsxEmit.ReactJSX, target: ts.ScriptTarget.ES2022 },
  });
  const compiled = { exports: {} };
  const resolve = (id) => {
    if (id.endsWith(".css")) return { default: new Proxy({}, { get: (_, key) => key }) };
    if (id === "next/link") return { default: ({ children, ...props }) => React.createElement("a", props, children) };
    if (id === "next/image") return { default: ({ priority, unoptimized, ...props }) => {
      void priority; void unoptimized;
      return React.createElement("img", props);
    } };
    if (id === "next/navigation") return {
      notFound() { throw new Error("NOT_FOUND"); },
      permanentRedirect(path) { throw new Error(`REDIRECT:${path}`); },
    };
    if (id === "@/lib/blog-server") return server;
    if (id === "@/lib/i18n/provider") return { LanguageSelect: () => React.createElement("select", { "aria-label": "Language" }) };
    if (id === "@/lib/i18n/server") return {
      getLocale: async () => locale,
      getTranslations: async () => (key, values) => dictionary.translate(locale, key, values),
    };
    if (id.startsWith("@/")) return load(new URL(`../../${id.slice(2)}.ts`, import.meta.url));
    if (id.startsWith(".")) {
      for (const ext of [".tsx", ".ts"]) {
        try { return load(new URL(`${id}${ext}`, url)); }
        catch (error) { if (error.code !== "ENOENT") throw error; }
      }
    }
    return require(id);
  };
  new Function("require", "module", "exports", outputText)(resolve, compiled, compiled.exports);
  cache.set(url.href, compiled.exports);
  return compiled.exports;
}

async function resolveTree(node) {
  node = await node;
  if (Array.isArray(node)) return Promise.all(React.Children.toArray(node).map(resolveTree));
  if (!React.isValidElement(node)) return node;
  if (typeof node.type === "function") {
    const result = await resolveTree(node.type(node.props));
    return React.isValidElement(result) ? React.cloneElement(result, { key: node.key }) : result;
  }
  if (!Object.hasOwn(node.props, "children")) return node;
  return React.cloneElement(node, {}, await resolveTree(node.props.children));
}

const dictionary = load(new URL("../../lib/i18n/messages.ts", import.meta.url));
const blog = load(new URL("../../lib/blog.ts", import.meta.url));
const components = load(new URL("./_components.tsx", import.meta.url));
const render = async (element) => renderToStaticMarkup(await resolveTree(element));
const visible = (html, text) => assert.ok(html.includes(renderToStaticMarkup(React.createElement(React.Fragment, null, text))), `${locale}: ${text}`);
const routes = [
  ["../blogs/page.tsx", { searchParams: Promise.resolve({}) }, "All articles"],
  ["../blogs/category/[slug]/page.tsx", { params: Promise.resolve({ slug: "public" }) }, "Articles"],
  ["../blogs/tag/[slug]/page.tsx", { params: Promise.resolve({ slug: blog.taxonomySlug(post.tags[0]) }) }, "Tagged articles"],
  ["../blogs/series/page.tsx", {}, "All series"],
  ["../blogs/series/[slug]/page.tsx", { params: Promise.resolve({ slug: "series" }) }, "In this series"],
  ["../blogs/[slug]/page.tsx", { params: Promise.resolve({ slug: "original" }) }, "About this article"],
  ["../users/[authorID]/page.tsx", { params: Promise.resolve({ authorID: "author" }) }, "Published articles"],
];
for (locale of ["en", "vi", "zh-Hans", "hi"]) {
  const t = (key, values) => dictionary.translate(locale, key, values);
  for (const [path, props, label] of routes) {
    const route = load(new URL(path, import.meta.url));
    const html = await render(React.createElement(route.default, props));
    visible(html, t(label));
    visible(html, post.title);
    if (path.startsWith("../blogs/")) {
      const legacy = load(new URL(path.replace("../blogs/", "./"), import.meta.url));
      assert.equal(legacy.default, route.default, `Legacy route diverged: ${path}`);
    }
  }
  const card = await render(React.createElement(components.PostCard, { post }));
  for (const original of [post.title, post.excerpt, post.category, post.tags[0]]) visible(card, original);
  visible(card, t("{count} min read", { count: 1 }));
  visible(card, blog.formatBlogDate(post.publishedAt, locale));
  assert.equal(blog.formatBlogDate(post.publishedAt, locale), new Intl.DateTimeFormat(locale, {
    year: "numeric", month: "short", day: "numeric", timeZone: "UTC",
  }).format(new Date("2026-09-09T00:00:00Z")));
  const about = await render(React.createElement(components.ArticleAbout, { post }));
  visible(about, "क्ष");
  visible(about, post.author.name);
  visible(about, post.author.role);
  const article = load(new URL("../blogs/[slug]/page.tsx", import.meta.url));
  const props = { params: Promise.resolve({ slug: post.slug }) };
  const html = await render(React.createElement(article.default, props));
  visible(html, "Original {name}");
  visible(html, "codelocal .");
  assert.ok(html.includes('aria-current="page"'));
  const metadata = await article.generateMetadata(props);
  assert.equal(metadata.title, post.title);
  assert.equal(metadata.alternates.canonical, "/blogs/original");
  missing = true;
  assert.equal((await article.generateMetadata(props)).title, t("Article not found"));
  await assert.rejects(article.default(props), /NOT_FOUND/);
  missing = false;
  const query = "<script>alert(1)</script>";
  const empty = await render(React.createElement(components.EmptyState, { query }));
  visible(empty, t("Nothing matched “{query}”. Try a broader search.", { query }));
  assert.ok(!empty.includes(query));
}
assert.equal(dictionary.translate("en", "{count} articles", { count: 1 }), "1 article");
assert.equal(dictionary.translate("en", "{count} parts", { count: 1 }), "1 part");
console.log("Public Blog: seven routes render in four languages; originals, dates, metadata and escaping preserved.");

for (locale of ["en", "vi", "zh-Hans", "hi"]) {
  const t = (key, values) => dictionary.translate(locale, key, values);
  for (const [slug, title, sectionCount] of [
    ["privacy", "Privacy Policy", 5], ["security", "Security", 4],
    ["terms", "Terms of Use", 5], ["support", "Support", 3],
  ]) {
    const route = load(new URL(`../${slug}/page.tsx`, import.meta.url));
    const html = await render(React.createElement(route.default));
    visible(html, t(title));
    visible(html, t("Publisher: CodeLocal.Cloud"));
    const date = new Intl.DateTimeFormat(locale, { year: "numeric", month: "long", day: "numeric", timeZone: "UTC" }).format(new Date("2026-08-19T00:00:00Z"));
    visible(html, t("Last updated: {date}", { date }));
    assert.match(html, /datetime="2026-08-19"/i);
    assert.equal((html.match(/<article /g) ?? []).length, sectionCount);
    assert.deepEqual(await route.generateMetadata(), { title: t(title), alternates: { canonical: `/${slug}` } });
    // Every authored translation must survive async page and shared-shell rendering.
    const source = readFileSync(new URL(`../${slug}/page.tsx`, import.meta.url), "utf8");
    for (const match of source.matchAll(/\bt\("([^"]+)"\)/g)) visible(html, t(match[1]));
    if (slug === "support") {
      for (const command of ["codelocal --version", "codelocal", "codelocal ."]) assert.ok(html.includes(`<code>${command}</code>`));
      visible(html, process.env.CODELOCAL_SUPPORT_EMAIL?.trim() || "support@codelocal.cloud");
      visible(html, process.env.CODELOCAL_SECURITY_EMAIL?.trim() || "security@codelocal.cloud");
    }
  }
}
console.log("Public documents: four routes preserve sections, commands, contact details and policy dates in four languages.");

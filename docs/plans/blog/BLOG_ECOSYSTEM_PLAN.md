# CodeLocal Blog Ecosystem Plan

## Goal

Build a public content system that supports both standalone articles and ordered series without coupling the web product to a heavy CMS on day one.

The public URL and content model should remain stable if the content source later moves from the repository to the CodeLocal backend or an external CMS.

## BA: three primary cases

### Case 1 — Standalone article

A writer publishes one article that does not belong to a series.

Requirements:

- Stable `/blog/:slug` URL.
- Title, excerpt, author, publish/update dates and reading time.
- Category and multiple tags.
- Structured article body without unsafe raw HTML.
- Related-article discovery.
- SEO metadata and sitemap discovery.

### Case 2 — Ordered series

A writer publishes multiple articles that form a deliberate learning path.

Requirements:

- Series is a first-class entity, not a tag convention.
- Stable `/blog/series/:slug` landing page.
- Explicit part number on every member article.
- Previous/next navigation.
- Series progress visible from the article page.
- Series status supports `active` and `complete`.
- A series may contain articles from different categories while retaining one series identity.

### Case 3 — Blog ecosystem discovery

A reader does not know the exact article they want and needs to explore the content graph.

Requirements:

- `/blog` discovery home.
- Search by title, excerpt, category and tags.
- Featured articles.
- Series directory.
- Category archives.
- Tag archives.
- Related posts.
- Sitemap + robots discovery for search engines.

## V1 architecture

### Content model

`web/src/lib/blog.ts` is the V1 content registry and query layer.

Entities:

- `BlogPost`
- `BlogSeries`
- `BlogAuthor`
- `BlogBlock`

The UI imports selectors instead of reading the backing array directly whenever it needs derived data. This keeps the route layer ready for a later storage adapter.

### Rendering

- Next.js App Router.
- Server Components by default.
- No new runtime dependencies.
- No client-side content state required for V1.
- Search uses a normal GET query (`/blog?q=...`) so results remain shareable and progressive-enhancement friendly.
- Article bodies use typed blocks instead of rendering arbitrary HTML.

### Route map

```text
/blog
/blog/:slug
/blog/series
/blog/series/:slug
/blog/category/:slug
/blog/tag/:slug
/sitemap.xml
/robots.txt
```

## Editorial workflow in V1

1. Add an author if needed.
2. Add or reuse a series.
3. Add a `BlogPost` entry.
4. For a series article, set `{ slug, part }` in `post.series`.
5. Verify the blog routes with `npm run check` from `web/`.
6. Merge through normal review/CI.

## Guardrails

- Slugs must be unique and stable after publication.
- Part numbers within one series must be unique.
- Do not expose secrets, internal-only implementation details or unverified product claims in public content.
- Do not render arbitrary HTML supplied by authors.
- Public blog reads must remain independent from authenticated dashboard state.

## V2 — editorial backend

Move the registry behind a `BlogRepository`-style adapter when non-developer authoring becomes necessary.

Suggested capabilities:

- Draft / scheduled / published / archived states.
- Rich editor with preview.
- Revision history.
- Authors and roles.
- Series drag-and-drop ordering with validation.
- Cover/social images.
- Per-post canonical/OG fields.
- Scheduled publishing.
- RSS/Atom feed.
- Search index when content volume outgrows in-memory filtering.

The V1 public routes and view components should not need to change when this storage migration happens.

## V3 — content intelligence

After enough content exists, connect Blog to the wider CodeLocal knowledge ecosystem:

- Semantic related posts.
- “Continue learning” paths across multiple series.
- Product docs ↔ blog cross-links.
- Changelog/release-note content type.
- AI-assisted draft generation with human publish approval.
- Automatic internal-link suggestions.
- Search intent and content-gap analytics.

## Definition of done for V1

- Standalone articles render correctly.
- Ordered series render correctly.
- Series prev/next works.
- Search works without JavaScript-specific state.
- Category and tag archives resolve.
- Related posts are generated deterministically.
- `sitemap.xml` includes blog discovery routes.
- `robots.txt` points to the sitemap.
- Responsive layout works on desktop and mobile.
- Lint, typecheck and production build pass.

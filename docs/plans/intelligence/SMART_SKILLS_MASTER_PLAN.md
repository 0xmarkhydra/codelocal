# Smart Skills Master Plan

## Goal

Make CodeLocal Skills usable by a lazy user: paste a link, drop files/folders, paste text, or reuse an existing `.skill.json`; CodeLocal detects the source, normalizes it into a safe Personal Knowledge Skill, and makes it immediately available to the same tenant-aware Skill runtime used by Dashboard + MCP.

This work must preserve the current Skill Intelligence invariants:

- reusable Skill scopes remain **System / Personal / Community**;
- project-specific facts and conventions stay in **Project Brain**; there is no Project Skill scope;
- Skills never add a new public MCP tool;
- explicit user instruction and current Project Brain evidence outrank Skill recommendations;
- user imports never self-grant System, Verified, runtime, shell, network, browser, credentials, destructive, or project-write authority;
- Community publishing stays explicit and continues through candidate → evaluation → canary → promoted;
- runtime-capable execution continues to use existing CodeLocal authorization.

## Product model

### Case A — Universal Add

One user entry point: **Add Skill**.

Supported inputs, progressively:

1. existing `.skill.json` package;
2. one or many text knowledge files (`.md`, `.txt`, `.json`, `.yaml`, `.yml`, `.csv`);
3. folder upload (browser sends the allowed text documents, preserving relative paths);
4. ZIP archive (server extracts allowed text documents only, with traversal and size limits);
5. pasted text;
6. public URL / raw docs;
7. public GitHub repository or repository subpath;
8. later: GitLab/Bitbucket/private Git through authenticated source adapters;
9. later: AI-authored Skill from a natural-language description.

The default disposition is **Personal + Knowledge + Auto**. The user does not choose package/manifest internals.

### Case B — Personal Skill without S3 dependency

Personal Skills must work when `CODELOCAL_SKILL_STORAGE_*` is absent.

Storage resolution:

1. configured S3/R2/MinIO object store when available;
2. PostgreSQL content-addressed package store as durable fallback;
3. existing bounded in-process package cache above the durable store.

Community/System package lifecycle can use the same durable abstraction. Object storage remains recommended for large production catalogs, but a missing bucket must not disable Personal Skill UX.

### Case C — Learned Skills

Reuse the existing `internal/learnedskills` execution-learning engine; do not create a second learning system.

Product rules:

- Project Brain stores project facts/conventions.
- Learned Skills store reusable execution recipes/patterns proven by successful work.
- keep current Candidate → Trusted / confidence / context-fingerprint / stale-detection semantics;
- surface Learned Skills in the Skills UI after the Cloud/runtime metadata contract is unified;
- later allow a trusted portable learned recipe to be promoted to Personal Skill explicitly or by safe policy;
- never auto-publish learned data to Community.

## Architecture

```text
Dashboard / Chat intent
        |
        v
Universal Skill Ingest API
        |
        +--> package passthrough (.skill.json)
        |
        +--> documents/text/archive/url adapter
                    |
                    v
          BuildArtifactFromDocuments
                    |
                    v
         Personal Knowledge manifest
                    |
                    v
               BuildPackage
                    |
                    v
          SkillImportService.ImportPersonal
                    |
                    v
      durable PackageStore + registry/channel
                    |
                    v
           tenant-aware Skill Runtime
          (Dashboard + MCP same catalog)
```

## Phase 1 — Durable Personal fallback

- add migration for immutable `codelocal_skill_packages` package blobs;
- implement PostgreSQL `SkillPackageObjectStore` with package-integrity verification;
- make `skillServicesForServer` prefer S3 when configured and otherwise use Postgres;
- expose storage mode in the management resource without blocking Add Skill;
- keep bounded package cache.

Acceptance:

- Personal import succeeds with no `CODELOCAL_SKILL_STORAGE_BUCKET`;
- imported package survives process restart;
- MCP/Cloud runtime can load the same package hash;
- invalid/corrupt packages are rejected on write and read.

## Phase 2 — Universal ingestion API

Add authenticated + CSRF-protected `POST /api/v1/skills/ingest`.

Input supports:

- `package` for backward-compatible `.skill.json`;
- `documents[]` for browser files/folder/paste;
- `archiveBase64` for ZIP;
- `sourceUrl` for bounded public source ingestion;
- optional display `name`.

Server behavior:

- derive canonical Personal Skill identity/version;
- force `scope=personal`, `kind=knowledge`, `verified=false`, zero capabilities;
- use existing deterministic bounded document ingestion;
- never execute imported source;
- import through the existing immutable package/registry path.

Security:

- request/package/document/archive size limits;
- ZIP path traversal and decompression-bomb limits;
- URL `https` only;
- reject credentials in URL;
- block loopback/link-local/private network destinations and unsafe redirects;
- allow only text response types / bounded bytes;
- repository adapters fetch source snapshots only and never execute repository code.

## Phase 3 — Lazy-user Dashboard UX

Replace separate visible `Import Personal` / `Publish Community` actions with one primary **Add Skill** inbox.

Inbox supports:

- paste GitHub/docs URL;
- paste text;
- drag/drop files;
- upload multiple files;
- choose folder;
- existing `.skill.json` remains accepted automatically.

After import:

- switch to My Skills;
- show a compact success notice;
- default mode is Auto;
- Community publishing moves to a secondary per-Skill action in a later UI tranche, rather than being a first-step decision.

Admin moderation controls remain separate.

## Phase 4 — Public repository adapters

Start with public GitHub:

- GitHub repo URL → pinned archive snapshot → extract allowed knowledge files;
- GitHub `blob`/raw URL → fetch that document;
- GitHub `tree` subpath → ingest only the selected subtree when resolvable.

Then add GitLab/Bitbucket through the same source adapter interface.

Never clone/execute arbitrary repository hooks on the Cloud gateway.

## Phase 5 — Learned Skills product integration

- expose user-safe Learned Skill metadata through authenticated Skills resource/API;
- add `Learned` UI view with confidence, success count, context scope, stale state;
- support Disable/Forget and explicit `Make available everywhere` only when the recipe is portable and trusted;
- feed corrections/failures back into the existing learned-skill evidence model;
- do not duplicate Project Brain facts into Learned Skills.

## Phase 6 — Chat-native ingestion

No public MCP `skills` tool.

The orchestration layer should recognize intents such as:

- “use this skill https://github.com/…”
- “turn this file into a skill”
- “remember this workflow for next time”

and call the same internal ingestion/learning services behind the existing compact MCP surface.

## Verification gates

For each tranche:

- `git diff --check` equivalent / patch sanity;
- targeted Go tests for storage, ingestion, archive and URL security;
- existing Skill import/runtime tests;
- web lint + typecheck + production build;
- verify no new public MCP tool/schema;
- verify Knowledge imports contain zero capabilities;
- verify System/Verified cannot be granted by user ingestion;
- verify Community routing still requires trusted evaluation evidence;
- verify Personal imported package is routable by the tenant-aware MCP runtime;
- manual dashboard flow: URL, Markdown, folder, ZIP, existing `.skill.json`.

## Rollout order

1. durable package fallback;
2. universal document/package ingest;
3. Add Skill inbox UI;
4. public URL/GitHub adapter;
5. Learned Skills UI/promotion;
6. chat-native ingestion/private Git/AI authoring.

This order deliberately makes the simplest user path functional before expanding source adapters, while keeping all existing Skill security/lifecycle boundaries intact.

# CodeLocal Plugin Platform — Master Plan

## 0. Decision

CodeLocal will build a first-class Plugin Platform inspired by the current ChatGPT plugin/app model, adapted to the existing MCP Hub, Skills, Project Brain, approval and local-runtime architecture.

Target product model:

```text
Plugin
├── Apps
│   ├── MCP connections and tools
│   ├── authentication requirements
│   └── future interactive UI resources/widgets
├── Skills
│   ├── knowledge
│   ├── workflows
│   └── runtime/hybrid skills
├── App Templates
└── metadata / permissions / trust / version / distribution
```

A Plugin is the installable/discoverable product unit. An App is the external data/action integration unit. A Skill is the reusable instruction/knowledge/workflow unit. An App Template is a reusable configuration for creating an App instance.

The existing `internal/mcphub` remains the local execution adapter for MCP servers. The existing `internal/skills` remains the skill engine. The Plugin domain sits above both and coordinates catalog, install state, connection state, permissions, versions, routing and UI.

## 1. BA — three cases

### Case A — Plugin = MCP server alias

Fastest, but too narrow. It cannot cleanly bundle Skills/Templates, gives a weak marketplace/developer model and couples product metadata directly to runtime configuration.

Decision: reject as the final architecture. Keep only as an import compatibility path.

### Case B — Plugin = App + auth + permissions + metadata

Good user-facing MVP: directory, install/connect, permissions and MCP-backed actions. But it leaves Skills outside the product bundle.

Decision: use as the MVP delivery slice, not the final domain model.

### Case C — Full Plugin Platform

Plugin is a versioned bundle containing Apps, Skills and Templates, with catalog, install state, permissions, review/trust, routing and future chat UI.

Decision: chosen target architecture. Build Case C's schema/boundaries and ship Case B's user-facing flow first.

## 2. Existing CodeLocal pieces to reuse

Do not rebuild these:

- `internal/mcphub`: MCP add/remove/list/info/probe/search/call, cached catalog, HTTP/stdio transports, env/header references and connect guard.
- `internal/skills`: immutable manifests, knowledge/workflow/runtime/hybrid kinds, capabilities, import/routing/policy.
- `internal/cloud`: Skill versions/channels/user states/evaluations/ratings provide the release-governance pattern.
- `internal/oauth`: authenticates external MCP clients to CodeLocal; keep separate from third-party App OAuth.
- existing workspace authorization, approval memory and security policy remain the execution source of truth.
- `web/`: Next.js Dashboard is the destination UI surface.

## 3. Non-goals

The first release will not expose every plugin tool as a new public CodeLocal MCP tool, bypass CodeLocal approvals, store plaintext tokens in manifests, execute arbitrary marketplace code in Cloud, duplicate the Skills registry, duplicate MCP Hub, or treat install-time consent as blanket write/delete approval.

## 4. Core architectural rules

1. Plugin is product metadata, not a new execution engine.
2. Apps delegate execution to MCP Hub or future bounded connector adapters.
3. Skills delegate to the existing Skills engine.
4. Plugin permissions are declarative requirements; final action authorization stays in CodeLocal security/approval policy.
5. Chat never receives every installed tool schema. It ranks Plugins/Components first, fetches an exact tool schema only when needed, then executes.
6. Install and connect are separate states: `available -> installed -> connection_required -> connected -> enabled`.
7. Published plugin versions are immutable; stable/canary are pointers.
8. Unknown component/capability/auth/transport values fail closed.
9. Secrets are handles/references only; never model-visible manifest values.

## 5. New Go domain

Create a cohesive package:

```text
internal/plugins/
  types.go
  manifest.go
  validation.go
  registry.go
  installer.go
  resolver.go
  permissions.go
  runtime_binding.go
  catalog.go
```

P0 implements only the pure contract files: types, manifest hashing, validation and tests. Runtime/persistence coupling comes later.

## 6. Manifest v1

Canonical shape:

```json
{
  "schemaVersion": 1,
  "id": "github-productivity",
  "name": "GitHub Productivity",
  "version": "1.0.0",
  "publisher": {"id":"github","name":"GitHub","verified":true},
  "description": "Work with repositories, issues and pull requests.",
  "components": [
    {
      "kind": "app",
      "id": "github",
      "app": {
        "transport": "mcp_http",
        "endpoint": "https://example.com/mcp",
        "auth": {"kind":"oauth2"},
        "capabilities": ["external_read","external_write"]
      }
    },
    {
      "kind": "skill",
      "id": "github-pr-review",
      "skill": {"skillId":"github-pr-review","version":"1.0.0"}
    }
  ],
  "scope": "community",
  "distribution": "public",
  "privacyPolicyUrl": "https://example.com/privacy",
  "termsUrl": "https://example.com/terms"
}
```

Secrets, access tokens, refresh tokens and API keys are deliberately absent from the schema.

## 7. Component types

### App

Initial transports:

- `mcp_http` for remote MCP Apps.
- `mcp_stdio_local` only for personal/internal developer mode.

App metadata declares auth kind, capabilities and optional tool namespace. Public remote MCP endpoints require HTTPS.

### Skill

References an immutable existing CodeLocal Skill version. Plugin install creates an association/enablement state; it does not duplicate the Skill artifact or registry.

### App Template

A reusable factory for configuring an App instance, such as self-hosted GitLab or internal Jira. Fields can be marked secret, but templates store only field definitions; secret values live in a credential provider.

## 8. Capability model

Initial stable vocabulary:

```text
project_read
project_write
local_shell
local_browser
local_credentials
external_read
external_write
external_delete
network
computer_control
background_task
```

MCP annotations such as `readOnlyHint` remain advisory metadata, not authorization proofs.

## 9. Persistence phases

Append domain-owned migrations after the current schema tail. Do not retrofit Plugins into Skill tables.

Planned tables:

- `codelocal_plugin_versions`: immutable manifest/version records and moderation state.
- `codelocal_plugin_channels`: stable/canary pointers.
- `codelocal_plugin_installations`: account/workspace install scope, enabled state and version policy.
- `codelocal_plugin_app_connections`: non-secret connection metadata and opaque credential references.
- `codelocal_plugin_grants`: install-level allowed/ask/denied preferences by capability; not final action authorization.
- `codelocal_plugin_evaluations`: review/eval results.
- `codelocal_plugin_ratings`: user reputation signal.

Suggested version states mirror proven Skills governance: draft/candidate/evaluating/canary/promoted/rejected/blocked/deprecated/rolled_back.

## 10. Cloud APIs

Feature-owned routes go in `internal/cloudserver/plugin_routes.go`.

User/catalog:

```text
GET    /api/v1/plugins
GET    /api/v1/plugins/{pluginId}
POST   /api/v1/plugins/{pluginId}/install
PATCH  /api/v1/plugins/{pluginId}/installation
DELETE /api/v1/plugins/{pluginId}/installation
POST   /api/v1/plugins/{pluginId}/apps/{appId}/connect
POST   /api/v1/plugins/{pluginId}/apps/{appId}/disconnect
GET    /api/v1/plugins/{pluginId}/apps/{appId}/connection
POST   /api/v1/plugins/import
POST   /api/v1/plugins/validate
POST   /api/v1/plugins/publish
```

Admin:

```text
GET  /api/v1/admin/plugins
POST /api/v1/admin/plugins/{pluginId}/versions/{version}/evaluate/start
POST /api/v1/admin/plugins/{pluginId}/versions/{version}/evaluate
POST /api/v1/admin/plugins/{pluginId}/versions/{version}/promote
POST /api/v1/admin/plugins/{pluginId}/versions/{version}/rollback
POST /api/v1/admin/plugins/{pluginId}/versions/{version}/block
```

Mutations reuse existing WebAuth, CSRF and rate-limit boundaries.

## 11. Dashboard UX

Add `Plugins` as a first-class navigation item near Chat/Intelligence.

Routes:

```text
/dashboard/plugins
/dashboard/plugins/[pluginId]
/dashboard/plugins/[pluginId]/settings
/dashboard/plugins/developer
```

Directory card: icon, name, publisher verification, one-line value, installed/connected state and concise permission summary. Default UI should hide MCP jargon.

Detail page: What it does, Apps, Skills, Permissions, Data & privacy, Publisher, Version and Install/Update/Uninstall.

Installed settings: enabled/disabled, account vs workspace scope, auto/prefer/manual/disabled routing mode, App reconnect/disconnect and optional pinned version.

Developer mode: import/validate manifest, add remote MCP endpoint, add local stdio MCP for personal development, inspect discovered tools, test privately and submit for review.

## 12. Chat routing

Do not dump the Plugin catalog or all tool schemas into every prompt.

```text
user message
-> detect plugin-relevant intent
-> rank enabled/installed Plugins
-> rank Components inside selected Plugins
-> if App tool is needed, search cached MCP Hub catalog
-> fetch exact selected tool schema
-> execute through existing approval path
```

Routing modes per installation: `auto`, `prefer`, `manual`, `disabled`. Add explicit `@plugin` invocation later.

Disconnected Apps must surface a connect requirement instead of silently failing or attempting credentials.

## 13. MCP Hub binding

An installed App materializes to the existing Hub under a deterministic internal namespace such as:

```text
plugin:<pluginId>:<appId>:<connectionId>
```

User-facing Plugin names remain independent of internal MCP config names.

The MCP Hub uses an opaque credential resolver so OAuth/API credentials can
live in encrypted cloud storage or the local environment without entering Hub
registry JSON or model-visible responses.

### System Plugin and execution routing

Penpot is the first default-installed System Plugin. Its manifest is immutable
CodeLocal metadata, it cannot be uninstalled, and its managed runtime binding
uses the reserved MCP server name `penpot`.

App definitions declare supported execution targets. The execution router owns
the stable connection identifiers and fails closed when a target is not
supported:

```text
local + device id -> local:<device>
cloud             -> cloud
```

Endpoints never imply the execution target. Existing local Plugin connections
remain local and retain their deterministic runtime binding.

Bearer values submitted through Dashboard connection setup are encrypted by the
existing runtime-secret store. Installation and connection records, public API
DTOs, MCP registry JSON and model-visible results retain only the opaque secret
reference. The authenticated local runtime materializes the value when the
connection is configured and resolves the reference immediately before building
the MCP transport header.

## 14. Third-party OAuth

Do not overload `internal/oauth`; that package is CodeLocal's OAuth server for MCP clients.

Create a separate future provider-connection boundary such as `internal/appauth`, responsible for OAuth state/PKCE, provider callback validation, encrypted token storage abstraction, refresh/revoke, scopes and health.

MVP can support `none`, env-reference and header-reference developer connections first while keeping the schema ready for OAuth2.

## 15. Security

- Fail closed on unknown capabilities, component kinds, transports or auth kinds.
- Install does not bypass action-time policy.
- Raw secrets never enter manifests, model results or logs.
- Public remote Apps use HTTPS.
- Local stdio Apps are personal/internal only and remain shell-policy/approval controlled.
- Public promotion captures source/ref/hash, manifest hash, publisher, eval result and permission delta.
- A version requesting broader permissions may require explicit user reconfirmation.

## 16. Version/update policy

Channels: stable and canary.

Install policies: follow stable, follow canary, pinned version.

Automatic updates require a promoted version, compatibility checks and no material unconfirmed permission expansion. Rollback changes a channel pointer; published immutable versions remain auditable.

## 17. Review/evaluation

Evaluation composes component checks:

- manifest schema/hash/publisher/URLs/no-secret checks;
- App MCP initialize/tool schema/HTTPS/permission coverage;
- referenced Skill existence/hash/evaluation state;
- destructive/open-world/data-transmission security review;
- useful description/assets/connection UX;
- permission delta against previous stable version.

Unreviewed community versions cannot become stable.

## 18. Public MCP surface

Do not add one top-level CodeLocal tool per Plugin. The existing compact `mcp` tool already provides lazy extension discovery/call. Dashboard Chat may use an internal Plugin resolver without widening the external MCP contract.

If public Plugin management is exposed later, prefer one action-driven tool such as `Code.plugins` rather than dozens of plugin-specific tools.

## 19. Observability

Record non-secret events for catalog views, installs, updates, uninstalls, enable/disable, connect success/failure, chat selection, component selection, invocation requests and permission outcomes.

Never log OAuth codes/tokens, bearer headers, API keys or local secret values.

Key metrics: install conversion, connect success, invocation success, approval friction, prompt token overhead, routing precision, task completion and uninstall rate.

## 20. Migration

Do not automatically upload/convert existing `~/.codelocal/mcp/registry.json` entries. Later provide an explicit `Convert existing MCP connection to Personal Plugin` action that references the local binding without uploading credentials.

Existing Skills stay independently visible; Plugins may reference them without changing standalone Skills behavior.

## 21. Implementation phases

### P0 — Domain contract

- master plan;
- `internal/plugins` types;
- validation;
- deterministic manifest hash;
- unit tests.

Exit: `go test ./internal/plugins` and `git diff --check` pass.

### P1 — Cloud registry persistence

Plugin versions, channels, installations, tenant isolation and registry store.

### P2 — Catalog APIs + Dashboard directory

Read APIs, `/dashboard/plugins`, detail page, nav, install/uninstall/enable state.

### P3 — MCP App binding

Personal remote MCP developer import, deterministic runtime binding, probe/search/call through existing approval path.

### P4 — Chat routing

Candidate index, auto/prefer/manual/disabled modes, component resolver and compact conversation metadata.

### P5 — Third-party OAuth2

Provider connection domain, PKCE/state, secure token storage adapter, refresh/revoke/reconnect.

### P6 — Skills bundle support

Plugin-to-Skill immutable references and composed evaluation without duplicate registries.

### P7 — Submission/review/publishing

Developer validate/import, community submit, admin evaluate/promote/rollback/block and reputation.

### P8 — Interactive UI resources/widgets

Safe resource contract, sandbox/CSP/origin restrictions and UI actions routed through normal permissions.

### P9 — App Templates + organization governance

Templates, internal distribution, workspace/admin allowlists.

### P10 — Marketplace polish

Featured/search/categories, publisher pages, deep links, trust badges, update notifications and analytics.

## 22. MVP acceptance criteria

A non-technical user can open Plugins, search/select, understand capability/privacy, install, connect, enable by account/workspace, ask naturally in Chat, receive normal approval for risky actions, and disable/uninstall cleanly.

A developer can import/validate a manifest, attach an MCP App and existing Skills, test privately, submit a community version, receive evaluation results and publish through stable/canary promotion without mutating prior versions.

## 23. Definition of done

Plugin Platform is not done when only a directory page exists. It is end-to-end only when:

```text
Catalog
-> Install state
-> Connection state
-> Permissions
-> MCP/Skill runtime binding
-> Chat selection
-> Approval/security
-> Invocation result
-> Disable/uninstall/update
-> Audit/observability
```

Core promise: **the user installs capabilities, not infrastructure**. CodeLocal keeps MCP, credentials, routing, security and runtime complexity behind the Plugin experience while preserving explicit control over what can read, write, connect or act.

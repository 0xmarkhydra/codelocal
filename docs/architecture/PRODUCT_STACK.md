# CodeLocal Product Stack

Status: **Accepted architecture decision**  
Date: **2026-08-19**  
Owner: **CodeLocal**

## Decision

CodeLocal is a multi-surface product. The canonical technology ownership is:

```text
backend/   -> Go
web/       -> Next.js + TypeScript
app/       -> Flutter + Dart (iOS + Android)
desktop/   -> Flutter + Dart (macOS + Windows + Linux)
cli/       -> Go
native/    -> OS-specific bridges only (Swift/macOS first)
docs/      -> Markdown architecture, plans, guides and operations
```

Go remains the canonical backend/runtime language. Next.js owns the browser product. Flutter owns user-facing mobile and desktop application shells. Native code exists only where Flutter/Go cannot safely or ergonomically access an OS capability.

## Surface responsibilities

### backend/

Owns server-side product capabilities:

- Cloud API and HTTP contracts;
- MCP gateway and protocol-facing services;
- authentication, authorization and security policy;
- Project Brain, knowledge, code intelligence and persistence;
- device/workspace coordination;
- usage, billing and organization state;
- database and durable jobs.

Primary language: **Go**.

### web/

Owns the browser product:

- codelocal.cloud landing and marketing surfaces;
- signup/login/account flows;
- dashboard and admin;
- Knowledge Graph and Code Graph visualization;
- billing and organization management;
- public/share/support/trust surfaces.

Primary stack: **Next.js App Router + TypeScript + React**.

The web app consumes explicit backend HTTP/API contracts. It must not duplicate backend authorization or entitlement truth in browser/client code.

For browser-session-bound reads, use same-origin versioned API requests and keep Go as the security authority. A direct Next canary may rewrite those requests to Go for deployment verification, but the production topology keeps the Go gateway on the public domain: Go authenticates protected page requests, serves APIs/transports directly, and proxies presentation-only routes to the private Next service. Before that presentation hop, Go strips browser Cookie, Authorization and client-IP/Railway edge headers so Next does not receive session secrets or security signals it does not own.

Migration rule: the existing Go-rendered web UI remains compiled as the rollback path until the corresponding Next.js route family reaches behavior/security parity and the Railway canary passes. MCP, `/client`, OAuth, pairing and backend APIs must never become dependent on Next availability merely to complete a UI migration.

### app/

Owns mobile applications:

- iOS;
- Android.

Primary stack: **Flutter + Dart**.

Prefer one shared Flutter product codebase with adaptive mobile/desktop features rather than duplicating business-client logic.

### desktop/

Owns desktop application UX:

- macOS;
- Windows;
- Linux.

Primary stack: **Flutter + Dart**.

Desktop is the application/control surface, not the low-level automation implementation. It communicates with the Go runtime and native bridges.

### cli/

Owns terminal/local-runtime user entry:

- `codelocal .` and CLI commands;
- local runtime lifecycle;
- workspace attachment;
- controlled filesystem/Git/terminal/MCP execution;
- local verification and approval interaction.

Primary language: **Go**.

### native/

Owns the smallest possible OS-specific capability bridges.

Examples:

- macOS: Swift worker for Accessibility/AXUIElement and ScreenCaptureKit;
- Windows: native bridge only when Win32/Windows-specific APIs are required;
- Linux: native/desktop integration only where required by Wayland/X11/portal APIs.

Native code must not become a second product/business-logic implementation.

## Communication model

```text
                    CodeLocal Backend (Go)
                   /        |          \
                  /         |           \
        Next.js Web     Flutter App   Flutter Desktop
                                            |
                                            v
                                     Go local runtime / CLI
                                            |
                                            v
                                      Native OS bridges
```

Rules:

1. backend is the authority for identity, authorization, billing and durable cloud state;
2. CLI/local runtime is the authority for local execution and workspace-bound capabilities;
3. web/app/desktop are clients and presentation surfaces;
4. native bridges expose bounded platform capabilities, not business rules;
5. Project Brain and MCP contracts must remain client/framework neutral.

## Web framework decision

CodeLocal web standardizes on the **stable Next.js release line using App Router**. Pin the exact reviewed version in `web/package.json` and update it through normal dependency/security review. Do not start new production browser surfaces in the legacy Go string-rendered UI once a Next.js equivalent boundary exists.

During migration:

- build new route families in `web/`;
- expose/consume explicit Go backend APIs;
- preserve existing production URLs where practical;
- migrate one route family at a time;
- verify auth/session/CSRF/tenant behavior before cutover;
- keep a simple rollback path until parity is proven;
- never use fake telemetry to make the dashboard look active.

## Flutter application-family decision

Flutter is the default UI framework for both mobile and desktop. Shared Dart packages/features should be reused where behavior is truly common, while responsive/adaptive shells may differ by device class.

Do not rewrite the Go local runtime or Swift Computer Engine in Dart merely for stack uniformity. Flutter should call those capabilities through stable local APIs/platform bridges.

## Repository migration

Target product-level structure:

```text
/
├── backend/
├── web/
├── app/
├── desktop/
├── cli/
├── native/
├── tools/
├── docs/
├── scripts/
├── legacy/
├── .github/
├── AGENTS.md
├── README.md
├── ROADMAP.md
├── STATUS.md
├── go.mod
├── go.sum
└── package.json
```

This is a convergence target, not permission for a one-shot physical move. Existing `cmd/` and `internal/` Go packages should move only when dependency direction and release compatibility are understood.

## Non-goals

- one language/framework for every surface;
- Flutter Web replacing the primary codelocal.cloud product;
- Next.js owning backend authorization/business truth;
- moving hundreds of Go files only to make the tree look symmetrical;
- deleting legacy TypeScript before its existing retirement gate passes;
- rewriting native Computer Engine functionality that already has a safer OS-native implementation.

## Verification expectation

Each migration slice must keep the existing relevant gates green. For web slices, require at minimum Next.js lint/typecheck/build, production dependency audit and repository structure checks. For Go boundary changes, add narrow Go tests and then the repository-standard `npm run test` / `npm run typecheck` gates, which intentionally scope canonical Go packages and avoid scanning JavaScript dependencies.

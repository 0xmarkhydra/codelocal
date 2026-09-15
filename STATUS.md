# CodeLocal implementation status

This file describes the **current public shipping boundary**, not historical prototypes. For architecture ownership, see [`docs/architecture/PRODUCT_STACK.md`](./docs/architecture/PRODUCT_STACK.md).

## Public repository extraction status

The public repository is being separated **manually** from CodeLocal's private Enterprise codebase. Components are reviewed, decoupled and cleaned before being exposed publicly rather than being mirrored automatically from the Enterprise tree.

This means repository readers may encounter transitional names, compatibility code, Enterprise-era metadata or source that is present for migration/history but is not part of the currently shipped public npm runtime. The target date for completing this separation and repository cleanup is **October 19, 2026**.

Until that cleanup is complete, use the shipped npm boundary documented below, `cmd/release`, and the canonical architecture documents to determine what is actually supported in the public edition.

## Shipped in the npm package

The supported install path is:

```bash
npm i -g codelocal
```

The npm release is built by the canonical Go release builder in `cmd/release`. It packages prebuilt native `codelocal` binaries for supported macOS/Linux/Windows architectures, the bounded native Computer Use helper, a tiny Node launcher, and managed runtime dependencies used by optional capabilities.

The executable runtime shipped to users is **Go**. `legacy/typescript-runtime/` is not the npm runtime.

### Local runtime / coding surface

- explicitly authorized workspaces;
- workspace-bound filesystem reads/search/edits with path traversal and symlink-escape checks;
- sensitive-path policy independent of `.gitignore`;
- file hashes and stale-write protection;
- project/repository discovery and deterministic structural code intelligence;
- Git read/write operations with guarded mutation paths;
- guarded terminal/process execution, PTY support on Unix, and process lifecycle controls;
- local approvals, audit metadata and idempotency protections;
- local MCP extension hub;
- optional Browser Automation and Computer Use capabilities selected during setup.

### Cloud-connected public flow

The documented remote MCP endpoint is `https://codelocal.cloud/mcp`. OAuth/session routing, pairing/device presence, workspace routing, and durable Project Brain continuity across conversations/devices use CodeLocal Cloud.

Raw repository source and secrets are not uploaded merely to provide Project Brain continuity. See [`docs/architecture/SECURITY_AND_PRIVACY.md`](./docs/architecture/SECURITY_AND_PRIVACY.md) for the trust boundary and offline/Cloud matrix.

## Source present in the repository but not the shipped npm runtime

The repository contains more than the npm client binary. In particular:

- `cmd/codelocal-cloud/` and `internal/cloud*` contain Cloud/backend services;
- `web/` contains the Next.js browser product;
- `legacy/typescript-runtime/` is quarantined compatibility/history code and must not receive new production runtime behavior;
- architecture plans and migration documents under `docs/plans/` describe accepted directions or work in progress and are not proof that a capability ships in the public npm client;
- the public/private Enterprise split is still being cleaned up, so source presence alone does not imply that an Enterprise/team capability is enabled or supported in the single-user public edition.

## Project Brain evidence boundary

Verified Experience is evidence-gated: a model statement alone cannot become durable verified experience. The public implementation requires successful verification/security/regression gates plus evidence/verification references before promotion. Secret-bearing execution evidence is sanitized before durable Cloud Experience is accepted.

A reviewable fixture is provided at [`internal/projectbrain/testdata/verified_experience_reuse.json`](./internal/projectbrain/testdata/verified_experience_reuse.json), with an executable test in `internal/projectbrain/experience_test.go`. The fixture demonstrates that one promoted project experience can be selected consistently from two different client/device contexts after that durable knowledge is available; Cloud transport/synchronization is a separate layer.

## Preview limitations that remain visible until resolved

### No OS-level terminal sandbox yet

**Important:** terminal commands execute on the user's host after CodeLocal policy and approval checks. Workspace filesystem containment, command classification and approvals are application-level guardrails; arbitrary shell execution is **not yet contained by a true OS-level sandbox** on macOS/Linux/Windows.

This warning is also shown during first-run capability setup and must remain visible here until a native sandbox strategy is implemented and validated.

### Windows PTY parity

Unix PTY support is implemented in the canonical Go runtime (`internal/process/pty_unix.go`). Windows PTY parity remains incomplete; the Windows implementation currently falls back to the supported non-PTY process path where necessary.

### Cross-language semantic parity

The deterministic structural engine works without a language server, and dedicated semantic integrations are incremental. Language-specific depth can differ depending on the installed/available analyzer or language server.

### Cloud dependency for remote MCP continuity

The local runtime can inspect and execute against already authorized local workspaces without uploading the repository, but the supported remote MCP flow, account/device routing and cross-device Project Brain continuity are not fully offline. A supported self-hosted public gateway is not currently claimed.

## Release rule

Do not describe a preview limitation as solved until the corresponding implementation and verification evidence exist. In particular, keep the OS-sandbox warning in first-run UX and this status document until native containment is real rather than inferred from command policy.

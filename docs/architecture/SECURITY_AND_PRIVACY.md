# CodeLocal Security & Privacy

## The trust boundary

CodeLocal is designed so the development machine remains the execution boundary.

```text
CodeLocal Cloud
  identity · routing · durable Project Brain knowledge
                 |
                 | authenticated connection
                 v
Local CodeLocal runtime
  source code · secrets · Git · terminal · browser/computer tools
                 |
                 v
Authorized project folders only
```

## What stays local

By default the local runtime owns sensitive execution state such as:

- raw repository source files;
- project secrets and credential files;
- terminal execution and raw command output;
- machine-specific paths and bindings;
- device credentials and local approval state;
- local code indexes that can be rebuilt from the repository;
- OS-level browser/computer automation permissions.

CodeLocal does not need to upload a whole repository to preserve Project Brain continuity.

## Offline and Cloud dependency matrix

"Local-first" describes where source access and execution happen; it does **not** mean every CodeLocal workflow is fully offline. The public MCP entrypoint, account continuity and cross-device Project Brain currently use CodeLocal Cloud.

| Capability | Fully offline after install? | Uses CodeLocal Cloud? | Notes |
| --- | --- | --- | --- |
| Local filesystem/Git inspection inside an already authorized workspace | Yes | No for the local operation itself | The local runtime enforces workspace and sensitive-path policy. |
| Local terminal/process execution | Yes | No for the process itself | Commands execute on the user's host after local policy/approval checks. There is not yet an OS-level sandbox. |
| Local project/code indexes | Yes | No | They are reconstructable from the authorized repository and are not the durable cross-device Project Brain. |
| Browser/Computer capability execution | Local capability | No for direct host execution | Optional and separately enabled; OS permissions remain local. Browser setup may require a network download. |
| Remote MCP use through `https://codelocal.cloud/mcp` | No | Yes | OAuth, session routing and delivery to a paired runtime require the Cloud gateway. |
| Pairing, device/workspace presence and routing | No | Yes | Cloud coordinates authenticated client-to-device/workspace routing. |
| Durable Project Brain continuity across conversations/devices | No | Yes | Cloud stores sanitized canonical knowledge, provenance and eligible verified Experience; raw source stays local by default. |
| Rebuilding local deterministic context from the repository | Yes | No | Works without durable Cloud knowledge but does not reproduce Cloud-only continuity/history. |

The public edition does not currently claim a supported self-hosted gateway. The repository contains Cloud service source while the public/private split is still being cleaned up, but `codelocal.cloud` remains the documented supported gateway. If self-hosting becomes a supported public-edition target, it should get an explicit deployment/security contract rather than being inferred from source availability.

## What is uploaded vs reconstructed

CodeLocal separates durable meaning from reconstructable local state. Raw repository source, local indexes, secrets, credentials and machine bindings remain local by default. Sanitized project facts, decisions, constraints, provenance, eligible verified Experience and derived retrieval projections can be stored in Cloud so the same user can recover useful context in another conversation or on another paired machine.

A local index is reconstructable from the workspace. Canonical Project Brain knowledge is durable product state and is not equivalent to uploading or mirroring the repository.

## What CodeLocal Cloud can store

Cloud storage is used for product state that must survive conversations or machines, including:

- account, device and authorized-workspace metadata;
- logical project/repository identity;
- canonical Project Brain knowledge such as sanitized project facts, decisions, constraints and provenance;
- verified Experience and portable workflow metadata when eligible;
- derived Knowledge Graph/vector projection state;
- bounded aggregate health/rollout telemetry.

The intended rule is: **store the durable meaning when safe, not the raw repository “just in case.”**

## Secrets are not Project Brain knowledge

Secrets, credentials, approval tokens, auth cookies and private keys must not become canonical durable knowledge.

Security policy outranks learned behavior. A remembered workflow cannot grant itself permission or weaken the current local approval policy.

## Workspace containment

A project must be explicitly authorized before it is available to any connected MCP client:

```bash
cd /path/to/project
codelocal .
```

CodeLocal also applies path and sensitive-location checks independently from `.gitignore`.

`.gitignore` helps retrieval quality. It is not treated as the security boundary.

## Approvals

Mutating or risky operations can require explicit approval. Approval decisions are scoped and do not imply a permanent global grant.

Examples include:

- Git writes/pushes;
- dangerous terminal commands;
- destructive file operations;
- open-world actions that the local policy classifies as risky.

Force-push is not part of the normal Git workflow.

## Semantic fallback

Semantic/vector retrieval is a derived acceleration layer, not the only way CodeLocal can understand the project.

When semantic canary health becomes unsafe — errors, timeouts, excessive latency or repeated no-value outcomes — the runtime can block that lane and fall back to deterministic retrieval.

This limits the blast radius of a degraded semantic provider/index.

## Knowledge correction

Durable knowledge is revisioned and can become stale, conflicted, superseded or revoked. Old history can remain auditable without continuing to appear as current truth.

## Multi-tenant isolation

Cloud project/knowledge operations are scoped to the authenticated tenant/user and logical project. Collective Intelligence, when enabled, uses a separate privacy boundary and does not expose another user's raw project knowledge.

Collective contribution/suggestion features are designed to be explicit, gated and aggregate-only rather than a shortcut for sharing private project memory across users.

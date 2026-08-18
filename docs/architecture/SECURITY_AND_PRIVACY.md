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

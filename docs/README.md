# CodeLocal User Docs

CodeLocal is a persistent project-intelligence and controlled-execution layer for AI coding assistants.

Start here:

- [User Guide](./guides/USER_GUIDE.md) — install, connect, authorize a project, work from chat, update and reset.
- [How CodeLocal Works](./guides/HOW_CODELOCAL_WORKS.md) — the Project Brain mental model, verification loop, cross-device continuity and safe learning.
- [Security & Privacy](./architecture/SECURITY_AND_PRIVACY.md) — what stays local, what may sync to CodeLocal Cloud, approvals and trust boundaries.
- [Product Stack](./architecture/PRODUCT_STACK.md) — canonical ownership: Go backend/CLI, Next.js web, Flutter mobile/desktop and bounded native bridges.
- [Hybrid Local + Cloud Runtime Master Plan](./plans/runtime/HYBRID_LOCAL_CLOUD_RUNTIME_MASTER_PLAN.md) — keep Local Runtime first-class while adding a managed, lazy-provisioned Cloud Sandbox with CodeLocal auto-running, managed Git, lifecycle/cost controls, GUI layers and provider-neutral compute.
- [Repository Structure](./architecture/REPOSITORY_STRUCTURE.md) — source ownership, documentation taxonomy, legacy quarantine and migration rules.
- [Beta Channel](./operations/BETA_CHANNEL.md) — how beta differs from stable, how to test it without misunderstanding the release channel.
- [npm Release Operations](./operations/NPM_RELEASE.md) — canonical maintainer command for stable/beta packaging, publishing, verification and failure handling.
- [Global Billing & International Payments Master Plan](./plans/platform/GLOBAL_BILLING_AND_INTERNATIONAL_PAYMENTS_MASTER_PLAN.md) — Vietnam-to-global SaaS billing, Lemon Squeezy launch path, provider-neutral architecture, webhooks, entitlements, payouts, compliance and production proof.
- [Code Graph + Technical Debt Master Plan](./plans/intelligence/CODE_GRAPH_AND_TECH_DEBT_MASTER_PLAN.md) — BA flows, Code Graph accuracy/UX contracts, technical-debt backlog, rollout phases and acceptance criteria.
- [Database Migration Rollout](./operations/MIGRATION_ROLLOUT.md) — staged Railway `startup -> only -> external` migration lifecycle, canary checks and rollback path.
- [Next.js Web Cutover](./operations/NEXT_WEB_CUTOVER.md) — Railway canary, Go-fronted presentation proxy, edge probe, production flags and instant rollback for the Next web migration.
- [OpenAI Plugin Submission Pack](./integrations/openai/submission/README.md) — listing, reviewer sandbox, test cases, policy mapping and submission gate.

## The shortest explanation

```text
AI model         = reasoning
Project Brain    = durable project context and verified experience
Local runtime    = controlled access to authorized files, Git and tools
Verification     = evidence that the work actually passed
```

CodeLocal does not try to remember everything. It tries to preserve only knowledge that remains useful after the current conversation ends.
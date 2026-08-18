# CodeLocal User Docs

CodeLocal is a persistent project-intelligence and controlled-execution layer for AI coding assistants.

Start here:

- [User Guide](./USER_GUIDE.md) — install, connect, authorize a project, work from chat, update and reset.
- [How CodeLocal Works](./HOW_CODELOCAL_WORKS.md) — the Project Brain mental model, verification loop, cross-device continuity and safe learning.
- [Security & Privacy](./SECURITY_AND_PRIVACY.md) — what stays local, what may sync to CodeLocal Cloud, approvals and trust boundaries.
- [Beta Channel](./BETA_CHANNEL.md) — how beta differs from stable, how to test it without misunderstanding the release channel.
- [Code Graph + Technical Debt Master Plan](./CODE_GRAPH_AND_TECH_DEBT_MASTER_PLAN.md) — BA flows, Code Graph accuracy/UX contracts, technical-debt backlog, rollout phases and acceptance criteria.
- [Database Migration Rollout](./MIGRATION_ROLLOUT.md) — staged Railway `startup -> only -> external` migration lifecycle, canary checks and rollback path.
- [OpenAI Plugin Submission Pack](./plugin-submission/README.md) — listing, reviewer sandbox, test cases, policy mapping and submission gate.

## The shortest explanation

```text
AI model         = reasoning
Project Brain    = durable project context and verified experience
Local runtime    = controlled access to authorized files, Git and tools
Verification     = evidence that the work actually passed
```

CodeLocal does not try to remember everything. It tries to preserve only knowledge that remains useful after the current conversation ends.

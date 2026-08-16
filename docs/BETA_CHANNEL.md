# CodeLocal Beta Channel

## Stable vs beta

CodeLocal uses npm distribution tags so a new runtime can be tested without immediately moving every user to it.

Stable:

```bash
npm i -g codelocal
# same channel as:
npm i -g codelocal@latest
```

Beta:

```bash
npm i -g codelocal@beta
```

The command you run afterward is still simply:

```bash
codelocal
```

`@beta` changes which package version npm installs. It does not create a separate “beta mode” inside the application.

## Important: global install replaces the current global version

If you already have stable installed globally, installing beta globally replaces the `codelocal` executable with the beta package:

```text
before: codelocal 1.x.y
install codelocal@beta
after:  codelocal 1.x.z-beta.n
```

Your normal local CodeLocal state, pairing and authorized workspaces are expected to survive a package update.

## Return to stable

```bash
npm i -g codelocal@latest
codelocal --version
```

## Why beta exists

A beta release lets CodeLocal validate a new client/server train against real projects while keeping stable users on the previous release.

Typical rollout:

```text
new dev build
   ↓
Cloud deployment with risky features OFF/shadow
   ↓
npm beta
   ↓
canary users
   ↓
health + compatibility + latency checks
   ↓
stable/latest
```

A beta should still pass the normal automated test/package gates. “Beta” means rollout risk is intentionally limited, not that basic verification is skipped.

## What to report while testing

Useful beta reports include:

- `codelocal --version`;
- whether the runtime connects successfully;
- whether the connected AI client sees the expected MCP tools/actions;
- workspace selection failures;
- unexpected approval behavior;
- Project Brain/knowledge results that are clearly stale or irrelevant;
- slow semantic recall or repeated fallbacks;
- migration/server errors;
- regressions in normal coding, Git or terminal workflows.

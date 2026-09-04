# npm Release Operations

This is the canonical maintainer runbook for publishing the public `codelocal` npm package.

## Production release command

From the repository root, the maintainer's canonical and usual npm publish command is:

```bash
npm run release:npm
```

Unless the maintainer explicitly asks for a dry run, validation-only run, or full-repository release gate, treat “release/push/publish npm” as this command and as a **CLI-only** release. Do not substitute `npm run release:npm:prepare`, and do not ask whether web should be included: web is excluded by default.

Choose `1` or press **Enter** for the production `latest` channel:

```text
CodeLocal npm release

  1) latest (stable) [default]
  2) beta

Choose channel [1]:
```

Choose `2` only for a beta release.

Do not use a standalone `npm publish` as the normal release procedure. `npm run release:npm` is the supported end-to-end command because it checks npm authentication, selects a version that is not already published, synchronizes source version files, runs the release gates, builds the package, publishes it with the selected dist-tag, and verifies registry convergence.

## What the command does

The release helper:

1. verifies npm authentication (or starts `npm login`);
2. reads versions already published for `codelocal`;
3. selects the next stable or beta version;
4. updates `package.json`, `package-lock.json`, and `internal/version/version.go`;
5. removes and rebuilds `.release/npm`;
6. runs `npm run release:npm:prepare`, including repository structure checks, Go tests/vet, package generation, and `npm pack --dry-run` (web applications are intentionally outside the npm CLI release path);
7. validates the staged package version and generated README;
8. publishes with the selected `latest` or `beta` dist-tag;
9. waits for npm to expose both the immutable version and updated dist-tag.

The package version cannot be overwritten after npm accepts it. Re-running the helper therefore selects the next available version rather than attempting to replace an existing release.

## Validation without publishing

To run the npm CLI release checks and build the staging package without publishing:

```bash
npm run release:npm:prepare
```

This checks repository structure and Go code only. It does not install, lint, typecheck, build, or audit the web applications. Use `npm run release:prepare` when you intentionally want the full repository release gate, including web checks.

To inspect an already generated staging package:

```bash
npm pack --dry-run ./.release/npm
```

`release:npm:prepare`, `release:prepare`, and `npm pack --dry-run` never publish anything.

To preview which channel and version the interactive helper would choose without changing files or publishing:

```bash
CODELOCAL_RELEASE_DRY_RUN=1 npm run release:npm
```

## Verify a production release

After a stable release, verify npm metadata and installation:

```bash
npm view codelocal@latest version --prefer-online
npm view codelocal dist-tags --prefer-online
npm i -g codelocal@latest
codelocal --version
```

For beta, replace `latest` with `beta`.

## Generated package

The staging directory is `.release/npm`. It contains compiled native binaries for supported operating systems and architectures, a JavaScript launcher, package metadata, and the generated public README. It is build output and must not be treated as the source of truth for version selection.

## Failure handling

- Authentication failure: complete the `npm login` started by the helper, then retry.
- Check or build failure: fix the reported failure and rerun `npm run release:npm`; nothing is published before the publish step.
- Version already exists: do not try to overwrite it. Rerun the helper so it chooses the next available version.
- Publish succeeds but registry verification times out: check the exact version and dist-tag with `npm view` before retrying. npm versions are immutable, so never blindly republish the same version.

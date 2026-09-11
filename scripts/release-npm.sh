#!/usr/bin/env bash
set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$ROOT_DIR"

PACKAGE_NAME="codelocal"

need() {
  if ! command -v "$1" >/dev/null 2>&1; then
    echo "✗ Missing required command: $1" >&2
    exit 1
  fi
}

need node
need npm
need go

printf '\nCodeLocal npm release\n\n'
printf '  1) latest (stable) [default]\n'
printf '  2) beta\n\n'
read -r -p 'Choose channel [1]: ' choice

case "${choice:-1}" in
  1|latest|stable)
    channel="latest"
    ;;
  2|beta)
    channel="beta"
    ;;
  *)
    echo "✗ Invalid choice: $choice" >&2
    echo "  Use 1/latest or 2/beta." >&2
    exit 1
    ;;
esac

local_version="$(node -p "require('./package.json').version")"
printf '\nChecking published versions on npm...\n'
published_versions="$(npm view "$PACKAGE_NAME" versions --json --prefer-online 2>/dev/null || printf '[]')"

next_version="$(
  CODELOCAL_RELEASE_CHANNEL="$channel" \
  CODELOCAL_LOCAL_VERSION="$local_version" \
  CODELOCAL_PUBLISHED_VERSIONS="$published_versions" \
  node <<'NODE'
const channel = process.env.CODELOCAL_RELEASE_CHANNEL;
const localVersion = process.env.CODELOCAL_LOCAL_VERSION || "0.0.0";
let versions;
try {
  versions = JSON.parse(process.env.CODELOCAL_PUBLISHED_VERSIONS || "[]");
} catch {
  versions = [];
}
if (!Array.isArray(versions)) versions = versions ? [versions] : [];

const stableRe = /^(\d+)\.(\d+)\.(\d+)$/;
const tuple = (v) => {
  const m = stableRe.exec(v);
  return m ? [Number(m[1]), Number(m[2]), Number(m[3])] : null;
};
const compare = (a, b) => a[0] - b[0] || a[1] - b[1] || a[2] - b[2];

const stable = versions.map(tuple).filter(Boolean).sort(compare);
const localBase = tuple(localVersion.replace(/-.+$/, "")) || [0, 0, 0];
const publishedBase = stable.length ? stable[stable.length - 1] : [0, 0, 0];
// Never let a stale/incomplete registry response make a release move backwards.
// The next base must be at least the version currently declared by the repository.
let base = compare(localBase, publishedBase) >= 0 ? localBase : publishedBase;

if (channel === "latest") {
  process.stdout.write(`${base[0]}.${base[1]}.${base[2] + 1}`);
  process.exit(0);
}

const betaBase = [base[0], base[1], base[2] + 1];
const prefix = `${betaBase[0]}.${betaBase[1]}.${betaBase[2]}-beta.`;
let maxBeta = -1;
for (const version of versions) {
  if (!version.startsWith(prefix)) continue;
  const n = Number(version.slice(prefix.length));
  if (Number.isInteger(n) && n > maxBeta) maxBeta = n;
}
process.stdout.write(`${prefix}${maxBeta + 1}`);
NODE
)"

install_command="npm i -g codelocal"
if [[ "$channel" == "beta" ]]; then
  install_command="npm i -g codelocal@beta"
fi

printf '\nRelease plan\n'
printf '  Channel : %s\n' "$channel"
printf '  Current : %s\n' "$local_version"
printf '  Next    : %s\n' "$next_version"
printf '  Install : %s\n\n' "$install_command"

if [[ "${CODELOCAL_RELEASE_DRY_RUN:-0}" == "1" ]]; then
  echo "Dry run only; no files changed and nothing published."
  exit 0
fi

if [[ "$local_version" != "$next_version" ]]; then
  npm version "$next_version" --no-git-tag-version >/dev/null
else
  printf 'Version files already target %s; skipping npm version.\n' "$next_version"
fi

CODELOCAL_VERSION="$next_version" node <<'NODE'
const fs = require('node:fs');
const path = 'internal/version/version.go';
const version = process.env.CODELOCAL_VERSION;
let source = fs.readFileSync(path, 'utf8');
if (!/const Version = "[^"]+"/.test(source)) {
  throw new Error(`Could not find Go runtime version in ${path}`);
}
source = source.replace(/const Version = "[^"]+"/, `const Version = "${version}"`);
fs.writeFileSync(path, source);
NODE

rm -rf .release
CODELOCAL_RELEASE_CHANNEL="$channel" npm run release:npm:prepare

staged_version="$(node -p "require('./.release/npm/package.json').version")"
if [[ "$staged_version" != "$next_version" ]]; then
  echo "✗ Staged package version is $staged_version, expected $next_version" >&2
  exit 1
fi

if ! grep -Fq "$install_command" .release/npm/README.md; then
  echo "✗ Generated README does not contain the expected install command:" >&2
  echo "  $install_command" >&2
  exit 1
fi

if [[ "$channel" == "latest" ]] && grep -Fq 'codelocal@beta' .release/npm/README.md; then
  echo "✗ Stable README still contains codelocal@beta; refusing to publish." >&2
  exit 1
fi

printf '\nPublishing codelocal@%s with npm tag %s...\n\n' "$next_version" "$channel"
npm publish ./.release/npm --tag "$channel"

printf '\nPublished. Waiting for npm registry to expose codelocal@%s and update %s...\n' "$next_version" "$channel"

registry_ready=0
max_registry_attempts="${CODELOCAL_REGISTRY_ATTEMPTS:-20}"
registry_retry_seconds="${CODELOCAL_REGISTRY_RETRY_SECONDS:-3}"

for ((attempt = 1; attempt <= max_registry_attempts; attempt++)); do
  published_version="$(npm view "$PACKAGE_NAME@$next_version" version --prefer-online 2>/dev/null || true)"
  tagged_version="$(npm view "$PACKAGE_NAME@$channel" version --prefer-online 2>/dev/null || true)"

  if [[ "$published_version" == "$next_version" && "$tagged_version" == "$next_version" ]]; then
    registry_ready=1
    break
  fi

  printf '  npm sync %d/%d: version=%s, %s=%s\n' \
    "$attempt" \
    "$max_registry_attempts" \
    "${published_version:-pending}" \
    "$channel" \
    "${tagged_version:-pending}"

  if (( attempt < max_registry_attempts )); then
    sleep "$registry_retry_seconds"
  fi
done

if [[ "$registry_ready" != "1" ]]; then
  echo "✗ npm accepted the publish, but registry metadata did not converge in time." >&2
  echo "  Expected version: $next_version" >&2
  echo "  Expected $channel tag: $next_version" >&2
  echo "  Verify with:" >&2
  echo "    npm view $PACKAGE_NAME@$next_version version --prefer-online" >&2
  echo "    npm view $PACKAGE_NAME@$channel version --prefer-online" >&2
  echo "    npm view $PACKAGE_NAME dist-tags --prefer-online" >&2
  exit 1
fi

printf '\n✓ Release ready: codelocal@%s (%s)\n\n' "$next_version" "$channel"
npm view "$PACKAGE_NAME" dist-tags --prefer-online
printf '\nInstalled by users with:\n  %s\n\n' "$install_command"
printf 'Version files are updated locally. Commit them when ready:\n'
printf '  git add package.json package-lock.json internal/version/version.go\n'
printf '  git commit -m "release: codelocal %s"\n\n' "$next_version"

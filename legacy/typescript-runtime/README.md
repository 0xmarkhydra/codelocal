# Legacy TypeScript runtime boundary

Status: **quarantined / compatibility-only**

The shipping CodeLocal runtime, CI path and npm release builder are native Go. Files in `legacy/typescript-runtime/src/` are retained only for compatibility/history while retirement evidence is collected.

Rules:

1. Do not add new production runtime behavior here.
2. Do not make root `package.json` build/test/release scripts execute `legacy/typescript-runtime/src/client-v2.ts`, `legacy/typescript-runtime/src/server-saas.ts` or `legacy/typescript-runtime/src/index.ts`.
3. Do not make `cmd/release` package the legacy TypeScript runtime.
4. New product/runtime work belongs in the native Go packages under `cmd/` and `internal/`.
5. Delete legacy files only after external consumers and deployment telemetry prove they are unused; quarantine is intentionally safer than bulk deletion.

## Physical deletion gate

The compatibility runtime may be physically removed only in a dedicated retirement change after all of these are true:

- at least two stable npm releases have shipped with the native-Go package and no production/release script references the legacy entrypoints;
- deployed CodeLocal services/runtime inventory confirms Go-native entrypoints only;
- public docs, support instructions and known integrations have been checked for direct `legacy/typescript-runtime/src/`, `client-v2` or `server-saas` usage;
- repository-wide search shows no non-test/non-history production dependency on the quarantined entrypoints;
- the retirement release notes explicitly call out removal for anyone still invoking repository-internal TypeScript files directly.

Until those checks have external evidence, keeping the files quarantined is the safer compatibility decision. New production behavior in this directory remains forbidden.

CI guards this boundary in `cmd/release/native_runtime_boundary_test.go`.

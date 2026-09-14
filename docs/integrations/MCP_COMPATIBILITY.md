# MCP compatibility and operational boundary

Status: implementation under verification; not an all-provider certification.

## Supported surfaces

| Surface | Transport / authentication | Boundary |
|---|---|---|
| CodeLocal public MCP gateway | Existing SDK-backed Streamable HTTP, modern request metadata and legacy initialization paths | Existing inbound OAuth authority; separate from outbound plugin OAuth |
| Local MCP Hub | stdio and Streamable HTTP, environment/header references | Requires an authorized online device; stdio execution stays on the device |
| Cloud plugin connector | Streamable HTTP over public HTTPS; no auth, manual Bearer, OAuth public clients | Cloud never runs uploaded commands or stdio; every tool call requires explicit chat approval |
| Outbound OAuth | RFC 9728 resource metadata; RFC 8414 and OIDC discovery; PKCE S256; exact issuer check; resource parameter; refresh rotation | Client ID Metadata Document preferred, public DCR fallback; unsupported multi-issuer/confidential-client/DPoP/mTLS configurations fail with a clear error |

Legacy HTTP+SSE is not a supported plugin transport in this change. Streamable HTTP responses using SSE are not the same as legacy HTTP+SSE. No automatic downgrade or cross-origin credential forwarding is attempted. Resources, prompts, sampling, elicitation and optional extensions are not advertised by the plugin chat surface. Compatibility with every protocol revision/provider is not claimed merely because an SDK is used.

## User approval

`call_plugin_tool` creates a pending operation; it does not execute tools. The backend stores its arguments encrypted, bound to user/session, exact plugin, connection version and tool. The browser fetches the authoritative operation before enabling **Allow once** or **Deny**. Approval is consumed atomically before dispatch; duplicate clicks, session changes, expiry or reconnect do not authorize another execution. MCP annotations including `readOnlyHint` are advisory, never an authorization source. Ask/Plan modes do not expose the tool-call operation.

Local calls retain local policy. A local approval challenge can be satisfied only after the web user approved that same backend-stored action. Runtime approval fingerprints include arguments. No device/Cloud fallback is selected implicitly when multiple connections exist.

A network error after dispatch means **outcome unknown**, not a safe retry. Check the provider before manually requesting a new operation. Disconnect prevents new calls; it cannot undo an operation already accepted by a provider.

## Credentials, restart and egress

Cloud credentials are encrypted separately from inherited device runtime settings and use authenticated encryption bound to user/plugin. Connection metadata and credential replacement are transactional. OAuth token refresh is serialized by a row lock. Short-lived MCP sessions are rebuilt from durable credentials for each operation, avoiding replica affinity and persistent bearer retention in a global session registry.

Old Cloud connections created by the earlier prototype must be reconnected. Their metadata is not proof of an active session. The UI labels saved discovery counts as historical rather than live readiness. A key rotation for an existing endpoint must be confirmed with successful discovery before replacing saved configuration.

Cloud requests are HTTPS only, reject URL credentials/query strings/fragments, disable redirects and environment proxies, reject all non-public DNS answers, and dial validated literal IPs. Credentials are injected only for the selected origin. Request-scoped deadlines, response limits, schema limits, catalog limits and per-account concurrent operation limits bound resources. Remote schema `$ref` fetching is disabled.

## Penpot is not an ordinary Bearer endpoint

The hosted CodeLocal adapter accepts signed CodeLocal grants, not native Penpot keys. Cloud grants are account-scoped and have a distinct token use from device grants. Probe-only grants can discover tools but cannot call tools; regular Cloud grants verify the current encrypted Cloud credential and profile ownership. The native key travels only inside the short-lived internal grant and to the configured private Penpot upstream. Signing does not encrypt a JWT.

Penpot MCP's editor bridge is a separate dependency. Successful MCP discovery does not prove an editor is open or that background design editing works. This change does not supply a headless Penpot editor or claim operation while all editors are closed. Real hosted create/edit/readback tests, including editor lifecycle and device-offline behavior, remain deployment acceptance requirements. Do not mark Penpot certified based on synthetic fixtures.

## Reproducible checks

```sh
go test ./internal/cloudmcp ./internal/cloudserver ./internal/oauth ./internal/penpot ./internal/localclient ./internal/mcphub
CODELOCAL_MCP_TEST_DATABASE_URL='postgres://postgres@127.0.0.1:PORT/postgres?sslmode=disable' go test -v ./internal/cloud -run 'TestPluginCredentialAAD|TestPluginCloudPersistenceAndApprovalIntegration'
CODELOCAL_PENPOT_TEST_DATABASE_URL='postgres://postgres@127.0.0.1:PORT/postgres?sslmode=disable' go test -v ./internal/penpot -run 'TestPenpotCloudGrantProbeAndRevocationIntegration|TestPenpotSignedRuntimeToAdapterIntegration'
node web/src/app/dashboard/design/penpot-design-frame.test.mjs
npm --prefix web run lint
npm --prefix web run typecheck
npm --prefix web run test:ui
npm --prefix web run build
```

Database checks create and remove isolated schemas on a **local test database**. A SKIP is not a pass. Keep a separate production acceptance record naming deployed builds, provider and protocol version, exact operation outcome, auth expiry/revocation and restart tests; do not include credentials in that record.

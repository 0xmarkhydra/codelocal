# Cloud Workspace Runtime — Human Test

Status: staging/manual acceptance checklist  
Scope: Auto runtime routing, Local compatibility, OpenSandbox fallback, managed-runtime identity, multi-replica routing, lifecycle and security.

## 0. Preconditions

Control plane (Railway/staging) must have:

```text
CODELOCAL_CLOUD_RUNTIME_ENABLED=1
CODELOCAL_OPENSANDBOX_URL=<OpenSandbox control-plane URL>
CODELOCAL_OPENSANDBOX_API_KEY=<secret>
CODELOCAL_CLOUD_RUNTIME_IMAGE=<published image built from Dockerfile.runtime>
PUBLIC_BASE_URL=<public CodeLocal control-plane origin>
```

Optional defaults:

```text
CODELOCAL_CLOUD_RUNTIME_PROFILE=general-small
CODELOCAL_CLOUD_RUNTIME_CPU=2
CODELOCAL_CLOUD_RUNTIME_MEMORY=4Gi
```

The source workspace must already belong to the signed-in user. For the current MVP, cloud hydration needs at least one credential-free HTTP(S) Git remote that the sandbox can clone without interactive authentication.

Use a disposable test repository containing:

```text
README.md
human-test.txt   # initial contents: before
```

Do not use production customer workspaces for destructive test steps.

---

## Case A — Auto keeps Local Runtime when Local is online

Goal: prove the cloud feature does not change the existing Local Runtime path.

1. Start CodeLocal on the test machine.
2. Confirm the test workspace is authorized and visible once in `workspace(action=list)` / Dashboard.
3. Keep `CODELOCAL_CLOUD_RUNTIME_ENABLED=1` on the server.
4. Ask CodeLocal to read `human-test.txt`.
5. Ask CodeLocal to write `local-ok` to a disposable file, then read it back.
6. Check OpenSandbox: no new sandbox should have been provisioned for this request.

Expected:

- Tool calls succeed through the local device.
- Returned `workspaceKey`, `deviceId`, `workspaceId` are the original local workspace values.
- No `cloud-*` device/workspace appears in the user-facing workspace list.
- Existing approval/idempotency behavior is unchanged.

FAIL if:

- Auto provisions cloud while the local runtime is healthy.
- A second/duplicate workspace appears.
- The returned workspace handle changes to a `cloud-*` identity.

---

## Case B — Local offline falls back to Cloud

Goal: prove a user can execute without keeping their personal machine online.

1. With the same signed-in account/workspace, stop the local runtime completely.
2. Do not delete/revoke the workspace from CodeLocal Cloud.
3. Confirm Dashboard/workspace list still shows the original workspace as offline/sleeping; it must not show a `cloud-*` workspace.
4. Ask CodeLocal to read `README.md` using the original workspace key.
5. Observe OpenSandbox provisioning.
6. After the first command succeeds, ask CodeLocal to create `cloud-human-test.txt` with text `cloud-ok` and read it back.
7. Run `git status` through CodeLocal.

Expected first-use lifecycle:

```text
Local unavailable
→ Auto selects Cloud provider
→ distributed runtime lease acquired
→ one-time bootstrap token issued
→ OpenSandbox created/reused
→ repository hydrated into /workspace
→ codelocal-cloud-runtime exchanges bootstrap token
→ managed runtime registers realtime control
→ workspace activates through /client
→ source workspace key aliases to managed runtime key internally
→ tool result returns using original product workspace identity
```

Visible expected result:

- User uses the SAME workspace selection as before.
- No reconnect, SSH, manual pairing or local CLI step is requested.
- `workspaceKey`, `deviceId`, `workspaceId` exposed to MCP/UI remain the ORIGINAL product workspace values.
- `cloud-*` identities are not visible in normal workspace lists.
- Read/write/shell/git tools execute successfully in Cloud.

FAIL if:

- User is asked to select a new cloud workspace.
- `cloud-*` appears in Dashboard/MCP workspace catalog.
- `/workspace` is exposed as the product project root when the original product root was different.
- Bootstrap token or managed credential appears in logs/tool output.

---

## Case C — Fail-safe behavior when Cloud cannot run

Goal: prove Auto does not hide authorization/security errors or run an unsafe/stale copy.

### C1. No durable credential-free source

1. Stop Local Runtime.
2. Choose a workspace without a cloud-cloneable HTTP(S) Git source.
3. Invoke a filesystem operation.

Expected:

- Cloud provider refuses hydration.
- No arbitrary blank/stale workspace is created.
- The error is actionable/retryable where appropriate.
- User data is not written somewhere else silently.

### C2. Workspace revoked / unauthorized

1. Revoke/remove access to the workspace.
2. Attempt a tool call using its old key.

Expected:

- Hard authorization/unavailable error.
- Auto MUST NOT provision a cloud sandbox as a workaround.

### C3. OpenSandbox outage

1. In staging only, point the provider at an unavailable OpenSandbox endpoint or stop the OpenSandbox service.
2. Keep Local Runtime offline.
3. Invoke a tool.

Expected:

- Request fails as provider unavailable/transient.
- No infinite retry loop.
- No misleading success or stale output.

---

## Case D — Reuse the same sandbox during an active session

Goal: prove compute is not provisioned per tool call.

1. Complete Case B and note the OpenSandbox sandbox ID.
2. Run 5–10 read/search/shell operations over several minutes.
3. Re-check OpenSandbox.

Expected:

- The same compatible sandbox is reused.
- Runtime alias remains valid across calls.
- No sandbox explosion from repeated tools.

FAIL if every tool call creates a new sandbox.

---

## Case E — Multi-replica routing

Run only on staging with 2+ CodeLocal gateway replicas sharing the same Redis/Postgres.

1. Provision Cloud runtime through any replica.
2. Confirm the managed runtime WebSocket is owned by one gateway instance.
3. Send the next MCP/tool request through a different gateway instance (normal load-balancer distribution is sufficient if observable; otherwise pin/request through each staging replica).
4. Read and then mutate a disposable file.

Expected:

- Redis runtime alias resolves original workspace key → managed cloud client key.
- Coordinator routes the request to the gateway that owns the runtime WebSocket.
- Tool succeeds without reprovisioning merely because the HTTP request landed on another replica.
- Product workspace identity remains original in the response.

---

## Case F — Bootstrap token is one-time and secret-safe

1. Capture a bootstrap token only in a controlled staging debugger; never paste it into issue/chat logs.
2. Allow the sandbox to exchange it once.
3. Replay the same exchange request.

Expected:

- First valid exchange succeeds.
- Replay is rejected.
- Server stores token lookup by digest, not plaintext token key.
- Runtime unsets bootstrap token after exchange.
- Runtime unsets repository JSON after hydration.
- Normal runtime/tool logs contain no bootstrap token, managed credential secret or provider API key.

---

## Case G — Idle expiry and wake-up

1. Complete Case B.
2. Leave the cloud runtime unused until its configured sandbox expiration/idle policy takes effect.
3. Confirm the old sandbox is no longer active.
4. Invoke the same original workspace again.

Expected:

- Old internal route does not cause permanent `workspace offline` loops.
- Auto provisions/reuses a valid fresh sandbox.
- Same product workspace key continues to work.
- No duplicate user-facing workspace is created after multiple wake cycles.

---

## Acceptance gate

Ship/merge the runtime foundation only when all applicable rows are PASS:

| Check | Result |
| --- | --- |
| Local online remains Local | ⬜ |
| Local offline falls back to Cloud | ⬜ |
| Original workspace identity preserved | ⬜ |
| Managed `cloud-*` workspace hidden | ⬜ |
| Read/write/shell/git operate in Cloud | ⬜ |
| Same sandbox reused during session | ⬜ |
| Multi-replica alias routing works | ⬜ |
| Bootstrap replay rejected | ⬜ |
| Secrets absent from logs/results | ⬜ |
| Authorization errors never trigger unsafe fallback | ⬜ |
| Idle expiry can reprovision cleanly | ⬜ |

## Known MVP limitation

Cloud hydration currently clones durable credential-free HTTP(S) Git sources. Private repositories that require credentials, uncommitted local-only changes, and a full persistent workspace snapshot/restore pipeline are **not proven by this checklist** and must not be advertised as supported until managed Git credential injection / snapshot persistence is implemented and separately tested.

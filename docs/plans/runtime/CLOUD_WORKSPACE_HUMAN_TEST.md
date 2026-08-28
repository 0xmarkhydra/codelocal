# Cloud Workspace Runtime — Human Acceptance Test

Status: staging/manual acceptance checklist  
Scope: Auto runtime routing, Local compatibility, OpenSandbox fallback, managed-runtime identity, multi-replica routing, one-time bootstrap, workspace hydration, idle checkpoint and snapshot restore.

## 0. Preconditions

Control plane (Railway/staging) must have:

```text
CODELOCAL_CLOUD_RUNTIME_ENABLED=1
CODELOCAL_OPENSANDBOX_URL=<OpenSandbox control-plane URL>
CODELOCAL_OPENSANDBOX_API_KEY=<secret>
CODELOCAL_CLOUD_RUNTIME_IMAGE=<published image built from Dockerfile.runtime>
PUBLIC_BASE_URL=<public CodeLocal control-plane origin>
```

Normal defaults:

```text
CODELOCAL_CLOUD_RUNTIME_PROFILE=general-small
CODELOCAL_CLOUD_RUNTIME_CPU=2
CODELOCAL_CLOUD_RUNTIME_MEMORY=4Gi
CODELOCAL_CLOUD_RUNTIME_IDLE_TIMEOUT=20m
CODELOCAL_CLOUD_RUNTIME_REAP_INTERVAL=1m
CODELOCAL_CLOUD_RUNTIME_SANDBOX_TTL=30m
```

For a fast staging checkpoint test, temporarily use:

```text
CODELOCAL_CLOUD_RUNTIME_IDLE_TIMEOUT=45s
CODELOCAL_CLOUD_RUNTIME_REAP_INTERVAL=5s
CODELOCAL_CLOUD_RUNTIME_SANDBOX_TTL=10m
```

The source workspace must already belong to the signed-in user. Initial cloud hydration currently requires at least one credential-free HTTP(S) Git remote that the sandbox can clone without interactive authentication. Use a disposable **public** test repository containing:

```text
README.md
human-test.txt   # initial contents: before
```

Do not use a production customer workspace for destructive test steps.

---

## Test order

Run in this order so each case proves one architectural property before the next case depends on it:

```text
A Local compatibility
→ B first Cloud fallback
→ C fail-closed behavior
→ D warm reuse
→ E multi-replica routing
→ F bootstrap/security
→ G idle snapshot persistence
→ H repeated sleep/wake
```

---

## Case A — Auto keeps Local Runtime when Local is online

**Goal:** prove Cloud support does not change the existing Local Runtime path.

1. Start CodeLocal on the test machine.
2. Confirm the test workspace is authorized and visible exactly once in `workspace(action=list)` / Dashboard.
3. Keep `CODELOCAL_CLOUD_RUNTIME_ENABLED=1` on the server.
4. Ask CodeLocal to read `human-test.txt`.
5. Ask CodeLocal to write `local-ok` to a disposable file, then read it back.
6. Check OpenSandbox.

**PASS:**

- Tool calls succeed through the local device.
- No new OpenSandbox is provisioned for these calls.
- Returned `workspaceKey`, `deviceId`, `workspaceId` are the original workspace values.
- No `cloud-*` device/workspace appears in the user-facing workspace list.
- Existing approval/idempotency behavior remains unchanged.

**FAIL:** Auto provisions Cloud while Local is healthy, a duplicate workspace appears, or the returned workspace handle changes to a managed cloud identity.

---

## Case B — Local offline falls back to Cloud

**Goal:** prove the user can execute without keeping the personal machine online.

1. Stop Local Runtime completely.
2. Do **not** delete/revoke the workspace from CodeLocal Cloud.
3. Confirm Dashboard/workspace list still shows the original product workspace offline/sleeping; it must not expose `cloud-*`.
4. Ask CodeLocal to read `README.md` using the original workspace selection/key.
5. Observe OpenSandbox provisioning.
6. Create `cloud-human-test.txt` containing `cloud-ok` and read it back.
7. Run `git status` through CodeLocal.

Expected first-use lifecycle:

```text
Local unavailable
→ Auto selects Cloud provider
→ distributed workspace/profile lease
→ one-time bootstrap token
→ OpenSandbox image provision
→ Git hydration into /workspace
→ sandbox generates Ed25519 identity
→ bootstrap exchange
→ managed runtime registers through existing realtime + /client protocol
→ original workspace key aliases to managed runtime key internally
→ tool result returns with original product workspace identity
```

**PASS:**

- Same workspace selection as before; no SSH, pairing, reconnect or local CLI instruction.
- Read/write/shell/git work in Cloud.
- UI/MCP still exposes the original `workspaceKey`, `deviceId`, `workspaceId`.
- Managed `cloud-*` workspace remains hidden.
- `/workspace` does not replace the original product path in user-facing identity.

**FAIL:** user must select a new Cloud workspace, managed identity leaks to UI/MCP, or bootstrap/provider secrets appear in result/log output.

---

## Case C — Fail closed instead of running the wrong workspace

### C1. No durable cloud-hydration source

1. Stop Local Runtime.
2. Use a workspace with no supported credential-free HTTP(S) Git source.
3. Invoke a filesystem operation.

**PASS:** Cloud refuses to run a blank/stale copy and no sandbox with arbitrary empty workspace is treated as success.

### C2. Workspace revoked / unauthorized

1. Revoke/remove the source workspace.
2. Attempt a call with its old key.

**PASS:** hard authorization/unavailable error. Auto must **not** create a sandbox to bypass product authorization.

### C3. OpenSandbox outage

1. Staging only: stop OpenSandbox or point its URL at an unavailable endpoint.
2. Keep Local offline.
3. Invoke a tool.

**PASS:** bounded provider-unavailable error; no infinite retry, stale success or duplicate sandbox storm.

### C4. Snapshot backend outage

Run only after Case G has created a Ready snapshot.

1. Make snapshot lookup fail in staging while keeping base Git reachable.
2. Wake the workspace.

**PASS:** CodeLocal does **not** silently clone Git and pretend the persisted workspace was restored. It fails closed until snapshot access is healthy.

---

## Case D — Warm sandbox reuse

**Goal:** prove compute is session/workspace scoped, not one sandbox per tool call.

1. Complete Case B and note the sandbox ID.
2. Run 5–10 read/search/shell operations over several minutes.
3. Re-check OpenSandbox.

**PASS:** same compatible sandbox is reused and its expiration/activity is renewed. Repeated tools do not create a sandbox explosion.

---

## Case E — Multi-replica Railway routing

Run staging with 2+ CodeLocal gateway replicas sharing the same Redis/Postgres.

1. Provision Cloud runtime through any replica.
2. Confirm managed runtime WebSocket ownership belongs to one gateway instance.
3. Send the next HTTP/MCP request through a different gateway instance.
4. Read then mutate a disposable file.

**PASS:**

```text
original workspace key
→ Redis runtime alias
→ managed runtime key
→ Coordinator owner lookup
→ correct Railway replica
→ existing runtime WebSocket
```

No reprovision occurs merely because the HTTP request landed on another replica, and product workspace identity remains unchanged.

---

## Case F — One-time bootstrap and secret safety

Use controlled staging diagnostics only. Never paste real values into chat/issues.

1. Provision a fresh Cloud sandbox.
2. Let it exchange the bootstrap token once.
3. Replay that exact exchange token/request.
4. Inspect runtime process environment after successful bootstrap.
5. Inspect application logs for the test request IDs.

**PASS:**

- First valid exchange succeeds; replay is rejected.
- Token lookup is digest-based; plaintext token is not a Redis key.
- Sandbox generates Ed25519 private key locally; server receives/stores only public key plus credential secret hash.
- Bootstrap token is removed from runtime process environment after exchange.
- Repository hydration JSON is removed after hydration.
- OpenSandbox API key is never injected into sandbox env.
- Normal logs/results contain no bootstrap token, managed credential secret or provider API key.

---

## Case G — Idle checkpoint preserves UNCOMMITTED workspace changes

**This is the primary durability test.** Use the fast staging idle values (`45s` / `5s`) above.

1. Ensure Local Runtime is stopped and Cloud workspace is active.
2. Create a file using CodeLocal:

```text
checkpoint-proof.txt
```

with a unique value, for example:

```text
checkpoint-2026-08-29-A
```

3. Run `git status` and verify the file is **untracked/uncommitted**.
4. Do not commit or push it.
5. Stop issuing calls and wait beyond the staging idle timeout.
6. Observe OpenSandbox snapshot lifecycle.
7. Confirm a snapshot with the deterministic CodeLocal name reaches `Ready`.
8. Confirm compute sandbox deletion is requested **only after** snapshot is Ready.
9. Invoke `read_file(checkpoint-proof.txt)` using the same original workspace.
10. Observe a new/reused sandbox restored from `snapshotId`.

**PASS:**

- Wake does not require Local machine or new workspace selection.
- `checkpoint-proof.txt` still contains exactly the unique value although it was never committed/pushed.
- The restored runtime reconnects using its persisted managed identity; it does not require replaying the consumed bootstrap token.
- Original product workspace identity remains unchanged.

**CRITICAL FAIL:** file disappears, Git base is silently cloned instead of snapshot restore, sandbox is deleted before snapshot becomes Ready, or snapshot failure causes data deletion.

---

## Case H — Repeated checkpoint/wake cycles

**Goal:** catch state/alias/device leaks that only appear after multiple cycles.

1. After Case G wakes successfully, modify `checkpoint-proof.txt` to:

```text
checkpoint-2026-08-29-B
```

2. Leave it uncommitted again.
3. Let the workspace checkpoint a second time.
4. Wake it again and read the file.
5. Repeat 3 cycles if practical.
6. Check normal workspace list and managed device/session counts.

**PASS:**

- Latest uncommitted value survives every cycle.
- Exactly one product workspace remains visible.
- No `cloud-*` product workspace duplication.
- No request loops forever on stale runtime alias.
- No new sandbox is created on every ordinary call inside one active cycle.

---

## Case I — Checkpoint failure retains compute/data

1. Staging only: make snapshot creation fail (for example unavailable snapshot backend/registry).
2. Put a unique uncommitted file in Cloud workspace.
3. Let idle reaper attempt checkpoint.

**PASS:**

- Snapshot transitions/fails with an error.
- Reaper logs a bounded warning.
- Compute sandbox is **not deleted** because snapshot was not Ready.
- Once snapshot service recovers, a later checkpoint succeeds and only then compute may be deleted.

---

## Acceptance table

| Gate | Result |
| --- | --- |
| A — Local online stays Local | ⬜ |
| B — Local offline falls back Cloud | ⬜ |
| B — Read/write/shell/git work in Cloud | ⬜ |
| Product workspace identity never changes | ⬜ |
| Managed `cloud-*` workspace stays hidden | ⬜ |
| C — No unsafe fallback on auth/source/provider failure | ⬜ |
| D — Same sandbox reused in active session | ⬜ |
| E — Cross-replica route succeeds | ⬜ |
| F — Bootstrap replay rejected | ⬜ |
| F — Secrets absent from logs/results | ⬜ |
| G — Uncommitted file survives snapshot/delete/restore | ⬜ |
| H — 3 sleep/wake cycles preserve latest state | ⬜ |
| I — Failed snapshot never deletes compute/data | ⬜ |

## Human tester report template

Copy this into the PR after running the checklist:

```text
Environment:
- Control plane commit:
- Runtime image digest:
- OpenSandbox backend/runtime:
- Railway replica count:
- Test workspace:

A Local compatibility: PASS/FAIL
B Cloud fallback: PASS/FAIL
C Fail-closed cases: PASS/FAIL
D Warm reuse: PASS/FAIL
E Multi-replica: PASS/FAIL/N/A
F Bootstrap/security: PASS/FAIL
G Uncommitted persistence: PASS/FAIL
H Repeated wake cycles: PASS/FAIL
I Snapshot failure safety: PASS/FAIL

Observed sandbox IDs:
Observed snapshot IDs:
Unexpected duplicate workspace/device: YES/NO
Secrets observed in normal logs/results: YES/NO
Notes:
```

## Current hydration scope

The first Cloud boot currently hydrates credential-free HTTP(S) Git remotes. After the first successful boot, OpenSandbox snapshots preserve subsequent uncommitted Cloud workspace changes across idle compute teardown/wake cycles.

Private repositories that require managed Git credentials still require the CodeLocal-managed Git credential injection path before they can be used as a **first-boot** hydration source. Keep that limitation explicit in staging/release notes; do not weaken secret policy by placing personal access tokens in Git remote URLs or sandbox environment variables.

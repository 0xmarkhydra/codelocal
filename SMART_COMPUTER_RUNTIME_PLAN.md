# CodeLocal Smart Computer Runtime

## Objective

Make ChatGPT + CodeLocal feel much closer to Codex Computer Use while preserving the user flow:

```bash
npm install -g codelocal
codelocal
```

ChatGPT remains responsible for intent, reasoning, and task-level decisions. CodeLocal provides compact state, execution hints, fast structured automation, guarded local input, and verification/recovery metadata.

## Product principles

1. Do not turn every task into GUI automation.
2. Prefer structured interfaces before visual coordinates: code/LSP/files -> shell/process -> browser DOM/Playwright -> Accessibility/UIA/AT-SPI -> screenshot/visual fallback -> raw coordinates.
3. Reduce MCP round-trips by batching observation, action, and verification where safe.
4. Browser, computer, shell, filesystem, Git, and extension calls keep their existing permission boundaries.
5. Privileged desktop execution stays on the paired user device.
6. Do not persist source contents, screenshots, raw terminal commands, or secrets in task memory.
7. Unsupported platform capabilities must be reported honestly; never emulate an independent cursor by stealing the user's physical pointer.
8. Preserve npm packaging and avoid requiring Python for the normal runtime.

## Architecture

```text
ChatGPT
  |
  | compact MCP tools
  v
Smart MCP Layer
  |
  +-- bounded task memory
  +-- routeHint
  +-- recovery advice
  |
  v
CodeLocal Runtime
  |-- Code / LSP / Files / Git
  |-- Shell / Process
  |-- Persistent Browser Session
  |-- Native Computer Controller
  |     |-- macOS Accessibility + CoreGraphics
  |     |-- Windows UIA + native input
  |     `-- Linux AT-SPI + session backend
  `-- Verification
```

## Review scope implemented on `feat/smart-computer-runtime`

### S1 — Smart desktop observation and semantic actions

Implemented:

- `computer(action=observe)` batches window metadata and optional accessibility tree retrieval.
- semantic desktop clicks accept human target text such as `Continue`.
- target resolution uses fresh accessibility data before coordinate fallback.
- target matching prefers exact accessible names and interactive controls, and penalizes disabled controls.
- Windows-style `node` roots and nested `children` are handled.
- `verify=true` returns a fresh post-action desktop observation in the same MCP call.
- semantic target geometry is retained when the backend provides bounds.

### S2 — Bounded ChatGPT-session working memory

Implemented as gateway working memory keyed by:

```text
user + MCP session + workspace
```

Stored fields are deliberately small:

```text
task
branch
touchedFiles
recentChecks (stable operation IDs only)
recentErrors (stable operation IDs only)
lastAction
updatedAt
```

Properties:

- bounded entry count
- 24-hour TTL
- session/workspace isolation
- no source contents
- no screenshots
- no raw terminal commands
- no secret-bearing command text
- survives model context compression while the gateway session remains alive

Non-goal for this PR: durable continuation across a cloud process restart or an unrelated new MCP session. Cross-thread handoff can build on this state model later without weakening session isolation.

### S3 — Hybrid execution hinting

Implemented:

- task intent is mapped to the cheapest reliable lane:

```text
code -> shell -> browser -> computer
```

- actual workspace capabilities constrain the recommendation.
- no available capability returns `none`; CodeLocal does not pretend Computer Use is available.
- `context` returns `routeHint` alongside task memory.
- the router recommends only; it never executes an action or crosses an approval boundary.

### S4 — Bounded recovery guidance

Implemented:

- classify compile, type, test, runtime, environment, permission, stale-UI, and unknown failures.
- permission failures never auto-retry.
- unknown failures never blind-retry.
- stale UI requires re-observation before bounded retry.
- compile/type/test recovery is capped at two attempts; runtime recovery at one.
- MCP error structured content receives conservative recovery advice.

This PR does not introduce an autonomous unbounded repair loop.

### S5 — Browser action + verification batching

The existing BrowserController already reuses a workspace-scoped Playwright session. This PR adds:

- `verify=true` for browser click/fill/press.
- post-action browser snapshot in the same MCP call.
- verification failure is returned as metadata instead of hiding a successfully completed primary action.

### S7 — Independent agent cursor foundation

Implemented on supported macOS sessions:

- visual cursor is a separate transparent Cocoa overlay.
- overlay ignores mouse events and does not become the input mechanism.
- semantic click still executes through Accessibility after authorization.
- local easing and click pulse avoid model round-trips for animation.
- cursor rendering is best-effort: overlay failure cannot fail an otherwise valid approved click.
- capability is enabled only for a usable single-display macOS Accessibility session.

Windows and Linux currently report the independent cursor as unavailable rather than moving the user's real pointer and calling it a virtual cursor. Their existing native automation paths remain unchanged.

## Safety invariants

These are release blockers:

- existing Computer Use opt-in stays intact.
- action-level authorization stays intact.
- cursor rendering never grants permissions or synthesizes its own input action.
- secure desktop / UAC-style protected surfaces remain unsupported.
- task memory must not store raw terminal commands or source contents.
- semantic lookup always refreshes the UI tree before acting on a human target.
- verification and recovery metadata never silently re-execute destructive actions.

## Testing added in this review scope

Unit coverage includes:

- semantic target scoring
- disabled-control preference
- Windows-style semantic tree traversal
- semantic target bounds retention
- automation protocol/capability gates
- browser `verify` forwarding
- computer semantic-target/observe forwarding
- task-state session/workspace isolation
- deterministic TTL expiry
- task-memory command secrecy
- route capability projection
- no-capability `LaneNone`
- conservative recovery classification and retry bounds
- MCP image transport marker handling

## Validation status

GitHub Actions is configured to run Go tests/vet/build on Ubuntu, macOS, and Windows plus npm packaging and cloud-image smoke builds.

At the time of this review branch, GitHub is not starting runners because the repository account has a Billing/spending-limit problem. Jobs terminate with `runner_id=0` and no steps. Therefore this branch must not be merged to `dev` solely on the basis of CI status; CI needs to be rerun after Billing is restored.

## Remaining roadmap after this review scope

These are intentionally not claimed as complete by PR #11:

- durable cross-thread/task handoff across unrelated MCP sessions
- source-hash/dependency-neighborhood caches for deeper coding-speed gains
- automatic targeted-test selection and safe parallel verification
- visual grounding model/provider fallback when accessibility/DOM cannot resolve a target
- multi-display macOS agent-cursor coordinate normalization
- independent Windows layered-window cursor overlay with DPI-safe coordinates
- Linux/X11 overlay where supported; Wayland remains compositor/policy dependent
- broader platform GUI smoke tests

## Integration strategy

PR #11 is a **Draft integration/review branch**, not a recommendation to merge all roadmap phases as one production change. It collects the tightly coupled foundation so reviewers can inspect the end-to-end contracts together.

Before production merge, the implementation may be squash-merged as one coherent foundation or split into smaller merge units if review identifies independent risk boundaries. In either case, `dev` must not receive the change until build/test validation is available and reviewer findings are resolved.

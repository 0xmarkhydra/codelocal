# CodeLocal Subagent — Master Plan (tham khảo OpenCode)

Status: Proposed
Date: 2026-09-05
Owner: CodeLocal
Related:
- `docs/plans/runtime/CODELOCAL_AGENT_OS_V3_MASTER_PLAN.md` (§21, §23 — adaptive scheduler, child brief)
- `docs/plans/runtime/UNIVERSAL_AGENT_RUNTIME_PLAN.md` (§25 — multi-agent orchestration)
- `docs/plans/integrations/MCP_TOOL_SURFACE_PLAN.md` (§5 — compact surface, không phình tool)

> Mục tiêu: đưa **subagent kiểu OpenCode** vào CodeLocal như một lớp ủy quyền có biên (bounded delegation),
> tái dùng nguyên V3 hiện có (Specialist, ChildBrief, TaskDAG, AgentGraph, Mailbox, Prepare/Execute),
> không thêm tool MCP mới nếu không cần thiết.

---

# 0. Quyết định tóm tắt

1. Subagent = **định nghĩa khai báo (declarative)** + **thực thi bị giới hạn** (depth, concurrency, token, write-scope, tool allowlist).
2. Không copy OpenCode nguyên xi. Chỉ hấp thụ pattern đã chứng minh, ép qua các bất biến V3 (INV-1..INV-12).
3. Tái dùng `internal/orchestration` làm lõi: `Specialist` → builtin subagents, `ChildBrief` → hợp đồng giao việc,
   `TaskDAG` + `AgentGraph` + `Mailbox` → topology bền, `PrepareAgentOS/ExecutePreparedAgentOS` → admission rồi mới dispatch.
4. V1: lead (ChatGPT / bounded `agent`) fan-out sang subagent qua **tham số `team` của tool `agent` hiện có** — không thêm top-level tool mới.

---

# 1. Tham khảo OpenCode — hấp thụ gì, loại gì

## 1.1 OpenCode làm gì (khảo sát)

```text
.opencode/agents/<name>.md  +  frontmatter
  description   // khi nào nên dùng subagent này
  mode          // primary | subagent | all
  model         // override model cho subagent, ví dụ "anthropic/claude-..."
  temperature
  tools         // allowlist, hỗ trợ glob/wildcard, ví dụ "read_*", "edit"
  permission    // allow | ask | deny per tool
  prompt        // system prompt riêng (body markdown)
```

- Primary agent gọi subagent qua **Task tool**: `Task(subagent_name, description, prompt)`.
- Subagent chạy trong **context riêng**, trả về **kết quả gọn**, không dump full transcript về primary.
- Discovery nhiều tầng: project-local → global → builtin.
- Model riêng cho từng subagent (explore dùng model rẻ/nhanh, implement dùng model mạnh).

## 1.2 Hấp thụ (absorb)

- Định nghĩa subagent bằng markdown + frontmatter, discovery nhiều tầng.
- `description` dùng để route (semantic trigger), không chỉ keyword.
- `mode` để phân biệt primary / subagent / all.
- Tool allowlist + permission per subagent.
- Context riêng + report gọn (`AgentReport`), không forward transcript.
- Model/profile override per subagent (dạng `EngineProfile`, không hard-code provider).

## 1.3 Không copy (do-not-copy)

- Không để subagent tự ý gọi host API / fs / network thô — mọi hành động qua Policy Kernel + Tool Program capability bindings.
- Không cho manifest/project config nới lỏng policy (INV-8 đơn điệu: chỉ siết, không nới).
- Không thêm public tool `spawn_agent / join_agent / mailbox_send` — giữ compact surface (V3 §32, MCP plan §8).
- Không hard-code brand model (`claude-*`, `gpt-*`) vào planning — routing dùng `EngineProfile` + capability (hiện `RecommendSpecialist` đã đúng hướng này).
- Không forward full transcript giữa các subagent (tốn token, rò rỉ secret).

---

# 2. Hiện trạng CodeLocal (đã có gì)

| Cần cho subagent | Hiện có | File |
|---|---|---|
| 7 vai trò bị giới hạn | `Specialist` + `DefaultSpecialistPolicies()` (tool allowlist, MaxDepth, MaxConcurrentChildren, TokenBudget, ReadOnly) | `internal/orchestration/specialists.go` |
| Hợp đồng giao việc + join fail-closed | `ChildBrief`, `IssueChildBrief`, `JoinTeamResults` (unknown brief → failed, unverified → needs_review) | `internal/orchestration/child_brief.go` |
| Phân tầng T0–T4, admission ngân sách | `ClassifyTaskTier`, `PlanAdaptiveTeam`, `ReserveTeamBudget` | `internal/orchestration/adaptive_scheduler.go` |
| Admission-then-dispatch | `PrepareAgentOS` (budget → route → graph → DAG, chưa start engine) | `internal/orchestration/agent_os.go` |
| Thực thi sóng DAG song song hữu hạn | `ExecutePreparedAgentOS` (parallel cap, lifecycle, settle reservation, verify, finalize lead) | `internal/orchestration/agent_os_executor.go` |
| Topology bền + mailbox bền | `agentruntime.AgentGraph`, `orchestration.TaskDAG`, `orchestration.Mailbox` | `graph.go`, `taskdag.go`, `mailbox.go` |
| Bounded executor hiện tại | `runBoundedAgent` (max 12 model steps / 20 ops, `autonomousStepPolicy`, auto-verify, learned-skill replay) | `internal/mcpgateway/agent_tool.go` |
| Planner + verification + quality gate | `BuildPlan`, `BuildVerificationPlan`, `EvaluateQuality` | `internal/orchestration/planner.go` |

**Khoảng trống:** chưa có định nghĩa subagent khai báo do user/project tự viết; chưa có model-override/temperature per subagent;
chưa có fan-out thật từ bounded `agent` (hiện chỉ chạy 1 luồng steps tuyến tính); `Specialist` hard-code 7 vai, chưa có registry mở rộng.

---

# 3. Định nghĩa Subagent CodeLocal

```text
SubagentDefinition
  name            // kebab-case, duy nhất trong scope
  description     // khi nào nên delegate (dùng cho routing)
  mode            // primary | subagent | all
  role            // ánh xạ Specialist hiện có (quick/investigator/implementer/tester/reviewer/security/deep)
  engineProfile   // fast | balanced | coding | reasoning | strong  (không nêu provider)
  temperature     // optional, bounded 0..1, default theo profile
  tools           // allowlist, hỗ trợ glob: "read_*", "lsp_*", "edit_*"
  permission      // map tool-prefix -> allow | ask | deny (deny thắng)
  tokenBudget     // cap, kế thừa DefaultSpecialistPolicies khi thiếu
  maxDepth        // default theo role
  maxConcurrent   // default theo role
  readOnly        // default theo role
  writeScopes     // glob write-scope mặc định, rỗng = không được ghi
  prompt          // body markdown: nhiệm vụ, nguyên tắc, output contract
```

Quan hệ với khái niệm cũ:

```text
Specialist (code)  ==  builtin SubagentDefinition (7 vai mặc định)
Skill (learned)    ==  workflow đã học, dùng bên trong 1 subagent
Subagent           ==  đơn vị ủy quyền có context + budget + tool-scope riêng
```

---

# 4. Kiến trúc mục tiêu

```text
ChatGPT / Lead
  └─ agent(objective, steps, team=[...])      // 1 MCP call, KHÔNG thêm tool mới
       ├─ context(action=task) grounding
       ├─ PlanAdaptiveTeam (tier → members)
       ├─ IssueChildBrief per member          // brief = join key
       ├─ ReserveTeamBudget (admission)
       ├─ AgentGraph.Spawn (lead → child, EdgeDelegate)
       ├─ TaskDAG.Add (node + BlockedBy + read/write scope)
       ├─ Mailbox assignment (task-node:<id>)
       ├─ ExecutePreparedAgentOS (sóng runnable, parallel cap)
       │    └─ mỗi child: context packet riêng + tool allowlist riêng
       │         + autonomousStepPolicy + Policy Kernel
       │         └─ AgentReport gọn (không transcript)
       └─ JoinTeamResults (complete / needs_review / failed)
            └─ verify.changes + required checks → finalize lead
```

Nguyên tắc:

- **Admission trước dispatch:** budget, route, graph, DAG ok hết mới start engine (đã có trong `PrepareAgentOS`).
- **Brief là join key:** kết quả không gắn brief đã issue → ignored, never merged (đã có trong `JoinTeamResults`).
- **Song song chỉ khi đáng:** T0/T1 single-agent; T2 implement+review; T3/T4 fan-out (giữ logic `PlanAdaptiveTeam`).

---

# 5. Định dạng file định nghĩa

Vị trí discovery (ưu tiên từ cao → thấp, cao thắng khi trùng tên):

```text
<cwd>/.codelocal/agents/*.md        // project-local, theo checkout
~/.codelocal/agents/*.md            // user-global
builtin                             // 7 Specialist hiện có
```

Ví dụ `.codelocal/agents/explorer.md`:

```markdown
---
name: explorer
description: Khảo sát nhanh codebase, tìm file/symbol liên quan. Chỉ đọc, không sửa.
mode: subagent
role: investigator
engineProfile: balanced
temperature: 0.2
tools:
  - read_*
  - search_*
  - lsp_*
  - context_*
permission:
  edit_*: deny
  terminal_*: deny
tokenBudget: 24000
readOnly: true
---

Bạn là subagent khảo sát. Chỉ trả AgentReport gọn:
- files liên quan (tối đa 10, kèm lý do 1 dòng)
- symbol/entrypoint chính
- giả thuyết nguyên nhân (tối đa 3)
- bước tiếp theo đề xuất
Không sửa code. Không đoán ngoài evidence.
```

Validation (fail-loud):

- `name` kebab-case, required; `mode` ∈ {primary, subagent, all}; `role` phải map được sang `Specialist` đã biết.
- `tools` glob hợp lệ, chuẩn hóa sort+dedupe (tái dùng `normalizeTaskStrings`).
- `permission` chỉ nhận allow|ask|deny; `deny` luôn thắng allow.
- `tokenBudget > 0`, `temperature` trong [0,1] nếu có.
- body prompt ≤ 8KB, quét secret thô (token/key) trước khi load — chứa secret → reject + cảnh báo.
- Project-local không được mở rộng tool vượt builtin role tương ứng (chỉ thu hẹp) — giữ INV-8.

---

# 6. Routing: khi nào delegate cho ai

Mở rộng `RecommendSpecialist` hiện có thành resolver 2 bước:

1. **Semantic match:** score `description` của registry subagent vs objective + taskKind (keyword hiện tại giữ làm fallback, sau nâng bằng Project Brain retrieval).
2. **Constraint gate:** tool yêu cầu ⊆ allowlist subagent; write cần thiết → loại readOnly; securitySensitive → chỉ `security`/reviewer chain.

`PlanAdaptiveTeam` giữ nguyên tier, nhưng member `Role` có thể là builtin **hoặc** tên subagent custom (resolve → Specialist + policy + prompt riêng).

---

# 7. Thực thi: context riêng + AgentReport

Mỗi child nhận **ContextPacket riêng**, biên từ ChildBrief:

```text
objective (1 việc, 1 câu)
mandatory rules (từ Project Brain, không trộn với repo text)
readPaths / writePaths
allowed tools (projection của định nghĩa)
tokenBudget còn lại
verification expectation
```

Child trả về **AgentReport** (lưu qua Mailbox, payload JSON bounded ~4KB):

```text
AgentReport
  briefId
  status            // done | blocked | failed
  summary           // ≤ 500 từ
  filesTouched[]    // tối đa 20
  evidenceRefs[]    // diagnostics / test / diff ref, không dump log thô
  verification      // đã tự chạy gì, pass/fail
  followups[]       // việc còn lại cho lead
```

Lead join bằng `JoinTeamResults` đã có: failed/unknown → failed; done-nhưng-chưa-verify → needs_review.

---

# 8. Tool projection & policy (quan trọng nhất)

- Trước mỗi engine request của child, project capability surface từ: `workspace perms + policy + sandbox + approval mode + engine capability + subagent allowlist/permission`.
- `autonomousStepPolicy` (agent_tool.go) mở rộng: nhận thêm `SubagentDefinition`, deny mặc định khi operation ngoài allowlist; `approvalToken` không bao giờ auto-consume (giữ nguyên).
- Hash-safe edit giữ nguyên: `edit.patch/format` ok; `replace/write/apply` cần `expectedHash`.
- `terminal.run` trong subagent: chỉ verification commands đã công nhận (`safeAutonomousVerificationCommand`) trừ khi brief cho phép tường minh + approval.
- Mọi side-effect vẫn qua Policy Kernel + approval broker; subagent không có đường tắt.

---

# 9. Budget / depth / concurrency

Giữ default `DefaultSpecialistPolicies`, cho custom override **thu hẹp**:

```text
depth: child.ParentDepth + 1 > policy.MaxDepth → ErrDelegationDepth
concurrency: ActiveChildren >= MaxConcurrentChildren → ErrDelegationConcurrency
token: ReserveTeamBudget trước dispatch; settle/release theo kết quả (đã có)
recursion: AgentGraph.pathExists guard chu trình; subagent không được spawn chính nó
```

---

# 10. MCP surface: không thêm tool

- Giữ 14 tool compact. Mở rộng schema `agent` thêm trường optional duy nhất:

```json
{ "team": [{ "subagent": "explorer", "objective": "...", "readPaths": [...], "writePaths": [...], "tokenBudget": 12000 }] }
```

- Tương thích ngược: thiếu `team` → hành vi bounded đơn luồng hiện tại.
- `team` rỗng + objective phức tạp → server tự `PlanAdaptiveTeam` (giữ default Auto).
- Mọi primitive nội bộ (`spawn`, `mailbox_send`, `worktree_create`, `patch_apply`, `context_compact`, `savepoint_create`) **không** thành public tool.

---

# 11. Lộ trình

## S0 — Khảo sát OpenCode (0.5–1 ngày)
- Chốt semantics `mode`, `permission`, Task-tool args; ghi ADR ngắn.
- Gate: bảng map OpenCode-field → CodeLocal-field được duyệt.

## S1 — Registry + validation (core, không đổi runtime)
- `internal/orchestration/subagents.go`: struct, parse frontmatter, discovery 3 tầng, validate, unit test (tên trùng, glob xấu, secret, deny-thắng).
- Builtin = 7 Specialist hiện có (snapshot test).
- Gate: `go test ./internal/orchestration/ -run Subagent`.

## S2 — Delegate 1 cấp (lead → 1 child)
- Nối registry → `IssueChildBrief` → `PrepareAgentOS` → `ExecutePreparedAgentOS` với `MaxParallel=1`.
- Context packet riêng + AgentReport qua Mailbox; join qua `JoinTeamResults`.
- Gate: demo `explorer → implement` trên 1 bug thật, resume sau restart không mất brief.

## S3 — Fan-out song song + join (T2–T4)
- Mở `team` trong schema `agent`; parallel cap theo tier; reviewer chain sau implement.
- Chaos: kill runtime giữa fan-out → rebuild từ event log, không double-dispatch (idempotency key đã có).
- Gate: benchmark T2/T3: quality ↑ hoặc bằng, token/op bounded.

## S4 — Project custom subagents
- Load `.codelocal/agents/*.md`, policy thu-hẹp-only, redaction secret.
- Gate: 1 repo mẫu định nghĩa `explorer` + `db-migrator` chạy e2e qua MCP thật.

## S5 — Học routing (sau cùng)
- Ghi `subagent_id + verified outcome` vào Experience; router ưu tiên subagent có verified success (min-sample + smoothing, như UAR9).
- Gate: warm-task token/task giảm không kèm quality regression.

---

# 12. Đo lường

```text
delegation_depth_histogram
delegation_fanout_count
subagent_verified_success_rate (per subagent_id)
tokens_per_verified_task (cold vs warm)
join_verdict_rate (complete / needs_review / failed)
stale_overwrite_attempts (= 0)
approval_bypass_attempts (= 0)
resume_recovery_rate
```

---

# 13. Rủi ro & KHÔNG làm

- Không song song hóa edit/gi write/terminal-approval để lấy wall-clock — mutation giữ ordered.
- Không replay skill lên subagent không tương thích capability.
- Không forward transcript thô; không persist secret vào brief/report/handoff.
- Không auto-routing học từ 1–2 mẫu anecdotal.
- Không bundle provider binary; không scrape credential provider.

---

# 14. Definition of Done (V1)

1. Định nghĩa subagent bằng markdown + frontmatter, 3 tầng discovery, validate fail-loud.
2. Lead delegate qua `agent.team`, admission-trước-dispatch, depth/concurrency/budget enforced.
3. Mỗi child có context + tool-scope riêng, trả AgentReport gọn qua Mailbox.
4. Join fail-closed (`JoinTeamResults`), verify độc lập quyết định done.
5. Không thêm public MCP tool; schema-size không tăng vật chất.
6. Restart giữa fan-out recover đúng topology, không mất/không trùng việc.
7. `go test ./internal/orchestration/ ./internal/mcpgateway/` xanh; chaos cơ bản (kill mid-fanout, duplicate brief, unverified child) có regression test.

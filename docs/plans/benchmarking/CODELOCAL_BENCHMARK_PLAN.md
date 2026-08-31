# CodeLocalBench — Benchmark Codex Direct vs Codex + CodeLocal

## 1. Mục tiêu

Xây một benchmark có thể chạy lại để trả lời bằng số liệu:

> Khi dùng cùng Codex, cùng model, cùng repository, cùng commit và cùng task, CodeLocal có giúp giảm lượng context/token cần thiết mà vẫn giữ hoặc tăng tỷ lệ hoàn thành task hay không?

Benchmark này không được tối ưu để tạo một con số marketing đẹp. Mục tiêu là tạo bằng chứng kỹ thuật có thể kiểm tra độc lập và có thể công khai raw result.

---

## 2. Thiết kế A/B benchmark

Mỗi task chạy ở hai chế độ:

### A — Codex Direct

Codex dùng tool/code navigation mặc định, không dùng CodeLocal.

### B — Codex + CodeLocal

Cùng Codex và cùng model, nhưng được dùng CodeLocal Project Brain, project map, LSP, call graph, memory, learned skills và các tool CodeLocal liên quan.

Các biến phải giữ nguyên giữa A và B:

- model và model settings;
- prompt;
- repository snapshot;
- commit SHA;
- dependency lockfiles;
- environment;
- timeout;
- grading rules;
- số trial.

Biến cần thay đổi duy nhất là có hay không có CodeLocal.

---

## 3. Freeze repository snapshot

Không benchmark trực tiếp trên working tree đang thay đổi.

Mỗi benchmark case cần:

1. Chọn repository nguồn.
2. Chọn commit SHA cố định.
3. Tạo disposable worktree/container/sandbox cho từng trial.
4. Reset về đúng SHA trước mỗi run.
5. Không để run trước ảnh hưởng run sau.

Metadata tối thiểu:

```json
{
  "benchmark_version": "1.0.0",
  "task_id": "auth-refresh-001",
  "repo": "0xmarkhydra/codelocal",
  "commit": "abc123...",
  "model": "...",
  "mode": "direct",
  "trial": 1
}
```

---

## 4. Xây bộ đề theo triết lý CursorBench

Không cần lấy nguyên đề CursorBench. CodeLocalBench nên lấy task thật từ lịch sử engineering đã từng giải quyết.

### Nhóm task

#### 4.1 Code search / navigation

- Hàm X được gọi từ đâu?
- API này phụ thuộc module nào?
- Tìm tất cả call-site bị ảnh hưởng khi đổi signature.

#### 4.2 Codebase understanding

- Giải thích luồng auth.
- Xác định component chịu trách nhiệm routing workspace.
- Tìm dependency path giữa hai subsystem.

#### 4.3 Bug fixing

- Fix regression có test tái hiện.
- Fix build Windows/macOS/Linux.
- Fix token refresh/session expiry.

#### 4.4 Feature implementation

- Thêm option nhỏ xuyên nhiều file.
- Thêm endpoint có test.
- Thêm CLI flag và cập nhật behavior.

#### 4.5 Refactor

- Tách responsibility khỏi file lớn.
- Rename API và cập nhật dependency.
- Loại bỏ implementation cũ mà không phá compatibility.

#### 4.6 Code review / bug finding

- Cho một diff cố định và yêu cầu tìm defect.
- Chấm theo ground-truth defects đã biết.

#### 4.7 Long-horizon / continuation

- Tiếp tục task từ một session trước.
- Tận dụng Project Brain/memory hợp lệ đã có.

Nhóm này đặc biệt quan trọng vì lợi thế của CodeLocal không chỉ nằm ở retrieval một lần mà còn ở project intelligence tích lũy qua nhiều phiên.

### Quy mô đề xuất

- Smoke benchmark: 10 task.
- Internal benchmark: 30–50 task.
- Public benchmark: 50–100+ task sau khi harness ổn định.

Không cần tạo 100 task ngay từ đầu. Ưu tiên harness và grader đúng trước.

---

## 5. Cấu trúc benchmark trong repo

```text
benchmarks/
  codelocalbench/
    VERSION
    tasks/
      search/
      understand/
      bugfix/
      feature/
      refactor/
      review/
      continuation/
    fixtures/
    graders/
    runner/
    results/
    reports/
```

Ví dụ task:

```json
{
  "id": "workspace-auth-001",
  "category": "bugfix",
  "repo": "0xmarkhydra/codelocal",
  "commit": "<fixed-sha>",
  "prompt": "Fix ...",
  "timeout_seconds": 1200,
  "grader": {
    "type": "test_command",
    "commands": [
      "go test ./internal/workspace/..."
    ]
  },
  "expected": {
    "must_pass": true
  }
}
```

---

## 6. Chấm đúng/sai trước, token sau

Một run ít token hơn nhưng fail task không được xem là tốt hơn.

Mỗi run phải có `task_success` do grader độc lập quyết định.

### Thứ tự ưu tiên grader

1. Test tự động.
2. Build/typecheck/lint nếu task liên quan.
3. Ground-truth assertions.
4. Hidden tests.
5. Reviewer rubric cho task khó tự động hóa.

Các task phụ thuộc reviewer rubric nên hạn chế trong V1 vì grading chủ quan.

Score tối thiểu:

```text
success = all_required_tests_pass && required_behavior_verified
```

Không dùng chính câu trả lời của agent để tự chấm agent.

---

## 7. Token telemetry

Nguồn chính phải là usage do Codex/model runtime report, không phải phép ước lượng `chars / 4` của MCP payload.

Ghi riêng nếu runtime/provider expose:

```text
input_tokens
cached_input_tokens
cache_write_input_tokens
output_tokens
reasoning_tokens
```

Không cộng tất cả rồi gọi chung là `tokens saved` mà không giải thích cache.

### Tách ít nhất hai loại

#### Fresh model input

```text
fresh_input_tokens
```

#### Cached context activity

```text
cached_input_tokens
```

Provider có pricing khác nhau cho cache, do đó báo cáo chi phí phải ghi lại pricing/version/date tại thời điểm benchmark.

---

## 8. Telemetry riêng của CodeLocal

CodeLocal cần ghi thêm để giải thích *tại sao* token giảm:

```text
mcp_request_bytes
mcp_response_bytes
mcp_payload_estimated_tokens

tool_calls
files_discovered
files_read
lines_read
bytes_read

project_map_hits
search_hits
lsp_hits
call_graph_hits
memory_hits
learned_skill_hits

context_candidates
context_items_returned
context_bytes_returned
```

`mcp_payload_estimated_tokens` chỉ là ước lượng context/payload, không phải billing/model token thật.

---

## 9. Metric chính

### 9.1 Task success rate

```text
successful_runs / total_runs
```

### 9.2 Tokens per successful task — headline metric

```text
TPS = total_model_tokens / successful_tasks
```

Nên báo cáo thêm:

```text
fresh_input_tokens_per_success
output_tokens_per_success
```

### 9.3 Cost per successful task

```text
CPS = total_estimated_model_cost / successful_tasks
```

Chỉ dùng khi có pricing rõ ràng.

### 9.4 Tool calls per successful task

Đo mức độ agent phải mò/tìm.

### 9.5 Context transferred per successful task

```text
context_bytes_returned / successful_tasks
```

Có thể quy đổi sang estimated context tokens nhưng phải ghi rõ đây không phải provider billing tokens.

### 9.6 Time per successful task

Đo wall-clock từ lúc task bắt đầu đến khi grader hoàn tất.

---

## 10. Công thức claim tiết kiệm

Ví dụ với `fresh_input_tokens_per_success`:

```text
reduction = 1 - (CodeLocal / Direct)
```

Ví dụ:

```text
Direct:    100,000 fresh input tokens / successful task
CodeLocal:  42,000 fresh input tokens / successful task

Reduction = 58%
```

Claim hợp lệ:

> Trong CodeLocalBench v1.0 trên bộ N task, Codex + CodeLocal dùng ít hơn 58% fresh input tokens trên mỗi task hoàn thành thành công so với Codex Direct, với cùng model và repo snapshots.

Không claim:

> CodeLocal luôn tiết kiệm 58% token.

---

## 11. Repeated trials và variance

Agent có tính ngẫu nhiên. Không nên chạy mỗi task đúng một lần.

### V1

- 3 trials/task để phát triển nhanh.

### Public benchmark

- 5 trials/task hoặc hơn nếu chi phí cho phép.

Ví dụ:

```text
50 tasks × 2 modes × 5 trials = 500 runs
```

Báo cáo:

- mean;
- median;
- standard deviation;
- p50/p90 duration;
- bootstrap 95% confidence interval cho metric reduction.

Nếu khoảng tin cậy quá rộng thì không nên dùng con số làm headline marketing.

---

## 12. Tránh contamination

Đây là phần dễ làm benchmark sai nhất.

Direct run không được hưởng:

- CodeLocal Project Brain;
- CodeLocal memory;
- learned skills từ run trước;
- artifact/index riêng của chế độ CodeLocal.

CodeLocal run phải tách rõ hai scenario:

### Cold CodeLocalBench

Project Brain/index được build từ snapshot nhưng không có memory của lời giải trước.

Mục tiêu: so sánh one-shot coding task công bằng.

### Warm / Longitudinal CodeLocalBench

CodeLocal được giữ project intelligence/memory hợp lệ từ lịch sử trước đó.

Mục tiêu: đo đúng giá trị `second brain` khi dự án được làm việc liên tục.

Hai benchmark phải báo cáo riêng, không trộn số.

---

## 13. V1 runner

CLI mục tiêu:

```bash
codelocal bench run --suite smoke
codelocal bench run --suite public --mode direct
codelocal bench run --suite public --mode codelocal
codelocal bench compare <run-a> <run-b>
codelocal bench report <comparison-id>
```

### Pipeline một trial

```text
Load task
  ↓
Create clean repo snapshot
  ↓
Configure mode
  ↓
Run Codex with fixed prompt/model
  ↓
Capture raw Codex JSON/events
  ↓
Capture CodeLocal telemetry if enabled
  ↓
Run independent grader
  ↓
Persist immutable result
```

---

## 14. Result schema đề xuất

```json
{
  "benchmark_version": "1.0.0",
  "task_id": "workspace-auth-001",
  "trial": 1,
  "mode": "codelocal",
  "repo_commit": "abc123",
  "model": "...",
  "success": true,
  "duration_ms": 312000,
  "usage": {
    "input_tokens": 42000,
    "cached_input_tokens": 21000,
    "output_tokens": 3400,
    "reasoning_tokens": 1800
  },
  "codelocal": {
    "tool_calls": 17,
    "files_read": 8,
    "lines_read": 1140,
    "context_bytes_returned": 89200,
    "project_map_hits": 4,
    "lsp_hits": 6,
    "call_graph_hits": 3,
    "memory_hits": 1,
    "learned_skill_hits": 0
  },
  "grader": {
    "passed": true,
    "commands": [
      {
        "command": "go test ./internal/workspace/...",
        "exit_code": 0
      }
    ]
  }
}
```

Raw Codex output phải được giữ nguyên cạnh normalized result để audit.

---

## 15. Report output

Ví dụ terminal report:

```text
CodeLocalBench v1.0
────────────────────────────────────────
Tasks                         50
Trials/task                    5

                         Direct     CodeLocal
Success rate              93.6%        94.8%
Fresh input/success       112.4K        43.1K
Cached input/success      221.0K        88.7K
Output/success              8.9K         8.1K
Tool calls/success           31           14
Files read/success            26            9
Time/success                7.8m         4.9m

Fresh input reduction       61.7%
95% CI                  58.2–64.9%
```

Headline chỉ được hiện khi success rate của CodeLocal không thấp hơn ngưỡng chấp nhận đã định nghĩa.

---

## 16. Public reproducibility

Khi public benchmark, phải publish:

- benchmark version;
- task definitions;
- repo + exact commit SHA;
- runner source;
- grader source;
- model name/config nếu được phép công bố;
- raw event logs đã loại secret;
- normalized results;
- report generator;
- cách chạy lại.

Mục tiêu là người ngoài có thể tự chạy:

```bash
git clone ...
codelocal bench reproduce --version 1.0.0
```

Không chỉ publish ảnh dashboard.

---

## 17. Những claim nên và không nên dùng

### Nên dùng

> Same Codex. Same task. Same repository snapshot. CodeLocal reduced fresh input tokens per successful task by X% on CodeLocalBench vN.

> CodeLocal reduced code context transferred by Y% while maintaining an equivalent task success rate.

### Không nên dùng

> CodeLocal always saves X% tokens.

> CodeLocal makes Codex X% smarter.

> CodeLocal reduced model tokens dựa chỉ trên `chars / 4` MCP estimate.

---

## 18. Implementation phases

### Phase 0 — Ground truth

- Chọn 10 task thật.
- Freeze commit cho từng task.
- Viết deterministic graders.
- Chạy tay A/B để xác minh phương pháp.

### Phase 1 — Benchmark runner

- Task schema.
- Snapshot/worktree lifecycle.
- Codex invocation adapter.
- Raw event capture.
- Grader runner.
- Result storage.
- Compare/report CLI.

### Phase 2 — CodeLocal telemetry

- Context bytes.
- File/line counters.
- Tool call counts.
- Project Brain/LSP/graph/memory/skill hit counters.
- Correlation ID giữa Codex run và MCP calls.

### Phase 3 — Statistical report

- repeated trials;
- mean/median;
- confidence interval;
- per-category breakdown;
- failure analysis.

### Phase 4 — Public CodeLocalBench

- 50–100+ tasks;
- sanitized public fixtures;
- reproducible runner;
- raw result publication;
- website benchmark page.

### Phase 5 — Multi-agent/provider

Sau khi Codex benchmark ổn định, mở rộng cùng harness cho:

- Claude Code + CodeLocal;
- Gemini CLI + CodeLocal;
- các MCP-compatible coding agents khác.

Không thay metric và grader giữa các provider nếu muốn so sánh chéo.

---

## 19. Definition of Done cho V1

V1 được xem là hoàn thành khi:

- có ít nhất 10 task thật thuộc tối thiểu 4 category;
- mỗi task có clean snapshot và deterministic grader;
- runner chạy được Direct và CodeLocal trên cùng task;
- capture được raw Codex usage;
- capture được CodeLocal context telemetry;
- có ít nhất 3 trials/task/mode;
- report tính được success rate và tokens per successful task;
- không trộn cached tokens với fresh input;
- raw results có thể audit lại;
- một developer khác có thể reproduce từ tài liệu mà không cần biết implementation nội bộ.

---

## 20. Quyết định kiến trúc hiện tại

CodeLocalBench nên là **benchmark native của CodeLocal**, không phụ thuộc Braintrust/Langfuse làm source of truth.

Có thể dùng tool bên ngoài để kiểm tra chéo hoặc visualize, nhưng nguồn dữ liệu canonical phải là:

1. raw Codex/provider usage events;
2. CodeLocal runtime telemetry;
3. deterministic benchmark graders;
4. immutable benchmark result artifacts.

Như vậy benchmark vừa phục vụ engineering optimization, vừa đủ mạnh để dùng làm bằng chứng public/marketing mà không phụ thuộc một SaaS đo lường bên thứ ba.

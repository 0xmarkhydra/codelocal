# Dashboard Chat — codelocal.cloud/dashboard (feat/dashboard-chat)

> Branch: `feat/dashboard-chat` (tách từ `main` e1d13bf, diverged 1 vs origin 3). Mục tiêu: chat ngay trên `https://codelocal.cloud/dashboard`, không cần vào ChatGPT nữa. Dùng **codelocal thay opencode**, model free do user gắn `CODELOCAL_LLM_*` sau.

## 1. Mục tiêu & phi chức năng
- Chat box hiển thị trên `/dashboard` dưới `LiveOverview`.
- Hiển thị logic func call (tool calling) rõ ràng: tên hàm, args, result, duration, status.
- Format chuẩn OpenAI-compatible để đổi provider free không sửa code.
- Không mất logic khi reload, có auth, rate-limit, truncate.

## 2. Kiến trúc hiện tại (đã làm)
- `web/src/app/dashboard/dashboard-chat.tsx` + `dashboard-chat.module.css` + `web/src/app/dashboard/page.tsx` (render `<DashboardChat/>`).
- `web/src/app/api/v1/dashboard/chat/route.ts` mock fallback khi thiếu `CODELOCAL_LLM_API_KEY`.
- Build pass: `typecheck` + `next build` 29/29 pages, `○ /dashboard` static.

## 3. Chuẩn API (sẽ giữ)
- Request: `POST /api/v1/dashboard/chat` (Go backend `internal/cloudserver/dashboard_chat_api.go`, Next.js thuần UI gọi qua rewrites `next.config.ts:39` `/api/v1/*` → `CODELOCAL_BACKEND_URL`) `{ message: string, history: {role:"user"|"assistant"|"tool", content:string, tool_call_id?:string}[] }`
- Response: `{ reply: string, tool_calls?: {id,name,arguments,result,durationMs,status:"done"|"error"}[], model?:string, mock?:boolean }` hoặc `{error}`
- Env **phải nằm Go server** (`railway.json`, `PORT=3333`, không phải `web/.env`): `CODELOCAL_LLM_API_KEY` (fallback `OPENAI_API_KEY`), `CODELOCAL_LLM_BASE_URL` (default `https://api.openai.com/v1`), `CODELOCAL_LLM_MODEL` (default `gpt-4o-mini`) — Next.js chỉ `CODELOCAL_BACKEND_URL`, không lộ key ra browser
- Upstream: Go gọi `POST {baseUrl}/chat/completions` với `tools` + `tool_choice:"auto"`, system prompt: "You are CodeLocal assistant..."

## 4. Phase 2 — Backend func calling (codelocal MCP)
- Định nghĩa `tools` động: lấy từ `internal/mcpgateway` / `internal/mcphub` / `learnedskills` thay vì hardcode. Các hàm dự kiến: `list_workspaces`, `get_workspace_detail`, `search_project_brain`, `recall_memory`, `list_devices`.
- Flow: LLM -> `tool_calls` -> vòng lặp gọi codelocal Go (`projectbrain`, `memory`, `codequality`) lấy `result` -> gọi lại LLM -> `reply` cuối. Timeout tool 5s, retry 1 lần.
- Trả `tool_calls` đầy đủ cho frontend, kể cả khi 1 tool fail (status error).

## 5. Phase 3 — Frontend hiển thị logic
- Mở rộng `ChatMsg` để chứa `tool` role, render pill `🔧 calling name(args)` expand JSON, badge duration, copy args/result, auto-truncate >2k chars.
- Lưu `history` gồm tool messages để LLM có context liên tục.
- Hỗ trợ 2 mode: **batch** (đợi xong) và **stream SSE** (`ReadableStream` + `text/event-stream`) để hiện `calling...` realtime.

## 6. Bổ sung để production (đã review còn thiếu)
1. **Auth & phân quyền:** check session/cookie + CSRF trên `POST /api/...`, chỉ user đã pair device và có workspace mới được gọi func liên quan Project Brain.
2. **Streaming realtime:** implement `GET /api/.../chat/stream` hoặc `POST` trả stream, frontend dùng `EventSource`/fetch stream.
3. **Tool registry động:** `GET /api/v1/mcp/tools` sync với MCP hub, không hardcode.
4. **Persist lịch sử:** lưu vào `internal/history` hoặc `Project Brain memory` + `localStorage` fallback, reload không mất, có API `GET /api/v1/dashboard/chat/history`.
5. **Bảo mật & giới hạn:** rate limit (e.g. 20 req/min/user), truncate result, không log secrets, sanitize args hiển thị.
6. **UI hoàn thiện:** markdown + code block + copy, collapse nhiều tool calls, mobile responsive, a11y, keyboard `Enter` gửi `Shift+Enter` xuống dòng.
7. **Test & quan sát:** `go test ./internal/...`, `npm run check --prefix web`, e2e mock tool, log trace `tool_calls` với requestId.

## 7. Tích hợp codelocal runtime
- Không dùng opencode server. Chat gọi trực tiếp codelocal Go runtime qua `internal/*` hoặc HTTP local khi `web` dev. Zen free model chỉ là LLM provider, logic tool vẫn do codelocal.
- Feature flag `DASHBOARD_CHAT_ENABLED` để rollout.

## 8. Verify & rollout
- `npm run typecheck --prefix web && npm run build --prefix web` pass (29/29).
- `curl -X POST /api/v1/dashboard/chat -H "content-type: application/json" -d '{"message":"test"}'` trả mock khi thiếu key, trả real khi có key.
- Manual: `npm run dev --prefix web` -> `http://localhost:3000/dashboard` thấy box "Chat với CodeLocal", thử tool call `list_workspaces`.
- Commit trên `feat/dashboard-chat`, không đụng `main`.

## 9. Next steps (todo)
- [ ] Implement `tools` động + loop tool calling trong `route.ts`
- [ ] Thêm stream + frontend tool pill
- [ ] Thêm auth + rate limit
- [ ] Persist history vào Project Brain
- [ ] Test + docs

---
*File này giữ logic không mất, mọi thay đổi phải cập nhật đây trước khi code.*

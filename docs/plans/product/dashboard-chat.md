# Dashboard Chat — codelocal.cloud/dashboard (feat/dashboard-chat)

> Branch: `feat/dashboard-chat` (tách từ `main` e1d13bf, diverged 1 vs origin 3). Mục tiêu: chat ngay trên `https://codelocal.cloud/dashboard`, không cần vào ChatGPT nữa. Dùng **codelocal thay opencode**, model free do user gắn `CODELOCAL_LLM_*` sau.

## 1. Mục tiêu & phi chức năng
- Chat box hiển thị trên `/dashboard` dưới `LiveOverview`.
- Hiển thị logic func call (tool calling) rõ ràng: tên hàm, args, result, duration, status.
- Format chuẩn OpenAI-compatible để đổi provider free không sửa code.
- Không mất logic khi reload, có auth, rate-limit, truncate.

## 2. Kiến trúc hiện tại (đã làm)
- `web/src/app/dashboard/dashboard-chat.tsx` + `dashboard-chat.module.css` + `web/src/app/dashboard/page.tsx` (render `<DashboardChat/>` thuần UI, không localStorage).
- Go `internal/cloudserver/dashboard_chat_api.go` `POST /api/v1/dashboard/chat?stream=1` SSE stream như opencode `doStream`, `GET/DELETE /api/v1/dashboard/chat/history` lưu backend `codelocal_dashboard_chat` (migration 44), mock khi thiếu key vẫn hiện pill.
- Build pass: `go vet` + `typecheck` + `next build` 29/29 pages, `○ /dashboard` static, `ƒ /api/v1/dashboard/chat` Go.

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

## 6. Bổ sung để production (đã review — đã làm 2,4)
1. **Auth & phân quyền:** `authenticatedAPIIdentity` đã check session, còn thiếu CSRF + rate limit `webutil.RateLimit` cho chat.
2. **Streaming realtime:** ✅ Go `?stream=1` SSE `delta/tool_calls/done` như opencode, Next.js `ReadableStream` render dần.
3. **Tool registry động:** còn hardcode 4 tools, cần `GET /api/v1/mcp/tools` sync `mcpgateway`.
4. **Persist lịch sử:** ✅ Go `codelocal_dashboard_chat` (migration 44) + `GET/DELETE /api/v1/dashboard/chat/history`, FE không còn `localStorage`.
5. **Bảo mật & giới hạn:** cần rate limit 20 req/min, truncate >2k, không log secrets.
6. **UI hoàn thiện:** cơ bản pill expand đã có, còn thiếu markdown/code copy, `Shift+Enter`.
7. **Test & quan sát:** cần `dashboard_chat_api_test.go` + e2e stream.

## 7. Tích hợp codelocal runtime
- Không dùng opencode server. Chat gọi trực tiếp codelocal Go runtime qua `internal/*` hoặc HTTP local khi `web` dev. Zen free model chỉ là LLM provider, logic tool vẫn do codelocal.
- Feature flag `DASHBOARD_CHAT_ENABLED` để rollout.

## 8. Verify & rollout
- `npm run typecheck --prefix web && npm run build --prefix web` pass (29/29).
- `curl -X POST /api/v1/dashboard/chat -H "content-type: application/json" -d '{"message":"test"}'` trả mock khi thiếu key, trả real khi có key.
- Manual: `npm run dev --prefix web` -> `http://localhost:3000/dashboard` thấy box "Chat với CodeLocal", thử tool call `list_workspaces`.
- Commit trên `feat/dashboard-chat`, không đụng `main`.

## 9. Next steps (todo)
- [x] `tools` loop + stream SSE + frontend pill
- [x] History backend `codelocal_dashboard_chat` (FE pure UI)
- [ ] CSRF + rate limit cho chat
- [ ] Tool registry động từ MCP hub
- [ ] UI markdown + test e2e

## 9.1. Quyết định giao diện — Codex task workspace (2026-09-06)

- `/dashboard` dùng mô hình **task workspace quen thuộc của Codex**, không trình bày như một chatbot độc lập.
- Sidebar trái ưu tiên thao tác theo thứ tự: thương hiệu CodeLocal, tạo tác vụ, tìm/lịch sử tác vụ, dự án, thiết bị/cài đặt và tài khoản.
- Header của vùng chính hiển thị **tên tác vụ hiện tại + dự án đang chọn**; CodeLocal vẫn là thương hiệu sản phẩm ở sidebar, không chiếm tiêu đề mọi cuộc trò chuyện.
- Nội dung assistant hiển thị phẳng trên canvas; nội dung user dùng bubble trung tính. Tool calls được gom thành một khối có trạng thái thật (`running`, `approval_required`, `error`, `done`) và có thể mở chi tiết.
- Composer là điểm nhấn ở đáy, dùng ngôn ngữ “giao tác vụ”; giữ nguyên mode Ask/Plan/Agent, chọn model, chọn project, upload ảnh, mục tiêu và nút dừng.
- Mobile dùng drawer trái, giữ vùng hội thoại và composer toàn chiều rộng; các control phụ được thu gọn để không làm vỡ header.
- Chỉ mượn mô hình tương tác đã quen thuộc; nhận diện, nội dung, dữ liệu và quyền truy cập vẫn là CodeLocal.

## 10. Vision/Image support — root cause & fix plan (2026-09-05)

### 10.1. Hiện tượng
- User gửi ảnh trên Dashboard Chat (paste/file, hoặc text rỗng -> `"Phân tích ảnh này"`), AI trả lời mock chung chung (`"Đã nhận ảnh ... bytes"`, `"CodeLocal Go (mock - chưa gắn key LLM nào)"`) hoặc không mô tả nội dung ảnh.
- Reload / turn 2 thì ảnh biến mất khỏi context.

### 10.2. Quy tắc hiện tại (Cursor/Opencode style)
- Model nào thì gửi ảnh model đó: ảnh + workspace đính kèm luôn forward
  nguyên block `text + image_url` cho đúng model đã chọn.
- Không có lane vision riêng, không tự đổi model sau lưng. Model không hỗ trợ
  thì upstream trả lỗi thật, không mock "Đã nhận ảnh".
- `dashboardSharedLaneBlocked(req)` chỉ chặn secret/token lộ liễu. Ảnh và
  workspace context không bao giờ bị chặn.
- BE `dashboardChatAPI`: stream + batch đều dựng `user{ text + image_url:
  req.Image }` rồi gọi `dashboardLLMRoute(selection)` thẳng. `route==0` chỉ
  còn mock khi chưa gắn key nào.
- FE `uploadImage`: presign fail -> lặng lẽ multipart fallback, không hiện
  notice chặn. `friendlyChatFailure` chỉ giữ timeout/retry chung.

### 10.3. Đã dọn (2026-09-06)
- Xóa `Vision` trên `dashboardLLMTarget`, `dashboardModelSupportsVision`,
  `dashboardVisionModelPreference`, `dashboardVisionRoute`,
  `dashboardVisionBlockedMessage`, `dashboardChatVisionTarget`.
- Xóa `Community` trên `dashboardLLMTarget`, `dashboardCommunityEligible`,
  `dashboardCommunityWorkspaceAllowed`, `dashboardCommunityOptInEligible`,
  `dashboardCommunityBlockedMessage`, `CODELOCAL_ALLOW_COMMUNITY_WORKSPACE`.
- `dashboardLLMRoute(selection)` strict, không lọc community vì ảnh/workspace.
- Giữ lại: `Image` trong history + giữ 2 ảnh gần nhất,
  `dashboardChatCompactEphemeralImage` cap 1.5MB, `image_url` turn hiện tại.
- Verify: `go test ./internal/cloudserver -run TestDashboard` pass,
  `go vet ./internal/cloudserver` sạch, `npm run typecheck --prefix web` pass,
  `git diff --check` sạch.

---
*File này giữ logic không mất, mọi thay đổi phải cập nhật đây trước khi code.*

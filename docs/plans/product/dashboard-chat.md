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
- Env **phải nằm Go server** (`deploy/railway/backend.json`, `PORT=3333`, không phải `web/.env`): `CODELOCAL_LLM_API_KEY` (fallback `OPENAI_API_KEY`), `CODELOCAL_LLM_BASE_URL` (default `https://api.openai.com/v1`), `CODELOCAL_LLM_MODEL` (default `gpt-4o-mini`) — Next.js chỉ `CODELOCAL_BACKEND_URL`, không lộ key ra browser
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
- Sidebar trái ưu tiên thao tác theo thứ tự: thương hiệu CodeLocal, tạo tác vụ, tìm/lịch sử tác vụ theo dự án, điều hướng và tài khoản.
- Header của vùng chính hiển thị **tên tác vụ hiện tại + dự án đang chọn**; CodeLocal vẫn là thương hiệu sản phẩm ở sidebar, không chiếm tiêu đề mọi cuộc trò chuyện.
- Nội dung assistant hiển thị phẳng trên canvas; nội dung user dùng bubble trung tính. Tool calls được gom thành timeline có trạng thái thật (`running`, `approval_required`, `error`, `done`) và có thể mở chi tiết.
- Composer là điểm nhấn ở đáy, dùng ngôn ngữ “giao tác vụ”; giữ nguyên mode Ask/Plan/Agent, chọn model, project, upload ảnh, mục tiêu và nút dừng.
- Mobile dùng top bar, drawer lịch sử và context sheet; giữ vùng hội thoại/composer toàn chiều rộng và tôn trọng safe area bàn phím iOS.
- Chỉ mượn mô hình tương tác đã quen thuộc; nhận diện, nội dung, dữ liệu và quyền truy cập vẫn là CodeLocal.

## 10. Vision/Image support — root cause & fix plan (2026-09-05)

### 10.1. Hiện tượng
- User gửi ảnh trên Dashboard Chat (paste/file, hoặc text rỗng -> `"Phân tích ảnh này"`), AI trả lời mock chung chung (`"Đã nhận ảnh ... bytes"`, `"CodeLocal Go (mock - chưa gắn key LLM nào)"`) hoặc không mô tả nội dung ảnh.
- Reload / turn 2 thì ảnh biến mất khỏi context.

### 10.2. Luồng hiện tại (đã đọc code)
- FE `web/src/app/dashboard/dashboard-chat.tsx#uploadImage/send`:
  - B1: `POST /api/v1/dashboard/media/presign {sha256, contentType, size}` -> `{url, imageRef, upload:{required,url,method,headers}}`.
  - B2a: nếu `upload.required`, PUT trực tiếp lên S3 presigned URL; fail -> fallback `POST /api/v1/dashboard/media/upload` (proxy qua Go + S3, check sha256).
  - B2b: nếu presign fail (`media_not_configured` khi chưa cấu hình S3) -> `useMultipartFallback=true`, gửi `FormData{ payload: JSON, image: File }`.
  - Payload JSON: `{message, history[<=12], imageMeta?, workspace?, model, mode}`. `history` chỉ `{role, content}` — không có ảnh.
- BE `internal/cloudserver/dashboard_chat_api.go#decodeDashboardChatRequest`:
  - JSON: `req.Image` (base64/data-url) + `req.ImageMeta`.
  - Multipart: parse `payload` + file `image` (limit `mediaMaxBytes()+2MB`), validate `mediaExtension`, set `req.Image="data:<ct>;base64,..."`, return `ephemeralImage=true`.
- BE `internal/cloudserver/dashboard_chat_api.go#dashboardChatAPI`:
  - `storedImage=dashboardChatStoredImage(req)` (nếu `ImageMeta` hợp lệ -> JSON meta, else `req.Image`); nhưng `if ephemeralImage { storedImage="" }` nên ảnh multipart không persist.
  - Nếu `req.ImageMeta != nil`: `req.Image = dashboardChatPreparedImageURL(...)` (presigned GET, yêu cầu `Deduplicated==true`, else `media_upload_incomplete` -> 503). Nếu `s.Media==nil` -> 503 `media_not_configured`.
  - Turn hiện tại: `messages = [system] + modelHistory + [user{ text + image_url }] + toolTranscript`. Turn hiện tại có ảnh đúng.
  - `modelHistory = dashboardExecutionHistory(...)` từ DB, không phải FE `history`.
- BE `internal/cloudserver/dashboard_chat_history.go#dashboardPersistedHistoryMessages` + `dashboardRequestHistoryMessages`:
  - Chỉ dựng `{role, content}` (+ `tool_call_id/name`), bỏ hoàn toàn `message.Image`. Ảnh các turn trước không bao giờ quay lại LLM context.
- Router `internal/cloudserver/dashboard_llm_router.go`:
  - `dashboardCommunityEligible(req)` = false khi `req.Image!="" || req.ImageMeta!=nil || req.Workspace!=nil || sensitive`. Mặc định `allowCommunity=false` khi có ảnh/workspace.
  - `dashboardLLMRoute(selection, allowCommunity)` lọc `if target.Community && !allowCommunity { skip }`. `dashboardEmperoTarget` (GLM/Qwen) và `dashboardMuseTarget` (Muse Spark 1.3) đều `Community:true`. Nếu ShopAIKey (`CODELOCAL_SHOPAIKEY_API_KEY`) chưa cấu hình -> `route` rỗng -> rơi vào nhánh mock stream (`Đã nhận ảnh...`), không gọi LLM vision thật.
  - `CODELOCAL_ALLOW_COMMUNITY_WORKSPACE=1` mới cho ảnh/workspace qua community lane (`dashboardCommunityOptInEligible`), mặc định off.
- Protocol `dashboardProtocolForModel`:
  - Zen (`opencode.ai/zen/v1`) + `muse-*` -> `responses` (`responsesInput` convert `image_url->{input_image}`). Muse Spark 1.3 free text-only hoặc gateway drop block ảnh.
  - Non-Zen + ShopAIKey + `gpt-*` -> `responses`, còn lại `chat_completions` (`image_url` giữ nguyên). Model mặc định Shop `qwen3.5-flash` chưa chắc vision.
- Media/S3 `internal/cloudserver/media.go`:
  - Chưa cấu hình `CODELOCAL_MEDIA_S3_*` (hoặc Skill storage fallback) -> `s.Media==nil` -> mọi `ImageMeta` đều 503, chỉ còn đường multipart base64.

### 10.3. Root cause chốt
1. `allowCommunity=false` khi có ảnh/workspace + ShopAIKey chưa cấu hình -> route rỗng -> mock, AI không bao giờ thấy ảnh.
2. Dù có route, default Auto về Muse Spark 1.3 (text-only) nên block `image_url/input_image` bị bỏ.
3. History (`dashboardPersistedHistoryMessages` + FE `history.slice(-12)`) drop ảnh -> turn 2 mất context.
4. Multipart fallback (`ephemeralImage`) cố ý `storedImage=""` để tránh DB phình -> không persist, history API (`dashboardChatHistoryImageURL`) không có gì để resolve.
5. `ImageMeta` yêu cầu S3 deduplicated + presigned GET TTL 3 phút (`defaultMediaURLTTL`); S3 chưa bật -> 503.

### 10.4. Plan fix (ghi để implement tiếp)
- [x] Task 1 — Vision routing: `Vision` trên `dashboardLLMTarget`, `dashboardModelSupportsVision` (Muse/GLM/Qwen = text-only), `dashboardVisionRoute` ưu tiên Shop vision, batch+stream resolve `selection` qua `dashboardChatVisionTarget`; không vision route -> lỗi rõ `dashboardVisionBlockedMessage` thay vì mock `Đã nhận ảnh`.
- [x] Task 2 — Giữ ảnh trong history: `dashboardChatHistoryItem.Image` + `dashboardRequestHistoryMessages`/`dashboardPersistedHistoryMessages` rebuild `content:[{text},{image_url}]`, giữ tối đa 2 ảnh inline gần nhất (meta JSON S3 chỉ resolve ở turn 1); FE gửi kèm `image` cho history user có ảnh.
- [x] Task 3 — Persist multipart: `storedImage=""` -> `dashboardChatCompactEphemeralImage` (cap 1.5MB data-url); `GET /history` vẫn resolve được URL hiển thị.
- [x] Task 4 — FE feedback: `media_not_configured`/`media_upload_incomplete`/thiếu vision route báo rõ trong `friendlyChatFailure` + `uploadImage`; nút gửi đã disable khi `imageUploading`.
- [x] Verify: `go test ./internal/cloudserver -run 'Dashboard(Vision|PersistedHistory|...)'` pass, `go vet` + `npm run typecheck --prefix web` pass, `git diff --check` sạch. Manual còn lại: S3 off (multipart) + S3 on (presign) + turn 2 vẫn thấy ảnh.

---
*File này giữ logic không mất, mọi thay đổi phải cập nhật đây trước khi code.*

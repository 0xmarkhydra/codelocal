package cloudserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

const (
	dashboardToolCallsPerSegment  = 32
	dashboardMaxAutoSegments      = 4
	dashboardMaxToolRounds        = dashboardToolCallsPerSegment * dashboardMaxAutoSegments
	dashboardMaxToolCalls         = dashboardToolCallsPerSegment * dashboardMaxAutoSegments
	dashboardDuplicateResultLimit = 3
	dashboardLLMRetryAttempts     = 3
)

type dashboardChatTokenUsage struct {
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	TotalTokens  int64 `json:"totalTokens"`
}

func (u *dashboardChatTokenUsage) normalize() {
	if u.TotalTokens == 0 {
		u.TotalTokens = u.InputTokens + u.OutputTokens
	}
}

func (u *dashboardChatTokenUsage) add(other dashboardChatTokenUsage) {
	u.InputTokens += other.InputTokens
	u.OutputTokens += other.OutputTokens
	u.TotalTokens += other.TotalTokens
	u.normalize()
}

func (s *Server) recordDashboardChatUsage(r *http.Request, userID, model, protocol string, usage dashboardChatTokenUsage) {
	usage.normalize()
	if s == nil || s.Store == nil || (usage.InputTokens == 0 && usage.OutputTokens == 0 && usage.TotalTokens == 0) {
		return
	}
	_ = s.Store.RecordDashboardChatUsage(context.Background(), cloud.DashboardChatUsageEvent{
		EventID: dashboardChatMessageID(r, userID, "usage:"+protocol), UserID: userID,
		ThreadID: dashboardChatThreadID(r), Model: model, Protocol: protocol,
		InputTokens: usage.InputTokens, OutputTokens: usage.OutputTokens, TotalTokens: usage.TotalTokens,
	})
}

type dashboardSafeRerouteError struct {
	Err error
}

func (e *dashboardSafeRerouteError) Error() string {
	if e == nil || e.Err == nil {
		return "safe reroute"
	}
	return e.Err.Error()
}

func (e *dashboardSafeRerouteError) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}

func dashboardSafeToReroute(err error) bool {
	var safe *dashboardSafeRerouteError
	return errors.As(err, &safe)
}

func dashboardIsTransientLLMError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && (netErr.Timeout() || netErr.Temporary()) {
		return true
	}
	var upstream *httpError
	if errors.As(err, &upstream) {
		switch upstream.Status {
		case 408, 425, 429, 500, 502, 503, 504:
			return true
		}
	}
	return false
}

func dashboardCanonicalJSON(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "{}"
	}
	var value any
	if json.Unmarshal([]byte(raw), &value) != nil {
		return raw
	}
	encoded, err := json.Marshal(value)
	if err != nil {
		return raw
	}
	return string(encoded)
}

func dashboardToolProgressFingerprint(call llmToolCall, result string) string {
	progress := strings.TrimSpace(result)
	if processProgress := dashboardRunningProcessProgress(result); processProgress != "" {
		progress = processProgress
	}
	raw := call.Name + "\n" + dashboardCanonicalJSON(call.Arguments) + "\n" + progress
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func dashboardToolResultStatus(result string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(result), &payload) != nil {
		return "done"
	}
	if nested, ok := payload["result"].(map[string]any); ok {
		status, _ := nested["status"].(string)
		if running, _ := nested["running"].(bool); running {
			return "running"
		}
		if status == "approval_required" {
			return "approval_required"
		}
		if status == "blocked" {
			return "error"
		}
	}
	if ok, exists := payload["ok"].(bool); exists && !ok {
		return "error"
	}
	if errText, exists := payload["error"].(string); exists && strings.TrimSpace(errText) != "" {
		return "error"
	}
	return "done"
}

func dashboardFinalSynthesisMessages(messages []map[string]any, reason string) []map[string]any {
	follow := append([]map[string]any{}, messages...)
	instruction := "Tool execution is paused. Do not call any more tools in this response. Give the user a concise final answer based only on the tool results already present. State what was actually completed and what is still pending. If the safe execution budget was reached, say that plainly and tell the user the completed tool work has been checkpointed for the next turn; never misdescribe that condition as a closed runtime session. Mention an approval or runtime error only when one actually exists. Do not expose raw HTTP codes or internal identifiers."
	if strings.TrimSpace(reason) != "" {
		instruction += " Execution stopped because: " + reason + "."
	}
	return append(follow, map[string]any{"role": "system", "content": instruction})
}

func dashboardFallbackReply(results []dashboardToolCall, reasons ...string) string {
	reason := ""
	if len(reasons) > 0 {
		reason = strings.ToLower(strings.TrimSpace(reasons[0]))
	}
	if strings.Contains(reason, "budget") {
		return "Lượt này đã chạm ngưỡng thực thi an toàn. Các kết quả tool đã hoàn thành được checkpoint; bạn gửi “tiếp tục” để nối từ đúng trạng thái đó, không cần đọc lại từ đầu."
	}
	if len(results) == 0 {
		return "Kết nối xử lý vừa bị gián đoạn. CodeLocal đã thử lại tự động nhưng chưa hoàn tất. Bạn gửi “tiếp tục” là mình nối tiếp ngay."
	}
	completed := 0
	failed := 0
	for _, result := range results {
		if result.Status == "error" {
			failed++
		} else {
			completed++
		}
	}
	if failed > 0 {
		return "Mình đã thực hiện được một phần công việc và giữ nguyên các kết quả đã có, nhưng một bước runtime đang bị chặn/lỗi. Bạn gửi “tiếp tục” để mình thử lại từ trạng thái hiện tại."
	}
	if completed > 0 {
		return "Mình đã thực hiện các bước trên dự án nhưng phần tổng hợp cuối vừa bị gián đoạn. Kết quả đã làm vẫn được giữ nguyên; bạn gửi “tiếp tục” để mình nối tiếp mà không làm lại từ đầu."
	}
	return "Có chút gián đoạn xử lý. Bạn gửi “tiếp tục” để CodeLocal nối tiếp ngay."
}

func dashboardFriendlyStreamError(err error) string {
	var selectedModelErr *dashboardSelectedModelError
	if errors.As(err, &selectedModelErr) {
		return selectedModelErr.Error()
	}
	if dashboardIsTransientLLMError(err) {
		return "Kết nối xử lý đang gián đoạn. CodeLocal đã thử lại tự động nhưng chưa hoàn tất; bạn gửi “tiếp tục” để nối tiếp."
	}
	return "CodeLocal gặp lỗi khi xử lý yêu cầu. Các thao tác đã hoàn thành vẫn được giữ nguyên; bạn có thể gửi “tiếp tục” để thử tiếp."
}

package cloudserver

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"strings"
)

const (
	dashboardMaxToolRounds        = 14
	dashboardMaxToolCalls         = 32
	dashboardDuplicateResultLimit = 3
	dashboardLLMRetryAttempts     = 3
)

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
	raw := call.Name + "\n" + dashboardCanonicalJSON(call.Arguments) + "\n" + strings.TrimSpace(result)
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func dashboardToolResultStatus(result string) string {
	var payload map[string]any
	if json.Unmarshal([]byte(result), &payload) != nil {
		return "done"
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
	instruction := "Tool execution is finished. Do not call any more tools. Give the user a concise final answer based only on the tool results already present. State what was actually completed, what is still pending, and mention a real approval/error only when one exists. Never expose internal orchestration limits or HTTP error codes."
	if strings.TrimSpace(reason) != "" {
		instruction += " Execution stopped because: " + reason + "."
	}
	return append(follow, map[string]any{"role": "system", "content": instruction})
}

func dashboardFallbackReply(results []dashboardToolCall) string {
	if len(results) == 0 {
		return "Kết nối xử lý vừa bị gián đoạn. Thánh Gióng đã thử lại tự động nhưng chưa hoàn tất. Bạn gửi “tiếp tục” là mình nối tiếp ngay."
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
	return "Có chút gián đoạn xử lý. Bạn gửi “tiếp tục” để Thánh Gióng nối tiếp ngay."
}

func dashboardFriendlyStreamError(err error) string {
	if dashboardIsTransientLLMError(err) {
		return "Kết nối xử lý đang gián đoạn. Thánh Gióng đã thử lại tự động nhưng chưa hoàn tất; bạn gửi “tiếp tục” để nối tiếp."
	}
	return "Thánh Gióng gặp lỗi khi xử lý yêu cầu. Các thao tác đã hoàn thành vẫn được giữ nguyên; bạn có thể gửi “tiếp tục” để thử tiếp."
}

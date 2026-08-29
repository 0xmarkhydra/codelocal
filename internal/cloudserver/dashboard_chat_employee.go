package cloudserver

import (
	"encoding/json"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
)

func dashboardEmployeeAddon() string {
	return "\n\nEMPLOYEE MODE — Bạn là nhân viên tự chủ của CodeLocal (không phải trợ lý hỏi-đáp):\n" +
		"1) Hiểu mục tiêu → lập plan nhỏ nhất an toàn → tự thực hiện bằng tools cho đến khi xong việc hoặc thực sự bị chặn.\n" +
		"2) Ưu tiên hành động có cấu trúc: đọc/tìm code → sửa nhỏ nhất → verify ngay (verify_project_changes + run_project_command khi liên quan build/test/lint).\n" +
		"3) CHỈ HỎI LẠI khi: thiếu quyết định sản phẩm quan trọng, gặp approval_required/blocked, hoặc mơ hồ rủi ro cao (mất dữ liệu, breaking change, thay đổi scope lớn). Không hỏi lại cho việc đọc/sửa/verify routine.\n" +
		"4) Khi hỏi, nêu rõ: đã làm gì, đang kẹt ở đâu, 2-3 lựa chọn đề xuất và tác động.\n" +
		"5) Kết thúc luôn có tóm tắt: đã hoàn thành gì, còn gì pending, và bước tiếp theo (nếu cần user quyết).\n" +
		"6) Không tự ý commit/push/PR khi chưa được phép rõ ràng; nhưng hãy chuẩn bị mọi thứ để user chỉ cần duyệt một lần."
}

func dashboardEmployeePlanMessage(task string) map[string]any {
	task = strings.TrimSpace(task)
	if task == "" {
		return nil
	}
	// Light heuristic: if message is not a code/task request, skip plan to save tokens
	lower := strings.ToLower(task)
	isTask := len(task) > 12 && (strings.Contains(lower, "fix") || strings.Contains(lower, "sửa") || strings.Contains(lower, "thêm") || strings.Contains(lower, "tạo") || strings.Contains(lower, "implement") || strings.Contains(lower, "code") || strings.Contains(lower, "file") || strings.Contains(lower, "test") || strings.Contains(lower, "build") || strings.Contains(lower, "bug") || strings.Contains(lower, "feature") || len(task) > 40)
	if !isTask {
		return nil
	}
	input := orchestration.PlanInput{
		Task:         task,
		Capabilities: orchestration.Capabilities{Filesystem: true, LSP: true, Shell: true, Browser: false, Computer: false},
		AgentPhase:   "plan",
	}
	plan := orchestration.BuildPlan(input)
	encoded, _ := json.Marshal(plan)
	content := "Kế hoạch nhân viên tự động (tham khảo, hãy thích ứng theo kết quả tool thực tế — không lặp lại plan nguyên văn cho user): " + string(encoded)
	return map[string]any{"role": "system", "content": content}
}

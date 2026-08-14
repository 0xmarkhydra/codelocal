package orchestration

import "strings"

type Lane string

const (
	LaneNone     Lane = "none"
	LaneCode     Lane = "code"
	LaneShell    Lane = "shell"
	LaneBrowser  Lane = "browser"
	LaneComputer Lane = "computer"
)

type Capabilities struct {
	Filesystem bool
	LSP        bool
	Shell      bool
	Browser    bool
	Computer   bool
}

type Decision struct {
	Primary   Lane   `json:"primary"`
	Fallbacks []Lane `json:"fallbacks,omitempty"`
	Reason    string `json:"reason"`
}

func containsAny(text string, words ...string) bool {
	for _, word := range words {
		if strings.Contains(text, word) {
			return true
		}
	}
	return false
}

// Route recommends the cheapest reliable execution surface. It never executes
// actions or bypasses approval; existing CodeLocal policy remains authoritative.
func Route(task string, caps Capabilities) Decision {
	text := strings.ToLower(strings.TrimSpace(task))
	codeIntent := containsAny(text,
		"code", "bug", "fix", "refactor", "test", "build", "file", "function", "class", "dependency", "git",
		"mã nguồn", "sửa code", "sửa lỗi", "tối ưu", "kiểm thử", "tệp", "hàm", "phụ thuộc", "xây dựng",
	)
	browserIntent := containsAny(text,
		"browser", "website", "web page", "chrome", "safari", "firefox", "form", "dom",
		"trình duyệt", "trang web", "biểu mẫu", "điền form", "mở web",
	)
	desktopIntent := containsAny(text,
		"desktop", "window", "finder", "explorer", "paint", "settings", "application", "click", "drag", "screen",
		"máy tính", "cửa sổ", "ứng dụng", "nhấp", "bấm", "kéo", "màn hình", "cài đặt",
	)
	shellIntent := containsAny(text,
		"terminal", "command", "cli", "script", "package manager",
		"dòng lệnh", "chạy lệnh", "lệnh shell", "trình quản lý gói",
	)

	if codeIntent && (caps.LSP || caps.Filesystem) {
		fallbacks := []Lane{}
		if caps.Shell {
			fallbacks = append(fallbacks, LaneShell)
		}
		if browserIntent && caps.Browser {
			fallbacks = append(fallbacks, LaneBrowser)
		}
		if desktopIntent && caps.Computer {
			fallbacks = append(fallbacks, LaneComputer)
		}
		return Decision{Primary: LaneCode, Fallbacks: fallbacks, Reason: "prefer structured code intelligence over UI automation"}
	}
	if shellIntent && caps.Shell {
		return Decision{Primary: LaneShell, Reason: "task maps directly to a local runtime action"}
	}
	if browserIntent && caps.Browser {
		fallbacks := []Lane{}
		if caps.Computer {
			fallbacks = append(fallbacks, LaneComputer)
		}
		return Decision{Primary: LaneBrowser, Fallbacks: fallbacks, Reason: "prefer structured browser automation over desktop input"}
	}
	if desktopIntent && caps.Computer {
		return Decision{Primary: LaneComputer, Reason: "task requires native desktop interaction"}
	}
	if caps.LSP || caps.Filesystem {
		return Decision{Primary: LaneCode, Reason: "default to structured local context"}
	}
	if caps.Shell {
		return Decision{Primary: LaneShell, Reason: "shell is the best available local surface"}
	}
	if caps.Browser {
		return Decision{Primary: LaneBrowser, Reason: "browser is the best available structured UI surface"}
	}
	if caps.Computer {
		return Decision{Primary: LaneComputer, Reason: "computer is the remaining execution surface"}
	}
	return Decision{Primary: LaneNone, Reason: "no supported execution surface is currently available"}
}

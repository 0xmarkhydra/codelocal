package orchestration

import "testing"

func TestRoutePrefersCodeForCodeTask(t *testing.T) {
	decision := Route("fix login bug and run tests", Capabilities{Filesystem: true, LSP: true, Shell: true, Browser: true, Computer: true})
	if decision.Primary != LaneCode {
		t.Fatalf("expected code lane, got %#v", decision)
	}
	if len(decision.Fallbacks) == 0 || decision.Fallbacks[0] != LaneShell {
		t.Fatalf("expected shell verification fallback, got %#v", decision.Fallbacks)
	}
}

func TestRouteRecognizesVietnameseCodeIntent(t *testing.T) {
	decision := Route("sửa lỗi đăng nhập và chạy kiểm thử", Capabilities{Filesystem: true, LSP: true, Shell: true})
	if decision.Primary != LaneCode {
		t.Fatalf("expected Vietnamese code task to use code lane, got %#v", decision)
	}
}

func TestRoutePrefersBrowserBeforeComputer(t *testing.T) {
	decision := Route("open browser website and fill form", Capabilities{Browser: true, Computer: true})
	if decision.Primary != LaneBrowser {
		t.Fatalf("expected browser lane, got %#v", decision)
	}
	if len(decision.Fallbacks) != 1 || decision.Fallbacks[0] != LaneComputer {
		t.Fatalf("expected computer fallback, got %#v", decision.Fallbacks)
	}
}

func TestRouteRecognizesVietnameseBrowserIntent(t *testing.T) {
	decision := Route("mở trình duyệt vào trang web và điền biểu mẫu", Capabilities{Browser: true, Computer: true})
	if decision.Primary != LaneBrowser {
		t.Fatalf("expected Vietnamese browser task to use browser lane, got %#v", decision)
	}
}

func TestRouteUsesComputerForNativeDesktopTask(t *testing.T) {
	decision := Route("click a window in desktop application", Capabilities{Computer: true})
	if decision.Primary != LaneComputer {
		t.Fatalf("expected computer lane, got %#v", decision)
	}
}

func TestRouteRecognizesVietnameseDesktopIntent(t *testing.T) {
	decision := Route("bấm nút trong cửa sổ ứng dụng", Capabilities{Computer: true})
	if decision.Primary != LaneComputer {
		t.Fatalf("expected Vietnamese desktop task to use computer lane, got %#v", decision)
	}
}

func TestRouteReportsUnavailableWhenNoCapabilityExists(t *testing.T) {
	decision := Route("continue task", Capabilities{})
	if decision.Primary != LaneNone {
		t.Fatalf("expected no available lane, got %#v", decision)
	}
}

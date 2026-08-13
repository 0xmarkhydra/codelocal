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

func TestRoutePrefersBrowserBeforeComputer(t *testing.T) {
	decision := Route("open browser website and fill form", Capabilities{Browser: true, Computer: true})
	if decision.Primary != LaneBrowser {
		t.Fatalf("expected browser lane, got %#v", decision)
	}
	if len(decision.Fallbacks) != 1 || decision.Fallbacks[0] != LaneComputer {
		t.Fatalf("expected computer fallback, got %#v", decision.Fallbacks)
	}
}

func TestRouteUsesComputerForNativeDesktopTask(t *testing.T) {
	decision := Route("click a window in desktop application", Capabilities{Computer: true})
	if decision.Primary != LaneComputer {
		t.Fatalf("expected computer lane, got %#v", decision)
	}
}

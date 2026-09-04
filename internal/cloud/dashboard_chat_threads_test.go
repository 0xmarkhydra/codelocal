package cloud

import (
	"strings"
	"testing"
)

func TestDashboardChatAutoTitle(t *testing.T) {
	if got := DashboardChatAutoTitle("  Sửa   lỗi đăng nhập\ntrên mobile  "); got != "Sửa lỗi đăng nhập trên mobile" {
		t.Fatalf("auto title=%q", got)
	}
	if got := DashboardChatAutoTitle(" \n\t "); got != "Cuộc trò chuyện mới" {
		t.Fatalf("empty auto title=%q", got)
	}
	if got := []rune(DashboardChatAutoTitle(strings.Repeat("a", 80))); len(got) != 48 {
		t.Fatalf("auto title length=%d want 48", len(got))
	}
}

func TestDashboardChatThreadMetadataNormalization(t *testing.T) {
	if got := normalizeDashboardChatThreadTitle("   "); got != "Cuộc trò chuyện mới" {
		t.Fatalf("empty title=%q", got)
	}
	if got := len([]rune(normalizeDashboardChatThreadTitle(strings.Repeat("ừ", 150)))); got != 120 {
		t.Fatalf("title length=%d want 120", got)
	}
	if got := normalizeDashboardChatThreadModel("   "); got != "auto" {
		t.Fatalf("empty model=%q", got)
	}
	if got := len([]rune(normalizeDashboardChatWorkspaceKey(strings.Repeat("d", 600)))); got != 500 {
		t.Fatalf("workspace key length=%d want 500", got)
	}
}

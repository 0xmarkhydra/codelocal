package ui

import (
	"strings"
	"testing"
)

func TestDashboardPageWrapsStickySidebarInFullHeightColumn(t *testing.T) {
	html := DashboardPage(DashboardOptions{
		Title:   "Administration",
		Active:  "admin",
		Email:   "admin@example.com",
		CSRF:    "csrf",
		Body:    `<div style="height:2400px">long content</div>`,
		IsAdmin: true,
	})

	if !strings.Contains(html, `<div class="shell"><div class="sidebar-column"><aside class="sidebar">`) {
		t.Fatal("dashboard sidebar must be wrapped by sidebar-column so its background can span the full document height")
	}
	if !strings.Contains(styles, `.sidebar-column{`) || !strings.Contains(styles, `.sidebar{position:sticky;top:0;height:100dvh`) {
		t.Fatal("dashboard styles must keep a full-height sidebar column with sticky viewport content")
	}
}

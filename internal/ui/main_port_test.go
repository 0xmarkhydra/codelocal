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

func TestDashboardPageDoesNotRenderStandaloneTokenUsageTab(t *testing.T) {
	html := DashboardPage(DashboardOptions{
		Title:  "Overview",
		Active: "overview",
		Email:  "user@example.com",
		CSRF:   "csrf",
	})

	if strings.Contains(html, `href="/dashboard/usage"`) || strings.Contains(html, `>Token usage</span>`) {
		t.Fatal("token usage belongs inside Overview and must not render as a standalone sidebar tab")
	}
}

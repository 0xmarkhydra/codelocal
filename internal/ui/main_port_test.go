package ui

import (
	"strings"
	"testing"
)

func TestDashboardPagePinsSidebarOnDesktop(t *testing.T) {
	html := DashboardPage(DashboardOptions{
		Title:   "Administration",
		Active:  "admin",
		Email:   "admin@example.com",
		CSRF:    "csrf",
		Body:    `<div style="height:2400px">long content</div>`,
		IsAdmin: true,
	})

	if !strings.Contains(html, `<div class="shell"><div class="sidebar-column"><aside class="sidebar">`) {
		t.Fatal("dashboard sidebar must keep a reserved sidebar column next to page content")
	}
	if !strings.Contains(styles, `.sidebar{position:fixed;z-index:20;top:0;left:0;width:264px;height:100dvh`) {
		t.Fatal("desktop dashboard sidebar must stay fixed to the viewport while main content scrolls")
	}
	if !strings.Contains(styles, `@media(max-width:980px){.shell{grid-template-columns:230px minmax(0,1fr)}.sidebar{width:230px}`) {
		t.Fatal("fixed sidebar width must stay aligned with the responsive desktop grid column")
	}
	if !strings.Contains(styles, `@media(max-width:760px){.shell{display:block}`) || !strings.Contains(styles, `.sidebar{position:static;z-index:auto;left:auto;width:auto;height:auto`) {
		t.Fatal("mobile dashboard must return the sidebar to normal document flow")
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

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
	if !strings.Contains(mainPortExtras, `height:100dvh;min-height:0;grid-template-columns:256px minmax(0,1fr);overflow:hidden`) {
		t.Fatal("dashboard shell must be locked to the viewport")
	}
	if !strings.Contains(mainPortExtras, `.sidebar-column{height:100dvh;overflow:hidden`) {
		t.Fatal("sidebar column must remain pinned inside the viewport")
	}
	if !strings.Contains(mainPortExtras, `.main{height:100dvh;max-width:none;margin:0;padding:38px 44px 72px;overflow-y:auto`) {
		t.Fatal("main content must own vertical scrolling independently of the sidebar")
	}
	if !strings.Contains(mainPortExtras, `@media(max-width:1100px) and (min-width:761px){.shell{grid-template-columns:78px minmax(0,1fr)}`) {
		t.Fatal("tablet dashboard must collapse into an icon rail")
	}
	if !strings.Contains(mainPortExtras, `.sidebar.nav-open{height:100dvh;overflow-y:auto}`) || !strings.Contains(html, `data-nav-toggle`) {
		t.Fatal("mobile dashboard must use a drawer instead of scrolling the desktop sidebar with page content")
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
	if !strings.Contains(html, `href="/dashboard/invite"`) || !strings.Contains(html, `>Invite</span>`) {
		t.Fatal("invite must render as its own sidebar tab")
	}
	if !strings.Contains(html, `href="/dashboard/leaderboard"`) || !strings.Contains(html, `>Leaderboard</span>`) {
		t.Fatal("leaderboard must render as its own sidebar tab")
	}
}

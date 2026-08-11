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

	if !strings.Contains(html, `<body class="dashboard-body page-admin"><div class="shell"><div class="sidebar-column"><aside class="sidebar">`) {
		t.Fatal("dashboard must scope its viewport lock and page styling to the dashboard body while keeping a reserved sidebar column")
	}
	if !strings.Contains(html, iosDashboardTheme) || !strings.Contains(iosDashboardTheme, `.dashboard-body .overview-hero`) || !strings.Contains(iosDashboardTheme, `backdrop-filter:saturate(175%) blur(30px)`) {
		t.Fatal("dashboard must include the scoped iOS glass design system")
	}
	for _, fakeFeature := range []string{"Pro Plan", "Upgrade Plan", "Billing", "Notifications"} {
		if strings.Contains(html, fakeFeature) {
			t.Fatalf("dashboard must not invent unsupported product feature %q", fakeFeature)
		}
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
	if strings.Contains(mainPortExtras, `.sidebar-foot form{display:none}`) || !strings.Contains(mainPortExtras, `.sidebar-foot form{display:block}`) {
		t.Fatal("tablet icon rail must keep a compact sign-out action available")
	}
	if !strings.Contains(html, `aria-label="Sign out"`) || !strings.Contains(html, `title="Leaderboard"`) {
		t.Fatal("tablet icon controls must remain discoverable and accessible")
	}
	if !strings.Contains(mainPortExtras, `.sidebar.nav-open{height:100dvh;overflow-y:auto}`) || !strings.Contains(html, `data-nav-toggle`) {
		t.Fatal("mobile dashboard must use a drawer instead of scrolling the desktop sidebar with page content")
	}
	if !strings.Contains(mainPortExtras, `.shell.nav-open .main{overflow:hidden}`) || !strings.Contains(dashboardScript, `shell?.classList.toggle('nav-open', open)`) {
		t.Fatal("open mobile navigation must lock the underlying content scroll")
	}
	if !strings.Contains(dashboardScript, `if (event.key === 'Escape') setNavOpen(false)`) {
		t.Fatal("mobile navigation must close with Escape")
	}
}

func TestLandingPageKeepsDocumentScroll(t *testing.T) {
	html := LandingPage(LandingOptions{Endpoint: "https://codelocal.cloud/mcp"})
	if strings.Contains(html, `<body class="dashboard-body">`) {
		t.Fatal("landing page must not inherit the dashboard viewport lock")
	}
	if !strings.Contains(html, `<body class="landing-body">`) {
		t.Fatal("landing page must use its own scoped visual shell")
	}
	if strings.Contains(html, mainPortExtras) {
		t.Fatal("landing page must not load dashboard-only CSS overrides")
	}
	if strings.Contains(mainPortExtras, `html,body{height:100%;overflow:hidden}`) {
		t.Fatal("shared UI styles must not globally disable landing page scrolling")
	}
}

func TestLandingPageMatchesIOSDashboardProductLanguage(t *testing.T) {
	html := LandingPage(LandingOptions{Endpoint: "https://codelocal.cloud/mcp"})
	for _, want := range []string{
		`Give ChatGPT hands on your machine.`,
		`You keep control.`,
		`ChatGPT ↔ CodeLocal ↔ your machine`,
		`Authorize projects, not your whole computer.`,
		`One lightweight machine runtime.`,
		`Sensitive actions stay gated.`,
		`From zero to connected in five steps.`,
		`Your machine is the security boundary.`,
		`data-copy-value="https://codelocal.cloud/mcp"`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("landing page must include product-aligned section %q", want)
		}
	}
	for _, unsupported := range []string{"Billing", "Upgrade Plan", "Pro Plan", "Unlimited GPT tokens"} {
		if strings.Contains(html, unsupported) {
			t.Fatalf("landing page must not invent unsupported feature %q", unsupported)
		}
	}
	if !strings.Contains(html, `npm install -g codelocal`) || strings.Contains(html, `npm install -g codelocal@beta`) {
		t.Fatal("public landing install instructions must use the stable codelocal package")
	}
	if strings.Contains(html, `class="cta-icon"`) {
		t.Fatal("final CTA must not render the decorative sparkle icon")
	}
	if !strings.Contains(html, `class="hero-chatgpt-logo"`) || !strings.Contains(html, `data:image/png;base64,`) {
		t.Fatal("landing hero must use the supplied ChatGPT logo artwork")
	}
	if !strings.Contains(html, `class="cta-command"`) || !strings.Contains(html, `data-copy-value="npm install -g codelocal"`) {
		t.Fatal("final CTA must surface a useful copyable stable install command")
	}
}

func TestLandingPageHasMobileSafeLayout(t *testing.T) {
	html := LandingPage(LandingOptions{Endpoint: "https://codelocal.cloud/mcp"})
	for _, want := range []string{
		`viewport-fit=cover`,
		`env(safe-area-inset-top)`,
		`@media(max-width:700px)`,
		`.landing-actions{align-items:stretch;flex-direction:column}`,
		`.product-grid{grid-template-columns:1fr`,
		`.security-points{grid-template-columns:1fr}`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("landing page must retain mobile-safe behavior %q", want)
		}
	}
}

func TestDashboardOverviewUsesHeroInsteadOfRedundantPageHeader(t *testing.T) {
	overview := DashboardPage(DashboardOptions{Title: "Overview", Active: "overview", Email: "user@example.com", CSRF: "csrf", Body: `<div class="overview-hero">hero</div>`})
	if strings.Contains(overview, `<header class="top">`) {
		t.Fatal("overview must start with the product hero instead of repeating a page header")
	}
	leaderboard := DashboardPage(DashboardOptions{Title: "Leaderboard", Active: "leaderboard", Email: "user@example.com", CSRF: "csrf"})
	if !strings.Contains(leaderboard, `<header class="top">`) {
		t.Fatal("non-overview dashboard pages must keep their page header")
	}
}

func TestDashboardPolishRemovesDecorativeBarsAndKeepsDistinctChatGPTMark(t *testing.T) {
	if !strings.Contains(iosDashboardTheme, `.dashboard-body .nav a.active:before{display:none}`) {
		t.Fatal("active navigation must not render the extra left indicator bar")
	}
	if !strings.Contains(iosDashboardTheme, `.dashboard-body .metric-card:before{display:none}`) {
		t.Fatal("dashboard metric cards must not render decorative color bars")
	}
	if !strings.Contains(iosDashboardTheme, `.dashboard-body .sidebar .signout-icon{display:none}`) || !strings.Contains(iosDashboardTheme, `.dashboard-body .sidebar .signout-icon{display:grid}`) {
		t.Fatal("desktop must use a clean sign-out label while the tablet rail keeps an accessible icon")
	}
	if strings.TrimSpace(Icon("chatgpt")) == "" || Icon("chatgpt") == Icon("connect") {
		t.Fatal("ChatGPT must have a distinct visual mark instead of reusing CodeLocal or connection artwork")
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

func TestAuthPageUsesKeyboardSafeMobileShell(t *testing.T) {
	html := Page("Welcome back", "Sign in to CodeLocal.", `<form><input class="input"></form>`)
	for _, want := range []string{
		`<body class="auth-body">`,
		`height:100dvh;min-height:100svh`,
		`env(safe-area-inset-top)`,
		`font-size:16px`,
		`window.visualViewport?.addEventListener('resize'`,
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("auth page must include keyboard-safe mobile behavior %q", want)
		}
	}
	if strings.Contains(html, `<body class="dashboard-body`) {
		t.Fatal("auth page must use its own shell instead of dashboard chrome")
	}
}

package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestMainCompactNumber(t *testing.T) {
	tests := []struct {
		value int64
		want  string
	}{
		{0, "0"},
		{999, "999"},
		{1_000, "1K"},
		{12_400, "12.4K"},
		{1_769_792, "1.77M"},
		{2_707_980, "2.71M"},
		{1_250_000_000, "1.25B"},
	}
	for _, tt := range tests {
		if got := mainCompactNumber(tt.value); got != tt.want {
			t.Fatalf("mainCompactNumber(%d) = %q, want %q", tt.value, got, tt.want)
		}
	}
}

func TestMainExactNumber(t *testing.T) {
	if got := mainExactNumber(1_769_792); got != "1,769,792" {
		t.Fatalf("mainExactNumber() = %q, want %q", got, "1,769,792")
	}
	if got := mainExactNumber(-12_345); got != "-12,345" {
		t.Fatalf("mainExactNumber(-12345) = %q, want %q", got, "-12,345")
	}
}

func TestMainUsageMetricShowsCompactAndExactUsage(t *testing.T) {
	html := mainUsageMetric("Last 24 hours", cloud.MCPUsageSummary{Calls: 296, TotalTokensEst: 1_769_792})
	if !strings.Contains(html, ">1.77M</div>") {
		t.Fatalf("usage metric must show a compact primary value: %s", html)
	}
	if !strings.Contains(html, "~1,769,792 estimated tokens · 296 tool calls") {
		t.Fatalf("usage metric must keep the exact estimate in supporting copy: %s", html)
	}
}

package webauth

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

func passwordChangeRequest(next string) *http.Request {
	body := "next=" + url.QueryEscape(next)
	r := httptest.NewRequest(http.MethodPost, "/account/password", strings.NewReader(body))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	return r
}

func TestPasswordChangeReturnPathUsesCanonicalAccountSurface(t *testing.T) {
	for _, next := range []string{
		"",
		"/dashboard/account",
		"/dashboard/legacy-account",
		"/dashboard/devices",
		"https://evil.example",
		"//evil.example",
	} {
		if got := passwordChangeReturnPath(passwordChangeRequest(next)); got != "/dashboard/account" {
			t.Fatalf("passwordChangeReturnPath(%q) = %q", next, got)
		}
	}
}

func TestPasswordChangeRedirectEscapesMessage(t *testing.T) {
	got := passwordChangeRedirect("/dashboard/account", "error", "bad & expired")
	want := "/dashboard/account?error=bad+%26+expired"
	if got != want {
		t.Fatalf("passwordChangeRedirect() = %q, want %q", got, want)
	}
}

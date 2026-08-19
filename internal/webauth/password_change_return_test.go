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

func TestPasswordChangeReturnPathAllowsOnlyKnownAccountSurfaces(t *testing.T) {
	for _, test := range []struct {
		next string
		want string
	}{
		{"", "/dashboard/account"},
		{"/dashboard/account", "/dashboard/account"},
		{"/dashboard/account-preview", "/dashboard/account-preview"},
		{"/dashboard/devices", "/dashboard/account"},
		{"https://evil.example", "/dashboard/account"},
		{"//evil.example", "/dashboard/account"},
	} {
		if got := passwordChangeReturnPath(passwordChangeRequest(test.next)); got != test.want {
			t.Fatalf("passwordChangeReturnPath(%q) = %q, want %q", test.next, got, test.want)
		}
	}
}

func TestPasswordChangeRedirectEscapesMessage(t *testing.T) {
	got := passwordChangeRedirect("/dashboard/account-preview", "error", "bad & expired")
	want := "/dashboard/account-preview?error=bad+%26+expired"
	if got != want {
		t.Fatalf("passwordChangeRedirect() = %q, want %q", got, want)
	}
}

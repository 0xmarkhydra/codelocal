package cloudserver

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSkillJSONCSRFAdaptsHeaderWithoutMutatingOriginalRequest(t *testing.T) {
	request := httptest.NewRequest(http.MethodPatch, "/api/v1/skills/ui-ux-pro/state", nil)
	request.Header.Set("X-CSRF-Token", "csrf-from-header")
	var received string
	handler := skillJSONCSRF(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		received = r.URL.Query().Get("csrf")
	}))
	handler.ServeHTTP(httptest.NewRecorder(), request)
	if received != "csrf-from-header" {
		t.Fatalf("adapted csrf=%q want %q", received, "csrf-from-header")
	}
	if original := request.URL.Query().Get("csrf"); original != "" {
		t.Fatalf("original request query mutated: csrf=%q", original)
	}
}

func TestSkillJSONCSRFPreservesExplicitQueryToken(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/skills/import?csrf=explicit", nil)
	request.Header.Set("X-CSRF-Token", "header")
	var received string
	skillJSONCSRF(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		received = r.URL.Query().Get("csrf")
	})).ServeHTTP(httptest.NewRecorder(), request)
	if received != "explicit" {
		t.Fatalf("explicit csrf token was overwritten: %q", received)
	}
}

func TestSkillJSONCSRFWithoutHeaderLeavesRequestUntouched(t *testing.T) {
	request := httptest.NewRequest(http.MethodPost, "/api/v1/skills/publish", nil)
	seen := request
	skillJSONCSRF(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		if r != seen {
			t.Fatal("request without X-CSRF-Token should not be cloned")
		}
	})).ServeHTTP(httptest.NewRecorder(), request)
}

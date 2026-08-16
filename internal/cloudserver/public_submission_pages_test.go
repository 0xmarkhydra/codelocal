package cloudserver

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOpenAIAppsChallengeReturnsExactConfiguredToken(t *testing.T) {
	t.Setenv("OPENAI_APPS_CHALLENGE_TOKEN", "openai-verification-token-123")
	recorder := httptest.NewRecorder()
	(&Server{}).openAIAppsChallenge(recorder, httptest.NewRequest("GET", "/.well-known/openai-apps-challenge", nil))
	if recorder.Code != 200 {
		t.Fatalf("challenge status=%d", recorder.Code)
	}
	if recorder.Body.String() != "openai-verification-token-123" {
		t.Fatalf("challenge body=%q", recorder.Body.String())
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("challenge cache control=%q", got)
	}
}

func TestOpenAIAppsChallengeIsUnavailableWithoutToken(t *testing.T) {
	t.Setenv("OPENAI_APPS_CHALLENGE_TOKEN", "")
	recorder := httptest.NewRecorder()
	(&Server{}).openAIAppsChallenge(recorder, httptest.NewRequest("GET", "/.well-known/openai-apps-challenge", nil))
	if recorder.Code != 404 {
		t.Fatalf("challenge status=%d want=404", recorder.Code)
	}
}

func TestPublicSubmissionPagesExposeRequiredTrustInformation(t *testing.T) {
	t.Setenv("CODELOCAL_PUBLISHER_NAME", "CodeLocal Test Publisher")
	t.Setenv("CODELOCAL_SUPPORT_EMAIL", "support@example.test")
	t.Setenv("CODELOCAL_SECURITY_EMAIL", "security@example.test")

	tests := []struct {
		name string
		run  func(*Server, *httptest.ResponseRecorder)
		want []string
	}{
		{"privacy", func(s *Server, w *httptest.ResponseRecorder) {
			s.privacyPage(w, httptest.NewRequest("GET", "/privacy", nil))
		}, []string{"Privacy Policy", "What CodeLocal Cloud stores", "Retention and deletion", "support@example.test"}},
		{"terms", func(s *Server, w *httptest.ResponseRecorder) {
			s.termsPage(w, httptest.NewRequest("GET", "/terms", nil))
		}, []string{"Terms of Use", "Authorized use", "Acceptable use"}},
		{"support", func(s *Server, w *httptest.ResponseRecorder) {
			s.supportPage(w, httptest.NewRequest("GET", "/support", nil))
		}, []string{"Support", "support@example.test", "security@example.test", "codelocal --version"}},
		{"security", func(s *Server, w *httptest.ResponseRecorder) {
			s.securityPage(w, httptest.NewRequest("GET", "/security", nil))
		}, []string{"Security", "Execution boundary", "MCP metadata", "security@example.test"}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			test.run(&Server{}, recorder)
			if recorder.Code != 200 {
				t.Fatalf("status=%d", recorder.Code)
			}
			body := recorder.Body.String()
			for _, want := range test.want {
				if !strings.Contains(body, want) {
					t.Fatalf("page missing %q", want)
				}
			}
		})
	}
}

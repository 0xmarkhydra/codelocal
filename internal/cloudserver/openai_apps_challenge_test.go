package cloudserver

import (
	"net/http/httptest"
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

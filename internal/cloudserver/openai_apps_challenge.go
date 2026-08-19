package cloudserver

import (
	"io"
	"net/http"
	"os"
	"strings"
)

func (s *Server) openAIAppsChallenge(w http.ResponseWriter, _ *http.Request) {
	token := strings.TrimSpace(os.Getenv("OPENAI_APPS_CHALLENGE_TOKEN"))
	if token == "" {
		http.NotFound(w, nil)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	_, _ = io.WriteString(w, token)
}

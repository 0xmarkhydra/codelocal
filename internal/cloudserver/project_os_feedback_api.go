package cloudserver

import (
	"encoding/json"
	"net/http"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) registerProjectOSFeedbackRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/feedback", s.projectOSFeedbackListAPI)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/feedback/signals", s.projectOSFeedbackSignalsAPI)
	submit := http.HandlerFunc(s.projectOSFeedbackSubmitAPI)
	mux.Handle("POST /api/v1/projects/{projectID}/feedback", webutil.RateLimit(s.Store, webutil.RateLimitOptions{
		Scope: "feedback-submit-ip", Limit: 30, Window: 10 * time.Minute,
	}, submit))
	mux.HandleFunc("POST /api/v1/projects/{projectID}/feedback/{feedbackID}/decision", s.projectOSFeedbackDecisionAPI)
}

type projectOSFeedbackSubmitRequest struct {
	Source string `json:"source"`
	Kind   string `json:"kind"`
	Title  string `json:"title"`
	Body   string `json:"body"`
}

func (s *Server) projectOSFeedbackSubmitAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	var req projectOSFeedbackSubmitRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	fb, err := s.Store.SubmitProjectFeedback(r.Context(), identity.User.ID, projectID, req.Source, req.Kind, req.Title, req.Body)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "feedback_submit_failed", "detail": err.Error()})
		return
	}
	webutil.JSON(w, http.StatusCreated, map[string]any{"feedback": fb})
}

func (s *Server) projectOSFeedbackListAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	items, err := s.Store.ListProjectFeedback(r.Context(), identity.User.ID, projectID, strings.TrimSpace(r.URL.Query().Get("status")), 30)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "feedback_list_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"feedback": items})
}

func (s *Server) projectOSFeedbackSignalsAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	signals, err := s.Store.ProjectFeedbackSignals(r.Context(), identity.User.ID, projectID, 3)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "feedback_signals_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"signals": signals})
}

type projectOSFeedbackDecisionRequest struct {
	ToStatus string `json:"toStatus"`
}

func (s *Server) projectOSFeedbackDecisionAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	feedbackID := strings.TrimSpace(r.PathValue("feedbackID"))
	var req projectOSFeedbackDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	fb, err := s.Store.TransitionProjectFeedback(r.Context(), identity.User.ID, feedbackID, req.ToStatus)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "feedback_decision_failed", "detail": err.Error()})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"feedback": fb})
}

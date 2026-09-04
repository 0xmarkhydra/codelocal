package cloudserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) registerProjectOSRequirementRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/projects/{projectID}/requirement-changes", s.projectOSRequirementListAPI)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/requirement-changes", s.projectOSRequirementCreateAPI)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/requirement-changes/{changeID}/apply", s.projectOSRequirementApplyAPI)
}

type projectOSRequirementCreateRequest struct {
	Source      string   `json:"source"`
	Summary     string   `json:"summary"`
	ChangedRefs []string `json:"changedRefs"`
}

func (s *Server) projectOSRequirementCreateAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	var req projectOSRequirementCreateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	change, err := s.Store.RecordRequirementChange(r.Context(), identity.User.ID, projectID, req.Source, req.Summary, req.ChangedRefs)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "requirement_record_failed", "detail": err.Error()})
		return
	}
	// Impact analysis only — no mutation. Caller applies explicitly.
	impactTasks := []orchestration.ImpactTask{}
	goals, _ := s.Store.ListProjectGoals(r.Context(), identity.User.ID, projectID, 5)
	for _, g := range goals {
		tasks, err := s.Store.ListProjectTasks(r.Context(), identity.User.ID, g.GoalID, 50)
		if err != nil {
			continue
		}
		for _, t := range tasks {
			impactTasks = append(impactTasks, orchestration.ImpactTask{ID: t.TaskID, Title: t.Title, Description: t.Description, Status: t.Status})
		}
	}
	var refs []string
	_ = json.Unmarshal(change.ChangedRefs, &refs)
	hits := orchestration.AnalyzeRequirementImpact(append(refs, change.Summary), impactTasks)
	webutil.JSON(w, http.StatusCreated, map[string]any{"change": change, "suggestedImpacts": hits})
}

type projectOSRequirementApplyRequest struct {
	GoalID  string   `json:"goalId"`
	PlanIDs []string `json:"planIds"`
	TaskIDs []string `json:"taskIds"`
}

func (s *Server) projectOSRequirementApplyAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	changeID := strings.TrimSpace(r.PathValue("changeID"))
	var req projectOSRequirementApplyRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	result, err := s.Store.ApplyRequirementImpact(r.Context(), identity.User.ID, changeID, req.GoalID, req.PlanIDs, req.TaskIDs)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "requirement_apply_failed", "detail": err.Error()})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"impact": result})
}

func (s *Server) projectOSRequirementListAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	changes, err := s.Store.ListRequirementChanges(r.Context(), identity.User.ID, projectID, 20)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "requirement_list_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"changes": changes})
}

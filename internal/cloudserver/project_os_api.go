package cloudserver

import (
	"encoding/json"
	"net/http"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) registerProjectOSRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/project-os/summary", s.projectOSSummaryAPI)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/work", s.projectOSWorkAPI)
	mux.HandleFunc("GET /api/v1/projects/{projectID}/company", s.projectOSCompanyAPI)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/goals", s.projectOSCreateGoalAPI)
	mux.HandleFunc("POST /api/v1/projects/{projectID}/plans/{planID}/approve", s.projectOSApprovePlanAPI)
	mux.HandleFunc("POST /api/v1/tasks/{taskID}/decision", s.projectOSTaskDecisionAPI)
	s.registerProjectOSRequirementRoutes(mux)
	s.registerProjectOSFeedbackRoutes(mux)
}

func (s *Server) projectOSSummaryAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	summary, err := s.Store.ProjectOSSummary(r.Context(), identity.User.ID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "project_os_unavailable"})
		return
	}
	needsYou, _ := s.Store.ProjectNeedsYou(r.Context(), identity.User.ID, "")
	goals, _ := s.Store.ListProjectGoals(r.Context(), identity.User.ID, "", 10)
	webutil.JSON(w, http.StatusOK, map[string]any{"summary": summary, "needsYou": needsYou, "recentGoals": goals})
}

func (s *Server) projectOSWorkAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	if projectID == "" {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "project_required"})
		return
	}
	tasks, err := s.Store.ListProjectWork(r.Context(), identity.User.ID, projectID)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "project_work_unavailable"})
		return
	}
	goals, _ := s.Store.ListProjectGoals(r.Context(), identity.User.ID, projectID, 20)
	needsYou, _ := s.Store.ProjectNeedsYou(r.Context(), identity.User.ID, projectID)
	webutil.JSON(w, http.StatusOK, map[string]any{"tasks": tasks, "goals": goals, "needsYou": needsYou})
}

func (s *Server) projectOSCompanyAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	if projectID == "" {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "project_required"})
		return
	}
	goals, _ := s.Store.ListProjectGoals(r.Context(), identity.User.ID, projectID, 5)
	tasks, _ := s.Store.ListProjectWork(r.Context(), identity.User.ID, projectID)
	needsYou, _ := s.Store.ProjectNeedsYou(r.Context(), identity.User.ID, projectID)
	active := map[string]any{"goal": nil, "activeTasks": []any{}, "waitingTasks": []any{}, "tester": map[string]int{}}
	if len(goals) > 0 {
		active["goal"] = goals[0]
	}
	activeList := []cloud.ProjectTask{}
	waitingList := []cloud.ProjectTask{}
	testerCounts := map[string]int{"reviewing": 0, "testing": 0, "done": 0}
	for _, t := range tasks {
		switch t.Status {
		case "READY", "ASSIGNED", "RUNNING":
			activeList = append(activeList, t)
		case "REVIEWING":
			testerCounts["reviewing"]++
			activeList = append(activeList, t)
		case "TESTING":
			testerCounts["testing"]++
			activeList = append(activeList, t)
		case "DONE":
			testerCounts["done"]++
		default:
			waitingList = append(waitingList, t)
		}
	}
	active["activeTasks"] = activeList
	active["waitingTasks"] = waitingList
	active["tester"] = testerCounts
	webutil.JSON(w, http.StatusOK, map[string]any{"company": active, "needsYou": needsYou})
}

type projectOSCreateGoalRequest struct {
	Title     string `json:"title"`
	Objective string `json:"objective"`
}

func (s *Server) projectOSCreateGoalAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	projectID := strings.TrimSpace(r.PathValue("projectID"))
	var req projectOSCreateGoalRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	goal, err := s.Store.CreateProjectGoal(r.Context(), identity.User.ID, projectID, req.Title, req.Objective, "user")
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "goal_create_failed", "detail": err.Error()})
		return
	}
	webutil.JSON(w, http.StatusCreated, map[string]any{"goal": goal})
}

type projectOSApprovePlanRequest struct {
	ExpectedRevision int64 `json:"expectedRevision"`
}

func (s *Server) projectOSApprovePlanAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	planID := strings.TrimSpace(r.PathValue("planID"))
	var req projectOSApprovePlanRequest
	_ = json.NewDecoder(r.Body).Decode(&req)
	plan, err := s.Store.ApproveProjectPlan(r.Context(), identity.User.ID, planID, identity.User.ID, req.ExpectedRevision)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "plan_approve_failed", "detail": err.Error()})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"plan": plan})
}

type projectOSTaskDecisionRequest struct {
	ToStatus         string `json:"toStatus"`
	ExpectedRevision int64  `json:"expectedRevision"`
	Reason           string `json:"reason"`
}

func (s *Server) projectOSTaskDecisionAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	taskID := strings.TrimSpace(r.PathValue("taskID"))
	var req projectOSTaskDecisionRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "invalid_json"})
		return
	}
	task, err := s.Store.TransitionProjectTask(r.Context(), identity.User.ID, taskID, req.ToStatus, req.ExpectedRevision)
	if err != nil {
		webutil.JSON(w, http.StatusBadRequest, map[string]string{"error": "task_decision_failed", "detail": err.Error()})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"task": task})
}

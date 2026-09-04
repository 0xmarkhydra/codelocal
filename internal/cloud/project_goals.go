package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ProjectGoalStatus string

const (
	ProjectGoalDraft           ProjectGoalStatus = "DRAFT"
	ProjectGoalPlanProposed    ProjectGoalStatus = "PLAN_PROPOSED"
	ProjectGoalWaitingApproval ProjectGoalStatus = "WAITING_APPROVAL"
	ProjectGoalApproved        ProjectGoalStatus = "APPROVED"
	ProjectGoalExecuting       ProjectGoalStatus = "EXECUTING"
	ProjectGoalVerifying       ProjectGoalStatus = "VERIFYING"
	ProjectGoalDone            ProjectGoalStatus = "DONE"
	ProjectGoalBlocked         ProjectGoalStatus = "BLOCKED"
	ProjectGoalCancelled       ProjectGoalStatus = "CANCELLED"
)

func validProjectGoalStatus(s string) bool {
	switch ProjectGoalStatus(s) {
	case ProjectGoalDraft, ProjectGoalPlanProposed, ProjectGoalWaitingApproval,
		ProjectGoalApproved, ProjectGoalExecuting, ProjectGoalVerifying,
		ProjectGoalDone, ProjectGoalBlocked, ProjectGoalCancelled:
		return true
	}
	return false
}

func validProjectGoalTransition(from, to string) bool {
	if from == to {
		return true
	}
	allowed := map[string]map[string]bool{
		"DRAFT":            {"PLAN_PROPOSED": true, "CANCELLED": true},
		"PLAN_PROPOSED":    {"WAITING_APPROVAL": true, "DRAFT": true, "CANCELLED": true},
		"WAITING_APPROVAL": {"APPROVED": true, "DRAFT": true, "CANCELLED": true},
		"APPROVED":         {"EXECUTING": true, "BLOCKED": true, "CANCELLED": true},
		"EXECUTING":        {"VERIFYING": true, "BLOCKED": true, "CANCELLED": true},
		"VERIFYING":        {"DONE": true, "EXECUTING": true, "BLOCKED": true, "CANCELLED": true},
		"BLOCKED":          {"EXECUTING": true, "DRAFT": true, "CANCELLED": true},
		"DONE":             {},
		"CANCELLED":        {},
	}
	return allowed[from][to]
}

type ProjectGoal struct {
	GoalID       string `json:"goalId"`
	UserID       string `json:"userId"`
	ProjectID    string `json:"projectId"`
	Title        string `json:"title"`
	Objective    string `json:"objective"`
	Status       string `json:"status"`
	ActivePlanID string `json:"activePlanId,omitempty"`
	CreatedBy    string `json:"createdByActor"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
	Revision     int64  `json:"revision"`
}

type ProjectPlanStatus string

const (
	ProjectPlanProposed        = "PROPOSED"
	ProjectPlanWaitingApproval = "WAITING_APPROVAL"
	ProjectPlanApproved        = "APPROVED"
	ProjectPlanRejected        = "REJECTED"
	ProjectPlanStale           = "STALE"
	ProjectPlanCancelled       = "CANCELLED"
)

func validProjectPlanStatus(s string) bool {
	switch s {
	case ProjectPlanProposed, ProjectPlanWaitingApproval, ProjectPlanApproved,
		ProjectPlanRejected, ProjectPlanStale, ProjectPlanCancelled:
		return true
	}
	return false
}

type ProjectPlan struct {
	PlanID             string          `json:"planId"`
	UserID             string          `json:"userId"`
	ProjectID          string          `json:"projectId"`
	GoalID             string          `json:"goalId"`
	Summary            string          `json:"summary"`
	Flow               json.RawMessage `json:"flow"`
	Assumptions        json.RawMessage `json:"assumptions"`
	AcceptanceCriteria json.RawMessage `json:"acceptanceCriteria"`
	Status             string          `json:"status"`
	ApprovedBy         string          `json:"approvedBy,omitempty"`
	ApprovedAt         int64           `json:"approvedAt,omitempty"`
	CreatedAt          int64           `json:"createdAt"`
	UpdatedAt          int64           `json:"updatedAt"`
	Revision           int64           `json:"revision"`
}

func normalizeProjectOSTitle(v string, max int) string {
	v = strings.Join(strings.Fields(strings.TrimSpace(v)), " ")
	if v == "" {
		return ""
	}
	if r := []rune(v); len(r) > max {
		v = string(r[:max])
	}
	return v
}

func (s *Store) ensureProjectOSProject(ctx context.Context, userID, projectID string) error {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if userID == "" || projectID == "" {
		return errors.New("project os requires user and project")
	}
	var exists bool
	if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM codelocal_projects WHERE user_id=$1 AND project_id=$2)`, userID, projectID).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		return errors.New("unknown project for tenant")
	}
	return nil
}

func (s *Store) CreateProjectGoal(ctx context.Context, userID, projectID, title, objective, actor string) (ProjectGoal, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	title = normalizeProjectOSTitle(title, 200)
	objective = strings.TrimSpace(objective)
	if len([]rune(objective)) > 4000 {
		objective = string([]rune(objective)[:4000])
	}
	actor = strings.TrimSpace(actor)
	if actor == "" {
		actor = "user"
	}
	if title == "" {
		return ProjectGoal{}, errors.New("goal title is required")
	}
	if err := s.ensureProjectOSProject(ctx, userID, projectID); err != nil {
		return ProjectGoal{}, err
	}
	now := time.Now().UnixMilli()
	g := ProjectGoal{
		GoalID: "goal_" + RandomHex(12), UserID: userID, ProjectID: projectID,
		Title: title, Objective: objective, Status: string(ProjectGoalDraft),
		CreatedBy: actor, CreatedAt: now, UpdatedAt: now, Revision: 1,
	}
	_, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_project_goals(goal_id,user_id,project_id,title,objective,status,active_plan_id,created_by_actor,created_at,updated_at,revision)
VALUES($1,$2,$3,$4,$5,$6,NULL,$7,$8,$8,1)`,
		g.GoalID, g.UserID, g.ProjectID, g.Title, g.Objective, g.Status, g.CreatedBy, g.CreatedAt)
	if err != nil {
		return ProjectGoal{}, err
	}
	_ = s.appendProjectOSEvent(ctx, "goal.created", userID, projectID, g.GoalID, "", "system", "", map[string]any{"title": title})
	return g, nil
}

func scanProjectGoal(row pgx.Row) (ProjectGoal, error) {
	var g ProjectGoal
	var activePlan *string
	err := row.Scan(&g.GoalID, &g.UserID, &g.ProjectID, &g.Title, &g.Objective, &g.Status, &activePlan, &g.CreatedBy, &g.CreatedAt, &g.UpdatedAt, &g.Revision)
	if err != nil {
		return ProjectGoal{}, err
	}
	if activePlan != nil {
		g.ActivePlanID = *activePlan
	}
	return g, nil
}

func (s *Store) GetProjectGoal(ctx context.Context, userID, goalID string) (ProjectGoal, error) {
	userID = strings.TrimSpace(userID)
	goalID = strings.TrimSpace(goalID)
	row := s.DB.QueryRow(ctx, `SELECT goal_id,user_id,project_id,title,objective,status,active_plan_id,created_by_actor,created_at,updated_at,revision FROM codelocal_project_goals WHERE user_id=$1 AND goal_id=$2`, userID, goalID)
	g, err := scanProjectGoal(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectGoal{}, errors.New("goal not found")
	}
	return g, err
}

func (s *Store) ListProjectGoals(ctx context.Context, userID, projectID string, limit int) ([]ProjectGoal, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	var rows pgx.Rows
	var err error
	if projectID == "" {
		rows, err = s.DB.Query(ctx, `SELECT goal_id,user_id,project_id,title,objective,status,active_plan_id,created_by_actor,created_at,updated_at,revision FROM codelocal_project_goals WHERE user_id=$1 ORDER BY updated_at DESC LIMIT $2`, userID, limit)
	} else {
		rows, err = s.DB.Query(ctx, `SELECT goal_id,user_id,project_id,title,objective,status,active_plan_id,created_by_actor,created_at,updated_at,revision FROM codelocal_project_goals WHERE user_id=$1 AND project_id=$2 ORDER BY updated_at DESC LIMIT $3`, userID, projectID, limit)
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectGoal{}
	for rows.Next() {
		g, err := scanProjectGoal(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, g)
	}
	return out, rows.Err()
}

func (s *Store) TransitionProjectGoal(ctx context.Context, userID, goalID, toStatus string, expectedRevision int64) (ProjectGoal, error) {
	toStatus = strings.TrimSpace(toStatus)
	if !validProjectGoalStatus(toStatus) {
		return ProjectGoal{}, errors.New("invalid goal status")
	}
	g, err := s.GetProjectGoal(ctx, userID, goalID)
	if err != nil {
		return ProjectGoal{}, err
	}
	if expectedRevision > 0 && g.Revision != expectedRevision {
		return ProjectGoal{}, errors.New("stale goal revision")
	}
	if !validProjectGoalTransition(g.Status, toStatus) {
		return ProjectGoal{}, errors.New("invalid goal transition")
	}
	now := time.Now().UnixMilli()
	tag, err := s.DB.Exec(ctx, `UPDATE codelocal_project_goals SET status=$1,updated_at=$2,revision=revision+1 WHERE user_id=$3 AND goal_id=$4 AND revision=$5`,
		toStatus, now, userID, goalID, g.Revision)
	if err != nil {
		return ProjectGoal{}, err
	}
	if tag.RowsAffected() != 1 {
		return ProjectGoal{}, errors.New("stale goal revision")
	}
	_ = s.appendProjectOSEvent(ctx, "goal.transition", userID, g.ProjectID, goalID, "", "system", "", map[string]any{"from": g.Status, "to": toStatus})
	return s.GetProjectGoal(ctx, userID, goalID)
}

func coerceJSONArray(raw json.RawMessage) string {
	if len(raw) == 0 || !json.Valid(raw) {
		return "[]"
	}
	trimmed := strings.TrimSpace(string(raw))
	if trimmed == "" {
		return "[]"
	}
	return trimmed
}

func (s *Store) CreateProjectPlan(ctx context.Context, userID, projectID, goalID, summary string, flow, assumptions, acceptance json.RawMessage) (ProjectPlan, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	goalID = strings.TrimSpace(goalID)
	summary = strings.TrimSpace(summary)
	if len([]rune(summary)) > 4000 {
		summary = string([]rune(summary)[:4000])
	}
	if summary == "" {
		return ProjectPlan{}, errors.New("plan summary is required")
	}
	goal, err := s.GetProjectGoal(ctx, userID, goalID)
	if err != nil {
		return ProjectPlan{}, err
	}
	if goal.ProjectID != projectID {
		return ProjectPlan{}, errors.New("goal does not belong to project")
	}
	now := time.Now().UnixMilli()
	p := ProjectPlan{
		PlanID: "plan_" + RandomHex(12), UserID: userID, ProjectID: projectID, GoalID: goalID,
		Summary: summary, Flow: json.RawMessage(coerceJSONArray(flow)),
		Assumptions:        json.RawMessage(coerceJSONArray(assumptions)),
		AcceptanceCriteria: json.RawMessage(coerceJSONArray(acceptance)),
		Status:             ProjectPlanWaitingApproval, CreatedAt: now, UpdatedAt: now, Revision: 1,
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ProjectPlan{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_project_plans(plan_id,user_id,project_id,goal_id,summary,flow_json,assumptions_json,acceptance_criteria_json,status,created_at,updated_at,revision)
VALUES($1,$2,$3,$4,$5,$6::jsonb,$7::jsonb,$8::jsonb,$9,$10,$10,1)`,
		p.PlanID, userID, projectID, goalID, summary, string(p.Flow), string(p.Assumptions), string(p.AcceptanceCriteria), p.Status, now); err != nil {
		return ProjectPlan{}, err
	}
	nextGoal := string(ProjectGoalWaitingApproval)
	if goal.Status == string(ProjectGoalDraft) {
		nextGoal = string(ProjectGoalPlanProposed)
	}
	if _, err := tx.Exec(ctx, `UPDATE codelocal_project_goals SET status=$1,active_plan_id=$2,updated_at=$3,revision=revision+1 WHERE user_id=$4 AND goal_id=$5`,
		nextGoal, p.PlanID, now, userID, goalID); err != nil {
		return ProjectPlan{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProjectPlan{}, err
	}
	if nextGoal == string(ProjectGoalPlanProposed) {
		_, _ = s.TransitionProjectGoal(ctx, userID, goalID, string(ProjectGoalWaitingApproval), 0)
	}
	_ = s.appendProjectOSEvent(ctx, "plan.proposed", userID, projectID, goalID, "", "chief", "", map[string]any{"planId": p.PlanID})
	return s.GetProjectPlan(ctx, userID, p.PlanID)
}

func scanProjectPlan(row pgx.Row) (ProjectPlan, error) {
	var p ProjectPlan
	var flow, assumptions, acceptance string
	var approvedBy *string
	err := row.Scan(&p.PlanID, &p.UserID, &p.ProjectID, &p.GoalID, &p.Summary, &flow, &assumptions, &acceptance, &p.Status, &approvedBy, &p.ApprovedAt, &p.CreatedAt, &p.UpdatedAt, &p.Revision)
	if err != nil {
		return ProjectPlan{}, err
	}
	p.Flow = json.RawMessage(flow)
	p.Assumptions = json.RawMessage(assumptions)
	p.AcceptanceCriteria = json.RawMessage(acceptance)
	if approvedBy != nil {
		p.ApprovedBy = *approvedBy
	}
	return p, nil
}

func (s *Store) GetProjectPlan(ctx context.Context, userID, planID string) (ProjectPlan, error) {
	row := s.DB.QueryRow(ctx, `SELECT plan_id,user_id,project_id,goal_id,summary,flow_json::text,assumptions_json::text,acceptance_criteria_json::text,status,approved_by,approved_at,created_at,updated_at,revision FROM codelocal_project_plans WHERE user_id=$1 AND plan_id=$2`, strings.TrimSpace(userID), strings.TrimSpace(planID))
	p, err := scanProjectPlan(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectPlan{}, errors.New("plan not found")
	}
	return p, err
}

func (s *Store) ApproveProjectPlan(ctx context.Context, userID, planID, approver string, expectedRevision int64) (ProjectPlan, error) {
	approver = strings.TrimSpace(approver)
	if approver == "" {
		approver = "user"
	}
	p, err := s.GetProjectPlan(ctx, userID, planID)
	if err != nil {
		return ProjectPlan{}, err
	}
	if expectedRevision > 0 && p.Revision != expectedRevision {
		return ProjectPlan{}, errors.New("stale plan revision")
	}
	if p.Status != ProjectPlanProposed && p.Status != ProjectPlanWaitingApproval {
		return ProjectPlan{}, errors.New("plan is not awaiting approval")
	}
	now := time.Now().UnixMilli()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ProjectPlan{}, err
	}
	defer tx.Rollback(ctx)
	tag, err := tx.Exec(ctx, `UPDATE codelocal_project_plans SET status='APPROVED',approved_by=$1,approved_at=$2,updated_at=$2,revision=revision+1 WHERE user_id=$3 AND plan_id=$4 AND revision=$5`,
		approver, now, userID, planID, p.Revision)
	if err != nil {
		return ProjectPlan{}, err
	}
	if tag.RowsAffected() != 1 {
		return ProjectPlan{}, errors.New("stale plan revision")
	}
	if _, err := tx.Exec(ctx, `UPDATE codelocal_project_goals SET status='APPROVED',active_plan_id=$1,updated_at=$2,revision=revision+1 WHERE user_id=$3 AND goal_id=$4`,
		planID, now, userID, p.GoalID); err != nil {
		return ProjectPlan{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ProjectPlan{}, err
	}
	_ = s.appendProjectOSEvent(ctx, "plan.approved", userID, p.ProjectID, p.GoalID, "", "user", approver, map[string]any{"planId": planID})
	// Move goal into EXECUTING only when task creation starts; keep APPROVED here.
	approved, err := s.GetProjectPlan(ctx, userID, planID)
	if err != nil {
		return ProjectPlan{}, err
	}
	return approved, nil
}

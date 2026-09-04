package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ProjectTask struct {
	TaskID               string          `json:"taskId"`
	UserID               string          `json:"userId"`
	ProjectID            string          `json:"projectId"`
	GoalID               string          `json:"goalId"`
	PlanID               string          `json:"planId"`
	Title                string          `json:"title"`
	Description          string          `json:"description"`
	Status               string          `json:"status"`
	TaskKind             string          `json:"taskKind"`
	RequiredCapabilities json.RawMessage `json:"requiredCapabilities"`
	AssignedAgentRole    string          `json:"assignedAgentRole,omitempty"`
	ExecutionTaskID      string          `json:"executionTaskId,omitempty"`
	Priority             int             `json:"priority"`
	RetryCount           int             `json:"retryCount"`
	CreatedAt            int64           `json:"createdAt"`
	UpdatedAt            int64           `json:"updatedAt"`
	Revision             int64           `json:"revision"`
	DependsOn            []string        `json:"dependsOn,omitempty"`
}

var validProjectTaskStatuses = map[string]bool{
	"PLANNED": true, "READY": true, "ASSIGNED": true, "RUNNING": true,
	"REVIEWING": true, "TESTING": true, "DONE": true, "BLOCKED": true,
	"WAITING_HUMAN": true, "WAITING_EXTERNAL": true, "RETRYING": true,
	"STALE": true, "FAILED": true, "CANCELLED": true,
}

var projectTaskTransitions = map[string]map[string]bool{
	"PLANNED":          {"READY": true, "CANCELLED": true, "STALE": true},
	"READY":            {"ASSIGNED": true, "BLOCKED": true, "CANCELLED": true, "STALE": true},
	"ASSIGNED":         {"RUNNING": true, "BLOCKED": true, "CANCELLED": true, "STALE": true},
	"RUNNING":          {"REVIEWING": true, "FAILED": true, "BLOCKED": true, "RETRYING": true, "CANCELLED": true, "STALE": true},
	"REVIEWING":        {"TESTING": true, "RUNNING": true, "FAILED": true, "BLOCKED": true, "CANCELLED": true},
	"TESTING":          {"DONE": false, "RUNNING": true, "FAILED": true, "BLOCKED": true, "WAITING_HUMAN": true, "CANCELLED": true},
	"DONE":             {},
	"BLOCKED":          {"READY": true, "RETRYING": true, "WAITING_HUMAN": true, "CANCELLED": true},
	"WAITING_HUMAN":    {"READY": true, "RETRYING": true, "CANCELLED": true},
	"WAITING_EXTERNAL": {"READY": true, "RETRYING": true, "CANCELLED": true},
	"RETRYING":         {"READY": true, "ASSIGNED": true, "CANCELLED": true},
	"STALE":            {"PLANNED": true, "CANCELLED": true},
	"FAILED":           {"RETRYING": true, "WAITING_HUMAN": true, "BLOCKED": true, "CANCELLED": true},
	"CANCELLED":        {},
}

func validProjectTaskTransition(from, to string) bool {
	if from == to {
		return true
	}
	// TESTING->DONE is forbidden: only tester verdict path may mark DONE.
	if from == "TESTING" && to == "DONE" {
		return false
	}
	return projectTaskTransitions[from][to]
}

func normalizeTaskKind(v string) string {
	v = strings.ToLower(strings.TrimSpace(v))
	switch v {
	case "coding", "review", "testing", "docs", "research", "ops":
		return v
	case "":
		return "coding"
	}
	return ""
}

type ProjectTaskSpec struct {
	Title                string
	Description          string
	TaskKind             string
	RequiredCapabilities []string
	AssignedAgentRole    string
	Priority             int
	DependsOn            []string
}

func detectTaskDependencyCycle(ids []string, edges map[string][]string) bool {
	state := map[string]int{}
	var visit func(n string) bool
	visit = func(n string) bool {
		if state[n] == 1 {
			return true
		}
		if state[n] == 2 {
			return false
		}
		state[n] = 1
		for _, dep := range edges[n] {
			if visit(dep) {
				return true
			}
		}
		state[n] = 2
		return false
	}
	for _, id := range ids {
		if state[id] == 0 && visit(id) {
			return true
		}
	}
	return false
}

func (s *Store) CreateProjectTaskGraph(ctx context.Context, userID, projectID, goalID, planID string, specs []ProjectTaskSpec) ([]ProjectTask, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	goalID = strings.TrimSpace(goalID)
	planID = strings.TrimSpace(planID)
	if len(specs) == 0 || len(specs) > 25 {
		return nil, errors.New("task graph must contain 1-25 tasks")
	}
	goal, err := s.GetProjectGoal(ctx, userID, goalID)
	if err != nil {
		return nil, err
	}
	if goal.ProjectID != projectID {
		return nil, errors.New("goal does not belong to project")
	}
	plan, err := s.GetProjectPlan(ctx, userID, planID)
	if err != nil {
		return nil, err
	}
	if plan.GoalID != goalID || plan.ProjectID != projectID {
		return nil, errors.New("plan does not belong to goal")
	}
	if plan.Status != ProjectPlanApproved {
		return nil, errors.New("plan must be APPROVED before task creation")
	}
	now := time.Now().UnixMilli()
	tasks := make([]ProjectTask, 0, len(specs))
	seenTitles := map[string]bool{}
	for i := range specs {
		spec := &specs[i]
		spec.Title = normalizeProjectOSTitle(spec.Title, 200)
		if spec.Title == "" {
			return nil, errors.New("task title is required")
		}
		lower := strings.ToLower(spec.Title)
		if seenTitles[lower] {
			return nil, errors.New("duplicate task title")
		}
		seenTitles[lower] = true
		spec.Description = strings.TrimSpace(spec.Description)
		if len([]rune(spec.Description)) > 4000 {
			spec.Description = string([]rune(spec.Description)[:4000])
		}
		kind := normalizeTaskKind(spec.TaskKind)
		if kind == "" {
			return nil, errors.New("invalid task kind")
		}
		spec.TaskKind = kind
		spec.AssignedAgentRole = strings.TrimSpace(spec.AssignedAgentRole)
		if len(spec.RequiredCapabilities) > 10 {
			return nil, errors.New("too many required capabilities")
		}
	}
	ids := make([]string, 0, len(specs))
	for range specs {
		ids = append(ids, "task_"+RandomHex(10))
	}
	titleToID := map[string]string{}
	for i, spec := range specs {
		titleToID[strings.ToLower(spec.Title)] = ids[i]
	}
	edges := map[string][]string{}
	for i, spec := range specs {
		deps := []string{}
		seen := map[string]bool{}
		for _, dep := range spec.DependsOn {
			dep = strings.TrimSpace(dep)
			if dep == "" {
				continue
			}
			resolved := dep
			if !strings.HasPrefix(dep, "task_") {
				mapped, ok := titleToID[strings.ToLower(dep)]
				if !ok {
					return nil, errors.New("unknown task dependency: " + dep)
				}
				resolved = mapped
			}
			if resolved == ids[i] {
				return nil, errors.New("task cannot depend on itself")
			}
			if !seen[resolved] {
				seen[resolved] = true
				deps = append(deps, resolved)
			}
		}
		sort.Strings(deps)
		edges[ids[i]] = deps
	}
	// All dependency targets must be part of this graph.
	members := map[string]bool{}
	for _, id := range ids {
		members[id] = true
	}
	for _, deps := range edges {
		for _, dep := range deps {
			if !members[dep] {
				return nil, errors.New("dependency target is not in task graph")
			}
		}
	}
	if detectTaskDependencyCycle(ids, edges) {
		return nil, errors.New("task dependency cycle detected")
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	for i, spec := range specs {
		caps, _ := json.Marshal(spec.RequiredCapabilities)
		if string(caps) == "null" || len(caps) == 0 {
			caps = []byte("[]")
		}
		status := "READY"
		if len(edges[ids[i]]) > 0 {
			status = "PLANNED"
		}
		t := ProjectTask{
			TaskID: ids[i], UserID: userID, ProjectID: projectID, GoalID: goalID, PlanID: planID,
			Title: spec.Title, Description: spec.Description, Status: status, TaskKind: spec.TaskKind,
			RequiredCapabilities: json.RawMessage(string(caps)), AssignedAgentRole: spec.AssignedAgentRole,
			Priority: spec.Priority, CreatedAt: now, UpdatedAt: now, Revision: 1, DependsOn: edges[ids[i]],
		}
		if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_project_tasks(task_id,user_id,project_id,goal_id,plan_id,title,description,status,task_kind,required_capabilities_json,assigned_agent_role,priority,created_at,updated_at,revision)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10::jsonb,$11,$12,$13,$13,1)`,
			t.TaskID, userID, projectID, goalID, planID, t.Title, t.Description, t.Status, t.TaskKind, string(t.RequiredCapabilities), t.AssignedAgentRole, t.Priority, now); err != nil {
			return nil, err
		}
		for _, dep := range edges[ids[i]] {
			if _, err := tx.Exec(ctx, `INSERT INTO codelocal_project_task_dependencies(user_id,project_id,task_id,depends_on_task_id,created_at) VALUES($1,$2,$3,$4,$5) ON CONFLICT DO NOTHING`,
				userID, projectID, ids[i], dep, now); err != nil {
				return nil, err
			}
		}
		tasks = append(tasks, t)
	}
	if _, err := tx.Exec(ctx, `UPDATE codelocal_project_goals SET status='EXECUTING',updated_at=$1,revision=revision+1 WHERE user_id=$2 AND goal_id=$3 AND status='APPROVED'`,
		now, userID, goalID); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	_ = s.appendProjectOSEvent(ctx, "task.graph_created", userID, projectID, goalID, "", "chief", "", map[string]any{"planId": planID, "taskCount": len(tasks)})
	return tasks, nil
}

func scanProjectTask(row pgx.Row) (ProjectTask, error) {
	var t ProjectTask
	var caps string
	err := row.Scan(&t.TaskID, &t.UserID, &t.ProjectID, &t.GoalID, &t.PlanID, &t.Title, &t.Description, &t.Status, &t.TaskKind, &caps, &t.AssignedAgentRole, &t.ExecutionTaskID, &t.Priority, &t.RetryCount, &t.CreatedAt, &t.UpdatedAt, &t.Revision)
	if err != nil {
		return ProjectTask{}, err
	}
	if caps == "" {
		caps = "[]"
	}
	t.RequiredCapabilities = json.RawMessage(caps)
	return t, nil
}

const projectTaskColumns = `task_id,user_id,project_id,goal_id,plan_id,title,description,status,task_kind,required_capabilities_json::text,assigned_agent_role,execution_task_id,priority,retry_count,created_at,updated_at,revision`

func (s *Store) GetProjectTask(ctx context.Context, userID, taskID string) (ProjectTask, error) {
	row := s.DB.QueryRow(ctx, `SELECT `+projectTaskColumns+` FROM codelocal_project_tasks WHERE user_id=$1 AND task_id=$2`, strings.TrimSpace(userID), strings.TrimSpace(taskID))
	t, err := scanProjectTask(row)
	if errors.Is(err, pgx.ErrNoRows) {
		return ProjectTask{}, errors.New("task not found")
	}
	if err != nil {
		return ProjectTask{}, err
	}
	deps, _ := s.projectTaskDependencies(ctx, userID, t.TaskID)
	t.DependsOn = deps
	return t, nil
}

func (s *Store) projectTaskDependencies(ctx context.Context, userID, taskID string) ([]string, error) {
	rows, err := s.DB.Query(ctx, `SELECT depends_on_task_id FROM codelocal_project_task_dependencies WHERE user_id=$1 AND task_id=$2 ORDER BY depends_on_task_id`, userID, taskID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var dep string
		if err := rows.Scan(&dep); err != nil {
			return nil, err
		}
		out = append(out, dep)
	}
	return out, rows.Err()
}

func (s *Store) ListProjectTasks(ctx context.Context, userID, goalID string, limit int) ([]ProjectTask, error) {
	if limit <= 0 || limit > 100 {
		limit = 50
	}
	rows, err := s.DB.Query(ctx, `SELECT `+projectTaskColumns+` FROM codelocal_project_tasks WHERE user_id=$1 AND goal_id=$2 ORDER BY priority DESC, created_at ASC LIMIT $3`, strings.TrimSpace(userID), strings.TrimSpace(goalID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectTask{}
	for rows.Next() {
		t, err := scanProjectTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		deps, _ := s.projectTaskDependencies(ctx, userID, out[i].TaskID)
		out[i].DependsOn = deps
	}
	return out, nil
}

func (s *Store) ListProjectWork(ctx context.Context, userID, projectID string) ([]ProjectTask, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+projectTaskColumns+` FROM codelocal_project_tasks WHERE user_id=$1 AND project_id=$2 AND status NOT IN ('DONE','CANCELLED') ORDER BY updated_at DESC LIMIT 100`, strings.TrimSpace(userID), strings.TrimSpace(projectID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectTask{}
	for rows.Next() {
		t, err := scanProjectTask(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		deps, _ := s.projectTaskDependencies(ctx, userID, out[i].TaskID)
		out[i].DependsOn = deps
	}
	return out, nil
}

func (s *Store) TransitionProjectTask(ctx context.Context, userID, taskID, toStatus string, expectedRevision int64) (ProjectTask, error) {
	toStatus = strings.TrimSpace(toStatus)
	if !validProjectTaskStatuses[toStatus] {
		return ProjectTask{}, errors.New("invalid task status")
	}
	t, err := s.GetProjectTask(ctx, userID, taskID)
	if err != nil {
		return ProjectTask{}, err
	}
	if expectedRevision > 0 && t.Revision != expectedRevision {
		return ProjectTask{}, errors.New("stale task revision")
	}
	if !validProjectTaskTransition(t.Status, toStatus) {
		return ProjectTask{}, errors.New("invalid task transition")
	}
	if (toStatus == "READY" || toStatus == "ASSIGNED") && len(t.DependsOn) > 0 {
		done, err := s.projectTaskDependenciesSatisfied(ctx, userID, taskID)
		if err != nil {
			return ProjectTask{}, err
		}
		if !done {
			return ProjectTask{}, errors.New("dependencies are not satisfied")
		}
		if toStatus == "ASSIGNED" && t.Status == "PLANNED" {
			// PLANNED must go through READY first.
			return ProjectTask{}, errors.New("task must be READY before ASSIGNED")
		}
	}
	now := time.Now().UnixMilli()
	retryBump := ""
	if toStatus == "RETRYING" {
		retryBump = ",retry_count=retry_count+1"
	}
	tag, err := s.DB.Exec(ctx, `UPDATE codelocal_project_tasks SET status=$1,updated_at=$2,revision=revision+1`+retryBump+` WHERE user_id=$3 AND task_id=$4 AND revision=$5`,
		toStatus, now, userID, taskID, t.Revision)
	if err != nil {
		return ProjectTask{}, err
	}
	if tag.RowsAffected() != 1 {
		return ProjectTask{}, errors.New("stale task revision")
	}
	_ = s.appendProjectOSEvent(ctx, "task.transition", userID, t.ProjectID, t.GoalID, taskID, "system", "", map[string]any{"from": t.Status, "to": toStatus})
	updated, err := s.GetProjectTask(ctx, userID, taskID)
	if err != nil {
		return ProjectTask{}, err
	}
	// Unblock dependents when a task reaches DONE (only via tester path).
	if updated.Status == "DONE" {
		_ = s.promoteReadyDependents(ctx, userID, updated.ProjectID, updated.GoalID, taskID)
	}
	return updated, nil
}

func (s *Store) projectTaskDependenciesSatisfied(ctx context.Context, userID, taskID string) (bool, error) {
	var pending int
	err := s.DB.QueryRow(ctx, `
SELECT COUNT(*)::int FROM codelocal_project_task_dependencies d
JOIN codelocal_project_tasks t ON t.user_id=d.user_id AND t.task_id=d.depends_on_task_id
WHERE d.user_id=$1 AND d.task_id=$2 AND t.status<>'DONE'`, userID, taskID).Scan(&pending)
	return pending == 0, err
}

func (s *Store) promoteReadyDependents(ctx context.Context, userID, projectID, goalID, doneTaskID string) error {
	rows, err := s.DB.Query(ctx, `SELECT task_id FROM codelocal_project_task_dependencies WHERE user_id=$1 AND depends_on_task_id=$2`, userID, doneTaskID)
	if err != nil {
		return err
	}
	var dependents []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		dependents = append(dependents, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	now := time.Now().UnixMilli()
	for _, depID := range dependents {
		satisfied, err := s.projectTaskDependenciesSatisfied(ctx, userID, depID)
		if err != nil || !satisfied {
			continue
		}
		tag, err := s.DB.Exec(ctx, `UPDATE codelocal_project_tasks SET status='READY',updated_at=$1,revision=revision+1 WHERE user_id=$2 AND task_id=$3 AND status='PLANNED'`, now, userID, depID)
		if err != nil || tag.RowsAffected() != 1 {
			continue
		}
		_ = s.appendProjectOSEvent(ctx, "task.ready", userID, projectID, goalID, depID, "chief", "", map[string]any{"unblockedBy": doneTaskID})
	}
	return nil
}

func (s *Store) BindProjectTaskExecution(ctx context.Context, userID, taskID, executionTaskID string) (ProjectTask, error) {
	executionTaskID = strings.TrimSpace(executionTaskID)
	if executionTaskID == "" {
		return ProjectTask{}, errors.New("execution task id is required")
	}
	t, err := s.GetProjectTask(ctx, userID, taskID)
	if err != nil {
		return ProjectTask{}, err
	}
	if t.ExecutionTaskID != "" && t.ExecutionTaskID != executionTaskID {
		return ProjectTask{}, errors.New("task already bound to another execution")
	}
	now := time.Now().UnixMilli()
	if _, err := s.DB.Exec(ctx, `UPDATE codelocal_project_tasks SET execution_task_id=$1,updated_at=$2,revision=revision+1 WHERE user_id=$3 AND task_id=$4`, executionTaskID, now, userID, taskID); err != nil {
		return ProjectTask{}, err
	}
	_ = s.appendProjectOSEvent(ctx, "task.execution_bound", userID, t.ProjectID, t.GoalID, taskID, "chief", "", map[string]any{"executionTaskId": executionTaskID})
	return s.GetProjectTask(ctx, userID, taskID)
}

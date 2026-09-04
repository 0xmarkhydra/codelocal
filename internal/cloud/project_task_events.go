package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type ProjectTaskEvent struct {
	EventID   string          `json:"eventId"`
	UserID    string          `json:"userId"`
	ProjectID string          `json:"projectId"`
	GoalID    string          `json:"goalId,omitempty"`
	TaskID    string          `json:"taskId,omitempty"`
	EventType string          `json:"eventType"`
	ActorType string          `json:"actorType"`
	ActorID   string          `json:"actorId,omitempty"`
	Payload   json.RawMessage `json:"payload"`
	CreatedAt int64           `json:"createdAt"`
}

type TesterVerdict struct {
	VerdictID string          `json:"verdictId"`
	UserID    string          `json:"userId"`
	ProjectID string          `json:"projectId"`
	TaskID    string          `json:"taskId"`
	Verdict   string          `json:"verdict"`
	Evidence  json.RawMessage `json:"evidence"`
	CreatedBy string          `json:"createdBy"`
	CreatedAt int64           `json:"createdAt"`
}

type NeedsYouItem struct {
	Kind     string `json:"kind"`
	Title    string `json:"title"`
	GoalID   string `json:"goalId,omitempty"`
	PlanID   string `json:"planId,omitempty"`
	TaskID   string `json:"taskId,omitempty"`
	Detail   string `json:"detail,omitempty"`
	Priority int    `json:"priority"`
}

func sanitizeProjectOSPayload(payload map[string]any) string {
	if payload == nil {
		return "{}"
	}
	safe := map[string]any{}
	for k, v := range payload {
		lower := strings.ToLower(k)
		if strings.Contains(lower, "secret") || strings.Contains(lower, "token") || strings.Contains(lower, "password") || strings.Contains(lower, "key") && strings.Contains(lower, "api") {
			continue
		}
		safe[k] = v
		if len(safe) >= 20 {
			break
		}
	}
	raw, err := json.Marshal(safe)
	if err != nil || len(raw) == 0 {
		return "{}"
	}
	if len(raw) > 8000 {
		return string(raw[:8000])
	}
	if !json.Valid(raw) {
		return "{}"
	}
	return string(raw)
}

func (s *Store) appendProjectOSEvent(ctx context.Context, eventType, userID, projectID, goalID, taskID, actorType, actorID string, payload map[string]any) error {
	if s == nil || s.DB == nil {
		return errors.New("store unavailable")
	}
	eventType = strings.TrimSpace(eventType)
	if eventType == "" || userID == "" || projectID == "" {
		return errors.New("event requires type, user and project")
	}
	actorType = strings.TrimSpace(actorType)
	if actorType == "" {
		actorType = "system"
	}
	var goalVal, taskVal any
	if strings.TrimSpace(goalID) != "" {
		goalVal = strings.TrimSpace(goalID)
	}
	if strings.TrimSpace(taskID) != "" {
		taskVal = strings.TrimSpace(taskID)
	}
	_, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_project_task_events(event_id,user_id,project_id,goal_id,task_id,event_type,actor_type,actor_id,safe_payload_json,created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9::jsonb,$10) ON CONFLICT(event_id) DO NOTHING`,
		"evt_"+RandomHex(12), userID, projectID, goalVal, taskVal, eventType, actorType, strings.TrimSpace(actorID), sanitizeProjectOSPayload(payload), time.Now().UnixMilli())
	return err
}

func (s *Store) AppendProjectTaskEvent(ctx context.Context, userID, projectID, goalID, taskID, eventType, actorType, actorID string, payload map[string]any) (ProjectTaskEvent, error) {
	if err := s.appendProjectOSEvent(ctx, eventType, userID, projectID, goalID, taskID, actorType, actorID, payload); err != nil {
		return ProjectTaskEvent{}, err
	}
	events, err := s.ListProjectTaskEvents(ctx, userID, taskID, goalID, 1)
	if err != nil || len(events) == 0 {
		return ProjectTaskEvent{EventType: eventType}, err
	}
	return events[0], nil
}

func scanProjectTaskEvent(row pgx.Row) (ProjectTaskEvent, error) {
	var e ProjectTaskEvent
	var goalVal, taskVal *string
	var payload string
	err := row.Scan(&e.EventID, &e.UserID, &e.ProjectID, &goalVal, &taskVal, &e.EventType, &e.ActorType, &e.ActorID, &payload, &e.CreatedAt)
	if err != nil {
		return ProjectTaskEvent{}, err
	}
	if goalVal != nil {
		e.GoalID = *goalVal
	}
	if taskVal != nil {
		e.TaskID = *taskVal
	}
	if payload == "" {
		payload = "{}"
	}
	e.Payload = json.RawMessage(payload)
	return e, nil
}

const projectTaskEventColumns = `event_id,user_id,project_id,goal_id,task_id,event_type,actor_type,actor_id,safe_payload_json::text,created_at`

func (s *Store) ListProjectTaskEvents(ctx context.Context, userID, taskID, goalID string, limit int) ([]ProjectTaskEvent, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	userID = strings.TrimSpace(userID)
	taskID = strings.TrimSpace(taskID)
	goalID = strings.TrimSpace(goalID)
	var rows pgx.Rows
	var err error
	switch {
	case taskID != "":
		rows, err = s.DB.Query(ctx, `SELECT `+projectTaskEventColumns+` FROM codelocal_project_task_events WHERE user_id=$1 AND task_id=$2 ORDER BY created_at DESC LIMIT $3`, userID, taskID, limit)
	case goalID != "":
		rows, err = s.DB.Query(ctx, `SELECT `+projectTaskEventColumns+` FROM codelocal_project_task_events WHERE user_id=$1 AND goal_id=$2 ORDER BY created_at DESC LIMIT $3`, userID, goalID, limit)
	default:
		return nil, errors.New("task or goal scope is required")
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ProjectTaskEvent{}
	for rows.Next() {
		e, err := scanProjectTaskEvent(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, e)
	}
	return out, rows.Err()
}

func (s *Store) RecordTesterVerdict(ctx context.Context, userID, taskID, verdict string, evidence map[string]any, createdBy string) (TesterVerdict, error) {
	verdict = strings.TrimSpace(verdict)
	if verdict != "DONE" && verdict != "NOT_DONE" {
		return TesterVerdict{}, errors.New("invalid tester verdict")
	}
	createdBy = strings.TrimSpace(createdBy)
	if createdBy == "" {
		createdBy = "tester"
	}
	// Only tester/reviewer/system actors may record verdicts; implementing agent cannot mark DONE directly.
	if createdBy != "tester" && createdBy != "reviewer" && createdBy != "system" && createdBy != "chief" {
		return TesterVerdict{}, errors.New("actor is not authorized to record tester verdict")
	}
	t, err := s.GetProjectTask(ctx, userID, taskID)
	if err != nil {
		return TesterVerdict{}, err
	}
	if t.Status != "TESTING" && t.Status != "REVIEWING" && t.Status != "RUNNING" {
		return TesterVerdict{}, errors.New("task is not awaiting verification")
	}
	now := time.Now().UnixMilli()
	v := TesterVerdict{
		VerdictID: "ver_" + RandomHex(12), UserID: userID, ProjectID: t.ProjectID, TaskID: taskID,
		Verdict: verdict, Evidence: json.RawMessage(sanitizeProjectOSPayload(evidence)),
		CreatedBy: createdBy, CreatedAt: now,
	}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return TesterVerdict{}, err
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO codelocal_project_tester_verdicts(verdict_id,user_id,project_id,task_id,verdict,evidence_json,created_by,created_at) VALUES($1,$2,$3,$4,$5,$6::jsonb,$7,$8)`,
		v.VerdictID, userID, t.ProjectID, taskID, verdict, string(v.Evidence), createdBy, now); err != nil {
		return TesterVerdict{}, err
	}
	nextStatus := "RUNNING"
	if verdict == "DONE" {
		nextStatus = "DONE"
	}
	if _, err := tx.Exec(ctx, `UPDATE codelocal_project_tasks SET status=$1,updated_at=$2,revision=revision+1 WHERE user_id=$3 AND task_id=$4`, nextStatus, now, userID, taskID); err != nil {
		return TesterVerdict{}, err
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_project_task_events(event_id,user_id,project_id,goal_id,task_id,event_type,actor_type,actor_id,safe_payload_json,created_at)
VALUES($1,$2,$3,$4,$5,$6,'tester',$7,$8::jsonb,$9)`,
		"evt_"+RandomHex(12), userID, t.ProjectID, t.GoalID, taskID, "tester."+strings.ToLower(verdict), createdBy, sanitizeProjectOSPayload(evidence), now); err != nil {
		return TesterVerdict{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return TesterVerdict{}, err
	}
	if verdict == "DONE" {
		_ = s.promoteReadyDependents(ctx, userID, t.ProjectID, t.GoalID, taskID)
		_ = s.maybeCompleteProjectGoal(ctx, userID, t.GoalID)
	}
	return v, nil
}

func (s *Store) maybeCompleteProjectGoal(ctx context.Context, userID, goalID string) error {
	var pending int
	if err := s.DB.QueryRow(ctx, `SELECT COUNT(*)::int FROM codelocal_project_tasks WHERE user_id=$1 AND goal_id=$2 AND status NOT IN ('DONE','CANCELLED')`, userID, goalID).Scan(&pending); err != nil {
		return err
	}
	if pending > 0 {
		// Move EXECUTING -> VERIFYING when only verification remains is handled by callers; keep EXECUTING here.
		return nil
	}
	goal, err := s.GetProjectGoal(ctx, userID, goalID)
	if err != nil {
		return err
	}
	if goal.Status != "EXECUTING" && goal.Status != "VERIFYING" && goal.Status != "APPROVED" {
		return nil
	}
	now := time.Now().UnixMilli()
	_, err = s.DB.Exec(ctx, `UPDATE codelocal_project_goals SET status='DONE',updated_at=$1,revision=revision+1 WHERE user_id=$2 AND goal_id=$3`, now, userID, goalID)
	if err != nil {
		return err
	}
	_ = s.appendProjectOSEvent(ctx, "goal.done", userID, goal.ProjectID, goalID, "", "tester", "", map[string]any{"taskCount": 0})
	return nil
}

func (s *Store) ProjectNeedsYou(ctx context.Context, userID, projectID string) ([]NeedsYouItem, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	out := []NeedsYouItem{}
	// Plan approvals.
	rows, err := s.DB.Query(ctx, `
SELECT g.goal_id,g.title,p.plan_id FROM codelocal_project_goals g
JOIN codelocal_project_plans p ON p.user_id=g.user_id AND p.plan_id=g.active_plan_id
WHERE g.user_id=$1 AND ($2='' OR g.project_id=$2) AND g.status='WAITING_APPROVAL' AND p.status IN ('PROPOSED','WAITING_APPROVAL')
ORDER BY g.updated_at DESC LIMIT 20`, userID, projectID)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		var goalID, title, planID string
		if err := rows.Scan(&goalID, &title, &planID); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, NeedsYouItem{Kind: "plan_approval", Title: "Phê duyệt plan: " + title, GoalID: goalID, PlanID: planID, Priority: 100})
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	// Blocked goals need replan decisions (e.g. after requirement impact).
	goalRows, err := s.DB.Query(ctx, `
SELECT goal_id,title FROM codelocal_project_goals
WHERE user_id=$1 AND ($2='' OR project_id=$2) AND status='BLOCKED'
ORDER BY updated_at DESC LIMIT 20`, userID, projectID)
	if err != nil {
		return nil, err
	}
	for goalRows.Next() {
		var goalID, title string
		if err := goalRows.Scan(&goalID, &title); err != nil {
			goalRows.Close()
			return nil, err
		}
		out = append(out, NeedsYouItem{Kind: "replan_required", Title: "Replan: " + title, GoalID: goalID, Priority: 80})
	}
	goalRows.Close()
	if err := goalRows.Err(); err != nil {
		return nil, err
	}
	// Waiting-human / blocked tasks.
	taskRows, err := s.DB.Query(ctx, `
SELECT task_id,goal_id,title,status FROM codelocal_project_tasks
WHERE user_id=$1 AND ($2='' OR project_id=$2) AND status IN ('WAITING_HUMAN','BLOCKED','FAILED')
ORDER BY updated_at DESC LIMIT 20`, userID, projectID)
	if err != nil {
		return nil, err
	}
	defer taskRows.Close()
	for taskRows.Next() {
		var taskID, goalID, title, status string
		if err := taskRows.Scan(&taskID, &goalID, &title, &status); err != nil {
			return nil, err
		}
		out = append(out, NeedsYouItem{Kind: "task_attention", Title: title + " (" + status + ")", GoalID: goalID, TaskID: taskID, Detail: status, Priority: 50})
	}
	if err := taskRows.Err(); err != nil {
		return nil, err
	}
	// Feedback clusters at signal threshold become bug/feature signals.
	signals, err := s.ProjectFeedbackSignals(ctx, userID, projectID, 3)
	if err == nil {
		for _, sig := range signals {
			if len(out) >= 40 {
				break
			}
			out = append(out, NeedsYouItem{Kind: "feedback_signal", Title: sig.Title, Detail: sig.Kind, Priority: 30})
		}
	}
	return out, taskRows.Err()
}

func (s *Store) ProjectOSSummary(ctx context.Context, userID string) (map[string]any, error) {
	userID = strings.TrimSpace(userID)
	var goalCount, runningCount, testingCount, doneCount, needsYou int
	_ = s.DB.QueryRow(ctx, `SELECT COUNT(*)::int FROM codelocal_project_goals WHERE user_id=$1 AND status NOT IN ('DONE','CANCELLED')`, userID).Scan(&goalCount)
	_ = s.DB.QueryRow(ctx, `SELECT COUNT(*)::int FROM codelocal_project_tasks WHERE user_id=$1 AND status IN ('READY','ASSIGNED','RUNNING')`, userID).Scan(&runningCount)
	_ = s.DB.QueryRow(ctx, `SELECT COUNT(*)::int FROM codelocal_project_tasks WHERE user_id=$1 AND status IN ('REVIEWING','TESTING')`, userID).Scan(&testingCount)
	_ = s.DB.QueryRow(ctx, `SELECT COUNT(*)::int FROM codelocal_project_tasks WHERE user_id=$1 AND status='DONE'`, userID).Scan(&doneCount)
	items, _ := s.ProjectNeedsYou(ctx, userID, "")
	needsYou = len(items)
	return map[string]any{
		"activeGoals": goalCount, "runningTasks": runningCount,
		"testingTasks": testingCount, "doneTasks": doneCount, "needsYou": needsYou,
	}, nil
}

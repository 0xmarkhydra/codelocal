package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

type RequirementChange struct {
	ChangeID    string          `json:"changeId"`
	UserID      string          `json:"userId"`
	ProjectID   string          `json:"projectId"`
	Source      string          `json:"source"`
	Summary     string          `json:"summary"`
	ChangedRefs json.RawMessage `json:"changedRefs"`
	CreatedAt   int64           `json:"createdAt"`
}

func validRequirementSource(s string) bool {
	switch s {
	case "docs", "chat", "feedback", "manual":
		return true
	}
	return false
}

func (s *Store) RecordRequirementChange(ctx context.Context, userID, projectID, source, summary string, changedRefs []string) (RequirementChange, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	source = strings.ToLower(strings.TrimSpace(source))
	if source == "" {
		source = "manual"
	}
	if !validRequirementSource(source) {
		return RequirementChange{}, errors.New("invalid requirement change source")
	}
	summary = strings.TrimSpace(summary)
	if len([]rune(summary)) > 2000 {
		summary = string([]rune(summary)[:2000])
	}
	if summary == "" {
		return RequirementChange{}, errors.New("requirement change summary is required")
	}
	refs := []string{}
	seen := map[string]bool{}
	for _, ref := range changedRefs {
		ref = strings.TrimSpace(ref)
		if ref == "" || seen[strings.ToLower(ref)] || len(refs) >= 30 {
			continue
		}
		seen[strings.ToLower(ref)] = true
		refs = append(refs, ref)
	}
	rawRefs, _ := json.Marshal(refs)
	if err := s.ensureProjectOSProject(ctx, userID, projectID); err != nil {
		return RequirementChange{}, err
	}
	now := time.Now().UnixMilli()
	change := RequirementChange{
		ChangeID: "req_" + RandomHex(12), UserID: userID, ProjectID: projectID,
		Source: source, Summary: summary, ChangedRefs: json.RawMessage(string(rawRefs)), CreatedAt: now,
	}
	if _, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_project_requirement_changes(change_id,user_id,project_id,source,summary,changed_refs_json,created_at)
VALUES($1,$2,$3,$4,$5,$6::jsonb,$7)`,
		change.ChangeID, userID, projectID, source, summary, string(change.ChangedRefs), now); err != nil {
		return RequirementChange{}, err
	}
	_ = s.appendProjectOSEvent(ctx, "requirement.changed", userID, projectID, "", "", "system", source, map[string]any{"changeId": change.ChangeID, "summary": summary})
	return change, nil
}

type RequirementImpactResult struct {
	StalePlans int      `json:"stalePlans"`
	StaleTasks int      `json:"staleTasks"`
	GoalStatus string   `json:"goalStatus"`
	TaskIDs    []string `json:"taskIds"`
	PlanIDs    []string `json:"planIds"`
}

// ApplyRequirementImpact marks the approved impact set stale and parks the goal for
// replan. Plans move to STALE; non-terminal tasks move to STALE; the goal moves to
// DRAFT (pre-execution) or BLOCKED (mid-execution) — both transitions the P0 state
// machine already permits. Stale executions must be rejected or re-created, never
// silently continued.
func (s *Store) ApplyRequirementImpact(ctx context.Context, userID, changeID, goalID string, planIDs, taskIDs []string) (RequirementImpactResult, error) {
	userID = strings.TrimSpace(userID)
	changeID = strings.TrimSpace(changeID)
	goalID = strings.TrimSpace(goalID)
	result := RequirementImpactResult{}
	goal, err := s.GetProjectGoal(ctx, userID, goalID)
	if err != nil {
		return result, err
	}
	now := time.Now().UnixMilli()
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return result, err
	}
	defer tx.Rollback(ctx)
	dedupePlans := map[string]bool{}
	for _, planID := range planIDs {
		planID = strings.TrimSpace(planID)
		if planID == "" || dedupePlans[planID] {
			continue
		}
		dedupePlans[planID] = true
		tag, err := tx.Exec(ctx, `UPDATE codelocal_project_plans SET status='STALE',updated_at=$1,revision=revision+1 WHERE user_id=$2 AND plan_id=$3 AND goal_id=$4 AND status IN ('PROPOSED','WAITING_APPROVAL','APPROVED')`,
			now, userID, planID, goalID)
		if err != nil {
			return result, err
		}
		if tag.RowsAffected() == 1 {
			result.StalePlans++
			result.PlanIDs = append(result.PlanIDs, planID)
		}
	}
	dedupeTasks := map[string]bool{}
	for _, taskID := range taskIDs {
		taskID = strings.TrimSpace(taskID)
		if taskID == "" || dedupeTasks[taskID] {
			continue
		}
		dedupeTasks[taskID] = true
		tag, err := tx.Exec(ctx, `UPDATE codelocal_project_tasks SET status='STALE',updated_at=$1,revision=revision+1 WHERE user_id=$2 AND task_id=$3 AND goal_id=$4 AND status NOT IN ('DONE','CANCELLED','STALE')`,
			now, userID, taskID, goalID)
		if err != nil {
			return result, err
		}
		if tag.RowsAffected() == 1 {
			result.StaleTasks++
			result.TaskIDs = append(result.TaskIDs, taskID)
		}
	}
	nextGoal := ""
	switch goal.Status {
	case "DRAFT", "PLAN_PROPOSED", "WAITING_APPROVAL":
		if goal.Status != "DRAFT" {
			nextGoal = "DRAFT"
		}
	case "APPROVED", "EXECUTING", "VERIFYING", "BLOCKED":
		nextGoal = "BLOCKED"
	}
	if nextGoal != "" {
		if _, err := tx.Exec(ctx, `UPDATE codelocal_project_goals SET status=$1,updated_at=$2,revision=revision+1 WHERE user_id=$3 AND goal_id=$4`,
			nextGoal, now, userID, goalID); err != nil {
			return result, err
		}
		result.GoalStatus = nextGoal
	} else {
		result.GoalStatus = goal.Status
	}
	if _, err := tx.Exec(ctx, `
INSERT INTO codelocal_project_task_events(event_id,user_id,project_id,goal_id,task_id,event_type,actor_type,actor_id,safe_payload_json,created_at)
VALUES($1,$2,$3,$4,NULL,'requirement.impact','system','',($5::jsonb),$6)`,
		"evt_"+RandomHex(12), userID, goal.ProjectID, goalID, sanitizeProjectOSPayload(map[string]any{"changeId": changeID, "stalePlans": result.StalePlans, "staleTasks": result.StaleTasks, "goalStatus": result.GoalStatus}), now); err != nil {
		return result, err
	}
	if err := tx.Commit(ctx); err != nil {
		return result, err
	}
	return result, nil
}

func (s *Store) ListRequirementChanges(ctx context.Context, userID, projectID string, limit int) ([]RequirementChange, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.DB.Query(ctx, `SELECT change_id,user_id,project_id,source,summary,changed_refs_json::text,created_at FROM codelocal_project_requirement_changes WHERE user_id=$1 AND project_id=$2 ORDER BY created_at DESC LIMIT $3`,
		strings.TrimSpace(userID), strings.TrimSpace(projectID), limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []RequirementChange{}
	for rows.Next() {
		var c RequirementChange
		var refs string
		if err := rows.Scan(&c.ChangeID, &c.UserID, &c.ProjectID, &c.Source, &c.Summary, &refs, &c.CreatedAt); err != nil {
			return nil, err
		}
		if refs == "" {
			refs = "[]"
		}
		c.ChangedRefs = json.RawMessage(refs)
		out = append(out, c)
	}
	return out, rows.Err()
}

package cloud

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	"github.com/jackc/pgx/v5"
)

type ProjectFeedback struct {
	FeedbackID string `json:"feedbackId"`
	UserID     string `json:"userId"`
	ProjectID  string `json:"projectId"`
	Source     string `json:"source"`
	Kind       string `json:"kind"`
	Title      string `json:"title"`
	Body       string `json:"body"`
	Status     string `json:"status"`
	DedupeKey  string `json:"dedupeKey"`
	CreatedAt  int64  `json:"createdAt"`
	UpdatedAt  int64  `json:"updatedAt"`
}

func normalizeFeedbackKind(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "bug", "feature", "praise":
		return strings.ToLower(strings.TrimSpace(v))
	}
	return "other"
}

func normalizeFeedbackSource(v string) string {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "widget", "chat":
		return strings.ToLower(strings.TrimSpace(v))
	}
	return "manual"
}

func validFeedbackTransition(from, to string) bool {
	if from == to {
		return true
	}
	allowed := map[string]map[string]bool{
		"new":       {"triaged": true, "dismissed": true},
		"triaged":   {"planned": true, "dismissed": true, "new": true},
		"planned":   {"resolved": true, "triaged": true},
		"resolved":  {},
		"dismissed": {"new": true},
	}
	return allowed[from][to]
}

func (s *Store) SubmitProjectFeedback(ctx context.Context, userID, projectID, source, kind, title, body string) (ProjectFeedback, error) {
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	title = normalizeProjectOSTitle(title, 200)
	body = strings.TrimSpace(body)
	if len([]rune(body)) > 4000 {
		body = string([]rune(body)[:4000])
	}
	if title == "" {
		return ProjectFeedback{}, errors.New("feedback title is required")
	}
	kind = normalizeFeedbackKind(kind)
	source = normalizeFeedbackSource(source)
	if err := s.ensureProjectOSProject(ctx, userID, projectID); err != nil {
		return ProjectFeedback{}, err
	}
	now := time.Now().UnixMilli()
	fb := ProjectFeedback{
		FeedbackID: "fb_" + RandomHex(12), UserID: userID, ProjectID: projectID,
		Source: source, Kind: kind, Title: title, Body: body, Status: "new",
		DedupeKey: orchestration.NormalizeFeedbackDedupeKey(kind, title),
		CreatedAt: now, UpdatedAt: now,
	}
	if _, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_project_feedback(feedback_id,user_id,project_id,source,kind,title,body,status,dedupe_key,created_at,updated_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$10)`,
		fb.FeedbackID, userID, projectID, source, kind, title, body, "new", fb.DedupeKey, now); err != nil {
		return ProjectFeedback{}, err
	}
	_ = s.appendProjectOSEvent(ctx, "feedback.submitted", userID, projectID, "", "", "user", source, map[string]any{"feedbackId": fb.FeedbackID, "kind": kind})
	return fb, nil
}

func (s *Store) ListProjectFeedback(ctx context.Context, userID, projectID, status string, limit int) ([]ProjectFeedback, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	userID = strings.TrimSpace(userID)
	projectID = strings.TrimSpace(projectID)
	status = strings.TrimSpace(status)
	const columns = `feedback_id,user_id,project_id,source,kind,title,body,status,dedupe_key,created_at,updated_at`
	var dbRows pgx.Rows
	var err error
	if status == "" {
		dbRows, err = s.DB.Query(ctx, `SELECT `+columns+` FROM codelocal_project_feedback WHERE user_id=$1 AND project_id=$2 ORDER BY created_at DESC LIMIT $3`, userID, projectID, limit)
	} else {
		dbRows, err = s.DB.Query(ctx, `SELECT `+columns+` FROM codelocal_project_feedback WHERE user_id=$1 AND project_id=$2 AND status=$3 ORDER BY created_at DESC LIMIT $4`, userID, projectID, status, limit)
	}
	if err != nil {
		return nil, err
	}
	defer dbRows.Close()
	out := []ProjectFeedback{}
	for dbRows.Next() {
		var fb ProjectFeedback
		if err := dbRows.Scan(&fb.FeedbackID, &fb.UserID, &fb.ProjectID, &fb.Source, &fb.Kind, &fb.Title, &fb.Body, &fb.Status, &fb.DedupeKey, &fb.CreatedAt, &fb.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, fb)
	}
	return out, dbRows.Err()
}

func (s *Store) TransitionProjectFeedback(ctx context.Context, userID, feedbackID, toStatus string) (ProjectFeedback, error) {
	toStatus = strings.TrimSpace(toStatus)
	var fb ProjectFeedback
	err := s.DB.QueryRow(ctx, `SELECT feedback_id,user_id,project_id,source,kind,title,body,status,dedupe_key,created_at,updated_at FROM codelocal_project_feedback WHERE user_id=$1 AND feedback_id=$2`,
		strings.TrimSpace(userID), strings.TrimSpace(feedbackID)).Scan(
		&fb.FeedbackID, &fb.UserID, &fb.ProjectID, &fb.Source, &fb.Kind, &fb.Title, &fb.Body, &fb.Status, &fb.DedupeKey, &fb.CreatedAt, &fb.UpdatedAt)
	if err != nil {
		return ProjectFeedback{}, errors.New("feedback not found")
	}
	if !validFeedbackTransition(fb.Status, toStatus) {
		return ProjectFeedback{}, errors.New("invalid feedback transition")
	}
	now := time.Now().UnixMilli()
	if _, err := s.DB.Exec(ctx, `UPDATE codelocal_project_feedback SET status=$1,updated_at=$2 WHERE user_id=$3 AND feedback_id=$4`, toStatus, now, userID, feedbackID); err != nil {
		return ProjectFeedback{}, err
	}
	fb.Status = toStatus
	fb.UpdatedAt = now
	_ = s.appendProjectOSEvent(ctx, "feedback.transition", userID, fb.ProjectID, "", "", "system", "", map[string]any{"feedbackId": feedbackID, "to": toStatus})
	return fb, nil
}

type FeedbackClusterView struct {
	DedupeKey string `json:"dedupeKey"`
	Kind      string `json:"kind"`
	Title     string `json:"title"`
	Count     int    `json:"count"`
}

// ProjectFeedbackSignals returns dedupe clusters at or above threshold among
// unresolved feedback. Only these clusters may surface as Needs You signals.
func (s *Store) ProjectFeedbackSignals(ctx context.Context, userID, projectID string, threshold int) ([]FeedbackClusterView, error) {
	if threshold <= 0 {
		threshold = 3
	}
	rows, err := s.DB.Query(ctx, `
SELECT dedupe_key,kind,MIN(title),COUNT(*)::int FROM codelocal_project_feedback
WHERE user_id=$1 AND project_id=$2 AND status NOT IN ('resolved','dismissed')
GROUP BY dedupe_key,kind HAVING COUNT(*) >= $3 ORDER BY COUNT(*) DESC LIMIT 20`,
		strings.TrimSpace(userID), strings.TrimSpace(projectID), threshold)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []FeedbackClusterView{}
	for rows.Next() {
		var v FeedbackClusterView
		if err := rows.Scan(&v.DedupeKey, &v.Kind, &v.Title, &v.Count); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

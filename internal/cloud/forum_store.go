package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

type forumRowScanner interface {
	Scan(dest ...any) error
}

const forumTopicSelect = `
SELECT t.topic_id,t.author_user_id,COALESCE(u.email,''),t.kind,t.title,t.body,t.status,t.severity,t.version,t.environment,
       t.reproduction_steps,t.expected_behavior,t.actual_behavior,t.tags,t.github_issue_url,t.github_issue_number,t.github_pr_url,
       t.resolution_note,t.created_at,t.updated_at,t.resolved_at,
       (SELECT COUNT(*) FROM codelocal_forum_comments c WHERE c.topic_id=t.topic_id AND c.deleted_at=0),
       (SELECT COUNT(*) FROM codelocal_forum_votes v WHERE v.topic_id=t.topic_id)
FROM codelocal_forum_topics t
LEFT JOIN codelocal_users u ON u.id=t.author_user_id`

func scanForumTopic(row forumRowScanner) (ForumTopic, error) {
	var topic ForumTopic
	var tagsJSON []byte
	if err := row.Scan(
		&topic.ID, &topic.AuthorUserID, &topic.AuthorEmail, &topic.Kind, &topic.Title, &topic.Body, &topic.Status,
		&topic.Severity, &topic.Version, &topic.Environment, &topic.ReproductionSteps, &topic.ExpectedBehavior, &topic.ActualBehavior,
		&tagsJSON, &topic.GitHubIssueURL, &topic.GitHubIssueNumber, &topic.GitHubPRURL, &topic.ResolutionNote,
		&topic.CreatedAt, &topic.UpdatedAt, &topic.ResolvedAt, &topic.CommentCount, &topic.VoteCount,
	); err != nil {
		return ForumTopic{}, err
	}
	if len(tagsJSON) > 0 {
		if err := json.Unmarshal(tagsJSON, &topic.Tags); err != nil {
			return ForumTopic{}, err
		}
	}
	if topic.Tags == nil {
		topic.Tags = []string{}
	}
	return topic, nil
}

func (s *Store) CreateForumTopic(ctx context.Context, input ForumTopicDraft) (ForumTopic, error) {
	input, err := normalizeForumDraft(input)
	if err != nil {
		return ForumTopic{}, err
	}
	tags, err := json.Marshal(input.Tags)
	if err != nil {
		return ForumTopic{}, err
	}
	now := time.Now().UnixMilli()
	id := "forum_" + RandomHex(12)
	_, err = s.DB.Exec(ctx, `
INSERT INTO codelocal_forum_topics(
 topic_id,author_user_id,kind,title,body,status,severity,version,environment,reproduction_steps,expected_behavior,actual_behavior,tags,
 created_at,updated_at
) VALUES($1,$2,$3,$4,$5,'open',$6,$7,$8,$9,$10,$11,$12::jsonb,$13,$13)`,
		id, input.AuthorUserID, input.Kind, input.Title, input.Body, input.Severity, input.Version, input.Environment,
		input.ReproductionSteps, input.ExpectedBehavior, input.ActualBehavior, string(tags), now,
	)
	if err != nil {
		return ForumTopic{}, err
	}
	return s.ForumTopicByID(ctx, id)
}

func (s *Store) ForumTopicByID(ctx context.Context, topicID string) (ForumTopic, error) {
	topicID = strings.TrimSpace(topicID)
	if topicID == "" {
		return ForumTopic{}, ErrForumNotFound
	}
	topic, err := scanForumTopic(s.DB.QueryRow(ctx, forumTopicSelect+` WHERE t.topic_id=$1 AND t.deleted_at=0`, topicID))
	if errors.Is(err, pgx.ErrNoRows) {
		return ForumTopic{}, ErrForumNotFound
	}
	return topic, err
}

func (s *Store) ListForumTopics(ctx context.Context, kind, status, query string, limit int) ([]ForumTopic, error) {
	if limit < 1 {
		limit = 100
	}
	if limit > 200 {
		limit = 200
	}
	kind = normalizeForumKind(kind)
	status = normalizeForumStatus(status)
	query = strings.TrimSpace(query)
	where := []string{"t.deleted_at=0"}
	args := []any{}
	if kind != "" {
		args = append(args, kind)
		where = append(where, fmt.Sprintf("t.kind=$%d", len(args)))
	}
	if status != "" {
		args = append(args, status)
		where = append(where, fmt.Sprintf("t.status=$%d", len(args)))
	}
	if query != "" {
		args = append(args, "%"+query+"%")
		where = append(where, fmt.Sprintf("(t.title ILIKE $%d OR t.body ILIKE $%d)", len(args), len(args)))
	}
	args = append(args, limit)
	sql := forumTopicSelect + " WHERE " + strings.Join(where, " AND ") + fmt.Sprintf(" ORDER BY CASE WHEN t.status IN ('open','under_review','planned','in_progress') THEN 0 ELSE 1 END,t.updated_at DESC LIMIT $%d", len(args))
	rows, err := s.DB.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ForumTopic{}
	for rows.Next() {
		topic, err := scanForumTopic(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, topic)
	}
	return out, rows.Err()
}

func (s *Store) CreateForumComment(ctx context.Context, topicID, authorUserID, body string) (ForumComment, error) {
	topicID = strings.TrimSpace(topicID)
	authorUserID = strings.TrimSpace(authorUserID)
	body = strings.TrimSpace(body)
	if topicID == "" || authorUserID == "" || body == "" || len(body) > 12000 {
		return ForumComment{}, ErrForumInvalid
	}
	if _, err := s.ForumTopicByID(ctx, topicID); err != nil {
		return ForumComment{}, err
	}
	id := "forumc_" + RandomHex(12)
	now := time.Now().UnixMilli()
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_forum_comments(comment_id,topic_id,author_user_id,body,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$5)`, id, topicID, authorUserID, body, now)
	if err != nil {
		return ForumComment{}, err
	}
	_, _ = s.DB.Exec(ctx, `UPDATE codelocal_forum_topics SET updated_at=$1 WHERE topic_id=$2`, now, topicID)
	return s.ForumCommentByID(ctx, id)
}

func (s *Store) ForumCommentByID(ctx context.Context, commentID string) (ForumComment, error) {
	var comment ForumComment
	err := s.DB.QueryRow(ctx, `
SELECT c.comment_id,c.topic_id,c.author_user_id,COALESCE(u.email,''),c.body,c.created_at,c.updated_at
FROM codelocal_forum_comments c LEFT JOIN codelocal_users u ON u.id=c.author_user_id
WHERE c.comment_id=$1 AND c.deleted_at=0`, strings.TrimSpace(commentID)).Scan(
		&comment.ID, &comment.TopicID, &comment.AuthorUserID, &comment.AuthorEmail, &comment.Body, &comment.CreatedAt, &comment.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ForumComment{}, ErrForumNotFound
	}
	return comment, err
}

func (s *Store) ListForumComments(ctx context.Context, topicID string) ([]ForumComment, error) {
	rows, err := s.DB.Query(ctx, `
SELECT c.comment_id,c.topic_id,c.author_user_id,COALESCE(u.email,''),c.body,c.created_at,c.updated_at
FROM codelocal_forum_comments c LEFT JOIN codelocal_users u ON u.id=c.author_user_id
WHERE c.topic_id=$1 AND c.deleted_at=0 ORDER BY c.created_at ASC,c.comment_id ASC`, strings.TrimSpace(topicID))
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ForumComment{}
	for rows.Next() {
		var comment ForumComment
		if err := rows.Scan(&comment.ID, &comment.TopicID, &comment.AuthorUserID, &comment.AuthorEmail, &comment.Body, &comment.CreatedAt, &comment.UpdatedAt); err != nil {
			return nil, err
		}
		out = append(out, comment)
	}
	return out, rows.Err()
}

func (s *Store) ToggleForumVote(ctx context.Context, topicID, userID string) (bool, int, error) {
	topicID = strings.TrimSpace(topicID)
	userID = strings.TrimSpace(userID)
	if topicID == "" || userID == "" {
		return false, 0, ErrForumInvalid
	}
	if _, err := s.ForumTopicByID(ctx, topicID); err != nil {
		return false, 0, err
	}
	command, err := s.DB.Exec(ctx, `DELETE FROM codelocal_forum_votes WHERE topic_id=$1 AND user_id=$2`, topicID, userID)
	if err != nil {
		return false, 0, err
	}
	voted := false
	if command.RowsAffected() == 0 {
		_, err = s.DB.Exec(ctx, `INSERT INTO codelocal_forum_votes(topic_id,user_id,created_at) VALUES($1,$2,$3) ON CONFLICT DO NOTHING`, topicID, userID, time.Now().UnixMilli())
		if err != nil {
			return false, 0, err
		}
		voted = true
	}
	var count int
	if err := s.DB.QueryRow(ctx, `SELECT COUNT(*) FROM codelocal_forum_votes WHERE topic_id=$1`, topicID).Scan(&count); err != nil {
		return false, 0, err
	}
	return voted, count, nil
}

func (s *Store) ForumViewerVoted(ctx context.Context, topicID, userID string) (bool, error) {
	var voted bool
	err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM codelocal_forum_votes WHERE topic_id=$1 AND user_id=$2)`, strings.TrimSpace(topicID), strings.TrimSpace(userID)).Scan(&voted)
	return voted, err
}

func (s *Store) AdminUpdateForumTopic(ctx context.Context, topicID string, input ForumAdminUpdate) (ForumTopic, error) {
	input, err := normalizeForumAdminUpdate(input)
	if err != nil {
		return ForumTopic{}, err
	}
	if _, err := s.ForumTopicByID(ctx, topicID); err != nil {
		return ForumTopic{}, err
	}
	now := time.Now().UnixMilli()
	resolvedAt := int64(0)
	if input.Status == "resolved" || input.Status == "closed" {
		resolvedAt = now
	}
	command, err := s.DB.Exec(ctx, `
UPDATE codelocal_forum_topics SET status=$1,severity=$2,github_issue_url=$3,github_issue_number=$4,github_pr_url=$5,resolution_note=$6,
 updated_at=$7,resolved_at=$8 WHERE topic_id=$9 AND deleted_at=0`, input.Status, input.Severity, input.GitHubIssueURL,
		input.GitHubIssueNumber, input.GitHubPRURL, input.ResolutionNote, now, resolvedAt, strings.TrimSpace(topicID))
	if err != nil {
		return ForumTopic{}, err
	}
	if command.RowsAffected() != 1 {
		return ForumTopic{}, ErrForumNotFound
	}
	return s.ForumTopicByID(ctx, topicID)
}

package cloud

import (
	"context"
	"encoding/json"
	"strings"
)

type DashboardChatMessage struct {
	ID        string          `json:"id"`
	UserID    string          `json:"userId"`
	ThreadID  string          `json:"threadId,omitempty"`
	Role      string          `json:"role"`
	Content   string          `json:"content"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
	Skills    json.RawMessage `json:"skills,omitempty"`
	Image     string          `json:"image,omitempty"`
	CreatedAt int64           `json:"createdAt"`
}

const dashboardChatMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_dashboard_chat (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 role TEXT NOT NULL CHECK (role IN ('user','assistant','tool')),
 content TEXT NOT NULL,
 tool_calls JSONB NOT NULL DEFAULT '[]'::jsonb,
 image TEXT,
 created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_dashboard_chat_user_time ON codelocal_dashboard_chat(user_id, created_at DESC);
`

const dashboardChatImageMigrationSQL = `ALTER TABLE codelocal_dashboard_chat ADD COLUMN IF NOT EXISTS image TEXT;`

func (s *Store) SaveDashboardChatMessage(ctx context.Context, msg DashboardChatMessage) error {
	// truncate image if too large for DB (8MB base64)
	img := msg.Image
	if len(img) > 2*1024*1024 {
		img = img[:2*1024*1024]
	}
	toolCalls := string(msg.ToolCalls)
	if toolCalls == "" {
		toolCalls = "[]"
	}
	skills := msg.Skills
	if len(skills) == 0 && msg.Role == "assistant" {
		skills = dashboardChatSkillsFromContext(ctx)
	}
	if len(skills) == 0 || !json.Valid(skills) {
		skills = json.RawMessage(`[]`)
	}
	var threadID any
	if msg.ThreadID != "" {
		threadID = msg.ThreadID
	}
	_, err := s.DB.Exec(ctx, `
INSERT INTO codelocal_dashboard_chat(id, user_id, thread_id, role, content, tool_calls, skills, image, created_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9)
ON CONFLICT (id) DO UPDATE SET
 thread_id=EXCLUDED.thread_id,
 content=EXCLUDED.content,
 tool_calls=EXCLUDED.tool_calls,
 skills=EXCLUDED.skills,
 image=EXCLUDED.image
WHERE codelocal_dashboard_chat.user_id=EXCLUDED.user_id AND codelocal_dashboard_chat.role=EXCLUDED.role`, msg.ID, msg.UserID, threadID, msg.Role, msg.Content, toolCalls, string(skills), img, msg.CreatedAt)
	return err
}

func (s *Store) ListDashboardChatHistory(ctx context.Context, userID string, limit int) ([]DashboardChatMessage, error) {
	return s.ListDashboardChatHistoryForThread(ctx, userID, "", limit)
}

func (s *Store) ListDashboardChatHistoryForThread(ctx context.Context, userID, threadID string, limit int) ([]DashboardChatMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	threadID = strings.TrimSpace(threadID)
	if threadID != "" {
		qRows, err := s.DB.Query(ctx, `
SELECT id, user_id, thread_id, role, content, tool_calls, skills, image, created_at
FROM (
 SELECT id, user_id, thread_id, role, content, tool_calls, skills, image, created_at
 FROM codelocal_dashboard_chat
 WHERE user_id=$1 AND thread_id=$2
 ORDER BY created_at DESC
 LIMIT $3
) recent
ORDER BY created_at ASC`, userID, threadID, limit)
		if err != nil {
			return nil, err
		}
		defer qRows.Close()
		var out []DashboardChatMessage
		for qRows.Next() {
			var m DashboardChatMessage
			var threadIDVal *string
			var toolCalls string
			var skills string
			var img *string
			if err := qRows.Scan(&m.ID, &m.UserID, &threadIDVal, &m.Role, &m.Content, &toolCalls, &skills, &img, &m.CreatedAt); err != nil {
				return nil, err
			}
			if threadIDVal != nil {
				m.ThreadID = *threadIDVal
			}
			m.ToolCalls = json.RawMessage(toolCalls)
			m.Skills = json.RawMessage(skills)
			if img != nil {
				m.Image = *img
			}
			out = append(out, m)
		}
		return out, qRows.Err()
	}
	qRows, err := s.DB.Query(ctx, `
SELECT id, user_id, thread_id, role, content, tool_calls, skills, image, created_at
FROM (
 SELECT id, user_id, thread_id, role, content, tool_calls, skills, image, created_at
 FROM codelocal_dashboard_chat
 WHERE user_id=$1
 ORDER BY created_at DESC
 LIMIT $2
) recent
ORDER BY created_at ASC`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer qRows.Close()
	var out []DashboardChatMessage
	for qRows.Next() {
		var m DashboardChatMessage
		var threadIDVal *string
		var toolCalls string
		var skills string
		var img *string
		if err := qRows.Scan(&m.ID, &m.UserID, &threadIDVal, &m.Role, &m.Content, &toolCalls, &skills, &img, &m.CreatedAt); err != nil {
			return nil, err
		}
		if threadIDVal != nil {
			m.ThreadID = *threadIDVal
		}
		m.ToolCalls = json.RawMessage(toolCalls)
		m.Skills = json.RawMessage(skills)
		if img != nil {
			m.Image = *img
		}
		out = append(out, m)
	}
	return out, qRows.Err()
}

func (s *Store) ClearDashboardChatHistory(ctx context.Context, userID string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM codelocal_dashboard_chat WHERE user_id=$1`, userID)
	return err
}

func (s *Store) ClearDashboardChatHistoryForThread(ctx context.Context, userID, threadID string) error {
	threadID = strings.TrimSpace(threadID)
	if threadID == "" {
		return s.ClearDashboardChatHistory(ctx, userID)
	}
	_, err := s.DB.Exec(ctx, `DELETE FROM codelocal_dashboard_chat WHERE user_id=$1 AND thread_id=$2`, userID, threadID)
	return err
}

package cloud

import (
	"context"
	"encoding/json"
)

type DashboardChatMessage struct {
	ID        string `json:"id"`
	UserID    string `json:"userId"`
	Role      string `json:"role"`
	Content   string `json:"content"`
	ToolCalls json.RawMessage `json:"tool_calls,omitempty"`
	CreatedAt int64  `json:"createdAt"`
}

const dashboardChatMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_dashboard_chat (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 role TEXT NOT NULL CHECK (role IN ('user','assistant','tool')),
 content TEXT NOT NULL,
 tool_calls JSONB NOT NULL DEFAULT '[]'::jsonb,
 created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_dashboard_chat_user_time ON codelocal_dashboard_chat(user_id, created_at DESC);
`

func (s *Store) SaveDashboardChatMessage(ctx context.Context, msg DashboardChatMessage) error {
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_dashboard_chat(id, user_id, role, content, tool_calls, created_at) VALUES($1,$2,$3,$4,$5,$6)`, msg.ID, msg.UserID, msg.Role, msg.Content, string(msg.ToolCalls), msg.CreatedAt)
	return err
}

func (s *Store) ListDashboardChatHistory(ctx context.Context, userID string, limit int) ([]DashboardChatMessage, error) {
	if limit <= 0 || limit > 100 {
		limit = 30
	}
	rows, err := s.DB.Query(ctx, `SELECT id, user_id, role, content, tool_calls, created_at FROM codelocal_dashboard_chat WHERE user_id=$1 ORDER BY created_at ASC LIMIT $2`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DashboardChatMessage
	for rows.Next() {
		var m DashboardChatMessage
		var toolCalls string
		if err := rows.Scan(&m.ID, &m.UserID, &m.Role, &m.Content, &toolCalls, &m.CreatedAt); err != nil {
			return nil, err
		}
		m.ToolCalls = json.RawMessage(toolCalls)
		out = append(out, m)
	}
	return out, rows.Err()
}

func (s *Store) ClearDashboardChatHistory(ctx context.Context, userID string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM codelocal_dashboard_chat WHERE user_id=$1`, userID)
	return err
}

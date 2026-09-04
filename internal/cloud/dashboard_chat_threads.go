package cloud

import (
	"context"
	"strings"
	"time"
)

type DashboardChatThread struct {
	ID           string `json:"id"`
	UserID       string `json:"userId"`
	Title        string `json:"title"`
	Model        string `json:"model"`
	WorkspaceKey string `json:"workspaceKey,omitempty"`
	CreatedAt    int64  `json:"createdAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

func normalizeDashboardChatThreadTitle(title string) string {
	title = strings.TrimSpace(title)
	if title == "" {
		return "Cuộc trò chuyện mới"
	}
	runes := []rune(title)
	if len(runes) > 120 {
		title = string(runes[:120])
	}
	return title
}

func normalizeDashboardChatThreadModel(model string) string {
	model = strings.TrimSpace(model)
	if model == "" {
		return "auto"
	}
	runes := []rune(model)
	if len(runes) > 240 {
		model = string(runes[:240])
	}
	return model
}

func normalizeDashboardChatWorkspaceKey(workspaceKey string) string {
	workspaceKey = strings.TrimSpace(workspaceKey)
	runes := []rune(workspaceKey)
	if len(runes) > 500 {
		workspaceKey = string(runes[:500])
	}
	return workspaceKey
}

const dashboardChatThreadMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_dashboard_chat_thread (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 title TEXT NOT NULL DEFAULT 'Cuộc trò chuyện mới',
 model TEXT NOT NULL DEFAULT 'auto',
 workspace_key TEXT,
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_dashboard_chat_thread_user_time ON codelocal_dashboard_chat_thread(user_id, updated_at DESC);
ALTER TABLE codelocal_dashboard_chat ADD COLUMN IF NOT EXISTS thread_id TEXT REFERENCES codelocal_dashboard_chat_thread(id) ON DELETE CASCADE;
CREATE INDEX IF NOT EXISTS idx_codelocal_dashboard_chat_thread_msg ON codelocal_dashboard_chat(thread_id, created_at);
-- backfill existing history into one thread per user (id deterministic from user_id)
INSERT INTO codelocal_dashboard_chat_thread(id, user_id, title, model, workspace_key, created_at, updated_at)
SELECT 'thr_legacy_' || md5(user_id), user_id, 'Cuộc trò chuyện trước đó', 'auto', NULL, COALESCE(MIN(created_at), (EXTRACT(EPOCH FROM NOW())*1000)::BIGINT), COALESCE(MAX(created_at), (EXTRACT(EPOCH FROM NOW())*1000)::BIGINT)
FROM codelocal_dashboard_chat WHERE thread_id IS NULL GROUP BY user_id
ON CONFLICT(id) DO NOTHING;
UPDATE codelocal_dashboard_chat SET thread_id = 'thr_legacy_' || md5(user_id) WHERE thread_id IS NULL;
`

func (s *Store) ListDashboardChatThreads(ctx context.Context, userID string) ([]DashboardChatThread, error) {
	rows, err := s.DB.Query(ctx, `SELECT id, user_id, title, model, workspace_key, created_at, updated_at FROM codelocal_dashboard_chat_thread WHERE user_id=$1 ORDER BY updated_at DESC LIMIT 100`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []DashboardChatThread
	for rows.Next() {
		var t DashboardChatThread
		var wk *string
		if err := rows.Scan(&t.ID, &t.UserID, &t.Title, &t.Model, &wk, &t.CreatedAt, &t.UpdatedAt); err != nil {
			return nil, err
		}
		if wk != nil {
			t.WorkspaceKey = *wk
		}
		out = append(out, t)
	}
	return out, rows.Err()
}

func (s *Store) CreateDashboardChatThread(ctx context.Context, userID, title, model, workspaceKey string, now int64) (DashboardChatThread, error) {
	title = normalizeDashboardChatThreadTitle(title)
	model = normalizeDashboardChatThreadModel(model)
	workspaceKey = normalizeDashboardChatWorkspaceKey(workspaceKey)
	id := "thr_" + RandomHex(12)
	if now == 0 {
		now = time.Now().UnixMilli()
	}
	var wk any
	if workspaceKey != "" {
		wk = workspaceKey
	}
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_dashboard_chat_thread(id, user_id, title, model, workspace_key, created_at, updated_at) VALUES($1,$2,$3,$4,$5,$6,$6)`, id, userID, title, model, wk, now)
	if err != nil {
		return DashboardChatThread{}, err
	}
	return DashboardChatThread{ID: id, UserID: userID, Title: title, Model: model, WorkspaceKey: workspaceKey, CreatedAt: now, UpdatedAt: now}, nil
}

func (s *Store) GetDashboardChatThread(ctx context.Context, userID, threadID string) (*DashboardChatThread, error) {
	var t DashboardChatThread
	var wk *string
	err := s.DB.QueryRow(ctx, `SELECT id, user_id, title, model, workspace_key, created_at, updated_at FROM codelocal_dashboard_chat_thread WHERE id=$1 AND user_id=$2`, threadID, userID).Scan(&t.ID, &t.UserID, &t.Title, &t.Model, &wk, &t.CreatedAt, &t.UpdatedAt)
	if err != nil {
		return nil, err
	}
	if wk != nil {
		t.WorkspaceKey = *wk
	}
	return &t, nil
}

func (s *Store) UpdateDashboardChatThreadTitle(ctx context.Context, userID, threadID, title string, now int64) error {
	title = normalizeDashboardChatThreadTitle(title)
	_, err := s.DB.Exec(ctx, `UPDATE codelocal_dashboard_chat_thread SET title=$1, updated_at=$2 WHERE id=$3 AND user_id=$4`, title, now, threadID, userID)
	return err
}

func (s *Store) UpdateDashboardChatThreadMeta(ctx context.Context, userID, threadID, model, workspaceKey string, now int64) error {
	model = normalizeDashboardChatThreadModel(model)
	workspaceKey = normalizeDashboardChatWorkspaceKey(workspaceKey)
	var wk any
	if workspaceKey != "" {
		wk = workspaceKey
	}
	_, err := s.DB.Exec(ctx, `UPDATE codelocal_dashboard_chat_thread SET model=$1, workspace_key=$2, updated_at=$3 WHERE id=$4 AND user_id=$5`, model, wk, now, threadID, userID)
	return err
}

func (s *Store) TouchDashboardChatThread(ctx context.Context, userID, threadID string, now int64) error {
	_, err := s.DB.Exec(ctx, `UPDATE codelocal_dashboard_chat_thread SET updated_at=GREATEST(updated_at,$1) WHERE id=$2 AND user_id=$3`, now, threadID, userID)
	return err
}

func (s *Store) DeleteDashboardChatThread(ctx context.Context, userID, threadID string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM codelocal_dashboard_chat_thread WHERE id=$1 AND user_id=$2`, threadID, userID)
	return err
}

func (s *Store) EnsureDashboardChatThreadForMessage(ctx context.Context, userID, threadID string, now int64) (string, error) {
	threadID = strings.TrimSpace(threadID)
	if threadID != "" {
		var exists bool
		if err := s.DB.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM codelocal_dashboard_chat_thread WHERE id=$1 AND user_id=$2)`, threadID, userID).Scan(&exists); err != nil {
			return "", err
		}
		if exists {
			return threadID, nil
		}
	}
	// create new thread lazily
	thr, err := s.CreateDashboardChatThread(ctx, userID, "Cuộc trò chuyện mới", "auto", "", now)
	if err != nil {
		return "", err
	}
	return thr.ID, nil
}

func DashboardChatAutoTitle(content string) string {
	cleaned := strings.TrimSpace(content)
	cleaned = strings.Join(strings.Fields(cleaned), " ")
	if cleaned == "" {
		return "Cuộc trò chuyện mới"
	}
	runes := []rune(cleaned)
	if len(runes) > 48 {
		cleaned = string(runes[:48])
	}
	return cleaned
}

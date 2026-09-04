package cloud

import (
	"context"
	"strings"
	"time"
)

// DashboardChatUsageEvent is one idempotent provider-reported model invocation.
type DashboardChatUsageEvent struct {
	EventID, UserID, ThreadID, Model, Protocol        string
	InputTokens, OutputTokens, TotalTokens, CreatedAt int64
}

type DashboardChatUsageSummary struct {
	Turns        int64 `json:"turns"`
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	TotalTokens  int64 `json:"totalTokens"`
}

const dashboardChatUsageMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_dashboard_chat_usage (
  event_id TEXT PRIMARY KEY,
  user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
  thread_id TEXT,
  model TEXT NOT NULL,
  protocol TEXT NOT NULL,
  input_tokens BIGINT NOT NULL CHECK (input_tokens >= 0),
  output_tokens BIGINT NOT NULL CHECK (output_tokens >= 0),
  total_tokens BIGINT NOT NULL CHECK (total_tokens >= 0),
  created_at BIGINT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_codelocal_dashboard_chat_usage_user_time ON codelocal_dashboard_chat_usage(user_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_codelocal_dashboard_chat_usage_thread_time ON codelocal_dashboard_chat_usage(user_id, thread_id, created_at DESC);
`

func (s *Store) RecordDashboardChatUsage(ctx context.Context, e DashboardChatUsageEvent) error {
	if strings.TrimSpace(e.EventID) == "" || strings.TrimSpace(e.UserID) == "" || e.InputTokens < 0 || e.OutputTokens < 0 || e.TotalTokens < 0 {
		return nil
	}
	if e.TotalTokens == 0 {
		e.TotalTokens = e.InputTokens + e.OutputTokens
	}
	// Missing usage is not zero usage; never manufacture a "reported" record.
	if e.InputTokens == 0 && e.OutputTokens == 0 && e.TotalTokens == 0 {
		return nil
	}
	if e.CreatedAt == 0 {
		e.CreatedAt = time.Now().UnixMilli()
	}
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_dashboard_chat_usage(event_id,user_id,thread_id,model,protocol,input_tokens,output_tokens,total_tokens,created_at) VALUES($1,$2,NULLIF($3,''),$4,$5,$6,$7,$8,$9) ON CONFLICT(event_id) DO NOTHING`, e.EventID, e.UserID, e.ThreadID, e.Model, e.Protocol, e.InputTokens, e.OutputTokens, e.TotalTokens, e.CreatedAt)
	return err
}

func (s *Store) DashboardChatUsageSummary(ctx context.Context, userID string, since int64) (DashboardChatUsageSummary, error) {
	var out DashboardChatUsageSummary
	err := s.DB.QueryRow(ctx, `SELECT COUNT(*),COALESCE(SUM(input_tokens),0),COALESCE(SUM(output_tokens),0),COALESCE(SUM(total_tokens),0) FROM codelocal_dashboard_chat_usage WHERE user_id=$1 AND ($2=0 OR created_at >= $2)`, userID, since).Scan(&out.Turns, &out.InputTokens, &out.OutputTokens, &out.TotalTokens)
	return out, err
}

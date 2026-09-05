package cloudserver

import (
	"log/slog"
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type usageWindowDTO struct {
	Calls                 int64 `json:"calls"`
	InputTokensEstimated  int64 `json:"inputTokensEstimated"`
	OutputTokensEstimated int64 `json:"outputTokensEstimated"`
	TotalTokensEstimated  int64 `json:"totalTokensEstimated"`
}

type reportedUsageWindowDTO struct {
	Turns        int64 `json:"turns"`
	InputTokens  int64 `json:"inputTokens"`
	OutputTokens int64 `json:"outputTokens"`
	TotalTokens  int64 `json:"totalTokens"`
}

type reportedUsageDTO struct {
	Source  string                 `json:"source"`
	Last1h  reportedUsageWindowDTO `json:"last1h"`
	Last24h reportedUsageWindowDTO `json:"last24h"`
	Last30d reportedUsageWindowDTO `json:"last30d"`
	AllTime reportedUsageWindowDTO `json:"allTime"`
}

type estimatedUsageDTO struct {
	Source  string         `json:"source"`
	Last1h  usageWindowDTO `json:"last1h"`
	Last24h usageWindowDTO `json:"last24h"`
	Last30d usageWindowDTO `json:"last30d"`
	AllTime usageWindowDTO `json:"allTime"`
}

type usageResourceDTO struct {
	// Legacy MCP fields remain during rolling deployments.
	Estimated bool              `json:"estimated"`
	Scope     string            `json:"scope"`
	Last1h    usageWindowDTO    `json:"last1h"`
	Last24h   usageWindowDTO    `json:"last24h"`
	Last30d   usageWindowDTO    `json:"last30d"`
	AllTime   usageWindowDTO    `json:"allTime"`
	WebChat   reportedUsageDTO  `json:"webChat"`
	MCP       estimatedUsageDTO `json:"mcp"`
}

func usageWindow(v cloud.MCPUsageSummary) usageWindowDTO {
	return usageWindowDTO{v.Calls, v.InputTokensEst, v.OutputTokensEst, v.TotalTokensEst}
}
func reportedUsageWindow(v cloud.DashboardChatUsageSummary) reportedUsageWindowDTO {
	return reportedUsageWindowDTO{v.Turns, v.InputTokens, v.OutputTokens, v.TotalTokens}
}

func buildUsageResourceDTO(m1, m24, m30, mall cloud.MCPUsageSummary, w1, w24, w30, wall cloud.DashboardChatUsageSummary) usageResourceDTO {
	return usageResourceDTO{true, dashboardUsageScope, usageWindow(m1), usageWindow(m24), usageWindow(m30), usageWindow(mall),
		reportedUsageDTO{"provider_reported", reportedUsageWindow(w1), reportedUsageWindow(w24), reportedUsageWindow(w30), reportedUsageWindow(wall)},
		estimatedUsageDTO{"payload_estimated", usageWindow(m1), usageWindow(m24), usageWindow(m30), usageWindow(mall)}}
}

func (s *Server) usageResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	// Usage is approximate telemetry, never a hard dependency for the
	// dashboard. A transient Redis/Postgres failure must degrade to an
	// empty-but-valid resource instead of 503 + "Usage chưa phản hồi".
	now := time.Now()
	since := []int64{now.Add(-time.Hour).UnixMilli(), now.Add(-24 * time.Hour).UnixMilli(), now.Add(-30 * 24 * time.Hour).UnixMilli(), 0}
	m := make([]cloud.MCPUsageSummary, 4)
	c := make([]cloud.DashboardChatUsageSummary, 4)
	for i, start := range since {
		var err error
		m[i], err = s.Store.MCPUsageSummary(r.Context(), identity.User.ID, start)
		if err == nil {
			c[i], err = s.Store.DashboardChatUsageSummary(r.Context(), identity.User.ID, start)
		}
		if err != nil {
			slog.Warn("dashboard usage telemetry degraded; returning empty window", "windowIndex", i, "error", err, "userId", identity.User.ID)
		}
	}
	webutil.JSON(w, http.StatusOK, buildUsageResourceDTO(m[0], m[1], m[2], m[3], c[0], c[1], c[2], c[3]))
}

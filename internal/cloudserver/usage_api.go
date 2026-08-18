package cloudserver

import (
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

type usageResourceDTO struct {
	Estimated bool           `json:"estimated"`
	Scope     string         `json:"scope"`
	Last24h   usageWindowDTO `json:"last24h"`
	Last30d   usageWindowDTO `json:"last30d"`
	AllTime   usageWindowDTO `json:"allTime"`
}

func usageWindow(value cloud.MCPUsageSummary) usageWindowDTO {
	return usageWindowDTO{
		Calls:                 value.Calls,
		InputTokensEstimated:  value.InputTokensEst,
		OutputTokensEstimated: value.OutputTokensEst,
		TotalTokensEstimated:  value.TotalTokensEst,
	}
}

func buildUsageResourceDTO(last24h, last30d, allTime cloud.MCPUsageSummary) usageResourceDTO {
	return usageResourceDTO{
		Estimated: true,
		Scope:     dashboardUsageScope,
		Last24h:   usageWindow(last24h),
		Last30d:   usageWindow(last30d),
		AllTime:   usageWindow(allTime),
	}
}

func (s *Server) usageResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}

	now := time.Now()
	last24h, err := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, now.Add(-24*time.Hour).UnixMilli())
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "usage_unavailable"})
		return
	}
	last30d, err := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, now.Add(-30*24*time.Hour).UnixMilli())
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "usage_unavailable"})
		return
	}
	allTime, err := s.Store.MCPUsageSummary(r.Context(), identity.User.ID, 0)
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "usage_unavailable"})
		return
	}

	webutil.JSON(w, http.StatusOK, buildUsageResourceDTO(last24h, last30d, allTime))
}

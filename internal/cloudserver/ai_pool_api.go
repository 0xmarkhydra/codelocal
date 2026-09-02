package cloudserver

import (
	"context"
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type aiPoolResource struct {
	Configured   bool     `json:"configured"`
	RoutingReady bool     `json:"routingReady"`
	Available    bool     `json:"available"`
	DashboardURL string   `json:"dashboardUrl,omitempty"`
	DefaultModel string   `json:"defaultModel,omitempty"`
	ModelCount   int      `json:"modelCount"`
	Models       []string `json:"models"`
}

func (s *Server) aiPoolResourceAPI(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.authenticatedAPIIdentity(w, r); !ok {
		return
	}
	config, configured := dashboardAIPoolConfigFromEnv()
	resource := aiPoolResource{Configured: configured, Models: []string{}}
	if !configured {
		webutil.JSON(w, http.StatusOK, resource)
		return
	}

	resource.DashboardURL = config.DashboardURL
	resource.DefaultModel = config.DefaultModel

	ctx, cancel := context.WithTimeout(r.Context(), 6*time.Second)
	defer cancel()
	models, err := dashboardAIPoolModels(ctx)
	if err == nil {
		all := dashboardAIPoolModelIDs(models, 0)
		resource.Available = true
		resource.ModelCount = len(all)
		for _, model := range all {
			if model == config.DefaultModel {
				resource.RoutingReady = true
				break
			}
		}
		if len(all) > 100 {
			resource.Models = append([]string(nil), all[:100]...)
		} else {
			resource.Models = append([]string(nil), all...)
		}
	}
	webutil.JSON(w, http.StatusOK, resource)
}

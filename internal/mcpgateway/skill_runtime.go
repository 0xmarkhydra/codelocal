package mcpgateway

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/orchestration"
	skillintel "github.com/0xmarkhydra/codelocal/internal/skills"
	"github.com/0xmarkhydra/codelocal/internal/taskstate"
)

type mcpSkillServices struct {
	Runtime *cloud.SkillRuntime
	Err     error
}

var mcpSkillServicesByService sync.Map

func skillServicesForMCP(s *Service) *mcpSkillServices {
	if s == nil || s.Store == nil {
		return &mcpSkillServices{Runtime: cloud.NewSkillRuntime(nil, nil)}
	}
	if cached, ok := mcpSkillServicesByService.Load(s); ok {
		return cached.(*mcpSkillServices)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	durable, _, configured, err := cloud.ResolveSkillPackageStore(ctx, s.Store)
	services := &mcpSkillServices{Err: err}
	if err != nil || !configured || durable == nil {
		services.Runtime = cloud.NewSkillRuntime(s.Store, nil)
		actual, _ := mcpSkillServicesByService.LoadOrStore(s, services)
		return actual.(*mcpSkillServices)
	}
	cache, cacheErr := skillintel.NewCachedPackageStore(durable, skillintel.DefaultPackageMemoryCacheBytes)
	if cacheErr != nil {
		services.Err = cacheErr
		services.Runtime = cloud.NewSkillRuntime(s.Store, nil)
		actual, _ := mcpSkillServicesByService.LoadOrStore(s, services)
		return actual.(*mcpSkillServices)
	}
	services.Runtime = cloud.NewSkillRuntime(s.Store, cache)
	actual, _ := mcpSkillServicesByService.LoadOrStore(s, services)
	return actual.(*mcpSkillServices)
}

func (s *Service) tenantAgentPlan(ctx context.Context, userID string, input orchestration.PlanInput) orchestration.AgentPlan {
	services := skillServicesForMCP(s)
	if services == nil || services.Runtime == nil || s == nil || s.Store == nil {
		return orchestration.BuildPlan(input)
	}
	if services.Err != nil {
		slog.Warn("MCP Skill package storage unavailable; continuing with registry-only tenant Skills", "error", services.Err)
	}
	snapshot, err := services.Runtime.Snapshot(ctx, userID)
	if err != nil {
		slog.Warn("tenant Skill runtime unavailable for MCP; using built-in Skill plan", "error", err)
		return orchestration.BuildPlan(input)
	}
	return orchestration.BuildPlanWithSkillEngine(input, snapshot.Engine, snapshot.PreferenceAffinity)
}

func (s *Service) tenantAgentPlanFromState(ctx context.Context, userID string, state taskstate.State, caps orchestration.Capabilities, project orchestration.ProjectProfile) orchestration.AgentPlan {
	return s.tenantAgentPlan(ctx, userID, planInputFromState(state, caps, project))
}

func clearSkillServicesForMCP(s *Service) {
	if s != nil {
		mcpSkillServicesByService.Delete(s)
	}
}

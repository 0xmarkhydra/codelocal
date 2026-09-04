package orchestration

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/agentruntime"
	"github.com/0xmarkhydra/codelocal/internal/runtimeevents"
	"github.com/0xmarkhydra/codelocal/internal/usage"
)

var ErrAgentOSPrepare = errors.New("agent os preparation failed")

type AgentOSRequest struct {
	WorkspaceKey  string                       `json:"workspaceKey"`
	TaskID        string                       `json:"taskId"`
	Objective     string                       `json:"objective"`
	Shape         TaskShape                    `json:"shape"`
	Verification  VerificationPlan             `json:"verification"`
	Budget        usage.Budget                 `json:"budget"`
	EngineHistory []agentruntime.EngineHistory `json:"engineHistory,omitempty"`
}

type PlannedAgent struct {
	Member  TeamMemberPlan               `json:"member"`
	AgentID string                       `json:"agentId"`
	NodeID  string                       `json:"nodeId"`
	Route   agentruntime.EngineCandidate `json:"route"`
}

type PreparedAgentOS struct {
	Runtime      *ShadowRuntime        `json:"-"`
	Kernel       *CodingKernel         `json:"-"`
	Evidence     *FailureEvidenceStore `json:"-"`
	Usage        *usage.Ledger         `json:"-"`
	Team         TeamPlan              `json:"team"`
	Agents       []PlannedAgent        `json:"agents"`
	LeadAgentID  string                `json:"leadAgentId"`
	Reservations []usage.Reservation   `json:"reservations"`
}

// PrepareAgentOS compiles routing/team decisions into the durable V3 primitives.
// It intentionally stops before starting external engine sessions. Production
// dispatch therefore cannot create half a team before budget, route, graph and
// DAG admission have all succeeded.
func PrepareAgentOS(ctx context.Context, events *runtimeevents.Store, registry *agentruntime.Registry, request AgentOSRequest) (*PreparedAgentOS, error) {
	request.WorkspaceKey = strings.TrimSpace(request.WorkspaceKey)
	request.TaskID = strings.TrimSpace(request.TaskID)
	request.Objective = strings.Join(strings.Fields(request.Objective), " ")
	if request.WorkspaceKey == "" || request.TaskID == "" || request.Objective == "" || registry == nil {
		return nil, ErrAgentOSPrepare
	}
	team := PlanAdaptiveTeam(request.Shape, nil)
	if len(team.Members) == 0 {
		return nil, ErrAgentOSPrepare
	}
	ledger := usage.NewLedger(request.Budget, usage.TaskUsage{TaskID: request.TaskID})
	reservations, err := ReserveTeamBudget(ledger, request.TaskID, team)
	if err != nil {
		return nil, err
	}
	releaseReservations := func() {
		for _, reservation := range reservations {
			_, _ = ledger.Release(reservation.ID)
		}
	}

	probes := registry.ProbeAll(ctx)
	planned := make([]PlannedAgent, 0, len(team.Members))
	for _, member := range team.Members {
		mode := agentruntime.ModeMutate
		if member.ReadOnly || member.VerificationOnly {
			mode = agentruntime.ModeReview
		}
		ranked := agentruntime.RankEngines(probes, agentruntime.RouteRequest{Mode: mode, EngineProfile: member.EngineProfile, Budget: ledger.Snapshot().Decision, History: request.EngineHistory})
		if len(ranked) == 0 {
			releaseReservations()
			return nil, fmt.Errorf("%w: no engine for %s", ErrAgentOSPrepare, member.ID)
		}
		agentID := "agent:" + request.TaskID + ":" + member.ID
		planned = append(planned, PlannedAgent{Member: member, AgentID: agentID, NodeID: "node:" + member.ID, Route: ranked[0]})
	}

	runtime, err := NewShadowRuntime(events, request.WorkspaceKey, request.TaskID, request.Verification)
	if err != nil {
		releaseReservations()
		return nil, err
	}
	leadPlan := planned[0]
	lead, err := runtime.Graph().Register(agentruntime.AgentIdentity{ID: leadPlan.AgentID, TaskID: request.TaskID, Role: agentruntime.RoleLead, EngineID: leadPlan.Route.EngineID})
	if err != nil {
		releaseReservations()
		return nil, err
	}
	leadPlan.AgentID = lead.ID
	planned[0] = leadPlan

	for index := range planned {
		item := &planned[index]
		if index > 0 {
			child, _, spawnErr := runtime.Graph().Spawn(lead.ID, agentruntime.AgentIdentity{ID: item.AgentID, TaskID: request.TaskID, Role: specialistAgentRole(item.Member.Role), EngineID: item.Route.EngineID}, agentruntime.EdgeDelegate)
			if spawnErr != nil {
				releaseReservations()
				return nil, spawnErr
			}
			item.AgentID = child.ID
		}
		blocked := make([]string, 0, len(item.Member.DependsOn))
		for _, dep := range item.Member.DependsOn {
			blocked = append(blocked, "node:"+dep)
		}
		node, addErr := runtime.DAG().Add(TaskNode{ID: item.NodeID, Subject: string(item.Member.Role), Description: request.Objective, OwnerAgentID: item.AgentID, BlockedBy: blocked})
		if addErr != nil {
			releaseReservations()
			return nil, addErr
		}
		item.NodeID = node.ID
	}
	kernel, err := NewCodingKernel(events, request.WorkspaceKey, request.TaskID, runtime)
	if err != nil {
		releaseReservations()
		return nil, err
	}
	evidence, err := NewFailureEvidenceStore(events, request.WorkspaceKey, request.TaskID)
	if err != nil {
		releaseReservations()
		return nil, err
	}
	return &PreparedAgentOS{Runtime: runtime, Kernel: kernel, Evidence: evidence, Usage: ledger, Team: team, Agents: planned, LeadAgentID: lead.ID, Reservations: reservations}, nil
}

func specialistAgentRole(role Specialist) agentruntime.AgentRole {
	switch role {
	case SpecialistInvestigator:
		return agentruntime.RoleInvestigator
	case SpecialistReviewer, SpecialistSecurity:
		return agentruntime.RoleReviewer
	case SpecialistTester:
		return agentruntime.RoleTester
	case SpecialistImplementer:
		return agentruntime.RoleImplementer
	default:
		return agentruntime.RoleSpecialist
	}
}

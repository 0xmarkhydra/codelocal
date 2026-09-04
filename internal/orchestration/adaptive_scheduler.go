package orchestration

import (
	"errors"
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/usage"
)

type TaskTier string

const (
	TaskTier0 TaskTier = "T0"
	TaskTier1 TaskTier = "T1"
	TaskTier2 TaskTier = "T2"
	TaskTier3 TaskTier = "T3"
	TaskTier4 TaskTier = "T4"
)

var ErrTeamBudgetDenied = errors.New("adaptive team budget denied")

type TaskShape struct {
	TaskKind         string `json:"taskKind,omitempty"`
	FilesEstimated   int    `json:"filesEstimated,omitempty"`
	Subsystems       int    `json:"subsystems,omitempty"`
	Architectural    bool   `json:"architectural,omitempty"`
	SecuritySensitive bool  `json:"securitySensitive,omitempty"`
	NeedsProductVerification bool `json:"needsProductVerification,omitempty"`
}

type TeamMemberPlan struct {
	ID              string           `json:"id"`
	Role            Specialist       `json:"role"`
	EngineProfile   string           `json:"engineProfile"`
	ReadOnly        bool             `json:"readOnly,omitempty"`
	TokenBudget     int64            `json:"tokenBudget"`
	DependsOn       []string         `json:"dependsOn,omitempty"`
	VerificationOnly bool            `json:"verificationOnly,omitempty"`
}

type TeamPlan struct {
	Tier       TaskTier        `json:"tier"`
	Members    []TeamMemberPlan `json:"members"`
	Parallelism int            `json:"parallelism"`
	Reason     string          `json:"reason"`
}

func ClassifyTaskTier(shape TaskShape) TaskTier {
	kind := strings.ToLower(strings.TrimSpace(shape.TaskKind))
	if shape.Architectural || shape.Subsystems >= 4 || shape.FilesEstimated >= 12 { return TaskTier4 }
	if shape.SecuritySensitive || shape.Subsystems >= 3 || shape.FilesEstimated >= 7 || strings.Contains(kind, "migration") { return TaskTier3 }
	if shape.Subsystems >= 2 || shape.FilesEstimated >= 3 || strings.Contains(kind, "refactor") { return TaskTier2 }
	if shape.FilesEstimated <= 1 && !shape.NeedsProductVerification && !strings.Contains(kind, "implement") { return TaskTier0 }
	return TaskTier1
}

// PlanAdaptiveTeam deliberately keeps T0/T1 single-agent. Parallel specialists
// appear only when task shape justifies their coordination/token overhead.
func PlanAdaptiveTeam(shape TaskShape, policies map[Specialist]SpecialistPolicy) TeamPlan {
	if policies == nil { policies = DefaultSpecialistPolicies() }
	tier := ClassifyTaskTier(shape)
	members := []TeamMemberPlan{}
	add := func(id string, role Specialist, deps ...string) {
		policy := policies[role]
		members = append(members, TeamMemberPlan{ID: id, Role: role, EngineProfile: policy.EngineProfile, ReadOnly: policy.ReadOnly, TokenBudget: policy.TokenBudget, DependsOn: deps})
	}
	switch tier {
	case TaskTier0:
		add("lead", SpecialistQuick)
	case TaskTier1:
		add("lead", RecommendSpecialist(shape.TaskKind, 2, shape.SecuritySensitive))
	case TaskTier2:
		add("implement", SpecialistImplementer)
		add("review", SpecialistReviewer, "implement")
	case TaskTier3:
		if shape.SecuritySensitive { add("security", SpecialistSecurity) } else { add("investigate", SpecialistInvestigator) }
		first := members[0].ID
		add("implement", SpecialistImplementer, first)
		add("test", SpecialistTester, "implement")
		add("review", SpecialistReviewer, "implement")
	case TaskTier4:
		add("investigate", SpecialistInvestigator)
		add("deep", SpecialistDeep, "investigate")
		add("implement", SpecialistImplementer, "investigate")
		add("test", SpecialistTester, "implement")
		add("review", SpecialistReviewer, "deep", "implement")
		if shape.SecuritySensitive { add("security", SpecialistSecurity, "implement") }
	}
	if shape.NeedsProductVerification {
		present := false
		for i := range members { if members[i].Role == SpecialistTester { members[i].VerificationOnly = true; present = true } }
		if !present && tier != TaskTier0 {
			policy := policies[SpecialistTester]
			members = append(members, TeamMemberPlan{ID: "product-verify", Role: SpecialistTester, EngineProfile: policy.EngineProfile, ReadOnly: true, TokenBudget: policy.TokenBudget, VerificationOnly: true})
		}
	}
	parallel := 1
	if tier == TaskTier3 { parallel = 2 }
	if tier == TaskTier4 { parallel = 3 }
	return TeamPlan{Tier: tier, Members: members, Parallelism: parallel, Reason: "bounded_adaptive_team"}
}

// ReserveTeamBudget performs admission before any child is dispatched. Failure
// rolls back reservations already made during this attempt.
func ReserveTeamBudget(ledger *usage.Ledger, taskID string, plan TeamPlan) ([]usage.Reservation, error) {
	if ledger == nil { return nil, ErrTeamBudgetDenied }
	reservations := []usage.Reservation{}
	for _, member := range plan.Members {
		id := "team:" + strings.TrimSpace(taskID) + ":" + member.ID
		estimate := usage.TaskUsage{TaskID: taskID, AgentID: member.ID, Tokens: usage.TokenBreakdown{Code: member.TokenBudget}, Estimated: true}
		reservation, _, err := ledger.Reserve(id, estimate)
		if err != nil {
			for _, prior := range reservations { _, _ = ledger.Release(prior.ID) }
			return nil, fmt.Errorf("%w: member=%s", ErrTeamBudgetDenied, member.ID)
		}
		reservations = append(reservations, reservation)
	}
	return reservations, nil
}

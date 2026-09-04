package orchestration

import (
	"errors"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/usage"
)

func TestAdaptiveTeamKeepsSimpleTasksSingleAgent(t *testing.T) {
	for _, shape := range []TaskShape{{TaskKind: "question", FilesEstimated: 1}, {TaskKind: "fix", FilesEstimated: 2}} {
		plan := PlanAdaptiveTeam(shape, nil)
		if len(plan.Members) != 1 || plan.Parallelism != 1 {
			t.Fatalf("simple task over-orchestrated: shape=%+v plan=%+v", shape, plan)
		}
	}
}

func TestAdaptiveTeamAddsBoundedRolesForComplexTask(t *testing.T) {
	plan := PlanAdaptiveTeam(TaskShape{TaskKind: "architecture migration", FilesEstimated: 14, Subsystems: 5, SecuritySensitive: true, NeedsProductVerification: true}, nil)
	if plan.Tier != TaskTier4 || len(plan.Members) < 5 || plan.Parallelism > 3 {
		t.Fatalf("unexpected complex team: %+v", plan)
	}
	roles := map[Specialist]bool{}
	for _, member := range plan.Members {
		roles[member.Role] = true
	}
	if !roles[SpecialistInvestigator] || !roles[SpecialistImplementer] || !roles[SpecialistReviewer] || !roles[SpecialistSecurity] || !roles[SpecialistTester] {
		t.Fatalf("missing bounded specialist roles: %+v", plan)
	}
}

func TestReserveTeamBudgetRollsBackOnDeniedMember(t *testing.T) {
	ledger := usage.NewLedger(usage.Budget{MaxTotalTokens: 30_000}, usage.TaskUsage{})
	plan := TeamPlan{Members: []TeamMemberPlan{{ID: "a", TokenBudget: 10_000}, {ID: "b", TokenBudget: 25_000}}}
	_, err := ReserveTeamBudget(ledger, "task", plan)
	if !errors.Is(err, ErrTeamBudgetDenied) {
		t.Fatalf("expected team budget denial, got %v", err)
	}
	if snapshot := ledger.Snapshot(); snapshot.OpenReservations != 0 || snapshot.Reserved.Tokens.Total() != 0 {
		t.Fatalf("failed admission leaked reservation: %+v", snapshot)
	}
}

func TestReserveTeamBudgetSucceedsBeforeDispatch(t *testing.T) {
	ledger := usage.NewLedger(usage.Budget{MaxTotalTokens: 100_000}, usage.TaskUsage{})
	plan := TeamPlan{Members: []TeamMemberPlan{{ID: "a", TokenBudget: 10_000}, {ID: "b", TokenBudget: 20_000}}}
	reservations, err := ReserveTeamBudget(ledger, "task", plan)
	if err != nil || len(reservations) != 2 {
		t.Fatalf("team admission failed: reservations=%+v err=%v", reservations, err)
	}
	if ledger.Snapshot().Reserved.Tokens.Total() != 30_000 {
		t.Fatalf("reservation total incorrect: %+v", ledger.Snapshot())
	}
}

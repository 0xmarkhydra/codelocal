package orchestration

import "testing"

func TestBuildPlanAutoSelectsUIUXForVagueVietnameseUITask(t *testing.T) {
	plan := BuildPlan(PlanInput{
		Task:         "Màn này nhìn xấu và khó chịu quá, làm đẹp hơn giúp tôi",
		Capabilities: Capabilities{Filesystem: true, LSP: true},
		Project: ProjectProfile{
			Languages:  []string{"TypeScript"},
			Frameworks: []string{"Next.js"},
		},
	})
	if len(plan.Skills.Selections) != 1 || plan.Skills.Selections[0].Skill.ID != "ui-ux-pro" {
		t.Fatalf("expected automatic ui-ux-pro selection, got %#v", plan.Skills.Selections)
	}
	if len(plan.Skills.Knowledge) == 0 || len(plan.Skills.Knowledge) > 4 {
		t.Fatalf("expected bounded skill knowledge, got %d", len(plan.Skills.Knowledge))
	}
}

func TestBuildPlanDoesNotUseUIUXForBackendTaskInNextProject(t *testing.T) {
	plan := BuildPlan(PlanInput{
		Task:         "fix Redis reconnect and exponential backoff",
		Capabilities: Capabilities{Filesystem: true, LSP: true, Shell: true},
		Project: ProjectProfile{
			Languages:  []string{"TypeScript"},
			Frameworks: []string{"Next.js"},
		},
	})
	if len(plan.Skills.Selections) != 0 {
		t.Fatalf("backend task must not activate UI skills, got %#v", plan.Skills.Selections)
	}
}

func TestBuildPlanSkipsSkillForTrivialCopyChange(t *testing.T) {
	plan := BuildPlan(PlanInput{
		Task:         "đổi chữ Login thành Sign in",
		Capabilities: Capabilities{Filesystem: true},
		Project:      ProjectProfile{Frameworks: []string{"React"}},
		TouchedFiles: []string{"web/login.tsx"},
	})
	if len(plan.Skills.Selections) != 0 {
		t.Fatalf("trivial copy change must not pay skill context cost, got %#v", plan.Skills.Selections)
	}
}

func TestBuildPlanKeepsBrainPrecedencePrinciple(t *testing.T) {
	plan := BuildPlan(PlanInput{Task: "redesign dashboard", Capabilities: Capabilities{Filesystem: true}, Project: ProjectProfile{Frameworks: []string{"React"}}})
	found := false
	for _, principle := range plan.Principles {
		if principle == "Project Brain and current repository evidence remain authoritative over reusable skill recommendations" {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("agent plan must preserve Project Brain precedence over reusable skill advice")
	}
}

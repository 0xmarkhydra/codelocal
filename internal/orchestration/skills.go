package orchestration

import skillintel "github.com/0xmarkhydra/codelocal/internal/skills"

// SkillPlan is projected inside the existing AgentPlan. Skills stay internal:
// users chat normally, while orchestration selects reusable expertise without
// expanding the public MCP tool surface.
type SkillPlan = skillintel.Plan

func buildSkillPlan(input PlanInput) SkillPlan {
	return buildSkillPlanWithEngine(input, nil, nil)
}

func buildSkillPlanWithEngine(input PlanInput, engine *skillintel.Engine, affinity map[string]float64) SkillPlan {
	if engine == nil {
		engine = skillintel.DefaultEngine()
	}
	stack := append(append([]string(nil), input.Project.Frameworks...), input.Project.Languages...)
	task := skillintel.ClassifyTask(skillintel.TaskEvidence{
		Query:        input.Task,
		Stack:        stack,
		TouchedFiles: input.TouchedFiles,
		RecentErrors: input.RecentErrors,
		Affinity:     affinity,
	})
	return engine.Plan(task)
}

// BuildPlanWithSkillEngine keeps routing, verification and quality semantics
// identical to BuildPlan while allowing Cloud/MCP callers to project a user's
// tenant Skill catalog and affinity. Local and legacy callers keep the built-in
// DefaultEngine path through BuildPlan.
func BuildPlanWithSkillEngine(input PlanInput, engine *skillintel.Engine, affinity map[string]float64) AgentPlan {
	plan := BuildPlan(input)
	plan.Skills = buildSkillPlanWithEngine(input, engine, affinity)
	return plan
}

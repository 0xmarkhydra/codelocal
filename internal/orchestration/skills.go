package orchestration

import skillintel "github.com/0xmarkhydra/codelocal/internal/skills"

// SkillPlan is projected inside the existing AgentPlan. Skills stay internal:
// users chat normally, while orchestration selects reusable expertise without
// expanding the public MCP tool surface.
type SkillPlan = skillintel.Plan

func buildSkillPlan(input PlanInput) SkillPlan {
	stack := append(append([]string(nil), input.Project.Frameworks...), input.Project.Languages...)
	task := skillintel.ClassifyTask(skillintel.TaskEvidence{
		Query:        input.Task,
		Stack:        stack,
		TouchedFiles: input.TouchedFiles,
		RecentErrors: input.RecentErrors,
	})
	return skillintel.DefaultEngine().Plan(task)
}

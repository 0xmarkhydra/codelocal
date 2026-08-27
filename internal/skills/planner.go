package skills

type Planner struct {
	router *Router
}

func NewPlanner(router *Router) *Planner {
	return &Planner{router: router}
}

func (p *Planner) Plan(task TaskContext) Plan {
	if p == nil || p.router == nil {
		return Plan{}
	}
	selections := p.router.Route(task)
	steps := make([]PlanStep, 0, len(selections))
	for index, selection := range selections {
		phase := "advise"
		switch selection.Skill.Kind {
		case KindWorkflow:
			phase = "workflow"
		case KindRuntime:
			phase = "execute"
		case KindHybrid:
			phase = "advise_then_execute"
		}
		steps = append(steps, PlanStep{
			Order:        index + 1,
			SkillID:      selection.Skill.ID,
			Phase:        phase,
			Reason:       selection.Reason,
			Capabilities: append([]Capability(nil), selection.Skill.Capabilities...),
		})
	}
	return Plan{Selections: selections, Steps: steps}
}

type Engine struct {
	registry *Registry
	router   *Router
	planner  *Planner
}

func NewEngine(registry *Registry) *Engine {
	if registry == nil {
		registry = DefaultRegistry()
	}
	router := NewRouter(registry)
	return &Engine{registry: registry, router: router, planner: NewPlanner(router)}
}

func DefaultEngine() *Engine {
	return NewEngine(DefaultRegistry())
}

func (e *Engine) Catalog() []Manifest {
	if e == nil || e.registry == nil {
		return nil
	}
	return e.registry.List()
}

func (e *Engine) Route(task TaskContext) []Selection {
	if e == nil || e.router == nil {
		return nil
	}
	return e.router.Route(task)
}

func (e *Engine) Plan(task TaskContext) Plan {
	if e == nil || e.planner == nil {
		return Plan{}
	}
	return e.planner.Plan(task)
}

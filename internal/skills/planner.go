package skills

import "sync"

type Planner struct {
	router    *Router
	knowledge KnowledgeStore
}

func NewPlanner(router *Router, knowledge KnowledgeStore) *Planner {
	return &Planner{router: router, knowledge: knowledge}
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
			SkillVersion: selection.Skill.Version,
			Phase:        phase,
			Reason:       selection.Reason,
			Capabilities: append([]Capability(nil), selection.Skill.Capabilities...),
		})
	}
	knowledge := []KnowledgeMatch(nil)
	if p.knowledge != nil {
		knowledge = p.knowledge.Search(task, selections, 4)
	}
	return Plan{Selections: selections, Steps: steps, Knowledge: knowledge}
}

type Engine struct {
	registry  *Registry
	router    *Router
	planner   *Planner
	knowledge KnowledgeStore
}

// NewEngine accepts an optional KnowledgeStore so Cloud/Desktop can provide a
// persistent indexed store while tests and the first built-in use memory.
func NewEngine(registry *Registry, stores ...KnowledgeStore) *Engine {
	if registry == nil {
		registry = DefaultRegistry()
	}
	knowledge := DefaultKnowledgeStore()
	if len(stores) > 0 && stores[0] != nil {
		knowledge = stores[0]
	}
	router := NewRouter(registry)
	return &Engine{registry: registry, router: router, planner: NewPlanner(router, knowledge), knowledge: knowledge}
}

var (
	defaultEngineOnce sync.Once
	defaultEngine     *Engine
)

func DefaultEngine() *Engine {
	defaultEngineOnce.Do(func() {
		defaultEngine = NewEngine(DefaultRegistry(), DefaultKnowledgeStore())
	})
	return defaultEngine
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

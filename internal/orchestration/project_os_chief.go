package orchestration

import (
	"encoding/json"
	"strings"
)

// ChatIntent classifies a dashboard chat request into the P0 Project OS behaviors.
// P0 does not require ML classification; deterministic rules + planner fallback is enough.
type ChatIntent string

const (
	ChatIntentAsk           ChatIntent = "ASK"
	ChatIntentDirectWork    ChatIntent = "DIRECT_WORK"
	ChatIntentGoalWork      ChatIntent = "GOAL_WORK"
	ChatIntentStatusControl ChatIntent = "STATUS_CONTROL"
)

func ClassifyProjectOSChatIntent(message string) ChatIntent {
	msg := strings.ToLower(strings.TrimSpace(message))
	if msg == "" {
		return ChatIntentAsk
	}
	for _, kw := range []string{"đến đâu", "tiến độ", "trạng thái", "status", "dừng", "ưu tiên", "hủy", "cancel", "stop", "progress"} {
		if strings.Contains(msg, kw) {
			return ChatIntentStatusControl
		}
	}
	// Broad autonomous triggers: new feature / project-scale work.
	goalTriggers := []string{"làm ", "xây", "xay dung", "xây dựng", "tạo mới", "làm mới", "onboarding", "payment mới", "v2", "mới cho"}
	for _, kw := range goalTriggers {
		if strings.Contains(msg, kw) && (strings.Contains(msg, "mới") || len([]rune(msg)) > 24) {
			return ChatIntentGoalWork
		}
	}
	// Explicit short coding commands stay on the direct path.
	if len([]rune(strings.TrimSpace(message))) <= 80 {
		for _, kw := range []string{"fix", "sửa", "đổi", "thêm", "xóa", "timeout", "lỗi", "bug"} {
			if strings.Contains(msg, kw) {
				return ChatIntentDirectWork
			}
		}
	}
	if strings.HasPrefix(msg, "làm") || strings.Contains(msg, "hãy làm") {
		return ChatIntentGoalWork
	}
	return ChatIntentAsk
}

// ChiefTaskSpec is the orchestration-level task proposal before cloud persistence.
// It mirrors cloud.ProjectTaskSpec without importing cloud (transport boundary).
type ChiefTaskSpec struct {
	Title                string   `json:"title"`
	Description          string   `json:"description"`
	TaskKind             string   `json:"taskKind"`
	RequiredCapabilities []string `json:"requiredCapabilities"`
	AssignedAgentRole    string   `json:"assignedAgentRole"`
	Priority             int      `json:"priority"`
	DependsOn            []string `json:"dependsOn"`
}

type ChiefDecompositionInput struct {
	PlanSummary        string
	AcceptanceCriteria []string
	FlowSteps          []string
	MaxTasks           int
}

func normalizeChiefTitle(v string) string {
	v = strings.Join(strings.Fields(strings.TrimSpace(v)), " ")
	if r := []rune(v); len(r) > 200 {
		v = string(r[:200])
	}
	return v
}

// DecomposeApprovedPlan is the P0 Chief planner: deterministic, bounded, and
// dependency-safe. It builds implement -> review -> testing chains from the
// approved acceptance criteria, then validates dedupe and cycles.
func DecomposeApprovedPlan(in ChiefDecompositionInput) ([]ChiefTaskSpec, error) {
	maxTasks := in.MaxTasks
	if maxTasks <= 0 {
		maxTasks = 8
	}
	if maxTasks > 12 {
		maxTasks = 12
	}
	criteria := []string{}
	for _, c := range in.AcceptanceCriteria {
		c = strings.TrimSpace(c)
		if c != "" {
			criteria = append(criteria, c)
		}
	}
	if len(criteria) == 0 {
		criteria = []string{strings.TrimSpace(in.PlanSummary)}
	}
	// Bound criteria to keep the graph small and reviewable.
	if len(criteria) > 3 {
		criteria = criteria[:3]
	}
	specs := []ChiefTaskSpec{}
	for idx, criterion := range criteria {
		base := normalizeChiefTitle(criterion)
		if base == "" {
			base = "Hoàn thiện hạng mục"
		}
		implTitle := normalizeChiefTitle("Triển khai: " + base)
		verifyTitle := normalizeChiefTitle("Kiểm thử: " + base)
		specs = append(specs, ChiefTaskSpec{
			Title: implTitle, Description: "Thực hiện: " + base + ". Kế thừa ngữ cảnh Project Brain và plan đã duyệt.",
			TaskKind: "coding", RequiredCapabilities: []string{"code", "test"},
			AssignedAgentRole: "implementer", Priority: 100 - idx*10,
		})
		specs = append(specs, ChiefTaskSpec{
			Title: verifyTitle, Description: "Review + kiểm thử độc lập cho: " + base,
			TaskKind: "testing", RequiredCapabilities: []string{"test", "review"},
			AssignedAgentRole: "tester", Priority: 90 - idx*10, DependsOn: []string{implTitle},
		})
		if len(specs) >= maxTasks {
			break
		}
	}
	if err := ValidateChiefTaskGraph(specs); err != nil {
		return nil, err
	}
	return specs, nil
}

// ValidateChiefTaskGraph rejects duplicates and dependency cycles before persistence.
func ValidateChiefTaskGraph(specs []ChiefTaskSpec) error {
	if len(specs) == 0 || len(specs) > 25 {
		return errChiefTaskGraph("task graph must contain 1-25 tasks")
	}
	seen := map[string]bool{}
	for _, s := range specs {
		if normalizeChiefTitle(s.Title) == "" {
			return errChiefTaskGraph("task title is required")
		}
		lower := strings.ToLower(normalizeChiefTitle(s.Title))
		if seen[lower] {
			return errChiefTaskGraph("duplicate task title: " + s.Title)
		}
		seen[lower] = true
		switch s.TaskKind {
		case "coding", "review", "testing", "docs", "research", "ops":
		default:
			return errChiefTaskGraph("invalid task kind: " + s.TaskKind)
		}
	}
	ids := make([]string, 0, len(specs))
	byTitle := map[string]string{}
	for i, s := range specs {
		id := string(rune('A' + i))
		ids = append(ids, id)
		byTitle[strings.ToLower(normalizeChiefTitle(s.Title))] = id
	}
	edges := map[string][]string{}
	for i, s := range specs {
		deps := []string{}
		for _, d := range s.DependsOn {
			d = strings.TrimSpace(d)
			if d == "" {
				continue
			}
			resolved, ok := byTitle[strings.ToLower(normalizeChiefTitle(d))]
			if !ok {
				return errChiefTaskGraph("unknown task dependency: " + d)
			}
			if resolved == ids[i] {
				return errChiefTaskGraph("task cannot depend on itself")
			}
			deps = append(deps, resolved)
		}
		edges[ids[i]] = deps
	}
	state := map[string]int{}
	var visit func(n string) bool
	visit = func(n string) bool {
		if state[n] == 1 {
			return true
		}
		if state[n] == 2 {
			return false
		}
		state[n] = 1
		for _, dep := range edges[n] {
			if visit(dep) {
				return true
			}
		}
		state[n] = 2
		return false
	}
	for _, id := range ids {
		if state[id] == 0 && visit(id) {
			return errChiefTaskGraph("task dependency cycle detected")
		}
	}
	return nil
}

type chiefGraphError string

func errChiefTaskGraph(msg string) error { return chiefGraphError(msg) }

func (e chiefGraphError) Error() string { return string(e) }

// ProjectOSPlanCard is the minimal chat response contract for plan proposals.
type ProjectOSPlanCard struct {
	GoalID             string   `json:"goalId"`
	PlanID             string   `json:"planId"`
	GoalTitle          string   `json:"goalTitle"`
	Summary            string   `json:"summary"`
	AcceptanceCriteria []string `json:"acceptanceCriteria"`
	Status             string   `json:"status"`
	RequiresApproval   bool     `json:"requiresApproval"`
}

func ParseAcceptanceCriteria(raw json.RawMessage) []string {
	var arr []string
	if err := json.Unmarshal(raw, &arr); err == nil {
		out := []string{}
		for _, v := range arr {
			if strings.TrimSpace(v) != "" {
				out = append(out, strings.TrimSpace(v))
			}
		}
		return out
	}
	var single string
	if err := json.Unmarshal(raw, &single); err == nil && strings.TrimSpace(single) != "" {
		return []string{strings.TrimSpace(single)}
	}
	return nil
}

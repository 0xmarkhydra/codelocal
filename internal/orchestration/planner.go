package orchestration

import (
	"path/filepath"
	"sort"
	"strings"
)

type TaskKind string

const (
	TaskKindCodeChange TaskKind = "code_change"
	TaskKindDebug      TaskKind = "debug"
	TaskKindReview     TaskKind = "review"
	TaskKindRelease    TaskKind = "release"
	TaskKindBrowser    TaskKind = "browser"
	TaskKindDesktop    TaskKind = "desktop"
	TaskKindShell      TaskKind = "shell"
	TaskKindGeneral    TaskKind = "general"
)

type ProjectProfile struct {
	Languages         []string
	Frameworks        []string
	BuildCommands     []string
	TestCommands      []string
	TypecheckCommands []string
	LintCommands      []string
}

type PlanInput struct {
	Task             string
	Capabilities     Capabilities
	Project          ProjectProfile
	TouchedFiles     []string
	LastAction       string
	RecentErrors     []string
	RecentChecks     []string
	AgentPhase       string
	Iteration        int
	RecoveryAttempts int
	PassedChecks     []string
	VerificationSeen bool
	DiagnosticDelta  int
	DiffObserved     bool
}

type PlanStep struct {
	ID     string `json:"id"`
	Phase  string `json:"phase"`
	Lane   Lane   `json:"lane"`
	Action string `json:"action"`
	Tool   string `json:"tool,omitempty"`
	Status string `json:"status"`
	Why    string `json:"why,omitempty"`
}

type VerificationCheck struct {
	Key      string `json:"key"`
	Command  string `json:"command"`
	Scope    string `json:"scope"`
	Required bool   `json:"required"`
	Reason   string `json:"reason"`
}

type VerificationPlan struct {
	Mode   string              `json:"mode"`
	Checks []VerificationCheck `json:"checks"`
}

type QualityEvaluation struct {
	Score         int      `json:"score"`
	Status        string   `json:"status"`
	MissingChecks []string `json:"missingChecks,omitempty"`
	Signals       []string `json:"signals,omitempty"`
}

type AgentPlan struct {
	Version           int               `json:"version"`
	TaskKind          TaskKind          `json:"taskKind"`
	Route             Decision          `json:"route"`
	Phase             string            `json:"phase"`
	NextAction        string            `json:"nextAction"`
	Steps             []PlanStep        `json:"steps"`
	Verification      VerificationPlan  `json:"verification"`
	Quality           QualityEvaluation `json:"quality"`
	MaxRepairAttempts int               `json:"maxRepairAttempts"`
	Principles        []string          `json:"principles"`
}

func taskKind(task string, input PlanInput) TaskKind {
	text := strings.ToLower(strings.TrimSpace(task))
	debug := containsAny(text, "bug", "fix", "repair", "broken", "error", "regression", "failing", "crash", "sửa lỗi", "lỗi", "hỏng")
	repairIntent := containsAny(text, "fix", "repair", "sửa lỗi", "khắc phục", "vá lỗi")
	review := containsAny(text, "review", "audit", "inspect", "đánh giá", "rà soát", "kiểm tra code")
	release := containsAny(text, "release", "publish", "deploy", "merge", "npm", "phát hành", "triển khai", "đẩy lên")
	browser := containsAny(text, "browser", "website", "chrome", "safari", "web page", "trình duyệt", "trang web")
	desktop := containsAny(text, "desktop", "window", "application", "finder", "máy tính", "cửa sổ", "ứng dụng")
	shell := containsAny(text, "terminal", "command", "cli", "script", "dòng lệnh", "chạy lệnh")

	switch {
	case review && !repairIntent && (input.Capabilities.Filesystem || input.Capabilities.LSP):
		return TaskKindReview
	case debug && (input.Capabilities.Filesystem || input.Capabilities.LSP):
		return TaskKindDebug
	case release && input.Capabilities.Shell:
		return TaskKindRelease
	case browser && input.Capabilities.Browser:
		return TaskKindBrowser
	case desktop && input.Capabilities.Computer:
		return TaskKindDesktop
	case shell && input.Capabilities.Shell && !input.Capabilities.Filesystem:
		return TaskKindShell
	case len(input.TouchedFiles) > 0 || len(input.Project.Languages) > 0 || input.Capabilities.Filesystem || input.Capabilities.LSP:
		return TaskKindCodeChange
	case input.Capabilities.Shell:
		return TaskKindShell
	default:
		return TaskKindGeneral
	}
}

func CheckKey(command string) string {
	text := strings.ToLower(strings.TrimSpace(command))
	switch {
	case text == "":
		return ""
	case strings.Contains(text, "git diff --check"):
		return "diff-check"
	case strings.Contains(text, "go test"), strings.Contains(text, "cargo test"), strings.Contains(text, "flutter test"), strings.Contains(text, "pytest"), strings.Contains(text, "npm test"), strings.Contains(text, "npm run test"), strings.Contains(text, "pnpm test"), strings.Contains(text, "yarn test"), strings.Contains(text, "bun test"), strings.Contains(text, "node --test"):
		return "test"
	case strings.Contains(text, "go vet"), strings.Contains(text, "eslint"), strings.Contains(text, "biome check"), strings.Contains(text, "npm run lint"), strings.Contains(text, "pnpm lint"), strings.Contains(text, "yarn lint"):
		return "lint"
	case strings.Contains(text, "typecheck"), strings.Contains(text, "tsc --noemit"), strings.Contains(text, "mypy"), strings.Contains(text, "pyright"), strings.Contains(text, "flutter analyze"):
		return "typecheck"
	case strings.Contains(text, "go build"), strings.Contains(text, "cargo build"), strings.Contains(text, "npm run build"), strings.Contains(text, "pnpm build"), strings.Contains(text, "yarn build"), strings.Contains(text, "bun run build"):
		return "build"
	default:
		return ""
	}
}

func hasExtension(paths []string, extensions ...string) bool {
	set := map[string]struct{}{}
	for _, ext := range extensions {
		set[strings.ToLower(ext)] = struct{}{}
	}
	for _, path := range paths {
		if _, ok := set[strings.ToLower(filepath.Ext(path))]; ok {
			return true
		}
	}
	return false
}

func onlyDocs(paths []string) bool {
	if len(paths) == 0 {
		return false
	}
	for _, path := range paths {
		ext := strings.ToLower(filepath.Ext(path))
		if ext != ".md" && ext != ".txt" && ext != ".rst" && ext != ".adoc" {
			return false
		}
	}
	return true
}

func manifestTouched(paths []string) bool {
	for _, path := range paths {
		base := strings.ToLower(filepath.Base(path))
		switch base {
		case "package.json", "package-lock.json", "pnpm-lock.yaml", "yarn.lock", "go.mod", "go.sum", "cargo.toml", "cargo.lock", "pyproject.toml", "requirements.txt", "pubspec.yaml":
			return true
		}
	}
	return false
}

func appendCheck(checks []VerificationCheck, seen map[string]struct{}, command, scope string, required bool, reason string) []VerificationCheck {
	command = strings.TrimSpace(command)
	if command == "" {
		return checks
	}
	identity := strings.ToLower(command)
	if _, ok := seen[identity]; ok {
		return checks
	}
	seen[identity] = struct{}{}
	key := CheckKey(command)
	if key == "" {
		key = "project-check"
	}
	return append(checks, VerificationCheck{Key: key, Command: command, Scope: scope, Required: required, Reason: reason})
}

func firstCommand(values []string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func goTestCommand(paths []string) string {
	if len(paths) == 0 {
		return "go test ./..."
	}
	dirs := map[string]struct{}{}
	for _, path := range paths {
		if strings.ToLower(filepath.Ext(path)) != ".go" {
			continue
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if dir == "." || dir == "" {
			return "go test ./..."
		}
		dirs[dir] = struct{}{}
	}
	if len(dirs) == 0 || len(dirs) > 3 {
		return "go test ./..."
	}
	items := make([]string, 0, len(dirs))
	for dir := range dirs {
		items = append(items, "./"+strings.TrimPrefix(dir, "./"))
	}
	sort.Strings(items)
	return "go test " + strings.Join(items, " ")
}

func BuildVerificationPlan(input PlanInput) VerificationPlan {
	checks := []VerificationCheck{}
	seen := map[string]struct{}{}
	if onlyDocs(input.TouchedFiles) {
		checks = appendCheck(checks, seen, "git diff --check", "changed files", true, "catch whitespace errors without running unrelated suites")
		return VerificationPlan{Mode: "docs-only", Checks: checks}
	}

	goChanged := hasExtension(input.TouchedFiles, ".go")
	tsChanged := hasExtension(input.TouchedFiles, ".ts", ".tsx", ".js", ".jsx", ".mjs", ".cjs")
	wide := manifestTouched(input.TouchedFiles)

	checks = appendCheck(checks, seen, "git diff --check", "changed files", true, "cheap structural sanity check before expensive verification")

	if goChanged {
		command := goTestCommand(input.TouchedFiles)
		if wide {
			command = "go test ./..."
		}
		checks = appendCheck(checks, seen, command, "affected Go packages", true, "run the smallest causally relevant Go test scope first")
		if len(input.Project.LintCommands) > 0 {
			checks = appendCheck(checks, seen, firstCommand(input.Project.LintCommands), "Go/project lint", false, "reuse the project's configured lint command")
		} else {
			checks = appendCheck(checks, seen, "go vet ./...", "Go workspace", false, "catch static issues not covered by package tests")
		}
	}

	if tsChanged {
		if command := firstCommand(input.Project.TypecheckCommands); command != "" {
			checks = appendCheck(checks, seen, command, "TypeScript workspace", true, "type errors are a high-signal regression check for changed TS/JS code")
		}
		if command := firstCommand(input.Project.TestCommands); command != "" {
			checks = appendCheck(checks, seen, command, "project tests", true, "run the configured test suite after static checks")
		}
	}

	if wide {
		if command := firstCommand(input.Project.BuildCommands); command != "" {
			checks = appendCheck(checks, seen, command, "workspace build", true, "manifest or dependency changes can affect the whole build graph")
		}
		if command := firstCommand(input.Project.TestCommands); command != "" {
			checks = appendCheck(checks, seen, command, "workspace tests", true, "dependency changes require a broad regression pass")
		}
	}

	if len(checks) == 1 {
		for _, group := range [][]string{input.Project.TypecheckCommands, input.Project.TestCommands, input.Project.BuildCommands, input.Project.LintCommands} {
			if command := firstCommand(group); command != "" {
				checks = appendCheck(checks, seen, command, "project", true, "use the project's own verification command")
				break
			}
		}
	}
	if len(checks) > 6 {
		checks = checks[:6]
	}
	mode := "targeted"
	if wide || len(input.TouchedFiles) == 0 {
		mode = "project-aware"
	}
	return VerificationPlan{Mode: mode, Checks: checks}
}

func EvaluateQuality(input PlanInput, verification VerificationPlan) QualityEvaluation {
	score := 100
	signals := []string{}
	missing := []string{}
	passed := map[string]struct{}{}
	for _, key := range input.PassedChecks {
		if key = strings.TrimSpace(key); key != "" {
			passed[key] = struct{}{}
		}
	}

	if len(input.RecentErrors) > 0 {
		score -= 35
		signals = append(signals, "unresolved recent failure")
	}
	if !input.VerificationSeen {
		score -= 20
		signals = append(signals, "verify.changes has not produced fresh evidence")
	} else if input.DiagnosticDelta > 0 {
		score -= 35
		signals = append(signals, "diagnostic regression detected")
	} else {
		signals = append(signals, "no diagnostic regression detected")
	}
	if len(input.TouchedFiles) > 0 && !input.DiffObserved {
		score -= 10
		signals = append(signals, "changed-file diff has not been observed")
	}

	requiredKeys := map[string]struct{}{}
	for _, check := range verification.Checks {
		if !check.Required || check.Key == "project-check" {
			continue
		}
		requiredKeys[check.Key] = struct{}{}
	}
	for key := range requiredKeys {
		if _, ok := passed[key]; !ok {
			missing = append(missing, key)
			score -= 12
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		signals = append(signals, "required verification checks are still pending")
	}
	if score < 0 {
		score = 0
	}
	status := "ready"
	switch {
	case len(input.RecentErrors) > 0 || input.DiagnosticDelta > 0:
		status = "blocked"
	case !input.VerificationSeen || len(missing) > 0:
		status = "verifying"
	case score < 85:
		status = "review"
	}
	return QualityEvaluation{Score: score, Status: status, MissingChecks: missing, Signals: signals}
}

func currentPhase(input PlanInput) string {
	if len(input.RecentErrors) > 0 {
		return "recover"
	}
	if phase := strings.TrimSpace(input.AgentPhase); phase != "" {
		return phase
	}
	action := strings.ToLower(strings.TrimSpace(input.LastAction))
	switch {
	case strings.HasPrefix(action, "edit."):
		return "verify"
	case strings.HasPrefix(action, "verify."), strings.HasPrefix(action, "terminal."):
		return "verify"
	case action != "":
		return "inspect"
	default:
		return "plan"
	}
}

func nextActionFor(phase string, kind TaskKind) string {
	if phase == "recover" {
		return "inspect recovery hint and gather fresh diagnostics before retry"
	}
	switch phase {
	case "plan":
		return "context: inspect ranked semantic context"
	case "inspect":
		if kind == TaskKindReview {
			return "verify: collect diagnostics and targeted checks"
		}
		return "edit: apply the smallest causally justified change"
	case "verify":
		return "verify: run missing required checks and re-evaluate quality"
	case "finalize":
		return "finalize result; perform Git/release action only when requested"
	default:
		return "context: refresh task context"
	}
}

func evidenceRoute(input PlanInput, kind TaskKind) Decision {
	scores := map[Lane]int{}
	evidence := []string{}
	available := func(lane Lane) bool {
		switch lane {
		case LaneCode:
			return input.Capabilities.Filesystem || input.Capabilities.LSP
		case LaneShell:
			return input.Capabilities.Shell
		case LaneBrowser:
			return input.Capabilities.Browser
		case LaneComputer:
			return input.Capabilities.Computer
		default:
			return false
		}
	}
	add := func(lane Lane, score int, reason string) {
		if !available(lane) || score <= 0 {
			return
		}
		scores[lane] += score
		if reason != "" {
			evidence = append(evidence, reason)
		}
	}

	// Capability availability is only a weak prior. Concrete task/repository
	// evidence below should dominate generic wording in the user prompt.
	add(LaneCode, 15, "structured repository surface available")
	add(LaneShell, 8, "guarded shell surface available")
	add(LaneBrowser, 6, "structured browser surface available")
	add(LaneComputer, 2, "native desktop surface available as last-resort UI lane")

	if len(input.TouchedFiles) > 0 {
		add(LaneCode, 85, "task already has changed repository files")
		if input.Capabilities.Shell {
			add(LaneShell, 18, "changed code may need local verification commands")
		}
	}
	if len(input.Project.Languages) > 0 {
		add(LaneCode, 35, "project language metadata confirms a code workspace")
	}
	if len(input.Project.TestCommands)+len(input.Project.TypecheckCommands)+len(input.Project.BuildCommands) > 0 {
		add(LaneShell, 12, "project advertises executable verification commands")
	}

	last := strings.ToLower(strings.TrimSpace(input.LastAction))
	switch {
	case strings.HasPrefix(last, "edit."), strings.HasPrefix(last, "lsp."), strings.HasPrefix(last, "read."), strings.HasPrefix(last, "search."), strings.HasPrefix(last, "verify."), strings.HasPrefix(last, "context."):
		add(LaneCode, 45, "current task history is on the structured code surface")
	case strings.HasPrefix(last, "terminal."):
		add(LaneShell, 42, "current task history is on the shell surface")
	case strings.HasPrefix(last, "browser."):
		add(LaneBrowser, 55, "current task history is on the browser surface")
	case strings.HasPrefix(last, "computer."):
		add(LaneComputer, 55, "current task history is on the native desktop surface")
	}

	switch kind {
	case TaskKindDebug:
		add(LaneCode, 80, "debug task needs definitions, diagnostics and causal code relationships")
		add(LaneShell, 30, "debug task needs reproduction or verification after diagnosis")
	case TaskKindCodeChange:
		add(LaneCode, 75, "code-change task should use structured edits and semantic context")
		add(LaneShell, 22, "code-change task needs bounded verification")
	case TaskKindReview:
		add(LaneCode, 85, "review task is evidence-first and should avoid unnecessary mutation")
	case TaskKindRelease:
		add(LaneShell, 75, "release task needs deterministic build/package commands")
		add(LaneCode, 35, "release should inspect repository/version state before side effects")
	case TaskKindBrowser:
		add(LaneBrowser, 95, "task explicitly maps to structured browser automation")
		add(LaneComputer, 20, "desktop UI is a fallback only when browser structure is unavailable")
	case TaskKindDesktop:
		add(LaneComputer, 95, "task requires a native desktop application")
	case TaskKindShell:
		add(LaneShell, 90, "task maps directly to a guarded local command")
	default:
		if len(input.Project.Languages) > 0 {
			add(LaneCode, 35, "ambiguous task stays on repository evidence instead of keyword guessing")
		}
	}

	if input.AgentPhase == "verify" {
		add(LaneCode, 30, "verification phase needs fresh diagnostics/diff evidence")
		add(LaneShell, 25, "verification phase may run required project checks")
	}
	if input.AgentPhase == "recover" || len(input.RecentErrors) > 0 {
		if strings.HasPrefix(last, "browser.") {
			add(LaneBrowser, 25, "recovery should first refresh the failing browser surface")
		} else if strings.HasPrefix(last, "computer.") {
			add(LaneComputer, 25, "recovery should first refresh the failing desktop surface")
		} else {
			add(LaneCode, 35, "recovery should inspect diagnostics and causal code evidence before retry")
		}
	}

	type rankedLane struct {
		lane  Lane
		score int
	}
	ranked := []rankedLane{}
	for _, lane := range []Lane{LaneCode, LaneShell, LaneBrowser, LaneComputer} {
		if available(lane) {
			ranked = append(ranked, rankedLane{lane: lane, score: scores[lane]})
		}
	}
	sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].score > ranked[j].score })
	if len(ranked) == 0 {
		return Decision{Primary: LaneNone, Reason: "no supported execution surface is currently available", Scores: scores}
	}
	fallbacks := []Lane{}
	for _, candidate := range ranked[1:] {
		if candidate.score >= 15 {
			fallbacks = append(fallbacks, candidate.lane)
		}
	}
	confidence := 0.55
	if len(ranked) == 1 {
		confidence = 0.95
	} else {
		delta := ranked[0].score - ranked[1].score
		confidence += float64(delta) / 200.0
		if confidence > 0.98 {
			confidence = 0.98
		}
	}
	if len(evidence) > 12 {
		evidence = evidence[:12]
	}
	return Decision{
		Primary: ranked[0].lane, Fallbacks: fallbacks,
		Reason:     "evidence-weighted routing prefers the highest-confidence structured capability",
		Confidence: confidence, Evidence: evidence, Scores: scores,
	}
}

func planStep(id, phase string, lane Lane, action, tool, why string) PlanStep {
	return PlanStep{ID: id, Phase: phase, Lane: lane, Action: action, Tool: tool, Status: "pending", Why: why}
}

func decomposeTask(kind TaskKind, route Decision) []PlanStep {
	primary := route.Primary
	if primary == LaneNone {
		primary = LaneCode
	}
	switch kind {
	case TaskKindDebug:
		return []PlanStep{
			planStep("reproduce", "inspect", primary, "reproduce or localize the failure from diagnostics, tests, logs and exact call relationships", "context/lsp/read/terminal", "do not patch before the causal failure is grounded"),
			planStep("diagnose", "inspect", LaneCode, "trace definitions, references, callers and recent diff to form one causal hypothesis", "lsp/read/git", "repository relationships outrank keyword matching"),
			planStep("repair", "act", LaneCode, "apply the smallest hash-safe change that addresses the causal hypothesis", "edit", "minimize unrelated mutation"),
			planStep("verify", "verify", LaneCode, "compare diagnostics/diff and run the smallest required checks", "verify/terminal", "successful editing is not completion evidence"),
			planStep("recover", "recover", primary, "classify failure, refresh stale evidence and retry only within the bounded recovery budget", "context/lsp/verify", "avoid blind repeated fixes"),
			planStep("quality", "finalize", LaneCode, "finalize only when diagnostic regression is zero and all required checks have passed", "context", "quality gate decides readiness"),
		}
	case TaskKindReview:
		return []PlanStep{
			planStep("scope", "inspect", LaneCode, "identify changed/high-risk files, public contracts and dependency edges", "context/git/lsp", "review starts from impact rather than broad scanning"),
			planStep("analyze", "inspect", LaneCode, "inspect diagnostics, references, callers, error paths and tests without mutating code", "lsp/read/verify", "separate findings from implementation"),
			planStep("validate", "verify", LaneCode, "run only checks needed to confirm material findings", "verify/terminal", "avoid expensive suites with no causal value"),
			planStep("quality", "finalize", LaneCode, "rank findings by evidence and unresolved risk", "context", "review quality is evidence coverage, not number of comments"),
		}
	case TaskKindRelease:
		return []PlanStep{
			planStep("release-state", "inspect", LaneCode, "inspect branch, diff, version state and release scripts", "context/git/read", "release side effects must be grounded in repository state"),
			planStep("quality-gate", "verify", LaneCode, "require diagnostics and project-aware checks before packaging", "verify/terminal", "never publish an unverified tree"),
			planStep("package", "act", LaneShell, "run the repository-defined build/package path after approval", "terminal", "reuse project release scripts rather than inventing commands"),
			planStep("release", "finalize", LaneShell, "commit/push/publish only when explicitly requested and approved", "git/terminal", "external side effects remain approval-bound"),
		}
	case TaskKindBrowser:
		return []PlanStep{
			planStep("observe", "inspect", LaneBrowser, "capture structured browser state and identify semantic targets", "browser", "DOM/accessibility state is safer than coordinates"),
			planStep("act", "act", LaneBrowser, "perform the smallest semantic browser action", "browser", "keep action scoped to the intended page"),
			planStep("verify", "verify", LaneBrowser, "observe fresh page state and verify the intended effect", "browser", "UI success requires post-action evidence"),
			planStep("recover", "recover", LaneBrowser, "refresh stale DOM state and retry within the bounded budget", "browser", "never blindly repeat a stale target"),
		}
	case TaskKindDesktop:
		return []PlanStep{
			planStep("observe", "inspect", LaneComputer, "inspect the scoped accessibility/vision scene", "computer", "prefer semantic background control"),
			planStep("act", "act", LaneComputer, "perform semantic background actions before physical input fallback", "computer", "avoid taking over the user's cursor"),
			planStep("verify", "verify", LaneComputer, "reobserve the same window and verify the intended state transition", "computer", "desktop actions need fresh window-scoped evidence"),
			planStep("recover", "recover", LaneComputer, "refresh stale window identity and request fresh approval before physical fallback", "computer", "recovery must preserve user control and privacy"),
		}
	default:
		return []PlanStep{
			planStep("inspect", "inspect", primary, "retrieve the smallest high-confidence context packet and exact relationships", "context/lsp/read", "ground the task before acting"),
			planStep("act", "act", primary, "perform the smallest safe action on the highest-confidence capability", "edit/browser/computer/terminal", "structured actions precede raw fallbacks"),
			planStep("verify", "verify", LaneCode, "compare fresh evidence and run context-aware required checks", "verify/terminal", "completion requires evidence"),
			planStep("recover", "recover", primary, "classify failure and retry only after refreshing causal evidence", "context/verify", "bounded recovery prevents loops"),
			planStep("quality", "finalize", LaneCode, "gate completion on required checks and unresolved regressions", "context", "result quality is explicit"),
		}
	}
}

func BuildPlan(input PlanInput) AgentPlan {
	kind := taskKind(input.Task, input)
	route := evidenceRoute(input, kind)
	phase := currentPhase(input)
	verification := BuildVerificationPlan(input)
	quality := EvaluateQuality(input, verification)
	if phase == "verify" && quality.Status == "ready" {
		phase = "finalize"
	}
	next := nextActionFor(phase, kind)

	steps := decomposeTask(kind, route)
	activePhase := phase
	if activePhase == "plan" {
		activePhase = "inspect"
	}
	for i := range steps {
		steps[i].Status = "pending"
		if steps[i].Phase == activePhase {
			steps[i].Status = "current"
		}
	}

	return AgentPlan{
		Version:           1,
		TaskKind:          kind,
		Route:             route,
		Phase:             phase,
		NextAction:        next,
		Steps:             steps,
		Verification:      verification,
		Quality:           quality,
		MaxRepairAttempts: 2,
		Principles: []string{
			"capabilities and repository evidence outrank generic keyword hints",
			"act on the cheapest structured surface, then observe fresh evidence",
			"never mark work ready while required checks or diagnostic regressions remain",
			"recovery is bounded and permission failures never auto-retry",
		},
	}
}

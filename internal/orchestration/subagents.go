package orchestration

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var ErrInvalidSubagent = errors.New("invalid subagent definition")

const (
	SubagentSourceBuiltin = "builtin"
	SubagentSourceGlobal  = "global"
	SubagentSourceProject = "project"

	MaxSubagentPromptBytes = 8 * 1024

	SubagentModePrimary  = "primary"
	SubagentModeSubagent = "subagent"
	SubagentModeAll      = "all"
)

// SubagentDefinition is the declarative bounded-delegation unit.
// Specialist (code) == builtin SubagentDefinition; custom markdown files
// overlay builtins via discovery: project > global > builtin.
type SubagentDefinition struct {
	Name          string            `json:"name"`
	Description   string            `json:"description"`
	Mode          string            `json:"mode"`
	Role          Specialist        `json:"role"`
	EngineProfile string            `json:"engineProfile"`
	Temperature   float64           `json:"temperature,omitempty"`
	HasTemp       bool              `json:"hasTemperature,omitempty"`
	Tools         []string          `json:"tools,omitempty"`
	Permission    map[string]string `json:"permission,omitempty"`
	TokenBudget   int64             `json:"tokenBudget"`
	MaxDepth      int               `json:"maxDepth"`
	MaxConcurrent int               `json:"maxConcurrent"`
	ReadOnly      bool              `json:"readOnly,omitempty"`
	WriteScopes   []string          `json:"writeScopes,omitempty"`
	Prompt        string            `json:"prompt"`
	Source        string            `json:"source,omitempty"`
	Path          string            `json:"path,omitempty"`
}

// AgentReport is the bounded child -> lead result contract (S2 ready).
// Payload must stay small (~4KB); no raw transcript forwarding.
type AgentReport struct {
	BriefID       string   `json:"briefId"`
	Status        string   `json:"status"`
	Summary       string   `json:"summary,omitempty"`
	FilesTouched  []string `json:"filesTouched,omitempty"`
	EvidenceRefs  []string `json:"evidenceRefs,omitempty"`
	Verification  string   `json:"verification,omitempty"`
	Followups     []string `json:"followups,omitempty"`
}

var (
	subagentNameRe       = regexp.MustCompile(`^[a-z][a-z0-9-]{0,63}$`)
	subagentToolRe       = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_.-]*\*?$`)
	subagentWriteScopeRe = regexp.MustCompile(`^[A-Za-z0-9_./*-]+$`)
	secretPatterns       = []*regexp.Regexp{
		regexp.MustCompile(`(?i)(api[_-]?key|api[_-]?secret|secret[_-]?key|access[_-]?token|auth[_-]?token|password|passwd)\s*[:=]\s*\S{4,}`),
		regexp.MustCompile(`sk-[A-Za-z0-9]{10,}`),
		regexp.MustCompile(`ghp_[A-Za-z0-9]{10,}`),
		regexp.MustCompile(`gho_[A-Za-z0-9]{10,}`),
		regexp.MustCompile(`xox[bap]-[A-Za-z0-9-]{10,}`),
		regexp.MustCompile(`AKIA[0-9A-Z]{16}`),
		regexp.MustCompile(`-----BEGIN (RSA )?PRIVATE KEY-----`),
	}
)

func builtinSubagentDescriptions() map[Specialist]string {
	return map[Specialist]string{
		SpecialistQuick:        "Trả lời nhanh / tra cứu hẹp. Chỉ đọc, không sửa.",
		SpecialistInvestigator: "Khảo sát codebase, chẩn đoán nguyên nhân. Chỉ đọc, không sửa.",
		SpecialistImplementer:  "Implement / fix / refactor có ghi file trong scope.",
		SpecialistTester:       "Chạy kiểm thử, xác minh sản phẩm. Chỉ đọc source.",
		SpecialistReviewer:     "Review diff, quality gate độc lập. Chỉ đọc.",
		SpecialistSecurity:     "Review bảo mật, phát hiện secret / injection / authz. Chỉ đọc.",
		SpecialistDeep:         "Task kiến trúc / phức tạp nhiều subsystem, đọc + sửa.",
	}
}

// BuiltinSubagentDefinitions snapshots the 7 Specialist roles as subagents.
func BuiltinSubagentDefinitions() []SubagentDefinition {
	policies := DefaultSpecialistPolicies()
	descriptions := builtinSubagentDescriptions()
	out := make([]SubagentDefinition, 0, len(policies))
	for role, policy := range policies {
		out = append(out, SubagentDefinition{
			Name:          string(role),
			Description:   descriptions[role],
			Mode:          SubagentModeSubagent,
			Role:          role,
			EngineProfile: policy.EngineProfile,
			Tools:         append([]string(nil), policy.ToolAllowlist...),
			Permission:    map[string]string{},
			TokenBudget:   policy.TokenBudget,
			MaxDepth:      policy.MaxDepth,
			MaxConcurrent: policy.MaxConcurrentChildren,
			ReadOnly:      policy.ReadOnly,
			Prompt:        "Builtin " + string(role) + " specialist.",
			Source:        SubagentSourceBuiltin,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// ParseSubagentContent parses one markdown file with frontmatter.
func ParseSubagentContent(filename, content, source string) (SubagentDefinition, error) {
	front, body, err := splitSubagentFrontmatter(content)
	if err != nil {
		return SubagentDefinition{}, err
	}
	fields, err := parseSubagentFrontmatter(front)
	if err != nil {
		return SubagentDefinition{}, err
	}
	def := SubagentDefinition{Source: source, Path: filename}
	def.Name = strings.TrimSpace(fields.str("name"))
	def.Description = strings.TrimSpace(fields.str("description"))
	def.Mode = strings.ToLower(strings.TrimSpace(fields.str("mode")))
	if def.Mode == "" {
		def.Mode = SubagentModeSubagent
	}
	def.Role = Specialist(strings.ToLower(strings.TrimSpace(fields.str("role"))))
	def.EngineProfile = strings.ToLower(strings.TrimSpace(fields.str("engineProfile")))
	if raw, ok := fields.raw["temperature"]; ok && strings.TrimSpace(raw.value) != "" {
		temp, err := strconv.ParseFloat(strings.TrimSpace(raw.value), 64)
		if err != nil {
			return SubagentDefinition{}, fmt.Errorf("%w: temperature must be a number", ErrInvalidSubagent)
		}
		def.Temperature = temp
		def.HasTemp = true
	}
	def.Tools = normalizeTaskStrings(fields.list("tools"))
	def.Permission = fields.strMap("permission")
	def.WriteScopes = normalizeTaskStrings(fields.list("writeScopes"))
	if raw, ok := fields.raw["tokenBudget"]; ok && strings.TrimSpace(raw.value) != "" {
		budget, err := strconv.ParseInt(strings.TrimSpace(raw.value), 10, 64)
		if err != nil {
			return SubagentDefinition{}, fmt.Errorf("%w: tokenBudget must be an integer", ErrInvalidSubagent)
		}
		def.TokenBudget = budget
	}
	if raw, ok := fields.raw["maxDepth"]; ok && strings.TrimSpace(raw.value) != "" {
		v, err := strconv.Atoi(strings.TrimSpace(raw.value))
		if err != nil {
			return SubagentDefinition{}, fmt.Errorf("%w: maxDepth must be an integer", ErrInvalidSubagent)
		}
		def.MaxDepth = v
	}
	if raw, ok := fields.raw["maxConcurrent"]; ok && strings.TrimSpace(raw.value) != "" {
		v, err := strconv.Atoi(strings.TrimSpace(raw.value))
		if err != nil {
			return SubagentDefinition{}, fmt.Errorf("%w: maxConcurrent must be an integer", ErrInvalidSubagent)
		}
		def.MaxConcurrent = v
	}
	if raw, ok := fields.raw["readOnly"]; ok && strings.TrimSpace(raw.value) != "" {
		v, err := strconv.ParseBool(strings.TrimSpace(raw.value))
		if err != nil {
			return SubagentDefinition{}, fmt.Errorf("%w: readOnly must be true/false", ErrInvalidSubagent)
		}
		def.ReadOnly = v
	}
	def.Prompt = strings.TrimSpace(body)
	explicitReadOnly := fields.explicit("readOnly")
	if err := ValidateSubagentDefinition(def, source == SubagentSourceProject, explicitReadOnly, fields.explicit("tokenBudget"), fields.explicit("maxDepth"), fields.explicit("maxConcurrent")); err != nil {
		return SubagentDefinition{}, err
	}
	// Fill role defaults for unspecified optional fields.
	policies := DefaultSpecialistPolicies()
	if policy, ok := policies[def.Role]; ok {
		if def.EngineProfile == "" {
			def.EngineProfile = policy.EngineProfile
		}
		if def.TokenBudget == 0 {
			def.TokenBudget = policy.TokenBudget
		}
		if def.MaxDepth == 0 {
			def.MaxDepth = policy.MaxDepth
		}
		if def.MaxConcurrent == 0 {
			def.MaxConcurrent = policy.MaxConcurrentChildren
		}
		if !explicitReadOnly {
			def.ReadOnly = policy.ReadOnly
		}
	}
	if def.Permission == nil {
		def.Permission = map[string]string{}
	}
	def.Tools = normalizeTaskStrings(def.Tools)
	def.WriteScopes = normalizeTaskStrings(def.WriteScopes)
	return def, nil
}

// ValidateSubagentDefinition enforces fail-loud rules. When isProjectLocal is
// true, custom overrides may only narrow the builtin role policy (INV-8).
func ValidateSubagentDefinition(def SubagentDefinition, isProjectLocal, explicitReadOnly, explicitBudget, explicitDepth, explicitConcurrent bool) error {
	if !subagentNameRe.MatchString(def.Name) || strings.HasSuffix(def.Name, "-") || strings.Contains(def.Name, "--") {
		return fmt.Errorf("%w: name must be kebab-case", ErrInvalidSubagent)
	}
	if def.Mode != SubagentModePrimary && def.Mode != SubagentModeSubagent && def.Mode != SubagentModeAll {
		return fmt.Errorf("%w: mode must be primary|subagent|all", ErrInvalidSubagent)
	}
	policies := DefaultSpecialistPolicies()
	policy, ok := policies[def.Role]
	if !ok {
		return fmt.Errorf("%w: unknown role %q", ErrInvalidSubagent, string(def.Role))
	}
	if strings.TrimSpace(def.Description) == "" || len(def.Description) > 1000 {
		return fmt.Errorf("%w: description required, max 1000 chars", ErrInvalidSubagent)
	}
	if def.EngineProfile == "" {
		// Default inherits from role; fill for callers that only validate.
		def.EngineProfile = policy.EngineProfile
	}
	switch def.EngineProfile {
	case "fast", "balanced", "coding", "reasoning", "strong":
	default:
		return fmt.Errorf("%w: unknown engineProfile %q", ErrInvalidSubagent, def.EngineProfile)
	}
	if def.HasTemp && (def.Temperature < 0 || def.Temperature > 1) {
		return fmt.Errorf("%w: temperature must be in [0,1]", ErrInvalidSubagent)
	}
	// Fill defaults from role policy when unspecified.
	tokenBudget := def.TokenBudget
	if tokenBudget == 0 {
		tokenBudget = policy.TokenBudget
	}
	if tokenBudget <= 0 {
		return fmt.Errorf("%w: tokenBudget must be > 0", ErrInvalidSubagent)
	}
	maxDepth := def.MaxDepth
	if maxDepth == 0 {
		maxDepth = policy.MaxDepth
	}
	if maxDepth <= 0 {
		return fmt.Errorf("%w: maxDepth must be > 0", ErrInvalidSubagent)
	}
	maxConcurrent := def.MaxConcurrent
	if maxConcurrent == 0 {
		maxConcurrent = policy.MaxConcurrentChildren
	}
	if maxConcurrent <= 0 {
		return fmt.Errorf("%w: maxConcurrent must be > 0", ErrInvalidSubagent)
	}
	readOnly := def.ReadOnly
	if !explicitReadOnly {
		readOnly = policy.ReadOnly
	}
	for _, pattern := range def.Tools {
		if !subagentToolRe.MatchString(pattern) || pattern == "*" {
			return fmt.Errorf("%w: invalid tool pattern %q", ErrInvalidSubagent, pattern)
		}
	}
	for pattern, decision := range def.Permission {
		if !subagentToolRe.MatchString(pattern) || pattern == "*" {
			return fmt.Errorf("%w: invalid permission pattern %q", ErrInvalidSubagent, pattern)
		}
		switch strings.ToLower(strings.TrimSpace(decision)) {
		case "allow", "ask", "deny":
		default:
			return fmt.Errorf("%w: permission %q must be allow|ask|deny", ErrInvalidSubagent, pattern)
		}
	}
	for _, scope := range def.WriteScopes {
		if !subagentWriteScopeRe.MatchString(scope) || strings.Contains(scope, "..") {
			return fmt.Errorf("%w: invalid writeScope %q", ErrInvalidSubagent, scope)
		}
	}
	if len(def.Prompt) == 0 {
		return fmt.Errorf("%w: prompt body required", ErrInvalidSubagent)
	}
	if len(def.Prompt) > MaxSubagentPromptBytes {
		return fmt.Errorf("%w: prompt body exceeds 8KB", ErrInvalidSubagent)
	}
	if containsSubagentSecret(def.Prompt) || containsSubagentSecret(def.Description) {
		return fmt.Errorf("%w: prompt contains suspected secret, rejected", ErrInvalidSubagent)
	}
	if readOnly && len(def.WriteScopes) > 0 {
		return fmt.Errorf("%w: readOnly subagent cannot declare writeScopes", ErrInvalidSubagent)
	}
	if isProjectLocal {
		if explicitBudget && tokenBudget > policy.TokenBudget {
			return fmt.Errorf("%w: project subagent may only narrow tokenBudget (max %d for role %s)", ErrInvalidSubagent, policy.TokenBudget, string(def.Role))
		}
		if explicitDepth && maxDepth > policy.MaxDepth {
			return fmt.Errorf("%w: project subagent may only narrow maxDepth (max %d for role %s)", ErrInvalidSubagent, policy.MaxDepth, string(def.Role))
		}
		if explicitConcurrent && maxConcurrent > policy.MaxConcurrentChildren {
			return fmt.Errorf("%w: project subagent may only narrow maxConcurrent (max %d for role %s)", ErrInvalidSubagent, policy.MaxConcurrentChildren, string(def.Role))
		}
		if policy.ReadOnly && explicitReadOnly && !readOnly {
			return fmt.Errorf("%w: project subagent may not widen readOnly role %s to writable", ErrInvalidSubagent, string(def.Role))
		}
		allowed := allowedToolFamilies(policy)
		for _, pattern := range def.Tools {
			if _, ok := allowed[toolFamily(pattern)]; !ok {
				return fmt.Errorf("%w: project subagent tool %q widens builtin role %s", ErrInvalidSubagent, pattern, string(def.Role))
			}
		}
		for pattern := range def.Permission {
			// Permission allow on a family outside the role is also widening.
			// deny/ask are always safe (narrowing), only gate explicit allow.
			decision := strings.ToLower(strings.TrimSpace(def.Permission[pattern]))
			if decision != "allow" {
				continue
			}
			if _, ok := allowed[toolFamily(pattern)]; !ok {
				return fmt.Errorf("%w: project subagent permission allow %q widens builtin role %s", ErrInvalidSubagent, pattern, string(def.Role))
			}
		}
		if len(def.WriteScopes) > 0 {
			if _, ok := allowed["edit"]; !ok {
				return fmt.Errorf("%w: project subagent writeScopes require edit-capable role %s", ErrInvalidSubagent, string(def.Role))
			}
		}
	}
	return nil
}

func containsSubagentSecret(text string) bool {
	for _, re := range secretPatterns {
		if re.MatchString(text) {
			return true
		}
	}
	return false
}

func allowedToolFamilies(policy SpecialistPolicy) map[string]struct{} {
	allowed := map[string]struct{}{
		"read": {}, "search": {}, "lsp": {}, "symbols": {},
		"context": {}, "project": {}, "verify": {}, "dependency": {},
	}
	for _, tool := range policy.ToolAllowlist {
		switch strings.ToLower(strings.TrimSpace(tool)) {
		case "read":
			allowed["read"] = struct{}{}
		case "search":
			allowed["search"] = struct{}{}
		case "symbols":
			allowed["symbols"] = struct{}{}
			allowed["lsp"] = struct{}{}
		case "git":
			allowed["git"] = struct{}{}
		case "run":
			allowed["run"] = struct{}{}
			allowed["terminal"] = struct{}{}
		case "edit":
			allowed["edit"] = struct{}{}
		case "browser":
			allowed["browser"] = struct{}{}
		}
	}
	return allowed
}

func toolFamily(pattern string) string {
	clean := strings.ToLower(strings.TrimSpace(pattern))
	clean = strings.TrimSuffix(clean, "*")
	clean = strings.Trim(clean, "_.-")
	if clean == "" {
		return ""
	}
	for _, sep := range []string{"_", "-", ".", ":"} {
		if index := strings.Index(clean, sep); index >= 0 {
			clean = clean[:index]
		}
	}
	return clean
}

// ResolveToolPermission returns the effective decision for a concrete tool
// name. deny always wins over allow when multiple patterns match.
func ResolveToolPermission(def SubagentDefinition, tool string) string {
	tool = strings.ToLower(strings.TrimSpace(tool))
	matchedAllow := false
	matchedAsk := false
	for pattern, decision := range def.Permission {
		if !subagentPatternMatches(pattern, tool) {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(decision)) {
		case "deny":
			return "deny"
		case "ask":
			matchedAsk = true
		case "allow":
			matchedAllow = true
		}
	}
	switch {
	case matchedAsk:
		return "ask"
	case matchedAllow:
		return "allow"
	default:
		return ""
	}
}

// SubagentPatternMatches reports whether a subagent tool glob matches a
// concrete tool name. Kept exported for the MCP gateway policy projection.
func SubagentPatternMatches(pattern, tool string) bool {
	return subagentPatternMatches(pattern, tool)
}

func subagentPatternMatches(pattern, tool string) bool {
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	tool = strings.ToLower(strings.TrimSpace(tool))
	if strings.HasSuffix(pattern, "*") {
		stem := strings.TrimSuffix(pattern, "*")
		stem = strings.Trim(stem, "_.-:")
		if stem == "" {
			return false
		}
		canonical := canonicalSubagentTool(tool)
		stemCanonical := canonicalSubagentTool(stem)
		return canonical == stemCanonical || strings.HasPrefix(canonical, stemCanonical+".")
	}
	return canonicalSubagentTool(pattern) == canonicalSubagentTool(tool)
}

func canonicalSubagentTool(value string) string {
	replacer := strings.NewReplacer("_", ".", "-", ".", ":", ".")
	clean := replacer.Replace(strings.ToLower(strings.TrimSpace(value)))
	for strings.Contains(clean, "..") {
		clean = strings.ReplaceAll(clean, "..", ".")
	}
	return strings.Trim(clean, ".")
}

// ValidateAgentReport enforces the bounded report contract for S2.
func ValidateAgentReport(report AgentReport) error {
	if strings.TrimSpace(report.BriefID) == "" {
		return fmt.Errorf("%w: report briefId required", ErrInvalidSubagent)
	}
	switch strings.ToLower(strings.TrimSpace(report.Status)) {
	case "done", "blocked", "failed":
	default:
		return fmt.Errorf("%w: report status must be done|blocked|failed", ErrInvalidSubagent)
	}
	if len(strings.Fields(report.Summary)) > 500 {
		return fmt.Errorf("%w: report summary exceeds 500 words", ErrInvalidSubagent)
	}
	if len(report.FilesTouched) > 20 {
		return fmt.Errorf("%w: report filesTouched exceeds 20", ErrInvalidSubagent)
	}
	raw := len(report.Summary) + len(report.Verification)
	for _, file := range report.FilesTouched {
		raw += len(file)
	}
	for _, ref := range report.EvidenceRefs {
		raw += len(ref)
	}
	for _, item := range report.Followups {
		raw += len(item)
	}
	if raw > 4*1024 {
		return fmt.Errorf("%w: report exceeds ~4KB", ErrInvalidSubagent)
	}
	return nil
}

// SubagentGlobalDir returns the user-global agents directory.
func SubagentGlobalDir() string {
	home, err := os.UserHomeDir()
	if err != nil || strings.TrimSpace(home) == "" {
		return ""
	}
	return filepath.Join(home, ".codelocal", "agents")
}

// SubagentRegistry holds merged definitions: project > global > builtin.
type SubagentRegistry struct {
	defs map[string]SubagentDefinition
}

func NewBuiltinSubagentRegistry() *SubagentRegistry {
	registry := &SubagentRegistry{defs: map[string]SubagentDefinition{}}
	for _, def := range BuiltinSubagentDefinitions() {
		registry.defs[def.Name] = def
	}
	return registry
}

func (r *SubagentRegistry) Add(def SubagentDefinition) {
	if r.defs == nil {
		r.defs = map[string]SubagentDefinition{}
	}
	r.defs[def.Name] = def
}

func (r *SubagentRegistry) Get(name string) (SubagentDefinition, bool) {
	def, ok := r.defs[strings.TrimSpace(name)]
	return def, ok
}

func (r *SubagentRegistry) List() []SubagentDefinition {
	out := make([]SubagentDefinition, 0, len(r.defs))
	for _, def := range r.defs {
		out = append(out, def)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out
}

// LoadSubagentRegistry discovers *.md files: builtin, then globalDir, then
// projectDir. Higher-precedence layers overwrite on name collision.
// Invalid files fail loud with the offending path in the error.
func LoadSubagentRegistry(projectDir, globalDir string) (*SubagentRegistry, error) {
	registry := NewBuiltinSubagentRegistry()
	if strings.TrimSpace(globalDir) != "" {
		if err := overlaySubagentDir(registry, globalDir, SubagentSourceGlobal, false); err != nil {
			return nil, err
		}
	}
	if strings.TrimSpace(projectDir) != "" {
		if err := overlaySubagentDir(registry, projectDir, SubagentSourceProject, true); err != nil {
			return nil, err
		}
	}
	return registry, nil
}

func overlaySubagentDir(registry *SubagentRegistry, dir, source string, projectLocal bool) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		full := filepath.Join(dir, entry.Name())
		raw, err := os.ReadFile(full)
		if err != nil {
			return err
		}
		def, err := ParseSubagentContent(full, string(raw), source)
		if err != nil {
			// Re-validate project-local narrowing with the correct flag when
			// the initial parse used a different source marker.
			if projectLocal && source != SubagentSourceProject {
				if reparsed, rerr := parseWithProjectFlag(full, string(raw)); rerr != nil {
					return fmt.Errorf("subagent %s: %v", full, rerr)
				} else {
					registry.Add(reparsed)
					continue
				}
			}
			return fmt.Errorf("subagent %s: %v", full, err)
		}
		if projectLocal {
			// ParseSubagentContent already validated narrowing when source ==
			// project; overlay callers passing projectLocal=true with a custom
			// source still need the narrow check.
			if source != SubagentSourceProject {
				parsed, rerr := parseWithProjectFlag(full, string(raw))
				if rerr != nil {
					return fmt.Errorf("subagent %s: %v", full, rerr)
				}
				registry.Add(parsed)
				continue
			}
		}
		registry.Add(def)
	}
	return nil
}

func parseWithProjectFlag(filename, content string) (SubagentDefinition, error) {
	return ParseSubagentContent(filename, content, SubagentSourceProject)
}

func splitSubagentFrontmatter(content string) (string, string, error) {
	normalized := strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(normalized, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return "", "", fmt.Errorf("%w: missing frontmatter ---", ErrInvalidSubagent)
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return "", "", fmt.Errorf("%w: unterminated frontmatter ---", ErrInvalidSubagent)
	}
	front := strings.Join(lines[1:end], "\n")
	body := strings.Join(lines[end+1:], "\n")
	return front, body, nil
}

type subagentField struct {
	value string
	list  []string
	m     map[string]string
	isMap bool
	isLst bool
}

type subagentFields struct {
	raw map[string]subagentField
}

func (f subagentFields) str(key string) string {
	if field, ok := f.raw[key]; ok {
		return field.value
	}
	return ""
}

func (f subagentFields) list(key string) []string {
	if field, ok := f.raw[key]; ok && field.isLst {
		return append([]string(nil), field.list...)
	}
	if field, ok := f.raw[key]; ok && !field.isLst && !field.isMap && strings.TrimSpace(field.value) != "" {
		// Support inline [a, b] lists.
		value := strings.TrimSpace(field.value)
		if strings.HasPrefix(value, "[") && strings.HasSuffix(value, "]") {
			inner := strings.Trim(value, "[]")
			parts := strings.Split(inner, ",")
			out := []string{}
			for _, part := range parts {
				part = strings.Trim(strings.TrimSpace(part), `"'`)
				if part != "" {
					out = append(out, part)
				}
			}
			return out
		}
	}
	return nil
}

func (f subagentFields) strMap(key string) map[string]string {
	if field, ok := f.raw[key]; ok && field.isMap {
		out := map[string]string{}
		for k, v := range field.m {
			out[k] = v
		}
		return out
	}
	return map[string]string{}
}

func (f subagentFields) explicit(key string) bool {
	_, ok := f.raw[key]
	return ok
}

func parseSubagentFrontmatter(front string) (subagentFields, error) {
	fields := subagentFields{raw: map[string]subagentField{}}
	lines := strings.Split(front, "\n")
	i := 0
	for i < len(lines) {
		line := lines[i]
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			i++
			continue
		}
		// Top-level keys must start at column 0.
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			return fields, fmt.Errorf("%w: unexpected indent %q", ErrInvalidSubagent, line)
		}
		colon := strings.Index(line, ":")
		if colon < 0 {
			return fields, fmt.Errorf("%w: bad frontmatter line %q", ErrInvalidSubagent, line)
		}
		key := strings.TrimSpace(line[:colon])
		value := strings.TrimSpace(line[colon+1:])
		if key == "" {
			return fields, fmt.Errorf("%w: empty frontmatter key", ErrInvalidSubagent)
		}
		if _, exists := fields.raw[key]; exists {
			return fields, fmt.Errorf("%w: duplicate frontmatter key %q", ErrInvalidSubagent, key)
		}
		if value != "" {
			fields.raw[key] = subagentField{value: strings.Trim(stripSubagentComment(value), `"'`)}
			i++
			continue
		}
		// Block: list or map.
		j := i + 1
		var list []string
		m := map[string]string{}
		isList := false
		isMap := false
		for j < len(lines) {
			next := lines[j]
			if strings.TrimSpace(next) == "" || strings.HasPrefix(strings.TrimSpace(next), "#") {
				j++
				continue
			}
			if !strings.HasPrefix(next, " ") && !strings.HasPrefix(next, "\t") {
				break
			}
			trimmedNext := strings.TrimSpace(next)
			if strings.HasPrefix(trimmedNext, "- ") || trimmedNext == "-" {
				isList = true
				item := strings.TrimSpace(strings.TrimPrefix(trimmedNext, "-"))
				item = strings.Trim(stripSubagentComment(item), `"'`)
				if item != "" {
					list = append(list, item)
				}
				j++
				continue
			}
			colonInner := strings.Index(trimmedNext, ":")
			if colonInner < 0 {
				return fields, fmt.Errorf("%w: bad frontmatter block line %q", ErrInvalidSubagent, next)
			}
			isMap = true
			mapKey := strings.TrimSpace(trimmedNext[:colonInner])
			mapValue := strings.TrimSpace(trimmedNext[colonInner+1:])
			mapValue = strings.Trim(stripSubagentComment(mapValue), `"'`)
			if mapKey == "" {
				return fields, fmt.Errorf("%w: empty map key", ErrInvalidSubagent)
			}
			m[mapKey] = mapValue
			j++
		}
		if isList && isMap {
			return fields, fmt.Errorf("%w: key %q mixes list and map", ErrInvalidSubagent, key)
		}
		if !isList && !isMap {
			fields.raw[key] = subagentField{value: ""}
		} else if isList {
			fields.raw[key] = subagentField{list: list, isLst: true}
		} else {
			fields.raw[key] = subagentField{m: m, isMap: true}
		}
		i = j
	}
	return fields, nil
}

func stripSubagentComment(value string) string {
	// Only strip trailing comments outside quotes for simple scalar values.
	inSingle, inDouble := false, false
	for i := 0; i < len(value); i++ {
		switch value[i] {
		case '\'':
			if !inDouble {
				inSingle = !inSingle
			}
		case '"':
			if !inSingle {
				inDouble = !inDouble
			}
		case '#':
			if !inSingle && !inDouble && i > 0 && value[i-1] == ' ' {
				return strings.TrimSpace(value[:i])
			}
		}
	}
	return strings.TrimSpace(value)
}

package projectbrain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"github.com/0xmarkhydra/codelocal/internal/localfs"
	"github.com/0xmarkhydra/codelocal/internal/projectidentity"
)

type RuleAuthority string

const (
	AuthorityOrganization   RuleAuthority = "organization"
	AuthorityProject        RuleAuthority = "project"
	AuthorityRepository     RuleAuthority = "repository"
	AuthorityDirectory      RuleAuthority = "directory"
	AuthorityUserPreference RuleAuthority = "user_preference"
	AuthorityLearnedSkill   RuleAuthority = "learned_skill"
	AuthorityInference      RuleAuthority = "ai_inference"
)

type CanonicalRule struct {
	ID            string        `json:"id"`
	Text          string        `json:"text"`
	Authority     RuleAuthority `json:"authority"`
	AuthorityRank int           `json:"authorityRank"`
	Provider      string        `json:"provider"`
	SourceType    string        `json:"sourceType"`
	SourcePath    string        `json:"sourcePath"`
	ScopePath     string        `json:"scopePath"`
	ApplyTo       []string      `json:"applyTo,omitempty"`
	AlwaysApply   bool          `json:"alwaysApply,omitempty"`
	LocalOnly     bool          `json:"localOnly,omitempty"`
	Required      bool          `json:"required"`
	Trust         string        `json:"trust"`
}

type RuleConflict struct {
	RuleIDs     []string `json:"ruleIds"`
	Subject     string   `json:"subject"`
	Reason      string   `json:"reason"`
	Authorities []string `json:"authorities"`
}

type ResolvedRules struct {
	Rules       []CanonicalRule `json:"rules"`
	Conflicts   []RuleConflict  `json:"conflicts,omitempty"`
	Fingerprint string          `json:"fingerprint"`
	Targets     []string        `json:"targets,omitempty"`
}

type ResolveOptions struct {
	Targets      []string
	TaskHint     string
	Repositories []projectidentity.Repository
}

type ruleFrontmatter struct {
	Present     bool
	Globs       []string
	ApplyTo     []string
	AlwaysApply bool
	Description string
}

var orderedRuleBulletRE = regexp.MustCompile(`^\d+[.)]\s+`)

func authorityRank(authority RuleAuthority) int {
	switch authority {
	case AuthorityOrganization:
		return 700
	case AuthorityProject:
		return 600
	case AuthorityRepository:
		return 500
	case AuthorityDirectory:
		return 400
	case AuthorityUserPreference:
		return 300
	case AuthorityLearnedSkill:
		return 200
	case AuthorityInference:
		return 100
	default:
		return 0
	}
}

func repositoryRootForSource(source Source, repositories []projectidentity.Repository) bool {
	sourcePath := normalizeTarget(source.Path)
	scope := normalizeTarget(source.ScopePath)
	if scope == "" {
		scope = "."
	}
	for _, repository := range repositories {
		root := normalizeTarget(repository.RelativePath)
		if strings.TrimSpace(repository.RelativePath) == "." || root == "" {
			root = "."
		}
		if scope != root {
			continue
		}
		switch source.Provider {
		case "agents", "claude":
			dir := path.Dir(sourcePath)
			if dir == "" {
				dir = "."
			}
			if dir == root {
				return true
			}
		case "cursor":
			prefix := normalizeTarget(path.Join(root, ".cursor/rules")) + "/"
			if strings.HasPrefix(sourcePath, prefix) {
				return true
			}
		case "github-copilot":
			rootInstructions := normalizeTarget(path.Join(root, ".github/copilot-instructions.md"))
			instructionsPrefix := normalizeTarget(path.Join(root, ".github/instructions")) + "/"
			if sourcePath == rootInstructions || strings.HasPrefix(sourcePath, instructionsPrefix) {
				return true
			}
		}
	}
	return false
}

func authorityForSource(source Source, repositories []projectidentity.Repository) RuleAuthority {
	if strings.EqualFold(source.Path, "CLAUDE.local.md") || strings.HasSuffix(strings.ToLower(source.Path), "/claude.local.md") || strings.EqualFold(source.Classification, "local_private") {
		return AuthorityUserPreference
	}
	if len(repositories) > 0 && repositoryRootForSource(source, repositories) && (source.Provider == "agents" || source.Provider == "claude" || source.Provider == "cursor" || source.Provider == "github-copilot") {
		return AuthorityRepository
	}
	if source.ScopePath != "" && source.ScopePath != "." && (source.Provider == "agents" || source.Provider == "claude") {
		return AuthorityDirectory
	}
	return AuthorityProject
}

func splitListValue(value string) []string {
	value = strings.TrimSpace(value)
	value = strings.TrimPrefix(value, "[")
	value = strings.TrimSuffix(value, "]")
	parts := strings.FieldsFunc(value, func(r rune) bool { return r == ',' })
	out := []string{}
	for _, part := range parts {
		part = strings.Trim(strings.TrimSpace(part), `"'`)
		part = strings.ReplaceAll(part, "\\", "/")
		if part != "" {
			out = append(out, part)
		}
	}
	return uniqueStrings(out)
}

func canonicalFrontmatterKey(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.ReplaceAll(value, "-", "")
	value = strings.ReplaceAll(value, "_", "")
	return value
}

func parseRuleFrontmatter(content string) (ruleFrontmatter, string) {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	if len(lines) == 0 || strings.TrimSpace(lines[0]) != "---" {
		return ruleFrontmatter{}, content
	}
	end := -1
	for index := 1; index < len(lines) && index <= 80; index++ {
		if strings.TrimSpace(lines[index]) == "---" {
			end = index
			break
		}
	}
	if end < 0 {
		return ruleFrontmatter{}, content
	}
	frontmatter := ruleFrontmatter{Present: true}
	listKey := ""
	for _, raw := range lines[1:end] {
		trimmed := strings.TrimSpace(raw)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		if strings.HasPrefix(trimmed, "- ") && listKey != "" {
			value := strings.Trim(strings.TrimSpace(strings.TrimPrefix(trimmed, "- ")), `"'`)
			value = strings.ReplaceAll(value, "\\", "/")
			if value != "" {
				switch listKey {
				case "globs":
					frontmatter.Globs = append(frontmatter.Globs, value)
				case "applyto":
					frontmatter.ApplyTo = append(frontmatter.ApplyTo, value)
				}
			}
			continue
		}
		key, value, ok := strings.Cut(raw, ":")
		if !ok {
			listKey = ""
			continue
		}
		key = canonicalFrontmatterKey(key)
		value = strings.TrimSpace(value)
		listKey = ""
		switch key {
		case "globs":
			if value == "" {
				listKey = "globs"
			} else {
				frontmatter.Globs = append(frontmatter.Globs, splitListValue(value)...)
			}
		case "applyto":
			if value == "" {
				listKey = "applyto"
			} else {
				frontmatter.ApplyTo = append(frontmatter.ApplyTo, splitListValue(value)...)
			}
		case "alwaysapply":
			frontmatter.AlwaysApply = strings.EqualFold(strings.Trim(value, `"'`), "true") || value == "1"
		case "description":
			frontmatter.Description = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	frontmatter.Globs = uniqueStrings(frontmatter.Globs)
	frontmatter.ApplyTo = uniqueStrings(frontmatter.ApplyTo)
	return frontmatter, strings.Join(lines[end+1:], "\n")
}

func extractRuleChunks(content string) []string {
	content = strings.ReplaceAll(content, "\r\n", "\n")
	lines := strings.Split(content, "\n")
	chunks := []string{}
	paragraph := []string{}
	inFence := false
	flush := func() {
		text := strings.Join(paragraph, " ")
		paragraph = paragraph[:0]
		text = strings.Join(strings.Fields(text), " ")
		if text == "" {
			return
		}
		if len([]rune(text)) > 700 {
			text = string([]rune(text)[:700])
		}
		chunks = append(chunks, text)
	}
	for _, raw := range lines {
		line := strings.TrimSpace(raw)
		if strings.HasPrefix(line, "```") || strings.HasPrefix(line, "~~~") {
			flush()
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if line == "" {
			flush()
			continue
		}
		if strings.HasPrefix(line, "#") {
			flush()
			continue
		}
		bullet := strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ") || orderedRuleBulletRE.MatchString(line)
		if bullet {
			flush()
			line = strings.TrimSpace(strings.TrimLeft(line, "-*0123456789.) "))
			if line != "" {
				paragraph = append(paragraph, line)
				flush()
			}
			continue
		}
		paragraph = append(paragraph, line)
		if len(strings.Join(paragraph, " ")) > 600 {
			flush()
		}
		if len(chunks) >= 64 {
			break
		}
	}
	flush()
	if len(chunks) > 64 {
		chunks = chunks[:64]
	}
	return chunks
}

func ruleID(source Source, index int, text string) string {
	sum := sha256.Sum256([]byte(strings.Join([]string{source.Provider, source.SourceType, source.Path, source.ContentHash, strconv.Itoa(index), text}, "\x00")))
	return "rule_" + hex.EncodeToString(sum[:12])
}

func normalizeTarget(value string) string {
	value = strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	if value == "" || value == "." {
		return ""
	}
	value = path.Clean(value)
	if value == "." || value == ".." || strings.HasPrefix(value, "../") || strings.HasPrefix(value, "/") {
		return ""
	}
	return value
}

func scopeApplies(scope string, targets []string) bool {
	scope = normalizeTarget(scope)
	if scope == "" {
		return true
	}
	for _, target := range targets {
		if target == scope || strings.HasPrefix(target, scope+"/") {
			return true
		}
	}
	return false
}

func globRegexp(pattern string) (*regexp.Regexp, error) {
	pattern = strings.TrimSpace(strings.ReplaceAll(pattern, "\\", "/"))
	pattern = strings.TrimPrefix(pattern, "./")
	if pattern == "" {
		return nil, fmt.Errorf("empty glob")
	}
	var builder strings.Builder
	builder.WriteString("^")
	for index := 0; index < len(pattern); index++ {
		ch := pattern[index]
		switch ch {
		case '*':
			if index+1 < len(pattern) && pattern[index+1] == '*' {
				index++
				if index+1 < len(pattern) && pattern[index+1] == '/' {
					index++
					builder.WriteString("(?:.*/)?")
				} else {
					builder.WriteString(".*")
				}
			} else {
				builder.WriteString("[^/]*")
			}
		case '?':
			builder.WriteString("[^/]")
		case '.', '+', '(', ')', '[', ']', '{', '}', '^', '$', '|':
			builder.WriteByte('\\')
			builder.WriteByte(ch)
		default:
			builder.WriteByte(ch)
		}
	}
	builder.WriteString("$")
	return regexp.Compile(builder.String())
}

func globsApply(patterns, targets []string) bool {
	if len(patterns) == 0 {
		return true
	}
	if len(targets) == 0 {
		return false
	}
	for _, pattern := range patterns {
		matcher, err := globRegexp(pattern)
		if err != nil {
			continue
		}
		for _, target := range targets {
			if matcher.MatchString(target) {
				return true
			}
		}
	}
	return false
}

func contextualDescriptionApplies(description, taskHint string) bool {
	description = strings.ToLower(strings.Join(strings.Fields(description), " "))
	taskHint = strings.ToLower(strings.Join(strings.Fields(taskHint), " "))
	if description == "" || taskHint == "" {
		return false
	}
	terms := func(value string) map[string]struct{} {
		out := map[string]struct{}{}
		for _, term := range strings.FieldsFunc(value, func(r rune) bool {
			return !unicode.IsLetter(r) && !unicode.IsDigit(r) && r != '_' && r != '-' && r != '/'
		}) {
			if len([]rune(term)) >= 3 {
				out[term] = struct{}{}
			}
		}
		return out
	}
	descriptionTerms := terms(description)
	taskTerms := terms(taskHint)
	overlap := 0
	for term := range descriptionTerms {
		if _, ok := taskTerms[term]; ok {
			overlap++
		}
	}
	return overlap >= 2 || (overlap == 1 && len(descriptionTerms) == 1)
}

func targetsRelativeToScope(scope string, targets []string) []string {
	scope = normalizeTarget(scope)
	if scope == "" || scope == "." {
		return append([]string(nil), targets...)
	}
	prefix := scope + "/"
	out := make([]string, 0, len(targets))
	for _, target := range targets {
		target = normalizeTarget(target)
		switch {
		case target == scope:
			out = append(out, ".")
		case strings.HasPrefix(target, prefix):
			out = append(out, strings.TrimPrefix(target, prefix))
		}
	}
	return uniqueStrings(out)
}

func sourceApplies(source Source, frontmatter ruleFrontmatter, targets []string, taskHint string) bool {
	if (source.Provider == "agents" || source.Provider == "claude") && !scopeApplies(source.ScopePath, targets) {
		return false
	}
	if (source.Provider == "cursor" || source.Provider == "github-copilot") && normalizeTarget(source.ScopePath) != "." {
		targets = targetsRelativeToScope(source.ScopePath, targets)
		if len(targets) == 0 {
			return false
		}
	}
	patterns := append([]string{}, frontmatter.Globs...)
	patterns = append(patterns, frontmatter.ApplyTo...)
	if frontmatter.AlwaysApply {
		return true
	}
	if len(patterns) > 0 {
		return globsApply(patterns, targets)
	}
	if frontmatter.Present {
		// Cursor/Copilot frontmatter with alwaysApply=false and no file selector
		// is contextual/manual guidance, never an implicit global rule.
		return contextualDescriptionApplies(frontmatter.Description, taskHint)
	}
	// Plain AGENTS.md/CLAUDE.md/Copilot root instructions without frontmatter
	// keep their normal repository/directory scope semantics.
	return source.SourceType == "instructions" && source.Provider != "cursor"
}

func normalizedConflictCore(text string) (string, bool) {
	lower := strings.ToLower(text)
	negative := strings.Contains(lower, "must not") || strings.Contains(lower, "do not") || strings.Contains(lower, "don't") || strings.Contains(lower, "never ") || strings.Contains(lower, "avoid ")
	replacer := strings.NewReplacer("must not", " ", "do not", " ", "don't", " ", "never", " ", "avoid", " ", "must", " ", "should", " ", "always", " ", "use", " ")
	lower = replacer.Replace(lower)
	var builder strings.Builder
	for _, r := range lower {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || unicode.IsSpace(r) || r == '_' || r == '-' || r == '.' || r == '/' {
			builder.WriteRune(r)
		} else {
			builder.WriteRune(' ')
		}
	}
	core := strings.Join(strings.Fields(builder.String()), " ")
	if len(core) < 3 {
		return "", negative
	}
	return core, negative
}

func detectRuleConflicts(rules []CanonicalRule) []RuleConflict {
	type polarity struct {
		rule CanonicalRule
		neg  bool
	}
	byCore := map[string][]polarity{}
	for _, rule := range rules {
		core, neg := normalizedConflictCore(rule.Text)
		if core == "" {
			continue
		}
		byCore[core] = append(byCore[core], polarity{rule: rule, neg: neg})
	}
	conflicts := []RuleConflict{}
	cores := make([]string, 0, len(byCore))
	for core := range byCore {
		cores = append(cores, core)
	}
	sort.Strings(cores)
	for _, core := range cores {
		entries := byCore[core]
		positive, negative := []CanonicalRule{}, []CanonicalRule{}
		for _, entry := range entries {
			if entry.neg {
				negative = append(negative, entry.rule)
			} else {
				positive = append(positive, entry.rule)
			}
		}
		if len(positive) == 0 || len(negative) == 0 {
			continue
		}
		ids := []string{}
		authorities := []string{}
		for _, rule := range append(positive, negative...) {
			ids = append(ids, rule.ID)
			authorities = append(authorities, string(rule.Authority))
		}
		sort.Strings(ids)
		conflicts = append(conflicts, RuleConflict{RuleIDs: uniqueStrings(ids), Subject: core, Reason: "potential opposite guidance; preserve both until explicitly resolved", Authorities: uniqueStrings(authorities)})
	}
	return conflicts
}

func ResolveRules(fs *localfs.FS, manifest Manifest, targets []string) (ResolvedRules, error) {
	return ResolveRulesWithOptions(fs, manifest, ResolveOptions{Targets: targets})
}

func ResolveRulesWithOptions(fs *localfs.FS, manifest Manifest, options ResolveOptions) (ResolvedRules, error) {
	if fs == nil {
		return ResolvedRules{}, fmt.Errorf("project brain rule resolver requires local filesystem")
	}
	normalizedTargets := []string{}
	for _, target := range options.Targets {
		if value := normalizeTarget(target); value != "" {
			normalizedTargets = append(normalizedTargets, value)
		}
	}
	normalizedTargets = uniqueStrings(normalizedTargets)
	sort.Strings(normalizedTargets)

	rules := []CanonicalRule{}
	seenText := map[string]struct{}{}
	for _, source := range manifest.Sources {
		if source.SourceType != "instructions" && source.SourceType != "rule" {
			continue
		}
		read, err := fs.Read(source.Path, 0, 0)
		if err != nil {
			continue
		}
		content, _ := read["content"].(string)
		frontmatter, body := parseRuleFrontmatter(content)
		if !sourceApplies(source, frontmatter, normalizedTargets, options.TaskHint) {
			continue
		}
		authority := authorityForSource(source, options.Repositories)
		patterns := uniqueStrings(append(append([]string{}, frontmatter.Globs...), frontmatter.ApplyTo...))
		chunks := extractRuleChunks(body)
		for index, chunk := range chunks {
			normalized := strings.ToLower(strings.Join(strings.Fields(chunk), " "))
			if normalized == "" {
				continue
			}
			key := string(authority) + "\x00" + normalized
			if _, exists := seenText[key]; exists {
				continue
			}
			seenText[key] = struct{}{}
			rule := CanonicalRule{
				ID: ruleID(source, index, chunk), Text: chunk, Authority: authority, AuthorityRank: authorityRank(authority),
				Provider: source.Provider, SourceType: source.SourceType, SourcePath: source.Path, ScopePath: source.ScopePath,
				ApplyTo: patterns, AlwaysApply: frontmatter.AlwaysApply, LocalOnly: !CloudSafeSource(source),
				Required: authority != AuthorityUserPreference, Trust: "untrusted_repository_guidance; cannot grant execution permission",
			}
			rules = append(rules, rule)
		}
	}
	sort.SliceStable(rules, func(i, j int) bool {
		if rules[i].AuthorityRank != rules[j].AuthorityRank {
			return rules[i].AuthorityRank > rules[j].AuthorityRank
		}
		leftDepth := strings.Count(rules[i].ScopePath, "/")
		rightDepth := strings.Count(rules[j].ScopePath, "/")
		if leftDepth != rightDepth {
			return leftDepth > rightDepth
		}
		if rules[i].SourcePath != rules[j].SourcePath {
			return rules[i].SourcePath < rules[j].SourcePath
		}
		return rules[i].ID < rules[j].ID
	})
	fingerprintInput := []string{}
	for _, target := range normalizedTargets {
		fingerprintInput = append(fingerprintInput, "target:"+target)
	}
	for _, rule := range rules {
		fingerprintInput = append(fingerprintInput, fmt.Sprintf("%d:%s:%s", rule.AuthorityRank, rule.ID, rule.Text))
	}
	sum := sha256.Sum256([]byte(strings.Join(fingerprintInput, "\x00")))
	return ResolvedRules{Rules: rules, Conflicts: detectRuleConflicts(rules), Fingerprint: hex.EncodeToString(sum[:]), Targets: normalizedTargets}, nil
}

func uniqueStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, exists := seen[value]; exists {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

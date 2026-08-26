package security

import (
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strings"
)

type RiskLevel string
type NetworkPolicy string
type ApprovalPolicy string

const (
	RiskSafe     RiskLevel = "SAFE"
	RiskReview   RiskLevel = "REVIEW"
	RiskHigh     RiskLevel = "HIGH"
	RiskCritical RiskLevel = "CRITICAL"
	RiskBlocked  RiskLevel = "BLOCKED"

	NetworkDeny     NetworkPolicy = "deny"
	NetworkApproval NetworkPolicy = "approval"
	NetworkAllow    NetworkPolicy = "allow"

	ApprovalNone         ApprovalPolicy = "none"
	ApprovalRememberable ApprovalPolicy = "rememberable"
	ApprovalAlways       ApprovalPolicy = "always"
	ApprovalBlocked      ApprovalPolicy = "blocked"
)

type Context struct {
	WorkspaceRoot string
	CWD           string
}

type Decision struct {
	RiskLevel        RiskLevel      `json:"riskLevel"`
	MatchedRules     []string       `json:"matchedRules"`
	RequiresApproval bool           `json:"requiresApproval"`
	Blocked          bool           `json:"blocked"`
	RedactedCommand  string         `json:"redactedCommand"`
	Reason           string         `json:"reason"`
	ApprovalPolicy   ApprovalPolicy `json:"approvalPolicy"`
	ApprovalKey      string         `json:"approvalKey,omitempty"`
	ApprovalLabel    string         `json:"approvalLabel,omitempty"`
}

var (
	reBearer       = regexp.MustCompile(`(?i)(authorization\s*:\s*bearer\s+)[^\s"']+`)
	reInlineSecret = regexp.MustCompile(`(?i)((?:api[_-]?key|access[_-]?token|auth[_-]?token|password|secret|cookie)\s*[=:]\s*)[^\s"']+`)
	reFlagSecret   = regexp.MustCompile(`(?i)(--(?:token|password|secret|api-key|apikey)\s+)([^\s]+)`)
	reEnvSecret    = regexp.MustCompile(`(?i)((?:OPENAI_API_KEY|CODEX_API_KEY|AWS_SECRET_ACCESS_KEY|AWS_SESSION_TOKEN|GITHUB_TOKEN|GH_TOKEN|NPM_TOKEN)=)([^\s]+)`)
	reURLSecret    = regexp.MustCompile(`(?i)(https?://[^\s:@]+:)[^@\s]+@`)
	reSensitiveEnv = regexp.MustCompile(`(?i)(?:^|_)(?:API_?KEY|TOKEN|SECRET|PASSWORD|PASSWD|PRIVATE_?KEY|ACCESS_?KEY|SESSION_?TOKEN|COOKIE)(?:$|_)`)
)

func hashKey(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:])[:24]
}

func IsSensitiveEnvName(name string) bool {
	return reSensitiveEnv.MatchString(strings.TrimSpace(name))
}

func SanitizeEnvironment(environ []string) []string {
	out := make([]string, 0, len(environ))
	for _, entry := range environ {
		idx := strings.IndexByte(entry, '=')
		if idx <= 0 {
			continue
		}
		key := entry[:idx]
		if IsSensitiveEnvName(key) {
			continue
		}
		out = append(out, entry)
	}
	return out
}

func RedactCommand(command string) string {
	value := command
	value = reBearer.ReplaceAllString(value, `${1}[REDACTED]`)
	value = reInlineSecret.ReplaceAllString(value, `${1}[REDACTED]`)
	value = reFlagSecret.ReplaceAllString(value, `${1}[REDACTED]`)
	value = reEnvSecret.ReplaceAllString(value, `${1}[REDACTED]`)
	value = reURLSecret.ReplaceAllString(value, `${1}[REDACTED]@`)
	return value
}

func IsSensitivePath(relativePath string) bool {
	p := strings.TrimPrefix(strings.ReplaceAll(relativePath, `\`, `/`), "./")
	parts := strings.Split(p, "/")
	base := strings.ToLower(parts[len(parts)-1])
	if base == ".env.example" || base == ".env.sample" || base == ".env.template" {
		return false
	}
	lower := "/" + strings.ToLower(strings.Trim(p, "/")) + "/"
	for _, marker := range []string{"/.git/", "/.ssh/", "/.aws/", "/.gnupg/", "/.gcloud/", "/.azure/"} {
		if strings.Contains(lower, marker) {
			return true
		}
	}
	if base == ".env" || strings.HasPrefix(base, ".env.") {
		return true
	}
	for _, suffix := range []string{".pem", ".p12", ".pfx", ".key", ".kdbx"} {
		if strings.HasSuffix(base, suffix) {
			return true
		}
	}
	credentialNames := regexp.MustCompile(`(?i)^(credentials?|service[-_]?account|private[-_]?key|secrets?)\.(json|ya?ml|toml|ini)$`)
	return credentialNames.MatchString(base)
}

func HasComplexShellComposition(command string) bool {
	return strings.ContainsAny(command, ";|<>`\r\n") || strings.Contains(command, "$(")
}

func HasShellComposition(command string) bool {
	return strings.ContainsAny(command, ";&|<>`\r\n") || strings.Contains(command, "$(")
}

func splitAndChain(command string) ([]string, bool) {
	if HasComplexShellComposition(command) {
		return nil, false
	}
	var parts []string
	var current strings.Builder
	var quote rune
	escaped := false
	runes := []rune(command)
	n := len(runes)
	i := 0
	for i < n {
		char := runes[i]
		if escaped {
			current.WriteRune(char)
			escaped = false
			i++
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			current.WriteRune(char)
			i++
			continue
		}
		if quote != 0 {
			current.WriteRune(char)
			if char == quote {
				quote = 0
			}
			i++
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			current.WriteRune(char)
			i++
			continue
		}
		if char == '&' {
			if i+1 < n && runes[i+1] == '&' {
				sub := strings.TrimSpace(current.String())
				if sub == "" {
					return nil, false
				}
				parts = append(parts, sub)
				current.Reset()
				i += 2
				continue
			}
			return nil, false
		}
		current.WriteRune(char)
		i++
	}
	if quote != 0 || escaped {
		return nil, false
	}
	last := strings.TrimSpace(current.String())
	if last == "" {
		return nil, false
	}
	parts = append(parts, last)
	if len(parts) <= 1 {
		return nil, false
	}
	return parts, true
}

func shellWords(command string) ([]string, bool) {
	words := []string{}
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() > 0 {
			words = append(words, current.String())
			current.Reset()
		}
	}
	for _, char := range command {
		if escaped {
			current.WriteRune(char)
			escaped = false
			continue
		}
		if char == '\\' && quote != '\'' {
			escaped = true
			continue
		}
		if quote != 0 {
			if char == quote {
				quote = 0
			} else {
				current.WriteRune(char)
			}
			continue
		}
		if char == '\'' || char == '"' {
			quote = char
			continue
		}
		if char == ' ' || char == '\t' || char == '\r' || char == '\n' {
			flush()
			continue
		}
		current.WriteRune(char)
	}
	if escaped || quote != 0 {
		return nil, false
	}
	flush()
	return words, true
}

type commandParts struct {
	Words        []string
	CommandIndex int
	Executable   string
	Args         []string
}

var envAssignment = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*=`)

func parseCommand(command string) (*commandParts, bool) {
	words, ok := shellWords(command)
	if !ok {
		return nil, false
	}
	index := 0
	for index < len(words) && envAssignment.MatchString(words[index]) {
		index++
	}
	if index < len(words) && strings.EqualFold(filepath.Base(words[index]), "env") {
		envIndex := index
		index++
		for index < len(words) && (envAssignment.MatchString(words[index]) || strings.HasPrefix(words[index], "-")) {
			index++
		}
		if index >= len(words) {
			return &commandParts{Words: words, CommandIndex: envIndex, Executable: "env", Args: words[envIndex+1:]}, true
		}
	}
	if index >= len(words) {
		return &commandParts{Words: words, CommandIndex: index}, true
	}
	exec := strings.ToLower(filepath.Base(words[index]))
	return &commandParts{Words: words, CommandIndex: index, Executable: exec, Args: words[index+1:]}, true
}

func inside(root, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	value, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	if value == rootAbs {
		return true
	}
	rel, err := filepath.Rel(rootAbs, value)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func crossPlatformAbsolute(value string) bool {
	normalized := strings.ReplaceAll(strings.TrimSpace(value), "\\", "/")
	windowsDrive := len(normalized) >= 3 && normalized[1] == ':' && normalized[2] == '/'
	return strings.HasPrefix(normalized, "/") || windowsDrive
}

func pathCandidate(token string) string {
	raw := token
	if idx := strings.Index(token, "="); idx > 0 && strings.HasPrefix(token, "-") {
		raw = token[idx+1:]
	}
	lower := strings.ToLower(raw)
	for _, prefix := range []string{"http://", "https://", "ws://", "wss://", "ssh://"} {
		if strings.HasPrefix(lower, prefix) {
			return ""
		}
	}
	if raw == "/dev/null" || raw == "NUL" {
		return ""
	}
	if raw == "~" || strings.HasPrefix(raw, "~/") || crossPlatformAbsolute(raw) || raw == ".." || strings.HasPrefix(raw, "../") || strings.Contains(filepath.ToSlash(raw), "/../") {
		return raw
	}
	return ""
}

func explicitPathEscape(command string, ctx Context) string {
	if candidate := explicitPathEscapeParsed(command, ctx); candidate != "" {
		return candidate
	}
	if strings.Contains(command, "\\") {
		return explicitPathEscapeParsed(strings.ReplaceAll(command, "\\", "/"), ctx)
	}
	return ""
}

func explicitPathEscapeParsed(command string, ctx Context) string {
	if ctx.WorkspaceRoot == "" {
		return ""
	}
	parsed, ok := parseCommand(command)
	if !ok || parsed.CommandIndex >= len(parsed.Words) {
		return ""
	}
	cwd := ctx.CWD
	if cwd == "" {
		cwd = ctx.WorkspaceRoot
	}
	tokens := append([]string{parsed.Words[parsed.CommandIndex]}, parsed.Args...)
	home, _ := osUserHomeDir()
	for _, token := range tokens {
		candidate := pathCandidate(token)
		if candidate == "" {
			continue
		}
		if crossPlatformAbsolute(candidate) && !filepath.IsAbs(candidate) {
			return candidate
		}
		expanded := candidate
		if candidate == "~" {
			expanded = home
		} else if strings.HasPrefix(candidate, "~/") {
			expanded = filepath.Join(home, candidate[2:])
		} else if !filepath.IsAbs(candidate) {
			expanded = filepath.Join(cwd, candidate)
		}
		if !inside(ctx.WorkspaceRoot, expanded) {
			return candidate
		}
	}
	return ""
}

// Kept behind a variable to make policy tests deterministic across OSes.
var osUserHomeDir = os.UserHomeDir

func structuredApproval(command string) (key, label string, ok bool) {
	if HasComplexShellComposition(command) {
		return "", "", false
	}
	if subcmds, isChain := splitAndChain(command); isChain {
		normalized := strings.Join(strings.Fields(command), " ")
		redacted := RedactCommand(normalized)
		for _, sub := range subcmds {
			d := Classify(sub, NetworkApproval, Context{})
			if d.Blocked || d.RiskLevel > RiskReview || (d.RequiresApproval && d.ApprovalPolicy != ApprovalRememberable) {
				return "", "", false
			}
		}
		return "workspace-exec:" + hashKey(redacted), redacted, true
	}
	if strings.Contains(command, "&") {
		return "", "", false
	}
	parsed, valid := parseCommand(command)
	if !valid || parsed.Executable == "" {
		return "", "", false
	}
	normalized := strings.Join(strings.Fields(command), " ")
	redacted := RedactCommand(normalized)
	if parsed.Executable == "git" && len(parsed.Args) > 0 {
		sub := strings.ToLower(parsed.Args[0])
		rest := parsed.Args[1:]
		if sub == "commit" {
			return "git.commit", "Git commit in this workspace", true
		}
		if sub == "push" {
			for _, arg := range rest {
				lower := strings.ToLower(arg)
				if lower == "-f" || strings.HasPrefix(lower, "--force") || lower == "--delete" || lower == "--mirror" || lower == "--all" || lower == "--tags" || lower == "--prune" {
					return "", "", false
				}
			}
			positional := []string{}
			for _, arg := range rest {
				if !strings.HasPrefix(arg, "-") {
					positional = append(positional, arg)
				}
			}
			for _, arg := range positional {
				if strings.HasPrefix(arg, ":") {
					return "", "", false
				}
			}
			if len(positional) >= 2 {
				return "git.push:" + positional[0] + ":" + positional[1], "Git push " + positional[0] + " " + positional[1], true
			}
			return "", "", false
		}
		if sub == "tag" || sub == "merge" || sub == "rebase" || sub == "pull" || sub == "fetch" {
			return "git." + sub + ":" + hashKey(redacted), redacted, true
		}
	}
	if dependencyCommand(parsed.Executable, parsed.Args) {
		return "dependency:" + parsed.Executable + ":" + hashKey(redacted), redacted, true
	}
	rawExecutable := ""
	if parsed.CommandIndex < len(parsed.Words) {
		rawExecutable = parsed.Words[parsed.CommandIndex]
	}
	if strings.ContainsAny(rawExecutable, `/\`) || isInterpreter(parsed.Executable) || workspacePackageCommand(parsed.Executable, parsed.Args) {
		return "workspace-exec:" + hashKey(redacted), redacted, true
	}
	return "", "", false
}

func dependencyCommand(exec string, args []string) bool {
	if len(args) == 0 {
		return false
	}
	first := strings.ToLower(args[0])
	if contains([]string{"npm", "pnpm", "yarn", "bun", "pip", "pipx", "poetry", "uv", "cargo", "go"}, exec) && contains([]string{"install", "add", "remove", "uninstall", "update", "upgrade", "get"}, first) {
		return true
	}
	if (exec == "flutter" || exec == "dart") && len(args) > 1 && args[0] == "pub" && contains([]string{"add", "remove", "upgrade", "downgrade", "get"}, strings.ToLower(args[1])) {
		return true
	}
	return exec == "pod" && (first == "install" || first == "update")
}

func isInterpreter(exec string) bool {
	return contains([]string{"node", "python", "python3", "ruby", "perl", "php", "deno", "tsx", "ts-node", "jest", "vitest", "pytest", "mocha", "ava", "bash", "sh", "zsh", "fish", "pwsh", "powershell", "java", "swift", "swiftc", "dotnet", "xcodebuild"}, exec)
}

func workspacePackageCommand(exec string, args []string) bool {
	if exec == "npx" || contains([]string{"make", "just", "task", "gradle", "gradlew", "mvn", "mvnw"}, exec) {
		return true
	}
	if len(args) == 0 {
		return false
	}
	first := strings.ToLower(args[0])
	if contains([]string{"npm", "pnpm", "yarn", "bun"}, exec) && contains([]string{"run", "test", "start", "exec", "x"}, first) {
		return true
	}
	if exec == "cargo" && contains([]string{"run", "test", "build", "bench"}, first) {
		return true
	}
	if exec == "go" && contains([]string{"run", "test", "build", "generate"}, first) {
		return true
	}
	if (exec == "flutter" || exec == "dart") && contains([]string{"run", "test", "build", "compile"}, first) {
		return true
	}
	return exec == "dotnet" && contains([]string{"run", "test", "build", "publish"}, first)
}

func contains(values []string, value string) bool {
	for _, item := range values {
		if item == value {
			return true
		}
	}
	return false
}

func classifyAndChain(command string, subcmds []string, network NetworkPolicy, ctx Context) Decision {
	if network == "" {
		network = NetworkApproval
	}
	normalized := strings.Join(strings.Fields(command), " ")
	redacted := RedactCommand(normalized)
	rank := map[RiskLevel]int{RiskSafe: 0, RiskReview: 1, RiskHigh: 2, RiskCritical: 3, RiskBlocked: 4}
	highestRisk := RiskSafe
	blocked := false
	approval := false
	var allRules []string
	allRememberable := true

	for _, sub := range subcmds {
		d := Classify(sub, network, ctx)
		if rank[d.RiskLevel] > rank[highestRisk] {
			highestRisk = d.RiskLevel
		}
		if d.Blocked {
			blocked = true
		}
		if d.RequiresApproval {
			approval = true
			if d.ApprovalPolicy != ApprovalRememberable {
				allRememberable = false
			}
		}
		allRules = append(allRules, d.MatchedRules...)
	}

	unique := []string{}
	seen := map[string]struct{}{}
	for _, r := range allRules {
		if _, ok := seen[r]; ok {
			continue
		}
		seen[r] = struct{}{}
		unique = append(unique, r)
	}
	reason := "no risky policy rule matched"
	if len(unique) > 0 {
		reason = strings.Join(unique, "; ")
	}

	policy := ApprovalNone
	key, label := "", ""
	if blocked {
		policy = ApprovalBlocked
		highestRisk = RiskBlocked
	} else if approval {
		if highestRisk == RiskReview && allRememberable {
			policy = ApprovalRememberable
			key = "workspace-exec:" + hashKey(redacted)
			label = redacted
		} else {
			policy = ApprovalAlways
		}
	}

	return Decision{
		RiskLevel:        highestRisk,
		MatchedRules:     unique,
		RequiresApproval: !blocked && approval,
		Blocked:          blocked,
		RedactedCommand:  redacted,
		Reason:           reason,
		ApprovalPolicy:   policy,
		ApprovalKey:      key,
		ApprovalLabel:    label,
	}
}

func Classify(command string, network NetworkPolicy, ctx Context) Decision {
	if network == "" {
		network = NetworkApproval
	}
	if subcmds, isChain := splitAndChain(command); isChain {
		return classifyAndChain(command, subcmds, network, ctx)
	}
	normalized := strings.Join(strings.Fields(command), " ")
	rules := []string{}
	risk := RiskSafe
	blocked := false
	approval := false
	rank := map[RiskLevel]int{RiskSafe: 0, RiskReview: 1, RiskHigh: 2, RiskCritical: 3, RiskBlocked: 4}
	hit := func(match bool, reason string, level RiskLevel, block, approve bool) {
		if !match {
			return
		}
		rules = append(rules, reason)
		if rank[level] > rank[risk] {
			risk = level
		}
		blocked = blocked || block
		approval = approval || approve
	}
	lower := strings.ToLower(normalized)
	parsed, parsedOK := parseCommand(command)
	exec := ""
	args := []string{}
	if parsedOK && parsed != nil {
		exec, args = parsed.Executable, parsed.Args
	}

	hit(exec == "sudo", "privilege escalation", RiskBlocked, true, false)
	hit(contains([]string{"shutdown", "reboot", "halt", "diskutil", "mkfs", "fdisk", "gpt", "mount", "umount"}, exec), "system or disk administration", RiskBlocked, true, false)
	hit(strings.Contains(lower, "dd ") && strings.Contains(lower, "of=/dev/"), "raw device write", RiskBlocked, true, false)
	hit(regexp.MustCompile(`(?i)(^|\s)(~/?|/[^\s]*)?\.(ssh|aws|gnupg|gcloud|azure)(/|\s|$)`).MatchString(normalized), "credential directory access", RiskBlocked, true, false)
	hit(regexp.MustCompile(`(?i)\b(security\s+(find|dump|unlock|set)|gh\s+auth\s+token|git\s+credential)`).MatchString(normalized), "credential retrieval", RiskBlocked, true, false)
	if exec == "env" || exec == "printenv" || exec == "set" {
		if len(args) == 0 {
			hit(true, "environment secret enumeration", RiskBlocked, true, false)
		} else if exec == "printenv" && reSensitiveEnv.MatchString(args[0]) {
			hit(true, "sensitive environment variable access", RiskBlocked, true, false)
		}
	}
	for _, match := range regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?`).FindAllStringSubmatch(normalized, -1) {
		if len(match) > 1 && reSensitiveEnv.MatchString(match[1]) {
			hit(true, "sensitive environment variable expansion", RiskBlocked, true, false)
			break
		}
	}

	if escaped := explicitPathEscape(command, ctx); escaped != "" {
		hit(true, "explicit path escapes authorized workspace: "+RedactCommand(escaped), RiskBlocked, true, false)
	} else if ctx.WorkspaceRoot == "" && (strings.HasPrefix(normalized, "../") || strings.Contains(normalized, " ../")) {
		hit(true, "parent-directory traversal", RiskBlocked, true, false)
	}

	if exec == "git" && len(args) > 0 {
		sub := strings.ToLower(args[0])
		rest := strings.ToLower(strings.Join(args[1:], " "))
		destructive := (sub == "commit" && strings.Contains(rest, "--amend")) || (sub == "reset" && strings.Contains(rest, "--hard")) || (sub == "clean" && strings.Contains(rest, "-f")) || (sub == "push" && (strings.Contains(rest, "--force") || strings.Contains(rest, " -f") || strings.Contains(rest, "--delete") || strings.Contains(rest, "--mirror") || strings.Contains(rest, "--all") || strings.Contains(rest, "--tags") || strings.Contains(rest, "--prune"))) || (sub == "restore" && !strings.Contains(rest, "--staged"))
		hit(destructive, "destructive Git action", RiskCritical, false, true)
		hit(sub == "remote" && len(args) > 1 && contains([]string{"add", "remove", "rename", "set-url"}, strings.ToLower(args[1])), "Git remote configuration change", RiskCritical, false, true)
		hit(sub == "config" && !(strings.Contains(rest, "--get") || strings.Contains(rest, "--list") || strings.Contains(rest, " -l")), "Git configuration change", RiskCritical, false, true)
		hit(contains([]string{"commit", "push", "tag", "merge", "rebase", "pull", "fetch"}, sub), "Git write action", RiskReview, false, true)
	}
	if exec == "rm" && len(args) > 0 {
		for _, arg := range args {
			if strings.HasPrefix(arg, "-") && strings.Contains(arg, "r") {
				hit(true, "recursive delete", RiskCritical, false, true)
				break
			}
		}
	}
	if contains([]string{"prisma", "typeorm", "sequelize", "knex", "alembic", "rails"}, exec) && strings.Contains(lower, "migrat") {
		hit(true, "database migration", RiskCritical, false, true)
	}
	if contains([]string{"npm", "pnpm", "yarn", "bun"}, exec) && len(args) > 0 && contains([]string{"publish", "login", "logout"}, strings.ToLower(args[0])) {
		hit(true, "package registry or account action", RiskCritical, false, true)
	}
	if contains([]string{"chmod", "chown", "launchctl", "systemctl", "service"}, exec) {
		hit(true, "permission or service modification", RiskCritical, false, true)
	}
	if isInterpreter(exec) && len(args) > 0 && contains([]string{"-e", "-c", "-command", "/c"}, strings.ToLower(args[0])) {
		hit(true, "inline interpreter can bypass workspace path analysis", RiskCritical, false, true)
	}

	if workspacePackageCommand(exec, args) || (parsedOK && parsed != nil && parsed.CommandIndex < len(parsed.Words) && strings.ContainsAny(parsed.Words[parsed.CommandIndex], `/\`)) {
		hit(true, "workspace code execution", RiskReview, false, true)
	}
	if dependencyCommand(exec, args) {
		hit(true, "dependency or toolchain change", RiskReview, false, true)
	}

	networkExec := contains([]string{"curl", "wget", "ssh", "scp", "sftp", "ftp", "nc", "ncat", "telnet"}, exec)
	if networkExec {
		if network == NetworkDeny {
			hit(true, "network access denied by policy", RiskBlocked, true, false)
		} else if network == NetworkApproval {
			hit(true, "network or remote command", RiskCritical, false, true)
		}
	}
	if !blocked && HasShellComposition(command) {
		rules = append(rules, "composed shell command requires one-time review")
		if rank[RiskCritical] > rank[risk] {
			risk = RiskCritical
		}
		approval = true
	}
	if blocked {
		risk = RiskBlocked
	}
	unique := []string{}
	seen := map[string]struct{}{}
	for _, rule := range rules {
		if _, ok := seen[rule]; ok {
			continue
		}
		seen[rule] = struct{}{}
		unique = append(unique, rule)
	}
	reason := "no risky policy rule matched"
	if len(unique) > 0 {
		reason = strings.Join(unique, "; ")
	}
	policy := ApprovalNone
	key, label := "", ""
	if blocked {
		policy = ApprovalBlocked
	} else if approval {
		if risk == RiskReview {
			if k, l, ok := structuredApproval(command); ok {
				policy, key, label = ApprovalRememberable, k, l
			} else {
				policy = ApprovalAlways
			}
		} else {
			policy = ApprovalAlways
		}
	}
	return Decision{RiskLevel: risk, MatchedRules: unique, RequiresApproval: !blocked && approval, Blocked: blocked, RedactedCommand: RedactCommand(normalized), Reason: reason, ApprovalPolicy: policy, ApprovalKey: key, ApprovalLabel: label}
}

func ClassifyGitWrite(operation, detail string) Decision {
	return Classify(strings.TrimSpace("git "+operation+" "+detail), NetworkApproval, Context{})
}

func SortRules(rules []string) []string {
	out := append([]string(nil), rules...)
	sort.Strings(out)
	return out
}

func Platform() string { return runtime.GOOS }

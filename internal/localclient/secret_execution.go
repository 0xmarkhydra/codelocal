package localclient

import (
	"crypto/sha256"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	processmgr "github.com/0xmarkhydra/codelocal/internal/process"
	"github.com/0xmarkhydra/codelocal/internal/security"
)

const (
	maxOpaqueSecretBindings = 32
	maxOpaqueSourceBytes    = 2 * 1024 * 1024
	maxOpaqueSourceFiles    = 12
)

var opaqueHTTPURL = regexp.MustCompile(`https?://[^\s"'` + "`" + `<>]+`)

func normalizeOpaqueSecretNames(values []string) ([]string, error) {
	if len(values) > maxOpaqueSecretBindings {
		return nil, fmt.Errorf("at most %d runtime secrets may be requested", maxOpaqueSecretBindings)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		name := strings.TrimSpace(raw)
		if name == "" || !processmgr.CanInjectEnvKey(name) {
			return nil, fmt.Errorf("runtime secret name %q is not allowed", name)
		}
		if _, exists := seen[name]; exists {
			continue
		}
		seen[name] = struct{}{}
		out = append(out, name)
	}
	sort.Strings(out)
	return out, nil
}

func normalizeDeclaredNetworkHosts(values []string) ([]string, error) {
	if len(values) > maxOpaqueSecretBindings {
		return nil, fmt.Errorf("at most %d network hosts may be declared", maxOpaqueSecretBindings)
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, raw := range values {
		host := strings.ToLower(strings.TrimSpace(raw))
		if host == "" || len(host) > 253 || strings.Contains(host, "://") || strings.ContainsAny(host, "/\\@:\t\r\n ") || strings.HasPrefix(host, ".") || strings.HasSuffix(host, ".") {
			return nil, fmt.Errorf("network host %q must be a hostname without scheme, path, credentials, or port", raw)
		}
		if _, exists := seen[host]; exists {
			continue
		}
		seen[host] = struct{}{}
		out = append(out, host)
	}
	sort.Strings(out)
	return out, nil
}

func addDecisionRule(rules []string, rule string) []string {
	for _, existing := range rules {
		if existing == rule {
			return rules
		}
	}
	return append(rules, rule)
}

func opaqueSecretDecision(command string, base security.Decision, secrets, hosts []string) security.Decision {
	if base.Blocked || len(secrets) == 0 {
		return base
	}
	const rule = "opaque local secret execution"
	base.MatchedRules = addDecisionRule(base.MatchedRules, rule)
	if base.Reason == "" || base.Reason == "no risky policy rule matched" {
		base.Reason = rule
	} else if !strings.Contains(base.Reason, rule) {
		base.Reason += "; " + rule
	}
	if base.RiskLevel == security.RiskSafe {
		base.RiskLevel = security.RiskReview
	}
	base.RequiresApproval = true
	label := "Use secrets " + strings.Join(secrets, ", ") + " for " + security.RedactCommand(command)
	if len(hosts) > 0 {
		label += " -> " + strings.Join(hosts, ", ")
	}
	base.ApprovalLabel = label
	if base.ApprovalPolicy == security.ApprovalNone || base.ApprovalPolicy == security.ApprovalRememberable {
		payload := security.RedactCommand(command) + "\x00" + strings.Join(secrets, "\x00") + "\x00" + strings.Join(hosts, "\x00")
		sum := sha256.Sum256([]byte(payload))
		base.ApprovalPolicy = security.ApprovalRememberable
		base.ApprovalKey = fmt.Sprintf("secret-exec:%x", sum[:12])
	}
	return base
}

func prepareOpaqueSecretExecution(args map[string]any, command string, base security.Decision) ([]string, security.Decision, error) {
	secrets, err := normalizeOpaqueSecretNames(stringSlice(args["secrets"]))
	if err != nil {
		return nil, base, err
	}
	hosts, err := normalizeDeclaredNetworkHosts(stringSlice(args["networkHosts"]))
	if err != nil {
		return nil, base, err
	}
	return secrets, opaqueSecretDecision(command, base, secrets, hosts), nil
}

func mergeOpaqueStrings(values ...[]string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, group := range values {
		for _, value := range group {
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
	}
	sort.Strings(out)
	return out
}

func shellLikeTokens(value string) []string {
	out := []string{}
	var current strings.Builder
	var quote rune
	escaped := false
	flush := func() {
		if current.Len() == 0 {
			return
		}
		out = append(out, current.String())
		current.Reset()
	}
	for _, char := range value {
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
	flush()
	return out
}

func opaquePathInside(root, candidate string) bool {
	rootAbs, err := filepath.Abs(root)
	if err != nil {
		return false
	}
	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return false
	}
	rel, err := filepath.Rel(rootAbs, candidateAbs)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func opaqueCandidatePath(root, base, token string) string {
	token = strings.Trim(strings.TrimSpace(token), "'\"(),[]{}")
	if index := strings.Index(token, "="); index > 0 && strings.HasPrefix(token, "-") {
		token = token[index+1:]
	}
	if token == "" || strings.HasPrefix(token, "-") || strings.Contains(token, "://") {
		return ""
	}
	candidate := token
	if !filepath.IsAbs(candidate) {
		candidate = filepath.Join(base, filepath.FromSlash(candidate))
	}
	candidate = filepath.Clean(candidate)
	if !opaquePathInside(root, candidate) {
		return ""
	}
	rel, err := filepath.Rel(root, candidate)
	if err != nil || security.IsSensitivePath(filepath.ToSlash(rel)) {
		return ""
	}
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() || info.Size() > maxOpaqueSourceBytes {
		return ""
	}
	return candidate
}

func opaqueSourceDocuments(command, root, cwd string) []string {
	sources := []string{command}
	queue := []struct {
		base string
		text string
	}{{base: cwd, text: command}}
	seenPaths := map[string]struct{}{}
	for len(queue) > 0 && len(seenPaths) < maxOpaqueSourceFiles {
		current := queue[0]
		queue = queue[1:]
		for _, token := range shellLikeTokens(current.text) {
			path := opaqueCandidatePath(root, current.base, token)
			if path == "" {
				continue
			}
			if _, exists := seenPaths[path]; exists {
				continue
			}
			seenPaths[path] = struct{}{}
			raw, err := os.ReadFile(path)
			if err != nil {
				continue
			}
			text := string(raw)
			sources = append(sources, text)
			queue = append(queue, struct {
				base string
				text string
			}{base: filepath.Dir(path), text: text})
			if len(seenPaths) >= maxOpaqueSourceFiles {
				break
			}
		}
	}
	return sources
}

func inferOpaqueHosts(sources []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, source := range sources {
		for _, raw := range opaqueHTTPURL.FindAllString(source, -1) {
			parsed, err := url.Parse(strings.TrimRight(raw, ".,;:)"))
			if err != nil {
				continue
			}
			host := strings.ToLower(parsed.Hostname())
			if host == "" {
				continue
			}
			if _, exists := seen[host]; exists {
				continue
			}
			seen[host] = struct{}{}
			out = append(out, host)
		}
	}
	sort.Strings(out)
	if len(out) > maxOpaqueSecretBindings {
		out = out[:maxOpaqueSecretBindings]
	}
	return out
}

func (e *Engine) inferOpaqueSecretUse(command, root, cwd string) ([]string, []string) {
	e.mu.Lock()
	secretNames := make([]string, 0, len(e.runtimeSecrets))
	for name := range e.runtimeSecrets {
		if processmgr.CanInjectEnvKey(name) {
			secretNames = append(secretNames, name)
		}
	}
	e.mu.Unlock()
	if len(secretNames) == 0 {
		return nil, nil
	}
	sort.Strings(secretNames)
	sources := opaqueSourceDocuments(command, root, cwd)
	matched := []string{}
	for _, name := range secretNames {
		for _, source := range sources {
			if strings.Contains(source, name) {
				matched = append(matched, name)
				break
			}
		}
	}
	if len(matched) > maxOpaqueSecretBindings {
		matched = matched[:maxOpaqueSecretBindings]
	}
	if len(matched) == 0 {
		return nil, nil
	}
	return matched, inferOpaqueHosts(sources)
}

func (e *Engine) prepareOpaqueSecretExecution(args map[string]any, command, root, cwd string, base security.Decision) ([]string, security.Decision, error) {
	explicitSecrets, err := normalizeOpaqueSecretNames(stringSlice(args["secrets"]))
	if err != nil {
		return nil, base, err
	}
	explicitHosts, err := normalizeDeclaredNetworkHosts(stringSlice(args["networkHosts"]))
	if err != nil {
		return nil, base, err
	}
	inferredSecrets, inferredHosts := e.inferOpaqueSecretUse(command, root, cwd)
	secrets := mergeOpaqueStrings(explicitSecrets, inferredSecrets)
	if len(secrets) > maxOpaqueSecretBindings {
		return nil, base, fmt.Errorf("at most %d runtime secrets may be requested", maxOpaqueSecretBindings)
	}
	hosts := mergeOpaqueStrings(explicitHosts, inferredHosts)
	return secrets, opaqueSecretDecision(command, base, secrets, hosts), nil
}

package security

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/url"
	"regexp"
	"sort"
	"strings"
)

type PolicyEffect string
type SecretMode string
type SecretOutput string

const (
	EffectAllow  PolicyEffect = "allow"
	EffectPrompt PolicyEffect = "prompt"
	EffectDeny   PolicyEffect = "deny"

	SecretNone      SecretMode = "none"
	SecretConsume   SecretMode = "consume_internal"
	SecretExpose    SecretMode = "expose"
	SecretEnumerate SecretMode = "enumerate"

	SecretOutputNone     SecretOutput = "none"
	SecretOutputRedacted SecretOutput = "redacted"
	SecretOutputRaw      SecretOutput = "raw"
)

var ErrInvalidPolicyKernel = errors.New("invalid policy kernel configuration")

type PolicyRequest struct {
	ActorID       string       `json:"actorId"`
	AgentID       string       `json:"agentId,omitempty"`
	AgentRole     string       `json:"agentRole,omitempty"`
	Action        string       `json:"action"`
	Tool          string       `json:"tool,omitempty"`
	Command       string       `json:"command,omitempty"`
	Path          string       `json:"path,omitempty"`
	NetworkTarget string       `json:"networkTarget,omitempty"`
	WorkspaceRoot string       `json:"workspaceRoot,omitempty"`
	CWD           string       `json:"cwd,omitempty"`
	SecretMode    SecretMode   `json:"secretMode,omitempty"`
	SecretNames   []string     `json:"secretNames,omitempty"`
	SecretOutput  SecretOutput `json:"secretOutput,omitempty"`
}

type PolicyMatch struct {
	Action            string     `json:"action,omitempty"`
	Tool              string     `json:"tool,omitempty"`
	AgentRole         string     `json:"agentRole,omitempty"`
	PathPrefix        string     `json:"pathPrefix,omitempty"`
	NetworkHostSuffix string     `json:"networkHostSuffix,omitempty"`
	SecretMode        SecretMode `json:"secretMode,omitempty"`
	MinRisk           RiskLevel  `json:"minRisk,omitempty"`
}

type PolicyRule struct {
	ID       string       `json:"id"`
	Priority int          `json:"priority,omitempty"`
	Effect   PolicyEffect `json:"effect"`
	Match    PolicyMatch  `json:"match"`
	Reason   string       `json:"reason,omitempty"`
}

type PolicyDecision struct {
	Effect           PolicyEffect `json:"effect"`
	RiskLevel        RiskLevel    `json:"riskLevel"`
	RuleIDs          []string     `json:"ruleIds,omitempty"`
	ReasonCodes      []string     `json:"reasonCodes,omitempty"`
	RequiresApproval bool         `json:"requiresApproval"`
	Blocked          bool         `json:"blocked"`
	RedactedCommand  string       `json:"redactedCommand,omitempty"`
	SecretMode       SecretMode   `json:"secretMode"`
	ApprovalKey      string       `json:"approvalKey,omitempty"`
}

type PolicyKernel struct {
	rules   []PolicyRule
	network NetworkPolicy
}

func NewPolicyKernel(rules []PolicyRule, network NetworkPolicy) (*PolicyKernel, error) {
	if network == "" {
		network = NetworkApproval
	}
	if network != NetworkDeny && network != NetworkApproval && network != NetworkAllow {
		return nil, ErrInvalidPolicyKernel
	}
	seen := map[string]struct{}{}
	copyRules := append([]PolicyRule(nil), rules...)
	for i := range copyRules {
		copyRules[i].ID = strings.TrimSpace(copyRules[i].ID)
		copyRules[i].Reason = strings.TrimSpace(copyRules[i].Reason)
		if copyRules[i].ID == "" || !validPolicyEffect(copyRules[i].Effect) || !validRuleMatch(copyRules[i].Match) {
			return nil, ErrInvalidPolicyKernel
		}
		if _, exists := seen[copyRules[i].ID]; exists {
			return nil, ErrInvalidPolicyKernel
		}
		seen[copyRules[i].ID] = struct{}{}
	}
	sort.SliceStable(copyRules, func(i, j int) bool {
		if copyRules[i].Priority != copyRules[j].Priority {
			return copyRules[i].Priority > copyRules[j].Priority
		}
		return copyRules[i].ID < copyRules[j].ID
	})
	return &PolicyKernel{rules: copyRules, network: network}, nil
}

// Evaluate combines the mature command classifier, path/network policy, secret
// intent and declarative rules. Every downstream layer is monotonic: it may
// change ALLOW->PROMPT->DENY, but never loosen a stricter earlier decision.
func (k *PolicyKernel) Evaluate(input PolicyRequest) PolicyDecision {
	req := normalizePolicyRequest(input)
	decision := PolicyDecision{Effect: EffectAllow, RiskLevel: RiskSafe, SecretMode: effectiveSecretMode(req), ReasonCodes: []string{}}
	if req.Command != "" {
		legacy := Classify(req.Command, k.network, Context{WorkspaceRoot: req.WorkspaceRoot, CWD: req.CWD})
		decision.RiskLevel = legacy.RiskLevel
		decision.RedactedCommand = legacy.RedactedCommand
		decision.ReasonCodes = append(decision.ReasonCodes, legacy.MatchedRules...)
		if legacy.Blocked {
			decision.Effect = EffectDeny
		} else if legacy.RequiresApproval {
			decision.Effect = EffectPrompt
		}
	}
	if req.Path != "" && IsSensitivePath(req.Path) {
		tightenPolicy(&decision, EffectDeny, RiskBlocked, "sensitive_path_access")
	}
	if req.NetworkTarget != "" {
		switch k.network {
		case NetworkDeny:
			tightenPolicy(&decision, EffectDeny, RiskBlocked, "network_denied")
		case NetworkApproval:
			tightenPolicy(&decision, EffectPrompt, RiskCritical, "network_requires_approval")
		}
	}
	applySecretPolicy(&decision, req)
	for _, rule := range k.rules {
		if !ruleMatches(rule.Match, req, decision.RiskLevel) {
			continue
		}
		decision.RuleIDs = append(decision.RuleIDs, rule.ID)
		reason := rule.Reason
		if reason == "" {
			reason = "rule:" + rule.ID
		}
		tightenPolicy(&decision, rule.Effect, decision.RiskLevel, reason)
	}
	decision.RuleIDs = uniquePolicyStrings(decision.RuleIDs)
	decision.ReasonCodes = uniquePolicyStrings(decision.ReasonCodes)
	decision.Blocked = decision.Effect == EffectDeny
	decision.RequiresApproval = decision.Effect == EffectPrompt
	if decision.RequiresApproval {
		decision.ApprovalKey = policyApprovalKey(req, decision.SecretMode)
	}
	return decision
}

func applySecretPolicy(decision *PolicyDecision, req PolicyRequest) {
	switch decision.SecretMode {
	case SecretExpose:
		tightenPolicy(decision, EffectDeny, RiskBlocked, "secret_exposure_blocked")
	case SecretEnumerate:
		tightenPolicy(decision, EffectDeny, RiskBlocked, "secret_enumeration_blocked")
	case SecretConsume:
		if len(req.SecretNames) == 0 {
			tightenPolicy(decision, EffectDeny, RiskBlocked, "secret_consumption_missing_scope")
			return
		}
		if req.SecretOutput == SecretOutputRaw {
			tightenPolicy(decision, EffectDeny, RiskBlocked, "secret_raw_output_blocked")
			return
		}
		tightenPolicy(decision, EffectPrompt, RiskReview, "secret_internal_consumption_requires_capability")
	}
}

func effectiveSecretMode(req PolicyRequest) SecretMode {
	explicit := req.SecretMode
	if explicit == "" {
		explicit = SecretNone
	}
	detected := detectCommandSecretMode(req.Command)
	if secretModeRank(detected) > secretModeRank(explicit) {
		return detected
	}
	return explicit
}

func detectCommandSecretMode(command string) SecretMode {
	lower := strings.ToLower(command)
	if lower == "" {
		return SecretNone
	}
	parsed, ok := parseCommand(command)
	if ok && parsed != nil {
		if parsed.Executable == "env" || parsed.Executable == "set" || (parsed.Executable == "printenv" && len(parsed.Args) == 0) {
			return SecretEnumerate
		}
		if parsed.Executable == "printenv" && len(parsed.Args) > 0 && IsSensitiveEnvName(parsed.Args[0]) {
			return SecretExpose
		}
	}
	for _, match := range regexpSensitiveEnvRefs(command) {
		if IsSensitiveEnvName(match) {
			return SecretExpose
		}
	}
	if strings.Contains(lower, "process.env.") || strings.Contains(lower, "os.environ[") || strings.Contains(lower, "os.getenv(") {
		if containsOutputSink(lower) {
			return SecretExpose
		}
	}
	return SecretNone
}

func regexpSensitiveEnvRefs(command string) []string {
	matches := regexpEnvExpansion.FindAllStringSubmatch(command, -1)
	out := make([]string, 0, len(matches))
	for _, match := range matches {
		if len(match) > 1 {
			out = append(out, match[1])
		}
	}
	return out
}

func containsOutputSink(lower string) bool {
	for _, sink := range []string{"console.log", "console.error", "print(", "printf(", "fmt.println", "fmt.printf", "sys.stdout", "process.stdout", "echo "} {
		if strings.Contains(lower, sink) {
			return true
		}
	}
	return false
}

func normalizePolicyRequest(req PolicyRequest) PolicyRequest {
	req.ActorID = strings.TrimSpace(req.ActorID)
	req.AgentID = strings.TrimSpace(req.AgentID)
	req.AgentRole = strings.ToLower(strings.TrimSpace(req.AgentRole))
	req.Action = strings.ToLower(strings.TrimSpace(req.Action))
	req.Tool = strings.ToLower(strings.TrimSpace(req.Tool))
	req.Command = strings.TrimSpace(req.Command)
	req.Path = filepathSlash(req.Path)
	req.NetworkTarget = strings.TrimSpace(req.NetworkTarget)
	req.WorkspaceRoot = strings.TrimSpace(req.WorkspaceRoot)
	req.CWD = strings.TrimSpace(req.CWD)
	if req.SecretMode == "" {
		req.SecretMode = SecretNone
	}
	if req.SecretOutput == "" {
		req.SecretOutput = SecretOutputNone
	}
	names := make([]string, 0, len(req.SecretNames))
	for _, name := range req.SecretNames {
		name = strings.TrimSpace(name)
		if name != "" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	req.SecretNames = uniquePolicyStrings(names)
	return req
}

func tightenPolicy(decision *PolicyDecision, effect PolicyEffect, risk RiskLevel, reason string) {
	if policyEffectRank(effect) > policyEffectRank(decision.Effect) {
		decision.Effect = effect
	}
	if riskRank(risk) > riskRank(decision.RiskLevel) {
		decision.RiskLevel = risk
	}
	if strings.TrimSpace(reason) != "" {
		decision.ReasonCodes = append(decision.ReasonCodes, strings.TrimSpace(reason))
	}
}

func ruleMatches(match PolicyMatch, req PolicyRequest, risk RiskLevel) bool {
	if match.Action != "" && strings.ToLower(strings.TrimSpace(match.Action)) != req.Action {
		return false
	}
	if match.Tool != "" && strings.ToLower(strings.TrimSpace(match.Tool)) != req.Tool {
		return false
	}
	if match.AgentRole != "" && strings.ToLower(strings.TrimSpace(match.AgentRole)) != req.AgentRole {
		return false
	}
	if match.PathPrefix != "" && !strings.HasPrefix(req.Path, filepathSlash(match.PathPrefix)) {
		return false
	}
	if match.NetworkHostSuffix != "" && !strings.HasSuffix(networkHost(req.NetworkTarget), strings.ToLower(strings.TrimSpace(match.NetworkHostSuffix))) {
		return false
	}
	if match.SecretMode != "" && match.SecretMode != effectiveSecretMode(req) {
		return false
	}
	if match.MinRisk != "" && riskRank(risk) < riskRank(match.MinRisk) {
		return false
	}
	return true
}

func validRuleMatch(match PolicyMatch) bool {
	if match.SecretMode != "" && secretModeRank(match.SecretMode) == 0 {
		return false
	}
	if match.MinRisk != "" && riskRank(match.MinRisk) < 0 {
		return false
	}
	return true
}

func validPolicyEffect(effect PolicyEffect) bool {
	return effect == EffectAllow || effect == EffectPrompt || effect == EffectDeny
}

func policyEffectRank(effect PolicyEffect) int {
	switch effect {
	case EffectAllow:
		return 0
	case EffectPrompt:
		return 1
	case EffectDeny:
		return 2
	default:
		return -1
	}
}

func secretModeRank(mode SecretMode) int {
	switch mode {
	case SecretNone:
		return 1
	case SecretConsume:
		return 2
	case SecretExpose:
		return 3
	case SecretEnumerate:
		return 4
	default:
		return 0
	}
}

func riskRank(risk RiskLevel) int {
	switch risk {
	case RiskSafe:
		return 0
	case RiskReview:
		return 1
	case RiskHigh:
		return 2
	case RiskCritical:
		return 3
	case RiskBlocked:
		return 4
	default:
		return -1
	}
}

func networkHost(target string) string {
	value := strings.TrimSpace(target)
	if value == "" {
		return ""
	}
	if parsed, err := url.Parse(value); err == nil && parsed.Hostname() != "" {
		return strings.ToLower(parsed.Hostname())
	}
	if index := strings.IndexByte(value, ':'); index > 0 {
		value = value[:index]
	}
	return strings.ToLower(strings.TrimSpace(value))
}

func policyApprovalKey(req PolicyRequest, mode SecretMode) string {
	parts := []string{req.ActorID, req.AgentID, req.Action, req.Tool, req.Path, networkHost(req.NetworkTarget), string(mode), RedactCommand(req.Command)}
	parts = append(parts, req.SecretNames...)
	sum := sha256.Sum256([]byte(strings.Join(parts, "\x00")))
	return "policy:" + hex.EncodeToString(sum[:])[:24]
}

func uniquePolicyStrings(values []string) []string {
	seen := map[string]struct{}{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
	}
	return out
}

func filepathSlash(value string) string {
	return strings.TrimPrefix(strings.ReplaceAll(strings.TrimSpace(value), "\\", "/"), "./")
}

var regexpEnvExpansion = regexp.MustCompile(`\$\{?([A-Za-z_][A-Za-z0-9_]*)\}?`)

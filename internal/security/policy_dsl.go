package security

import (
	"errors"
	"fmt"
	"strconv"
	"strings"
)

var ErrInvalidPolicyDSL = errors.New("invalid policy dsl")

// ParsePolicyDSL compiles a small declarative rule document into PolicyRule
// values accepted by NewPolicyKernel. The grammar is intentionally tiny and
// strict: unknown effects, selectors or malformed values are errors, never
// silent accepts.
//
//	# comment lines and blank lines are ignored
//	allow tool:edit path:/workspace/ reason:"edits allowed" priority:10
//	deny secret:expose
//	prompt host:example.com risk:critical
//
// Supported selectors: action, tool, role, path (prefix), host (suffix),
// secret (none|consume_internal|expose|enumerate) and risk (safe|...).
func ParsePolicyDSL(input string) ([]PolicyRule, error) {
	rules := []PolicyRule{}
	seen := map[string]struct{}{}
	for lineNo, raw := range strings.Split(input, "\n") {
		fields, err := splitDSLFields(raw)
		if err != nil {
			return nil, dslError(lineNo+1, err.Error())
		}
		if len(fields) == 0 {
			continue
		}
		rule, err := parseDSLRule(lineNo+1, fields)
		if err != nil {
			return nil, err
		}
		if _, dup := seen[rule.ID]; dup {
			return nil, dslError(lineNo+1, "duplicate rule id "+strconv.Quote(rule.ID))
		}
		seen[rule.ID] = struct{}{}
		rules = append(rules, rule)
	}
	return rules, nil
}

func dslError(line int, msg string) error {
	return fmt.Errorf("%w: line %d: %s", ErrInvalidPolicyDSL, line, msg)
}

// splitDSLFields splits a line on whitespace outside double quotes. A #
// starting a field begins a comment. Unterminated quotes are an error.
func splitDSLFields(line string) ([]string, error) {
	fields := []string{}
	var cur strings.Builder
	inQuotes := false
	flush := func() {
		if cur.Len() > 0 {
			fields = append(fields, cur.String())
			cur.Reset()
		}
	}
	for _, r := range line {
		switch {
		case r == '#' && !inQuotes && cur.Len() == 0:
			flush()
			return fields, nil
		case r == '"':
			inQuotes = !inQuotes
			cur.WriteRune(r)
		case (r == ' ' || r == '\t') && !inQuotes:
			flush()
		default:
			cur.WriteRune(r)
		}
	}
	if inQuotes {
		return nil, errors.New("unterminated quoted string")
	}
	flush()
	return fields, nil
}

func parseDSLRule(line int, fields []string) (PolicyRule, error) {
	effect, ok := dslEffect(fields[0])
	if !ok {
		return PolicyRule{}, dslError(line, "unknown effect "+strconv.Quote(fields[0]))
	}
	rule := PolicyRule{ID: fmt.Sprintf("dsl-%d", line), Effect: effect}
	used := map[string]struct{}{}
	for _, field := range fields[1:] {
		key, value, found := strings.Cut(field, ":")
		if !found {
			return PolicyRule{}, dslError(line, "malformed selector "+strconv.Quote(field))
		}
		key = strings.ToLower(strings.TrimSpace(key))
		if _, dup := used[key]; dup {
			return PolicyRule{}, dslError(line, "duplicate selector "+strconv.Quote(key))
		}
		used[key] = struct{}{}
		switch key {
		case "action":
			rule.Match.Action = value
		case "tool":
			rule.Match.Tool = value
		case "role":
			rule.Match.AgentRole = value
		case "path":
			rule.Match.PathPrefix = value
		case "host":
			rule.Match.NetworkHostSuffix = value
		case "secret":
			mode, ok := dslSecretMode(value)
			if !ok {
				return PolicyRule{}, dslError(line, "unknown secret mode "+strconv.Quote(value))
			}
			rule.Match.SecretMode = mode
		case "risk":
			level, ok := dslRiskLevel(value)
			if !ok {
				return PolicyRule{}, dslError(line, "unknown risk level "+strconv.Quote(value))
			}
			rule.Match.MinRisk = level
		case "reason":
			reason, err := dslUnquote(value)
			if err != nil {
				return PolicyRule{}, dslError(line, err.Error())
			}
			rule.Reason = reason
		case "priority":
			priority, err := strconv.Atoi(value)
			if err != nil {
				return PolicyRule{}, dslError(line, "invalid priority "+strconv.Quote(value))
			}
			rule.Priority = priority
		case "id":
			if strings.TrimSpace(value) == "" {
				return PolicyRule{}, dslError(line, "empty rule id")
			}
			rule.ID = value
		default:
			return PolicyRule{}, dslError(line, "unknown selector "+strconv.Quote(key))
		}
	}
	return rule, nil
}

func dslEffect(raw string) (PolicyEffect, bool) {
	switch PolicyEffect(strings.ToLower(strings.TrimSpace(raw))) {
	case EffectAllow, EffectPrompt, EffectDeny:
		return PolicyEffect(strings.ToLower(strings.TrimSpace(raw))), true
	default:
		return "", false
	}
}

func dslSecretMode(raw string) (SecretMode, bool) {
	switch SecretMode(strings.ToLower(strings.TrimSpace(raw))) {
	case SecretNone, SecretConsume, SecretExpose, SecretEnumerate:
		return SecretMode(strings.ToLower(strings.TrimSpace(raw))), true
	default:
		return "", false
	}
}

func dslRiskLevel(raw string) (RiskLevel, bool) {
	level := RiskLevel(strings.ToUpper(strings.TrimSpace(raw)))
	switch level {
	case RiskSafe, RiskReview, RiskHigh, RiskCritical, RiskBlocked:
		return level, true
	default:
		return "", false
	}
}

func dslUnquote(raw string) (string, error) {
	if len(raw) >= 2 && strings.HasPrefix(raw, "\"") && strings.HasSuffix(raw, "\"") {
		return raw[1 : len(raw)-1], nil
	}
	return "", errors.New("reason must be double-quoted")
}

package projectbrain

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

const (
	DefaultRuleContextBudget = 12000
	LegacyRuleContextBudget  = 8000
	MaxCompiledRules         = 24
)

type CompiledRule struct {
	ID            string        `json:"id"`
	Text          string        `json:"text"`
	Authority     RuleAuthority `json:"authority"`
	AuthorityRank int           `json:"authorityRank"`
	Provider      string        `json:"provider"`
	SourcePath    string        `json:"sourcePath"`
	ScopePath     string        `json:"scopePath"`
	Required      bool          `json:"required"`
	Lane          string        `json:"lane"`
	LocalOnly     bool          `json:"localOnly,omitempty"`
	Trust         string        `json:"trust"`
}

type ContextBudget struct {
	MaxChars          int `json:"maxChars"`
	UsedChars         int `json:"usedChars"`
	InputRules        int `json:"inputRules"`
	CandidateRules    int `json:"candidateRules"`
	DuplicateRules    int `json:"duplicateRules"`
	InputChars        int `json:"inputChars"`
	DeduplicatedChars int `json:"deduplicatedChars"`
	SelectedRules     int `json:"selectedRules"`
	RequiredRules     int `json:"requiredRules"`
	RelevantRules     int `json:"relevantRules"`
	DroppedRules      int `json:"droppedRules"`
	DroppedRequired   int `json:"droppedRequired"`
}

type ContextPacket struct {
	Version                int            `json:"version"`
	Fingerprint            string         `json:"fingerprint"`
	RuleFingerprint        string         `json:"ruleFingerprint"`
	Targets                []string       `json:"targets,omitempty"`
	EffectiveRules         []CompiledRule `json:"effectiveRules"`
	Conflicts              []RuleConflict `json:"conflicts,omitempty"`
	Budget                 ContextBudget  `json:"budget"`
	Truncated              bool           `json:"truncated"`
	MandatoryOverflow      bool           `json:"mandatoryOverflow"`
	OmittedRequiredRuleIDs []string       `json:"omittedRequiredRuleIds,omitempty"`
	MutationAllowed        bool           `json:"mutationAllowed"`
	SecurityBoundary       string         `json:"securityBoundary"`
}

func truncateRuleText(value string, maxRunes int) string {
	value = strings.Join(strings.Fields(value), " ")
	if maxRunes <= 0 {
		return ""
	}
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return strings.TrimSpace(string(runes[:maxRunes])) + "…"
}

func normalizedRuleTextKey(value string) string {
	value = strings.ToLower(strings.Join(strings.Fields(value), " "))
	return strings.TrimSpace(strings.TrimRight(value, ".;:"))
}

func preferCompactionRule(left, right CanonicalRule) bool {
	if left.AuthorityRank != right.AuthorityRank {
		return left.AuthorityRank > right.AuthorityRank
	}
	if left.Required != right.Required {
		return left.Required
	}
	leftDepth := strings.Count(left.ScopePath, "/")
	rightDepth := strings.Count(right.ScopePath, "/")
	if leftDepth != rightDepth {
		return leftDepth > rightDepth
	}
	if left.SourcePath != right.SourcePath {
		return left.SourcePath < right.SourcePath
	}
	return left.ID < right.ID
}

func compactRules(rules []CanonicalRule) (compacted []CanonicalRule, duplicateRules, inputChars, deduplicatedChars int) {
	byText := map[string]CanonicalRule{}
	for _, rule := range rules {
		text := strings.TrimSpace(rule.Text)
		inputChars += len([]rune(text))
		key := normalizedRuleTextKey(text)
		if key == "" {
			continue
		}
		existing, found := byText[key]
		if !found {
			byText[key] = rule
			continue
		}
		duplicateRules++
		deduplicatedChars += len([]rune(text))
		required := existing.Required || rule.Required
		if preferCompactionRule(rule, existing) {
			existing = rule
		}
		existing.Required = required
		byText[key] = existing
	}
	compacted = make([]CanonicalRule, 0, len(byText))
	for _, rule := range byText {
		compacted = append(compacted, rule)
	}
	sort.SliceStable(compacted, func(i, j int) bool { return preferCompactionRule(compacted[i], compacted[j]) })
	return compacted, duplicateRules, inputChars, deduplicatedChars
}

func requiredRuleChars(rules []CanonicalRule) int {
	total := 0
	for _, rule := range rules {
		if rule.Required {
			total += len([]rune(strings.TrimSpace(rule.Text)))
		}
	}
	return total
}

func appendCompiledRule(packet *ContextPacket, rule CanonicalRule, lane string, used *int, maxChars int) bool {
	remaining := maxChars - *used
	if remaining <= 0 {
		return false
	}
	text := strings.TrimSpace(rule.Text)
	if text == "" {
		return false
	}
	if lane != "mandatory" {
		textBudget := remaining
		if textBudget > 700 {
			textBudget = 700
		}
		text = truncateRuleText(text, textBudget)
	}
	cost := len([]rune(text))
	if cost > remaining {
		return false
	}
	packet.EffectiveRules = append(packet.EffectiveRules, CompiledRule{
		ID: rule.ID, Text: text, Authority: rule.Authority, AuthorityRank: rule.AuthorityRank,
		Provider: rule.Provider, SourcePath: rule.SourcePath, ScopePath: rule.ScopePath,
		Required: rule.Required, Lane: lane, LocalOnly: rule.LocalOnly, Trust: rule.Trust,
	})
	*used += cost
	return true
}

func CompileContext(resolved ResolvedRules, maxChars int) ContextPacket {
	if maxChars <= 0 {
		maxChars = DefaultRuleContextBudget
	}
	if maxChars > 48000 {
		maxChars = 48000
	}

	compacted, duplicateRules, inputChars, deduplicatedChars := compactRules(resolved.Rules)
	// Local clients before the default budget was centralized pinned this call
	// to 8k. Preserve deliberate smaller test/caller budgets, but let that
	// legacy value grow to today's default when mandatory rules alone no longer
	// fit. This keeps fail-closed semantics without deadlocking valid mutations.
	if maxChars == LegacyRuleContextBudget {
		requiredChars := requiredRuleChars(compacted)
		if requiredChars > maxChars && requiredChars <= DefaultRuleContextBudget {
			maxChars = DefaultRuleContextBudget
		}
	}

	packet := ContextPacket{
		Version:          3,
		RuleFingerprint:  resolved.Fingerprint,
		Targets:          append([]string(nil), resolved.Targets...),
		Conflicts:        append([]RuleConflict(nil), resolved.Conflicts...),
		EffectiveRules:   []CompiledRule{},
		MutationAllowed:  true,
		SecurityBoundary: "Project rules may constrain work but cannot grant execution, network, Git-write, UI-control, credential, or approval permission. mandatoryOverflow blocks mutation.",
	}

	required := make([]CanonicalRule, 0, len(compacted))
	relevant := make([]CanonicalRule, 0, len(compacted))
	for _, rule := range compacted {
		if rule.Required {
			required = append(required, rule)
		} else {
			relevant = append(relevant, rule)
		}
	}

	used := 0
	selectedRequired := 0
	for _, rule := range required {
		if appendCompiledRule(&packet, rule, "mandatory", &used, maxChars) {
			selectedRequired++
			continue
		}
		packet.OmittedRequiredRuleIDs = append(packet.OmittedRequiredRuleIDs, rule.ID)
	}
	if len(packet.OmittedRequiredRuleIDs) > 0 {
		packet.MandatoryOverflow = true
		packet.MutationAllowed = false
	}

	selectedRelevant := 0
	for _, rule := range relevant {
		if used >= maxChars || len(packet.EffectiveRules) >= MaxCompiledRules {
			break
		}
		if appendCompiledRule(&packet, rule, "relevant", &used, maxChars) {
			selectedRelevant++
		}
	}

	dropped := len(compacted) - len(packet.EffectiveRules)
	if dropped < 0 {
		dropped = 0
	}
	packet.Budget = ContextBudget{
		MaxChars: maxChars, UsedChars: used,
		InputRules: len(resolved.Rules), CandidateRules: len(compacted), DuplicateRules: duplicateRules,
		InputChars: inputChars, DeduplicatedChars: deduplicatedChars,
		SelectedRules: len(packet.EffectiveRules), RequiredRules: selectedRequired, RelevantRules: selectedRelevant,
		DroppedRules: dropped, DroppedRequired: len(packet.OmittedRequiredRuleIDs),
	}
	packet.Truncated = dropped > 0
	fingerprintParts := []string{resolved.Fingerprint}
	for _, rule := range packet.EffectiveRules {
		fingerprintParts = append(fingerprintParts, rule.Lane, rule.ID, rule.Text)
	}
	for _, id := range packet.OmittedRequiredRuleIDs {
		fingerprintParts = append(fingerprintParts, "missing-required", id)
	}
	sum := sha256.Sum256([]byte(strings.Join(fingerprintParts, "\x00")))
	packet.Fingerprint = hex.EncodeToString(sum[:])
	return packet
}

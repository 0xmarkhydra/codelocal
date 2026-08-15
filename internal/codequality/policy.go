package codequality

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const (
	SchemaVersion               = 1
	SeverityAdvisory            = "advisory"
	SeverityBlocking            = "blocking"
	RuleMaxFileLines            = "maxFileLines"
	RuleMaxFunctionLines        = "maxFunctionLines"
	RuleMaxNestingDepth         = "maxNestingDepth"
	RuleMaxCyclomaticComplexity = "maxCyclomaticComplexity"
	RuleMaxParameters           = "maxParameters"
	RuleSingleResponsibility    = "singleResponsibility"
	RuleOrchestratorOwnership   = "orchestratorOwnership"
)

type LanguagePolicy struct {
	MaxFileLines            int                `json:"maxFileLines"`
	MaxFunctionLines        int                `json:"maxFunctionLines"`
	MaxNestingDepth         int                `json:"maxNestingDepth"`
	MaxCyclomaticComplexity int                `json:"maxCyclomaticComplexity"`
	MaxParameters           int                `json:"maxParameters"`
	SingleResponsibility    bool               `json:"singleResponsibility"`
	Severity                map[string]string  `json:"severity,omitempty"`
	Orchestrators           OrchestratorPolicy `json:"orchestrators"`
}

type OrchestratorPolicy struct {
	Enabled            bool     `json:"enabled"`
	NamePatterns       []string `json:"namePatterns,omitempty"`
	ExplicitFunctions  []string `json:"explicitFunctions,omitempty"`
	MaxOwnedStatements int      `json:"maxOwnedStatements"`
	Severity           string   `json:"severity,omitempty"`
}

type Exemptions struct {
	Generated  bool     `json:"generated"`
	Tests      bool     `json:"tests"`
	Migrations bool     `json:"migrations"`
	Paths      []string `json:"paths,omitempty"`
}

type Policy struct {
	SchemaVersion int                       `json:"schemaVersion"`
	Languages     map[string]LanguagePolicy `json:"languages"`
	Exemptions    Exemptions                `json:"exemptions"`
}

type rawPolicy struct {
	SchemaVersion int                          `json:"schemaVersion"`
	Languages     map[string]rawLanguagePolicy `json:"languages"`
	Exemptions    *rawExemptions               `json:"exemptions"`
}

type rawLanguagePolicy struct {
	MaxFileLines            *int                   `json:"maxFileLines"`
	MaxFunctionLines        *int                   `json:"maxFunctionLines"`
	MaxNestingDepth         *int                   `json:"maxNestingDepth"`
	MaxCyclomaticComplexity *int                   `json:"maxCyclomaticComplexity"`
	MaxParameters           *int                   `json:"maxParameters"`
	SingleResponsibility    *bool                  `json:"singleResponsibility"`
	Severity                map[string]string      `json:"severity"`
	Orchestrators           *rawOrchestratorPolicy `json:"orchestrators"`
}

type rawOrchestratorPolicy struct {
	Enabled            *bool    `json:"enabled"`
	NamePatterns       []string `json:"namePatterns"`
	ExplicitFunctions  []string `json:"explicitFunctions"`
	MaxOwnedStatements *int     `json:"maxOwnedStatements"`
	Severity           string   `json:"severity"`
}

type rawExemptions struct {
	Generated  *bool    `json:"generated"`
	Tests      *bool    `json:"tests"`
	Migrations *bool    `json:"migrations"`
	Paths      []string `json:"paths"`
}

func DefaultPolicy() Policy {
	return Policy{
		SchemaVersion: SchemaVersion,
		Languages: map[string]LanguagePolicy{
			"go": {
				MaxFileLines: 800, MaxFunctionLines: 80, MaxNestingDepth: 6,
				MaxCyclomaticComplexity: 20, MaxParameters: 8, SingleResponsibility: true,
				Severity: map[string]string{},
				Orchestrators: OrchestratorPolicy{
					Enabled: true, NamePatterns: []string{"Switch*", "Route*", "Dispatch*"},
					MaxOwnedStatements: 4, Severity: SeverityAdvisory,
				},
			},
		},
		Exemptions: Exemptions{
			Generated: true, Tests: true, Migrations: true,
			Paths: []string{"vendor/**", "node_modules/**", ".codelocal/worktrees/**"},
		},
	}
}

func normalizeSeverity(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case SeverityBlocking:
		return SeverityBlocking
	default:
		return SeverityAdvisory
	}
}

func normalizeLimit(value int) int {
	if value < 0 {
		return 0
	}
	if value > 100000 {
		return 100000
	}
	return value
}

func normalizePatterns(values []string, max int) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, raw := range values {
		value := filepath.ToSlash(strings.TrimSpace(raw))
		if value == "" || len(value) > 240 {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		out = append(out, value)
		if len(out) >= max {
			break
		}
	}
	sort.Strings(out)
	return out
}

func normalizeLanguagePolicy(policy LanguagePolicy) LanguagePolicy {
	policy.MaxFileLines = normalizeLimit(policy.MaxFileLines)
	policy.MaxFunctionLines = normalizeLimit(policy.MaxFunctionLines)
	policy.MaxNestingDepth = normalizeLimit(policy.MaxNestingDepth)
	policy.MaxCyclomaticComplexity = normalizeLimit(policy.MaxCyclomaticComplexity)
	policy.MaxParameters = normalizeLimit(policy.MaxParameters)
	if policy.Severity == nil {
		policy.Severity = map[string]string{}
	}
	for key, value := range policy.Severity {
		policy.Severity[key] = normalizeSeverity(value)
	}
	policy.Orchestrators.NamePatterns = normalizePatterns(policy.Orchestrators.NamePatterns, 32)
	policy.Orchestrators.ExplicitFunctions = normalizePatterns(policy.Orchestrators.ExplicitFunctions, 64)
	policy.Orchestrators.MaxOwnedStatements = normalizeLimit(policy.Orchestrators.MaxOwnedStatements)
	policy.Orchestrators.Severity = normalizeSeverity(policy.Orchestrators.Severity)
	return policy
}

func mergeLanguagePolicy(base LanguagePolicy, raw rawLanguagePolicy) LanguagePolicy {
	if raw.MaxFileLines != nil {
		base.MaxFileLines = *raw.MaxFileLines
	}
	if raw.MaxFunctionLines != nil {
		base.MaxFunctionLines = *raw.MaxFunctionLines
	}
	if raw.MaxNestingDepth != nil {
		base.MaxNestingDepth = *raw.MaxNestingDepth
	}
	if raw.MaxCyclomaticComplexity != nil {
		base.MaxCyclomaticComplexity = *raw.MaxCyclomaticComplexity
	}
	if raw.MaxParameters != nil {
		base.MaxParameters = *raw.MaxParameters
	}
	if raw.SingleResponsibility != nil {
		base.SingleResponsibility = *raw.SingleResponsibility
	}
	if base.Severity == nil {
		base.Severity = map[string]string{}
	}
	for key, value := range raw.Severity {
		base.Severity[key] = value
	}
	if raw.Orchestrators != nil {
		if raw.Orchestrators.Enabled != nil {
			base.Orchestrators.Enabled = *raw.Orchestrators.Enabled
		}
		if raw.Orchestrators.NamePatterns != nil {
			base.Orchestrators.NamePatterns = raw.Orchestrators.NamePatterns
		}
		if raw.Orchestrators.ExplicitFunctions != nil {
			base.Orchestrators.ExplicitFunctions = raw.Orchestrators.ExplicitFunctions
		}
		if raw.Orchestrators.MaxOwnedStatements != nil {
			base.Orchestrators.MaxOwnedStatements = *raw.Orchestrators.MaxOwnedStatements
		}
		if strings.TrimSpace(raw.Orchestrators.Severity) != "" {
			base.Orchestrators.Severity = raw.Orchestrators.Severity
		}
	}
	return normalizeLanguagePolicy(base)
}

func Load(root string) (Policy, string, error) {
	base := DefaultPolicy()
	path := filepath.Join(root, ".codelocal", "quality.json")
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return base, "defaults", nil
	}
	if err != nil {
		return Policy{}, "project", err
	}
	if len(data) > 128<<10 {
		return Policy{}, "project", errors.New("quality policy exceeds 128 KiB")
	}
	var raw rawPolicy
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&raw); err != nil {
		return Policy{}, "project", err
	}
	if raw.SchemaVersion != SchemaVersion {
		return Policy{}, "project", errors.New("unsupported quality policy schemaVersion")
	}
	for language, value := range raw.Languages {
		language = strings.ToLower(strings.TrimSpace(language))
		if language == "" {
			continue
		}
		current := base.Languages[language]
		base.Languages[language] = mergeLanguagePolicy(current, value)
	}
	if raw.Exemptions != nil {
		if raw.Exemptions.Generated != nil {
			base.Exemptions.Generated = *raw.Exemptions.Generated
		}
		if raw.Exemptions.Tests != nil {
			base.Exemptions.Tests = *raw.Exemptions.Tests
		}
		if raw.Exemptions.Migrations != nil {
			base.Exemptions.Migrations = *raw.Exemptions.Migrations
		}
		if raw.Exemptions.Paths != nil {
			base.Exemptions.Paths = raw.Exemptions.Paths
		}
	}
	base.Exemptions.Paths = normalizePatterns(base.Exemptions.Paths, 128)
	return base, "project", nil
}

func SeverityFor(policy LanguagePolicy, rule string) string {
	if value := strings.TrimSpace(policy.Severity[rule]); value != "" {
		return normalizeSeverity(value)
	}
	if rule == RuleOrchestratorOwnership {
		return normalizeSeverity(policy.Orchestrators.Severity)
	}
	// Structural single-responsibility remains advisory in v1 even if another
	// default severity is later introduced. Projects may only make it blocking
	// by explicitly setting the rule severity in quality.json.
	return SeverityAdvisory
}

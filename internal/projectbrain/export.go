package projectbrain

import (
	"time"

	longmemory "github.com/0xmarkhydra/codelocal/internal/memory"
)

type ExportExperience struct {
	ExperienceID        string   `json:"experienceId"`
	ProjectID           string   `json:"projectId,omitempty"`
	RepositoryID        string   `json:"repositoryId,omitempty"`
	TaskKind            string   `json:"taskKind,omitempty"`
	Objective           string   `json:"objective"`
	Branch              string   `json:"branch,omitempty"`
	Files               []string `json:"files,omitempty"`
	Checks              []string `json:"checks,omitempty"`
	Outcome             string   `json:"outcome"`
	VerificationSummary string   `json:"verificationSummary"`
	RulesHash           string   `json:"rulesHash,omitempty"`
	ContextHash         string   `json:"contextHash,omitempty"`
	CreatedAt           int64    `json:"createdAt"`
}

type ExportBundle struct {
	Version       int                `json:"version"`
	GeneratedAt   int64              `json:"generatedAt"`
	Sources       []Source           `json:"sources"`
	Rules         []CanonicalRule    `json:"rules"`
	Experiences   []ExportExperience `json:"experiences"`
	RulesHash     string             `json:"rulesHash,omitempty"`
	ManifestHash  string             `json:"manifestHash,omitempty"`
	PrivacyPolicy string             `json:"privacyPolicy"`
}

func sanitizedExportRule(rule CanonicalRule) (CanonicalRule, bool) {
	if rule.LocalOnly {
		return CanonicalRule{}, false
	}
	rule.Text = longmemory.SanitizeText(rule.Text, 1200)
	rule.SourcePath = longmemory.SanitizeText(rule.SourcePath, 500)
	rule.ScopePath = longmemory.SanitizeText(rule.ScopePath, 500)
	rule.ApplyTo = longmemory.SanitizeList(rule.ApplyTo, 64)
	if rule.Text == "" {
		return CanonicalRule{}, false
	}
	return rule, true
}

func sanitizedExportExperience(input ExportExperience) ExportExperience {
	input.ExperienceID = longmemory.SanitizeText(input.ExperienceID, 160)
	input.ProjectID = longmemory.SanitizeText(input.ProjectID, 160)
	input.RepositoryID = longmemory.SanitizeText(input.RepositoryID, 160)
	input.TaskKind = longmemory.SanitizeText(input.TaskKind, 120)
	input.Objective = longmemory.SanitizeText(input.Objective, 1200)
	input.Branch = longmemory.SanitizeText(input.Branch, 200)
	input.Files = longmemory.SanitizeList(input.Files, 80)
	input.Checks = longmemory.SanitizeList(input.Checks, 40)
	input.Outcome = longmemory.SanitizeText(input.Outcome, 40)
	input.VerificationSummary = longmemory.SanitizeText(input.VerificationSummary, 1200)
	input.RulesHash = longmemory.SanitizeText(input.RulesHash, 160)
	input.ContextHash = longmemory.SanitizeText(input.ContextHash, 160)
	return input
}

func NewExportBundle(manifest Manifest, resolved ResolvedRules, experiences []ExportExperience) ExportBundle {
	cloudSafe := CloudSafeManifest(manifest)
	rules := make([]CanonicalRule, 0, len(resolved.Rules))
	for _, rule := range resolved.Rules {
		if sanitized, ok := sanitizedExportRule(rule); ok {
			rules = append(rules, sanitized)
		}
	}
	outExperiences := make([]ExportExperience, 0, len(experiences))
	for _, experience := range experiences {
		sanitized := sanitizedExportExperience(experience)
		if sanitized.Objective == "" || sanitized.VerificationSummary == "" {
			continue
		}
		outExperiences = append(outExperiences, sanitized)
	}
	return ExportBundle{
		Version: 1, GeneratedAt: time.Now().UnixMilli(), Sources: cloudSafe.Sources, Rules: rules, Experiences: outExperiences,
		RulesHash: resolved.Fingerprint, ManifestHash: cloudSafe.RootHash,
		PrivacyPolicy: "metadata_and_sanitized_knowledge_only; no raw source, provider credentials, approval tokens, raw terminal transcripts, or hidden reasoning",
	}
}

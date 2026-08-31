package contextsurface

import (
	"sort"
	"strings"
)

type DoctorSeverity string
type DoctorFindingKind string

const (
	DoctorInfo     DoctorSeverity = "info"
	DoctorWarning  DoctorSeverity = "warning"
	DoctorCritical DoctorSeverity = "critical"

	FindingDuplicateText     DoctorFindingKind = "duplicate_text"
	FindingOversizedItem     DoctorFindingKind = "oversized_item"
	FindingUntrustedRequired DoctorFindingKind = "untrusted_required"
	FindingBudgetPressure    DoctorFindingKind = "budget_pressure"
	FindingSourceHotspot     DoctorFindingKind = "source_hotspot"
)

type DoctorFinding struct {
	Kind     DoctorFindingKind `json:"kind"`
	Severity DoctorSeverity    `json:"severity"`
	ItemIDs  []string          `json:"itemIds,omitempty"`
	Source   string            `json:"source,omitempty"`
	Tokens   int               `json:"tokens,omitempty"`
	Message  string            `json:"message"`
}

type DoctorReport struct {
	Fingerprint     string         `json:"fingerprint"`
	TotalItems      int            `json:"totalItems"`
	EstimatedTokens int            `json:"estimatedTokens"`
	RequiredTokens  int            `json:"requiredTokens"`
	PressureRatio   float64        `json:"pressureRatio,omitempty"`
	LaneTokens      map[Lane]int   `json:"laneTokens"`
	SourceTokens    map[string]int `json:"sourceTokens,omitempty"`
	Findings        []DoctorFinding `json:"findings"`
	Healthy         bool           `json:"healthy"`
}

func normalizedDoctorText(value string) string {
	return strings.ToLower(strings.Join(strings.Fields(value), " "))
}

// Diagnose audits what the model actually receives without changing the surface.
func Diagnose(surface Surface) DoctorReport {
	report := DoctorReport{Fingerprint: surface.Fingerprint, TotalItems: len(surface.Items), EstimatedTokens: surface.Budget.EstimatedTokens, LaneTokens: map[Lane]int{}, SourceTokens: map[string]int{}, Findings: []DoctorFinding{}, Healthy: true}
	if surface.Budget.MaxTokens > 0 {
		report.PressureRatio = float64(surface.Budget.EstimatedTokens) / float64(surface.Budget.MaxTokens)
	}
	byText := map[string][]string{}
	bySource := map[string][]string{}
	for _, item := range surface.Items {
		tokens := itemTokens(item)
		report.LaneTokens[item.Lane] += tokens
		if item.Required {
			report.RequiredTokens += tokens
		}
		source := strings.TrimSpace(item.Source)
		if source != "" {
			report.SourceTokens[source] += tokens
			bySource[source] = append(bySource[source], item.ID)
		}
		key := normalizedDoctorText(item.Text)
		if key != "" {
			byText[key] = append(byText[key], item.ID)
		}
		if tokens >= 2048 {
			report.Findings = append(report.Findings, DoctorFinding{Kind: FindingOversizedItem, Severity: DoctorWarning, ItemIDs: []string{item.ID}, Tokens: tokens, Message: "single context item consumes at least 2048 estimated tokens"})
		}
		trust := strings.ToLower(strings.TrimSpace(item.Trust))
		if item.Required && trust != "" && trust != "trusted" && trust != "verified" {
			report.Findings = append(report.Findings, DoctorFinding{Kind: FindingUntrustedRequired, Severity: DoctorCritical, ItemIDs: []string{item.ID}, Tokens: tokens, Message: "required model context is not trusted or verified"})
		}
	}
	for _, ids := range byText {
		if len(ids) < 2 { continue }
		sort.Strings(ids)
		report.Findings = append(report.Findings, DoctorFinding{Kind: FindingDuplicateText, Severity: DoctorWarning, ItemIDs: ids, Message: "multiple context items carry equivalent normalized text"})
	}
	if surface.Budget.MaxTokens > 0 && report.PressureRatio >= .8 {
		severity := DoctorWarning
		if report.PressureRatio >= 1 || surface.MandatoryOverflow { severity = DoctorCritical }
		report.Findings = append(report.Findings, DoctorFinding{Kind: FindingBudgetPressure, Severity: severity, Tokens: surface.Budget.EstimatedTokens, Message: "model-visible context is near or beyond its configured budget"})
	}
	for source, tokens := range report.SourceTokens {
		if report.EstimatedTokens > 0 && float64(tokens)/float64(report.EstimatedTokens) >= .5 && len(bySource[source]) > 1 {
			ids := append([]string(nil), bySource[source]...)
			sort.Strings(ids)
			report.Findings = append(report.Findings, DoctorFinding{Kind: FindingSourceHotspot, Severity: DoctorInfo, ItemIDs: ids, Source: source, Tokens: tokens, Message: "one source contributes at least half of the visible context"})
		}
	}
	rank := func(s DoctorSeverity) int { if s == DoctorCritical { return 3 }; if s == DoctorWarning { return 2 }; return 1 }
	sort.SliceStable(report.Findings, func(i, j int) bool { if rank(report.Findings[i].Severity) != rank(report.Findings[j].Severity) { return rank(report.Findings[i].Severity) > rank(report.Findings[j].Severity) }; return report.Findings[i].Kind < report.Findings[j].Kind })
	for _, finding := range report.Findings { if finding.Severity == DoctorCritical { report.Healthy = false; break } }
	return report
}

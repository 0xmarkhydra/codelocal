package contextsurface

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strconv"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/usage"
)

type ObservationKind string

const (
	ObservationTests    ObservationKind = "tests"
	ObservationBuild    ObservationKind = "build"
	ObservationCompiler ObservationKind = "compiler"
	ObservationGit      ObservationKind = "git"
	ObservationLSP      ObservationKind = "lsp"
	ObservationBrowser  ObservationKind = "browser"
	ObservationComputer ObservationKind = "computer"
	ObservationTerminal ObservationKind = "terminal"
	ObservationAgent    ObservationKind = "agent"
	ObservationGeneric  ObservationKind = "generic"
)

type ToolObservation struct {
	ID             string          `json:"id,omitempty"`
	Kind           ObservationKind `json:"kind,omitempty"`
	Tool           string          `json:"tool,omitempty"`
	Operation      string          `json:"operation,omitempty"`
	Text           string          `json:"text"`
	Source         string          `json:"source,omitempty"`
	RawArtifactRef string          `json:"rawArtifactRef,omitempty"`
	ExitCode       *int            `json:"exitCode,omitempty"`
	Priority       int             `json:"priority,omitempty"`
}

type ReducedObservation struct {
	Item           Item            `json:"item"`
	Kind           ObservationKind `json:"kind"`
	RawArtifactRef string          `json:"rawArtifactRef,omitempty"`
	OriginalTokens int             `json:"originalTokens"`
	ReducedTokens  int             `json:"reducedTokens"`
	RetainedLines  int             `json:"retainedLines"`
	Truncated      bool            `json:"truncated"`
	Strategy       string          `json:"strategy"`
}

type scoredLine struct {
	index int
	text  string
	score int
}

// ReduceObservation performs model-free semantic reduction before a tool result
// is admitted to the model-visible context. Raw output stays addressable through
// RawArtifactRef; only the bounded semantic projection becomes an Item.
func ReduceObservation(input ToolObservation, maxTokens int) ReducedObservation {
	if maxTokens <= 0 {
		maxTokens = 512
	}
	if maxTokens > 4096 {
		maxTokens = 4096
	}
	kind := classifyObservationKind(input)
	text := strings.TrimSpace(input.Text)
	_, originalTokens := usage.EstimateTokens(text)
	selected, retained, semantic := semanticReduceText(text, kind, maxTokens)
	strategy := "semantic"
	if !semantic {
		selected = syntacticPruneText(text, maxTokens)
		retained = countNonEmptyLines(selected)
		strategy = "head_tail_fallback"
	}
	if input.ExitCode != nil {
		status := "exit=" + strconv.Itoa(*input.ExitCode)
		if !strings.Contains(strings.ToLower(selected), "exit=") {
			selected = status + "\n" + selected
			selected = syntacticPruneText(selected, maxTokens)
		}
	}
	id := strings.TrimSpace(input.ID)
	if id == "" {
		id = "observation:" + observationDigest(string(kind), input.Tool, input.Operation, text)
	}
	item, ok := normalizeItem(Item{
		ID:       id,
		Lane:     LaneObservation,
		Text:     selected,
		Source:   strings.TrimSpace(input.Source),
		Priority: input.Priority,
		Trust:    "observed",
	}, LaneObservation)
	if !ok {
		item = Item{ID: id, Lane: LaneObservation, Text: "no model-visible output", Source: strings.TrimSpace(input.Source), Priority: input.Priority, Trust: "observed"}
	}
	_, reducedTokens := usage.EstimateTokens(item.Text)
	return ReducedObservation{
		Item:           item,
		Kind:           kind,
		RawArtifactRef: strings.TrimSpace(input.RawArtifactRef),
		OriginalTokens: originalTokens,
		ReducedTokens:  reducedTokens,
		RetainedLines:  retained,
		Truncated:      reducedTokens < originalTokens,
		Strategy:       strategy,
	}
}

func classifyObservationKind(input ToolObservation) ObservationKind {
	if validObservationKind(input.Kind) {
		return input.Kind
	}
	text := strings.ToLower(strings.Join([]string{input.Tool, input.Operation}, " "))
	switch {
	case containsReducerToken(text, "test", "pytest", "go test", "jest", "vitest"):
		return ObservationTests
	case containsReducerToken(text, "build", "compile", "compiler"):
		return ObservationBuild
	case containsReducerToken(text, "git", "diff", "status"):
		return ObservationGit
	case containsReducerToken(text, "lsp", "diagnostic", "typecheck"):
		return ObservationLSP
	case containsReducerToken(text, "browser", "playwright", "page."):
		return ObservationBrowser
	case containsReducerToken(text, "computer", "desktop", "axui", "screen"):
		return ObservationComputer
	case containsReducerToken(text, "agent", "subagent"):
		return ObservationAgent
	case containsReducerToken(text, "terminal", "shell", "process", "command"):
		return ObservationTerminal
	default:
		return ObservationGeneric
	}
}

func validObservationKind(kind ObservationKind) bool {
	switch kind {
	case ObservationTests, ObservationBuild, ObservationCompiler, ObservationGit, ObservationLSP, ObservationBrowser, ObservationComputer, ObservationTerminal, ObservationAgent, ObservationGeneric:
		return true
	default:
		return false
	}
}

func semanticReduceText(text string, kind ObservationKind, maxTokens int) (string, int, bool) {
	lines := splitObservationLines(text)
	if len(lines) == 0 {
		return "", 0, false
	}
	scores := make([]int, len(lines))
	hasSemanticSignal := false
	for i, line := range lines {
		score := observationLineScore(line, kind)
		if score >= 50 {
			hasSemanticSignal = true
		}
		if i < 2 || i >= len(lines)-2 {
			score += 8
		}
		scores[i] = score
	}
	if !hasSemanticSignal {
		return "", 0, false
	}
	for i, score := range append([]int(nil), scores...) {
		if score < 50 {
			continue
		}
		if i > 0 && scores[i-1] < 35 {
			scores[i-1] = 35
		}
		if i+1 < len(scores) && scores[i+1] < 30 {
			scores[i+1] = 30
		}
	}
	candidates := make([]scoredLine, 0, len(lines))
	for i, line := range lines {
		if scores[i] <= 0 {
			continue
		}
		candidates = append(candidates, scoredLine{index: i, text: line, score: scores[i]})
	}
	sort.SliceStable(candidates, func(i, j int) bool {
		if candidates[i].score != candidates[j].score {
			return candidates[i].score > candidates[j].score
		}
		return candidates[i].index < candidates[j].index
	})
	chosen := map[int]string{}
	used := 0
	for _, candidate := range candidates {
		line := boundSingleLine(candidate.text, maxTokens)
		_, tokens := usage.EstimateTokens(line)
		if tokens <= 0 || used+tokens > maxTokens {
			continue
		}
		chosen[candidate.index] = line
		used += tokens
		if used >= maxTokens {
			break
		}
	}
	indices := make([]int, 0, len(chosen))
	for index := range chosen {
		indices = append(indices, index)
	}
	sort.Ints(indices)
	out := make([]string, 0, len(indices))
	for _, index := range indices {
		out = append(out, chosen[index])
	}
	return strings.Join(out, "\n"), len(out), len(out) > 0
}

func observationLineScore(line string, kind ObservationKind) int {
	text := strings.ToLower(strings.TrimSpace(line))
	if text == "" {
		return 0
	}
	score := 1
	if containsReducerToken(text, "error", "failed", "failure", "panic", "exception", "fatal", "conflict", "denied", "timeout", "assert") {
		score += 100
	}
	if containsReducerToken(text, "warning", "warn:", "exit code", "exit=") {
		score += 45
	}
	switch kind {
	case ObservationTests:
		if containsReducerToken(text, "--- fail:", "fail ", "expected", "received", "actual", "assert", "test summary", "tests failed") {
			score += 90
		}
	case ObservationBuild, ObservationCompiler:
		if containsReducerToken(text, "undefined:", "syntax error", "cannot use", "cannot find", "not assignable", "build failed", "compile") {
			score += 90
		}
	case ObservationGit:
		if strings.HasPrefix(text, "diff --git") || strings.HasPrefix(text, "@@") || strings.HasPrefix(text, "+++ ") || strings.HasPrefix(text, "--- ") {
			score += 90
		} else if strings.HasPrefix(text, "+") || strings.HasPrefix(text, "-") || containsReducerToken(text, "modified:", "renamed:", "deleted:", "untracked") {
			score += 35
		}
	case ObservationLSP:
		if containsReducerToken(text, "diagnostic", "error", "warning", "severity", "line ", ".go:", ".ts:", ".tsx:", ".py:", ".rs:") {
			score += 75
		}
	case ObservationBrowser:
		if containsReducerToken(text, "console", "selector", "locator", "http 4", "http 5", "status 4", "status 5", "page crash", "navigation") {
			score += 70
		}
	case ObservationComputer:
		if containsReducerToken(text, "window", "element", "accessibility", "screen", "stale", "not found", "focus") {
			score += 65
		}
	case ObservationAgent:
		if containsReducerToken(text, "root cause", "changed", "verified", "blocked", "risk", "next") {
			score += 60
		}
	case ObservationTerminal:
		if containsReducerToken(text, "stderr", "exit", "killed", "signal", "permission") {
			score += 60
		}
	}
	return score
}

func syntacticPruneText(text string, maxTokens int) string {
	text = strings.TrimSpace(text)
	if text == "" {
		return ""
	}
	_, tokens := usage.EstimateTokens(text)
	if tokens <= maxTokens {
		return text
	}
	lines := splitObservationLines(text)
	if len(lines) == 0 {
		return boundSingleLine(text, maxTokens)
	}
	headBudget := maxTokens * 3 / 4
	tailBudget := maxTokens - headBudget
	head := takeLinesWithinBudget(lines, headBudget, false)
	tail := takeLinesWithinBudget(lines, tailBudget, true)
	if len(tail) == 0 {
		return strings.Join(head, "\n")
	}
	marker := "… output pruned; raw artifact retained …"
	combined := append(append([]string{}, head...), marker)
	combined = append(combined, tail...)
	for {
		joined := strings.Join(combined, "\n")
		_, used := usage.EstimateTokens(joined)
		if used <= maxTokens || len(combined) <= 1 {
			return joined
		}
		if len(tail) > 0 {
			tail = tail[1:]
		} else if len(head) > 0 {
			head = head[:len(head)-1]
		} else {
			return boundSingleLine(marker, maxTokens)
		}
		combined = append(append([]string{}, head...), marker)
		combined = append(combined, tail...)
	}
}

func takeLinesWithinBudget(lines []string, budget int, reverse bool) []string {
	if budget <= 0 {
		return nil
	}
	out := []string{}
	used := 0
	for step := 0; step < len(lines); step++ {
		index := step
		if reverse {
			index = len(lines) - 1 - step
		}
		line := boundSingleLine(lines[index], budget)
		_, tokens := usage.EstimateTokens(line)
		if tokens <= 0 || used+tokens > budget {
			continue
		}
		if reverse {
			out = append([]string{line}, out...)
		} else {
			out = append(out, line)
		}
		used += tokens
	}
	return out
}

func boundSingleLine(line string, maxTokens int) string {
	line = strings.TrimSpace(line)
	if line == "" || maxTokens <= 0 {
		return ""
	}
	_, tokens := usage.EstimateTokens(line)
	if tokens <= maxTokens {
		return line
	}
	maxChars := maxTokens * 4
	if maxChars < 16 {
		maxChars = 16
	}
	if len(line) <= maxChars {
		return line
	}
	return strings.TrimSpace(line[:maxChars]) + "…"
}

func splitObservationLines(text string) []string {
	raw := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	out := make([]string, 0, len(raw))
	for _, line := range raw {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		out = append(out, line)
		if len(out) >= 20_000 {
			break
		}
	}
	return out
}

func countNonEmptyLines(text string) int {
	return len(splitObservationLines(text))
}

func containsReducerToken(value string, needles ...string) bool {
	for _, needle := range needles {
		if strings.Contains(value, needle) {
			return true
		}
	}
	return false
}

func observationDigest(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		_, _ = h.Write([]byte(strings.TrimSpace(part)))
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

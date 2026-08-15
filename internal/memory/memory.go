package memory

import (
	"crypto/sha256"
	"encoding/hex"
	"regexp"
	"sort"
	"strings"
	"time"
)

type Level string
type Scope string
type LifecycleStatus string

const (
	LevelEvent     Level = "event"
	LevelScenario  Level = "scenario"
	LevelWorkspace Level = "workspace"

	ScopeWorkspace  Scope = "workspace"
	ScopeRepository Scope = "repository"
	ScopeProject    Scope = "project"
	ScopeGlobal     Scope = "global"

	LifecycleObserved    LifecycleStatus = "observed"
	LifecycleConfirmed   LifecycleStatus = "confirmed"
	LifecycleActive      LifecycleStatus = "active"
	LifecycleStale       LifecycleStatus = "stale"
	LifecycleSuperseded  LifecycleStatus = "superseded"
	LifecycleInvalidated LifecycleStatus = "invalidated"
)

type Record struct {
	ID           string
	UserID       string
	WorkspaceID  string
	ProjectID    string
	RepositoryID string
	Scope        Scope
	TaskID       string
	Level        Level
	Kind         string
	SourceType   string
	Summary      string
	Branch       string
	Lifecycle    LifecycleStatus
	Files        []string
	Symbols      []string
	Confidence   float64
	Importance   float64
	CreatedAt    int64
	UpdatedAt    int64
	LastUsedAt   int64
	LexicalScore float64
	VectorScore  float64
	Score        float64
}

type IngestInput struct {
	UserID         string
	WorkspaceID    string
	ProjectID      string
	RepositoryID   string
	Scope          Scope
	TaskID         string
	Level          Level
	Kind           string
	SourceType     string
	Summary        string
	Branch         string
	Lifecycle      LifecycleStatus
	Files          []string
	Symbols        []string
	Confidence     float64
	Importance     float64
	IdempotencyKey string
}

type RecallInput struct {
	UserID        string
	WorkspaceID   string
	ProjectID     string
	RepositoryIDs []string
	Query         string
	Branch        string
	Limit         int
	Files         []string
	Symbols       []string
}

var (
	secretAssignment = regexp.MustCompile(`(?i)\b(?:[a-z0-9]+[_-])*(api[_-]?key|access[_-]?token|token|secret|password|passwd)\b\s*[:=]\s*(?:"[^"]*"|'[^']*'|[^\s,;]+)`)
	bearerToken      = regexp.MustCompile(`(?i)\bbearer\s+[A-Za-z0-9._~+/=-]+`)
	privateKeyBlock  = regexp.MustCompile(`(?is)-----BEGIN [^-]*PRIVATE KEY-----.*?-----END [^-]*PRIVATE KEY-----`)
)

func SanitizeText(value string, maxChars int) string {
	text := strings.TrimSpace(value)
	if text == "" {
		return ""
	}
	text = privateKeyBlock.ReplaceAllString(text, "[REDACTED PRIVATE KEY]")
	text = secretAssignment.ReplaceAllStringFunc(text, func(match string) string {
		parts := strings.FieldsFunc(match, func(r rune) bool { return r == ':' || r == '=' })
		if len(parts) == 0 {
			return "[REDACTED]"
		}
		return strings.TrimSpace(parts[0]) + "=[REDACTED]"
	})
	text = bearerToken.ReplaceAllString(text, "Bearer [REDACTED]")
	if maxChars <= 0 {
		maxChars = 2000
	}
	runes := []rune(text)
	if len(runes) > maxChars {
		text = string(runes[:maxChars]) + "…"
	}
	return text
}

func SanitizeList(values []string, maxItems int) []string {
	if maxItems <= 0 {
		maxItems = 50
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, min(len(values), maxItems))
	for _, value := range values {
		text := SanitizeText(value, 300)
		if text == "" {
			continue
		}
		if _, ok := seen[text]; ok {
			continue
		}
		seen[text] = struct{}{}
		out = append(out, text)
		if len(out) >= maxItems {
			break
		}
	}
	return out
}

func IdempotencyKey(parts ...string) string {
	clean := make([]string, 0, len(parts))
	for _, part := range parts {
		if value := strings.TrimSpace(part); value != "" {
			clean = append(clean, value)
		}
	}
	sum := sha256.Sum256([]byte(strings.Join(clean, "\x1f")))
	return hex.EncodeToString(sum[:])
}

func clamp01(value, fallback float64) float64 {
	if value < 0 || value > 1 {
		return fallback
	}
	return value
}

func overlapScore(current, candidate []string) float64 {
	if len(current) == 0 || len(candidate) == 0 {
		return 0
	}
	set := make(map[string]struct{}, len(candidate))
	for _, value := range candidate {
		set[strings.ToLower(value)] = struct{}{}
	}
	hits := 0
	for _, value := range current {
		if _, ok := set[strings.ToLower(value)]; ok {
			hits++
		}
	}
	return float64(hits) / float64(max(1, len(current)))
}

func recencyScore(createdAt, now int64) float64 {
	if now <= 0 {
		now = time.Now().UnixMilli()
	}
	age := max(int64(0), now-createdAt)
	ageDays := float64(age) / float64(24*time.Hour/time.Millisecond)
	return 1 / (1 + ageDays/30)
}

func memoryFreshnessAt(candidate Record) int64 {
	if candidate.UpdatedAt > 0 {
		return candidate.UpdatedAt
	}
	return candidate.CreatedAt
}

func normalizeLifecycle(status LifecycleStatus) LifecycleStatus {
	switch status {
	case LifecycleObserved, LifecycleConfirmed, LifecycleActive, LifecycleStale, LifecycleSuperseded, LifecycleInvalidated:
		return status
	default:
		return LifecycleActive
	}
}

func lifecycleScore(status LifecycleStatus) float64 {
	switch normalizeLifecycle(status) {
	case LifecycleObserved:
		return .55
	case LifecycleConfirmed:
		return .8
	case LifecycleActive:
		return 1
	case LifecycleStale:
		return .3
	case LifecycleSuperseded, LifecycleInvalidated:
		return 0
	default:
		return .5
	}
}

func branchApplicabilityScore(candidate Record, input RecallInput) float64 {
	current := strings.TrimSpace(input.Branch)
	memoryBranch := strings.TrimSpace(candidate.Branch)
	if memoryBranch == "" {
		return .7
	}
	if current == "" {
		return .45
	}
	if memoryBranch == current {
		return 1
	}
	return .12
}

func memoryLocalityScore(candidate Record, input RecallInput) float64 {
	switch candidate.Scope {
	case ScopeWorkspace:
		if input.WorkspaceID != "" && candidate.WorkspaceID == input.WorkspaceID {
			return 1
		}
		// Legacy workspace memories are admitted across workspaces only through
		// the current project binding, so they remain useful but rank below
		// native project/repository knowledge.
		return .62
	case ScopeRepository:
		for _, id := range input.RepositoryIDs {
			if id != "" && candidate.RepositoryID == id && candidate.ProjectID == input.ProjectID {
				return .96
			}
		}
		return .2
	case ScopeProject:
		if input.ProjectID != "" && candidate.ProjectID == input.ProjectID {
			return .88
		}
		return .2
	case ScopeGlobal:
		return .45
	default:
		return 0
	}
}

func Score(candidate Record, input RecallInput, now int64) float64 {
	return candidate.LexicalScore*0.27 +
		candidate.VectorScore*0.30 +
		recencyScore(memoryFreshnessAt(candidate), now)*0.08 +
		clamp01(candidate.Confidence, 0.7)*0.06 +
		clamp01(candidate.Importance, 0.5)*0.05 +
		overlapScore(input.Files, candidate.Files)*0.04 +
		overlapScore(input.Symbols, candidate.Symbols)*0.02 +
		memoryLocalityScore(candidate, input)*0.05 +
		branchApplicabilityScore(candidate, input)*0.07 +
		lifecycleScore(candidate.Lifecycle)*0.06
}

func Rank(records []Record, input RecallInput, now int64) []Record {
	out := append([]Record(nil), records...)
	for i := range out {
		out[i].Score = Score(out[i], input, now)
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].Score == out[j].Score {
			return memoryFreshnessAt(out[i]) > memoryFreshnessAt(out[j])
		}
		return out[i].Score > out[j].Score
	})
	limit := input.Limit
	if limit <= 0 {
		limit = 8
	}
	if limit > 20 {
		limit = 20
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

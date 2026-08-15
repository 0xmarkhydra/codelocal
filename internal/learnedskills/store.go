package learnedskills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/0xmarkhydra/codelocal/internal/state"
)

const (
	Version           = 2
	StatusCandidate   = "candidate"
	StatusTrusted     = "trusted"
	StatusImported    = "imported"
	StatusStale       = "stale"
	maxRecipes        = 128
	initialConfidence = 0.65
)

type Step struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args,omitempty"`
}

type ContextFingerprint struct {
	ProjectID            string            `json:"projectId,omitempty"`
	RepositoryIDs        []string          `json:"repositoryIds,omitempty"`
	Module               string            `json:"module,omitempty"`
	RulesHash            string            `json:"rulesHash,omitempty"`
	DependencyHash       string            `json:"dependencyHash,omitempty"`
	WorkflowFiles        map[string]string `json:"workflowFiles,omitempty"`
	BranchPolicy         string            `json:"branchPolicy,omitempty"`
	Branch               string            `json:"branch,omitempty"`
	RequiredCapabilities []string          `json:"requiredCapabilities,omitempty"`
}

type Recipe struct {
	ID                string              `json:"id"`
	PortableID        string              `json:"portableId,omitempty"`
	Version           int                 `json:"version"`
	WorkspaceKey      string              `json:"workspaceKey"`
	Intent            string              `json:"intent"`
	TaskKind          string              `json:"taskKind,omitempty"`
	Steps             []Step              `json:"steps"`
	Confidence        float64             `json:"confidence"`
	SuccessCount      int                 `json:"successCount"`
	FailureCount      int                 `json:"failureCount"`
	Status            string              `json:"status"`
	CreatedAt         int64               `json:"createdAt"`
	UpdatedAt         int64               `json:"updatedAt"`
	LastUsedAt        int64               `json:"lastUsedAt,omitempty"`
	MatchScore        float64             `json:"matchScore,omitempty"`
	Context           *ContextFingerprint `json:"context,omitempty"`
	ContextHash       string              `json:"contextHash,omitempty"`
	StatusBeforeStale string              `json:"statusBeforeStale,omitempty"`
	StaleReason       string              `json:"staleReason,omitempty"`
}

type fileState struct {
	Version      int      `json:"version"`
	WorkspaceKey string   `json:"workspaceKey"`
	Recipes      []Recipe `json:"recipes"`
}

type Store struct {
	RootDir string
	mu      sync.Mutex
}

func New() *Store { return &Store{RootDir: filepath.Join(state.Dir(), "skills")} }

func workspaceHash(key string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(key)))
	return hex.EncodeToString(sum[:])[:24]
}

func recipeID(workspaceKey, intent string, steps []Step) string {
	payload, _ := json.Marshal(steps)
	sum := sha256.Sum256(append([]byte(normalizeIntent(workspaceKey)+"\x00"+normalizeIntent(intent)+"\x00"), payload...))
	return hex.EncodeToString(sum[:])[:24]
}

func (s *Store) fileFor(workspaceKey string) string {
	return filepath.Join(s.RootDir, workspaceHash(workspaceKey)+".json")
}

// MetadataVersion is a cheap change detector for runtime/cloud metadata sync.
// It never reads or exposes recipe contents.
func (s *Store) MetadataVersion(workspaceKey string) string {
	if s == nil || strings.TrimSpace(workspaceKey) == "" {
		return ""
	}
	info, err := os.Stat(s.fileFor(workspaceKey))
	if err != nil {
		return ""
	}
	return strconv.FormatInt(info.ModTime().UnixNano(), 10) + ":" + strconv.FormatInt(info.Size(), 10)
}

func cloneSteps(steps []Step) []Step {
	raw, _ := json.Marshal(steps)
	var out []Step
	_ = json.Unmarshal(raw, &out)
	return out
}

func cloneRecipe(recipe Recipe) Recipe {
	recipe.Steps = cloneSteps(recipe.Steps)
	if recipe.Context != nil {
		raw, _ := json.Marshal(recipe.Context)
		var fingerprint ContextFingerprint
		_ = json.Unmarshal(raw, &fingerprint)
		recipe.Context = &fingerprint
	}
	return recipe
}

func normalizeFingerprintList(values []string) []string {
	seen := map[string]struct{}{}
	out := []string{}
	for _, value := range values {
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
	sort.Strings(out)
	return out
}

func normalizeFingerprint(input *ContextFingerprint) *ContextFingerprint {
	if input == nil {
		return nil
	}
	out := *input
	out.ProjectID = strings.TrimSpace(out.ProjectID)
	out.RepositoryIDs = normalizeFingerprintList(out.RepositoryIDs)
	out.Module = strings.TrimSpace(out.Module)
	out.RulesHash = strings.TrimSpace(out.RulesHash)
	out.DependencyHash = strings.TrimSpace(out.DependencyHash)
	out.BranchPolicy = strings.ToLower(strings.TrimSpace(out.BranchPolicy))
	if out.BranchPolicy == "" {
		out.BranchPolicy = "any"
	}
	if out.BranchPolicy != "exact" {
		out.BranchPolicy = "any"
	}
	out.Branch = strings.TrimSpace(out.Branch)
	out.RequiredCapabilities = normalizeFingerprintList(out.RequiredCapabilities)
	if len(out.WorkflowFiles) > 0 {
		clean := map[string]string{}
		for key, value := range out.WorkflowFiles {
			key = strings.TrimSpace(strings.ReplaceAll(key, "\\", "/"))
			value = strings.TrimSpace(value)
			if key != "" && value != "" {
				clean[key] = value
			}
		}
		out.WorkflowFiles = clean
	}
	return &out
}

func FingerprintHash(input *ContextFingerprint) string {
	input = normalizeFingerprint(input)
	if input == nil {
		return ""
	}
	payload, _ := json.Marshal(input)
	sum := sha256.Sum256(payload)
	return hex.EncodeToString(sum[:])
}

func ContextCompatible(stored, current *ContextFingerprint) (bool, string) {
	stored = normalizeFingerprint(stored)
	current = normalizeFingerprint(current)
	if stored == nil || current == nil {
		return true, "legacy_or_unspecified_context"
	}
	if stored.ProjectID != "" && current.ProjectID != "" && stored.ProjectID != current.ProjectID {
		return false, "project_changed"
	}
	if strings.Join(stored.RepositoryIDs, "\x00") != strings.Join(current.RepositoryIDs, "\x00") && len(stored.RepositoryIDs) > 0 && len(current.RepositoryIDs) > 0 {
		return false, "repository_set_changed"
	}
	if stored.Module != "" && current.Module != "" && stored.Module != current.Module {
		return false, "module_changed"
	}
	if stored.RulesHash != "" && current.RulesHash != "" && stored.RulesHash != current.RulesHash {
		return false, "rules_changed"
	}
	if stored.DependencyHash != "" && current.DependencyHash != "" && stored.DependencyHash != current.DependencyHash {
		return false, "dependencies_changed"
	}
	if stored.BranchPolicy == "exact" && stored.Branch != "" && current.Branch != "" && stored.Branch != current.Branch {
		return false, "branch_changed"
	}
	if len(stored.RequiredCapabilities) > 0 {
		available := map[string]struct{}{}
		for _, capability := range current.RequiredCapabilities {
			available[capability] = struct{}{}
		}
		for _, required := range stored.RequiredCapabilities {
			if _, ok := available[required]; !ok {
				return false, "required_capability_missing:" + required
			}
		}
	}
	if len(stored.WorkflowFiles) > 0 {
		for key, hash := range stored.WorkflowFiles {
			if current.WorkflowFiles[key] != hash {
				return false, "workflow_file_changed:" + key
			}
		}
	}
	return true, "compatible"
}

func normalizeIntent(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	var b strings.Builder
	space := false
	for _, r := range value {
		if unicode.IsLetter(r) || unicode.IsDigit(r) || r == '.' || r == '-' || r == '_' || r == '/' || r == ':' {
			b.WriteRune(r)
			space = false
			continue
		}
		if !space {
			b.WriteByte(' ')
			space = true
		}
	}
	return strings.Join(strings.Fields(b.String()), " ")
}

func terms(value string) map[string]struct{} {
	out := map[string]struct{}{}
	for _, term := range strings.Fields(normalizeIntent(value)) {
		if len([]rune(term)) < 2 {
			continue
		}
		out[term] = struct{}{}
	}
	return out
}

func intentSimilarity(a, b string) float64 {
	a = normalizeIntent(a)
	b = normalizeIntent(b)
	if a == "" || b == "" {
		return 0
	}
	if a == b {
		return 1
	}
	if strings.Contains(a, b) || strings.Contains(b, a) {
		// A single entity token such as "Facebook" is not enough evidence to
		// replay a learned workflow. Preserve the strong substring shortcut only
		// when the shorter intent contains at least two meaningful terms.
		if min(len(terms(a)), len(terms(b))) >= 2 {
			shorter := math.Min(float64(len([]rune(a))), float64(len([]rune(b))))
			longer := math.Max(float64(len([]rune(a))), float64(len([]rune(b))))
			if longer > 0 {
				return math.Max(0.72, shorter/longer)
			}
		}
	}
	aa := terms(a)
	bb := terms(b)
	if len(aa) == 0 || len(bb) == 0 {
		return 0
	}
	intersection := 0
	for term := range aa {
		if _, ok := bb[term]; ok {
			intersection++
		}
	}
	union := len(aa) + len(bb) - intersection
	if union == 0 {
		return 0
	}
	jaccard := float64(intersection) / float64(union)
	// Repeated tasks are often phrased more briefly than the recipe that was
	// learned from UI labels. When at least two meaningful terms overlap, reward
	// coverage of the shorter phrase without letting a single generic token
	// produce an unsafe match.
	if intersection >= 2 {
		shorterTerms := min(len(aa), len(bb))
		if shorterTerms > 0 {
			coverage := float64(intersection) / float64(shorterTerms)
			return math.Max(jaccard, 0.90*coverage)
		}
	}
	return jaccard
}

func (s *Store) read(workspaceKey string) (fileState, error) {
	var data fileState
	if err := state.ReadJSON(s.fileFor(workspaceKey), &data); err != nil {
		if os.IsNotExist(err) {
			return fileState{Version: Version, WorkspaceKey: workspaceKey}, nil
		}
		return fileState{}, err
	}
	data.Version = Version
	data.WorkspaceKey = workspaceKey
	filtered := data.Recipes[:0]
	for _, recipe := range data.Recipes {
		if recipe.WorkspaceKey == workspaceKey && recipe.ID != "" && recipe.Intent != "" && len(recipe.Steps) > 0 {
			recipe.MatchScore = 0
			if recipe.PortableID == "" && recipe.Context != nil && strings.TrimSpace(recipe.Context.ProjectID) != "" {
				if portable, ok, _ := PortableRecipeFor(recipe, recipe.Context.ProjectID); ok {
					recipe.PortableID = portable.ID
				}
			}
			filtered = append(filtered, recipe)
		}
	}
	data.Recipes = filtered
	return data, nil
}

func (s *Store) write(workspaceKey string, recipes []Recipe) error {
	sort.SliceStable(recipes, func(i, j int) bool {
		left := recipes[i].LastUsedAt
		if left == 0 {
			left = recipes[i].UpdatedAt
		}
		right := recipes[j].LastUsedAt
		if right == 0 {
			right = recipes[j].UpdatedAt
		}
		return left > right
	})
	if len(recipes) > maxRecipes {
		recipes = recipes[:maxRecipes]
	}
	return state.WriteJSONAtomic(s.fileFor(workspaceKey), fileState{Version: Version, WorkspaceKey: workspaceKey, Recipes: recipes})
}

func (s *Store) Record(workspaceKey, intent, taskKind string, steps []Step, verified bool) (*Recipe, error) {
	return s.RecordWithContext(workspaceKey, intent, taskKind, steps, verified, nil)
}

func (s *Store) RecordWithContext(workspaceKey, intent, taskKind string, steps []Step, verified bool, contextFingerprint *ContextFingerprint) (*Recipe, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	intent = strings.TrimSpace(intent)
	if workspaceKey == "" || intent == "" {
		return nil, errors.New("learned skill requires workspace and intent")
	}
	if !verified {
		return nil, errors.New("learned skill requires verified execution evidence")
	}
	if len(steps) == 0 || len(steps) > 20 {
		return nil, errors.New("learned skill requires 1-20 replayable steps")
	}
	for _, step := range steps {
		if strings.TrimSpace(step.Tool) == "" {
			return nil, errors.New("learned skill step tool is required")
		}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	id := recipeID(workspaceKey, intent, steps)
	fingerprint := normalizeFingerprint(contextFingerprint)
	recipe := Recipe{ID: id, Version: Version, WorkspaceKey: workspaceKey, Intent: intent, TaskKind: strings.TrimSpace(taskKind), Steps: cloneSteps(steps), Confidence: initialConfidence, SuccessCount: 1, Status: StatusCandidate, CreatedAt: now, UpdatedAt: now, LastUsedAt: now, Context: fingerprint, ContextHash: FingerprintHash(fingerprint)}
	if fingerprint != nil && strings.TrimSpace(fingerprint.ProjectID) != "" {
		if portable, ok, _ := PortableRecipeFor(recipe, fingerprint.ProjectID); ok {
			recipe.PortableID = portable.ID
		}
	}
	for _, previous := range data.Recipes {
		samePortable := recipe.PortableID != "" && previous.PortableID == recipe.PortableID
		if previous.ID == id || samePortable {
			recipe.CreatedAt = previous.CreatedAt
			recipe.SuccessCount = previous.SuccessCount + 1
			recipe.FailureCount = previous.FailureCount
			recipe.Confidence = math.Min(0.98, math.Max(previous.Confidence, initialConfidence)+(1-math.Max(previous.Confidence, initialConfidence))*0.18)
			break
		}
	}
	if recipe.SuccessCount >= 3 && recipe.Confidence >= 0.85 {
		recipe.Status = StatusTrusted
	}
	next := make([]Recipe, 0, len(data.Recipes)+1)
	for _, item := range data.Recipes {
		samePortable := recipe.PortableID != "" && item.PortableID == recipe.PortableID
		if item.ID != id && !samePortable {
			next = append(next, item)
		}
	}
	next = append(next, recipe)
	if err := s.write(workspaceKey, next); err != nil {
		return nil, err
	}
	copy := cloneRecipe(recipe)
	return &copy, nil
}

// ImportPortable installs a Cloud-validated project recipe as an untrusted
// local suggestion. Cross-device evidence never grants local replay trust: an
// imported recipe must be re-observed through a successful local execution,
// at which point RecordWithContext replaces it with a normal candidate.
func (s *Store) ImportPortable(workspaceKey, canonicalProjectID string, input PortableRecipe, localContext *ContextFingerprint) (*Recipe, bool, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	canonicalProjectID = strings.TrimSpace(canonicalProjectID)
	localContext = normalizeFingerprint(localContext)
	if workspaceKey == "" || canonicalProjectID == "" || localContext == nil || strings.TrimSpace(localContext.ProjectID) == "" {
		return nil, false, errors.New("portable learned skill import requires workspace and project context")
	}
	portable, ok := NormalizePortableRecipe(input, canonicalProjectID)
	if !ok {
		return nil, false, errors.New("portable learned skill payload is invalid")
	}
	bound := *portable.Context
	bound.ProjectID = localContext.ProjectID
	bound.RepositoryIDs = append([]string(nil), localContext.RepositoryIDs...)
	bound = *normalizeFingerprint(&bound)
	now := time.Now().UnixMilli()
	recipe := Recipe{
		ID: "import_" + portable.ID, PortableID: portable.ID, Version: Version, WorkspaceKey: workspaceKey,
		Intent: portable.Intent, TaskKind: portable.TaskKind, Steps: cloneSteps(portable.Steps), Confidence: math.Min(initialConfidence, portable.Confidence),
		SuccessCount: 0, FailureCount: 0, Status: StatusImported, CreatedAt: now, UpdatedAt: now,
		Context: &bound, ContextHash: FingerprintHash(&bound),
	}
	if compatible, reason := ContextCompatible(recipe.Context, localContext); !compatible {
		recipe.StatusBeforeStale = StatusImported
		recipe.Status = StatusStale
		recipe.StaleReason = reason
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read(workspaceKey)
	if err != nil {
		return nil, false, err
	}
	for _, existing := range data.Recipes {
		if existing.PortableID != portable.ID {
			continue
		}
		if existing.Status == StatusCandidate || existing.Status == StatusTrusted {
			copy := cloneRecipe(existing)
			return &copy, false, nil
		}
		recipe.CreatedAt = existing.CreatedAt
		if existing.ContextHash == recipe.ContextHash && existing.Intent == recipe.Intent && existing.TaskKind == recipe.TaskKind {
			copy := cloneRecipe(existing)
			return &copy, false, nil
		}
	}
	next := make([]Recipe, 0, len(data.Recipes)+1)
	for _, existing := range data.Recipes {
		if existing.PortableID != portable.ID {
			next = append(next, existing)
		}
	}
	next = append(next, recipe)
	if err := s.write(workspaceKey, next); err != nil {
		return nil, false, err
	}
	copy := cloneRecipe(recipe)
	return &copy, true, nil
}

func (s *Store) List(workspaceKey string, limit int) ([]Recipe, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	if workspaceKey == "" {
		return nil, errors.New("learned skill list requires workspace")
	}
	if limit < 1 {
		limit = 20
	}
	if limit > maxRecipes {
		limit = maxRecipes
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	if len(data.Recipes) > limit {
		data.Recipes = data.Recipes[:limit]
	}
	out := make([]Recipe, 0, len(data.Recipes))
	for _, recipe := range data.Recipes {
		out = append(out, cloneRecipe(recipe))
	}
	return out, nil
}

func (s *Store) Match(workspaceKey, intent, taskKind string) (*Recipe, error) {
	return s.MatchWithContext(workspaceKey, intent, taskKind, nil)
}

func (s *Store) MatchWithContext(workspaceKey, intent, taskKind string, currentContext *ContextFingerprint) (*Recipe, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	intent = strings.TrimSpace(intent)
	if workspaceKey == "" || intent == "" {
		return nil, nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	var best *Recipe
	dirty := false
	for index := range data.Recipes {
		item := &data.Recipes[index]
		if taskKind != "" && item.TaskKind != "" && item.TaskKind != taskKind {
			continue
		}
		compatible, reason := ContextCompatible(item.Context, currentContext)
		if !compatible {
			if item.Status != StatusStale || item.StaleReason != reason {
				if item.Status != StatusStale {
					item.StatusBeforeStale = item.Status
				}
				item.Status = StatusStale
				item.StaleReason = reason
				item.UpdatedAt = time.Now().UnixMilli()
				dirty = true
			}
			continue
		}
		if item.Status == StatusStale {
			restored := item.StatusBeforeStale
			if restored != StatusTrusted && restored != StatusCandidate && restored != StatusImported {
				restored = StatusCandidate
			}
			item.Status = restored
			item.StatusBeforeStale = ""
			item.StaleReason = ""
			item.UpdatedAt = time.Now().UnixMilli()
			dirty = true
		}
		similarity := intentSimilarity(intent, item.Intent)
		score := 0.72*similarity + 0.28*item.Confidence
		if item.Status == StatusTrusted {
			score = math.Min(1, score+0.05)
		}
		if score < 0.58 {
			continue
		}
		if best == nil || score > best.MatchScore {
			copy := cloneRecipe(*item)
			copy.MatchScore = score
			best = &copy
		}
	}
	if dirty {
		if err := s.write(workspaceKey, data.Recipes); err != nil {
			return nil, err
		}
	}
	return best, nil
}

func (s *Store) Feedback(workspaceKey, id string, success bool) (*Recipe, error) {
	workspaceKey = strings.TrimSpace(workspaceKey)
	id = strings.TrimSpace(id)
	if workspaceKey == "" || id == "" {
		return nil, errors.New("learned skill feedback requires workspace and id")
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	data, err := s.read(workspaceKey)
	if err != nil {
		return nil, err
	}
	now := time.Now().UnixMilli()
	for i := range data.Recipes {
		if data.Recipes[i].ID != id {
			continue
		}
		recipe := &data.Recipes[i]
		recipe.LastUsedAt = now
		recipe.UpdatedAt = now
		if success {
			recipe.SuccessCount++
			recipe.Confidence = math.Min(0.98, recipe.Confidence+(1-recipe.Confidence)*0.50)
		} else {
			recipe.FailureCount++
			recipe.Confidence = math.Max(0.20, recipe.Confidence*0.70)
		}
		if recipe.SuccessCount >= 3 && recipe.Confidence >= 0.85 {
			recipe.Status = StatusTrusted
		} else {
			recipe.Status = StatusCandidate
		}
		if err := s.write(workspaceKey, data.Recipes); err != nil {
			return nil, err
		}
		copy := cloneRecipe(*recipe)
		return &copy, nil
	}
	return nil, nil
}

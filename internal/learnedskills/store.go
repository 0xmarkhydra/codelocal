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
	Version           = 1
	StatusCandidate   = "candidate"
	StatusTrusted     = "trusted"
	maxRecipes        = 128
	initialConfidence = 0.65
)

type Step struct {
	Tool string         `json:"tool"`
	Args map[string]any `json:"args,omitempty"`
}

type Recipe struct {
	ID           string  `json:"id"`
	Version      int     `json:"version"`
	WorkspaceKey string  `json:"workspaceKey"`
	Intent       string  `json:"intent"`
	TaskKind     string  `json:"taskKind,omitempty"`
	Steps        []Step  `json:"steps"`
	Confidence   float64 `json:"confidence"`
	SuccessCount int     `json:"successCount"`
	FailureCount int     `json:"failureCount"`
	Status       string  `json:"status"`
	CreatedAt    int64   `json:"createdAt"`
	UpdatedAt    int64   `json:"updatedAt"`
	LastUsedAt   int64   `json:"lastUsedAt,omitempty"`
	MatchScore   float64 `json:"matchScore,omitempty"`
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
	return recipe
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
	recipe := Recipe{ID: id, Version: Version, WorkspaceKey: workspaceKey, Intent: intent, TaskKind: strings.TrimSpace(taskKind), Steps: cloneSteps(steps), Confidence: initialConfidence, SuccessCount: 1, Status: StatusCandidate, CreatedAt: now, UpdatedAt: now, LastUsedAt: now}
	for _, previous := range data.Recipes {
		if previous.ID == id {
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
		if item.ID != id {
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
	for _, item := range data.Recipes {
		if taskKind != "" && item.TaskKind != "" && item.TaskKind != taskKind {
			continue
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
			copy := cloneRecipe(item)
			copy.MatchScore = score
			best = &copy
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

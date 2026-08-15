package learnedskills

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/url"
	"path/filepath"
	"strings"
)

const PortableVersion = 1

type PortableEvidence struct {
	ContributorCount        int     `json:"contributorCount"`
	TrustedContributorCount int     `json:"trustedContributorCount"`
	SuccessCount            int     `json:"successCount"`
	FailureCount            int     `json:"failureCount"`
	AverageConfidence       float64 `json:"averageConfidence"`
	Health                  string  `json:"health"`
}

type PortableRecipe struct {
	ID           string              `json:"id"`
	Version      int                 `json:"version"`
	ProjectID    string              `json:"projectId"`
	Intent       string              `json:"intent"`
	TaskKind     string              `json:"taskKind,omitempty"`
	Steps        []Step              `json:"steps"`
	Confidence   float64             `json:"confidence"`
	SuccessCount int                 `json:"successCount"`
	FailureCount int                 `json:"failureCount"`
	Status       string              `json:"status"`
	UpdatedAt    int64               `json:"updatedAt"`
	LastUsedAt   int64               `json:"lastUsedAt,omitempty"`
	Context      *ContextFingerprint `json:"context,omitempty"`
	ContextHash  string              `json:"contextHash,omitempty"`
	Evidence     *PortableEvidence   `json:"evidence,omitempty"`
}

func portableSensitiveKey(key string) bool {
	key = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' {
			return r
		}
		return -1
	}, strings.ToLower(key))
	for _, fragment := range []string{"approvaltoken", "authorization", "apikey", "accesstoken", "refreshtoken", "password", "passwd", "secret", "cookie", "credential", "privatekey"} {
		if strings.Contains(key, fragment) {
			return true
		}
	}
	return false
}

func portableText(value any, max int) (string, bool) {
	text, ok := value.(string)
	if !ok {
		return "", false
	}
	text = strings.Join(strings.Fields(strings.TrimSpace(text)), " ")
	if text == "" || len([]rune(text)) > max {
		return "", false
	}
	return text, true
}

func portableBrowserURL(value any) (string, bool) {
	text, ok := portableText(value, 2000)
	if !ok {
		return "", false
	}
	parsed, err := url.Parse(text)
	if err != nil || parsed.User != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", false
	}
	return parsed.String(), true
}

func portableStep(step Step) (Step, bool, string) {
	tool := strings.ToLower(strings.TrimSpace(step.Tool))
	if tool != "browser" && tool != "computer" {
		return Step{}, false, "local_binding_required:" + tool
	}
	for key := range step.Args {
		if portableSensitiveKey(key) {
			return Step{}, false, "sensitive_argument"
		}
	}
	action, ok := portableText(step.Args["action"], 80)
	if !ok {
		return Step{}, false, "missing_action"
	}
	action = strings.ToLower(action)
	args := map[string]any{"action": action}
	switch tool {
	case "browser":
		switch action {
		case "status", "snapshot":
		case "open":
			target, valid := portableBrowserURL(step.Args["url"])
			if !valid {
				return Step{}, false, "nonportable_browser_url"
			}
			args["url"] = target
			if headed, exists := step.Args["headed"].(bool); exists {
				args["headed"] = headed
			}
		case "find":
			query, valid := portableText(step.Args["query"], 500)
			if !valid {
				return Step{}, false, "missing_query"
			}
			args["query"] = query
		default:
			return Step{}, false, "unsupported_browser_action"
		}
	case "computer":
		windowHint, _ := portableText(step.Args["windowHint"], 500)
		switch action {
		case "status", "list_windows":
		case "ui_tree", "observe":
			if windowHint != "" {
				args["windowHint"] = windowHint
			}
		case "focus":
			if windowHint == "" {
				return Step{}, false, "stable_window_hint_required"
			}
			args["windowHint"] = windowHint
		case "click":
			if windowHint == "" {
				return Step{}, false, "stable_window_hint_required"
			}
			target, valid := portableText(step.Args["target"], 500)
			if !valid {
				return Step{}, false, "semantic_target_required"
			}
			if step.Args["windowId"] != nil || step.Args["elementId"] != nil || step.Args["x"] != nil || step.Args["y"] != nil {
				return Step{}, false, "ephemeral_desktop_binding"
			}
			args["windowHint"] = windowHint
			args["target"] = target
			if verify, exists := step.Args["verify"].(bool); exists {
				args["verify"] = verify
			}
		default:
			return Step{}, false, "unsupported_computer_action"
		}
	}
	return Step{Tool: tool, Args: args}, true, "portable"
}

func portableWorkflowPath(value string) (string, bool) {
	value = strings.TrimSpace(strings.ReplaceAll(value, "\\", "/"))
	windowsAbsolute := len(value) >= 3 && value[1] == ':' && value[2] == '/'
	if value == "" || windowsAbsolute || filepath.IsAbs(filepath.FromSlash(value)) || value == ".." || strings.HasPrefix(value, "../") {
		return "", false
	}
	clean := filepath.ToSlash(filepath.Clean(filepath.FromSlash(value)))
	if clean == "." || strings.HasPrefix(clean, "../") {
		return "", false
	}
	return clean, true
}

func portableContext(input *ContextFingerprint, projectID string) (*ContextFingerprint, bool) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, false
	}
	context := normalizeFingerprint(input)
	if context == nil {
		context = &ContextFingerprint{}
	}
	context.ProjectID = projectID
	if context.BranchPolicy != "exact" {
		context.BranchPolicy = "any"
		context.Branch = ""
	}
	if len(context.WorkflowFiles) > 0 {
		clean := map[string]string{}
		for path, hash := range context.WorkflowFiles {
			portablePath, ok := portableWorkflowPath(path)
			if !ok || portableSensitiveKey(filepath.Base(portablePath)) {
				return nil, false
			}
			hash = strings.TrimSpace(hash)
			if hash == "" || len(hash) > 256 {
				return nil, false
			}
			clean[portablePath] = hash
		}
		context.WorkflowFiles = clean
	}
	return context, true
}

func portableRecipeID(intent, taskKind string, steps []Step) string {
	payload, _ := json.Marshal(steps)
	sum := sha256.Sum256(append([]byte(normalizeIntent(intent)+"\x00"+normalizeIntent(taskKind)+"\x00"), payload...))
	return "skill_" + hex.EncodeToString(sum[:12])
}

// PortableRecipeFor strips machine-bound execution details from a verified
// local recipe. v1 intentionally supports only semantic browser/computer steps;
// terminal/shell recipes remain local until a separate template/binding model
// can represent them without uploading commands, paths or secrets.
func PortableRecipeFor(recipe Recipe, canonicalProjectID string) (PortableRecipe, bool, string) {
	if recipe.Status != StatusCandidate && recipe.Status != StatusTrusted {
		return PortableRecipe{}, false, "recipe_not_active"
	}
	intent := strings.Join(strings.Fields(strings.TrimSpace(recipe.Intent)), " ")
	if intent == "" || len([]rune(intent)) > 500 || len(recipe.Steps) == 0 || len(recipe.Steps) > 20 {
		return PortableRecipe{}, false, "invalid_recipe"
	}
	context, ok := portableContext(recipe.Context, canonicalProjectID)
	if !ok {
		return PortableRecipe{}, false, "nonportable_context"
	}
	steps := make([]Step, 0, len(recipe.Steps))
	for _, raw := range recipe.Steps {
		step, portable, reason := portableStep(raw)
		if !portable {
			return PortableRecipe{}, false, reason
		}
		steps = append(steps, step)
	}
	portable := PortableRecipe{
		Version: PortableVersion, ProjectID: strings.TrimSpace(canonicalProjectID), Intent: intent,
		TaskKind: strings.TrimSpace(recipe.TaskKind), Steps: steps, Confidence: recipe.Confidence,
		SuccessCount: max(0, recipe.SuccessCount), FailureCount: max(0, recipe.FailureCount), Status: recipe.Status,
		UpdatedAt: max(int64(0), recipe.UpdatedAt), LastUsedAt: max(int64(0), recipe.LastUsedAt), Context: context,
	}
	portable.ContextHash = FingerprintHash(context)
	portable.ID = portableRecipeID(portable.Intent, portable.TaskKind, portable.Steps)
	return portable, true, "portable"
}

// NormalizePortableRecipe revalidates an untrusted portable payload at the
// Cloud boundary and rewrites project identity to the authenticated canonical
// project binding. This prevents a client from using recipe content to escape
// its resolved tenant/project scope.
func NormalizePortableRecipe(input PortableRecipe, canonicalProjectID string) (PortableRecipe, bool) {
	recipe := Recipe{
		Intent: input.Intent, TaskKind: input.TaskKind, Steps: input.Steps, Confidence: input.Confidence,
		SuccessCount: input.SuccessCount, FailureCount: input.FailureCount, Status: input.Status,
		UpdatedAt: input.UpdatedAt, LastUsedAt: input.LastUsedAt, Context: input.Context,
	}
	portable, ok, _ := PortableRecipeFor(recipe, canonicalProjectID)
	if !ok {
		return PortableRecipe{}, false
	}
	if portable.Confidence < 0 || portable.Confidence > 1 {
		portable.Confidence = 0
	}
	return portable, true
}

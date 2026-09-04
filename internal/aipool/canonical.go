package aipool

import "strings"

var nineRouterProviderPrefixes = map[string]string{
	"cx":   "codex",
	"ag":   "antigravity",
	"cbai": "codebuddy-intl",
	"xai":  "xai",
	"cl":   "cline",

	// Legacy/current 9Router aliases kept for compatibility with older provider
	// connections. Unknown prefixes are intentionally not stripped.
	"cc":      "claude",
	"gc":      "gemini",
	"gh":      "github",
	"glm":     "glm",
	"minimax": "minimax",
	"kimi":    "kimi",
	"if":      "iflow",
	"qw":      "qwen",
	"kr":      "kiro",
}

var nineRouterModelNamespaces = map[string]struct{}{
	"openai": {}, "anthropic": {}, "google": {}, "kwaipilot": {},
}

// CanonicalModelID removes 9Router transport/provider prefixes. Some 9Router
// providers (for example Cline) add an extra vendor namespace, so those are
// removed only after a recognized 9Router provider prefix. The client-facing
// Pool API never exposes provider routing details.
func CanonicalModelID(upstream string) string {
	upstream = strings.TrimSpace(upstream)
	parts := strings.Split(upstream, "/")
	if len(parts) < 2 {
		return upstream
	}
	if _, hidden := nineRouterProviderPrefixes[strings.ToLower(parts[0])]; !hidden {
		return upstream
	}
	parts = parts[1:]
	for len(parts) > 1 {
		if _, hidden := nineRouterModelNamespaces[strings.ToLower(parts[0])]; !hidden {
			break
		}
		parts = parts[1:]
	}
	return strings.TrimSpace(strings.Join(parts, "/"))
}

func nineRouterProviderForModel(upstream string) string {
	parts := strings.SplitN(strings.TrimSpace(upstream), "/", 2)
	if len(parts) != 2 {
		return ""
	}
	return nineRouterProviderPrefixes[strings.ToLower(parts[0])]
}

func nineRouterProviderModelID(upstream string) string {
	parts := strings.SplitN(strings.TrimSpace(upstream), "/", 2)
	if len(parts) != 2 {
		return strings.TrimSpace(upstream)
	}
	if _, hidden := nineRouterProviderPrefixes[strings.ToLower(parts[0])]; !hidden {
		return strings.TrimSpace(upstream)
	}
	return strings.TrimSpace(parts[1])
}

func isProviderQualifiedModel(model string) bool {
	parts := strings.SplitN(strings.TrimSpace(model), "/", 2)
	if len(parts) != 2 {
		return false
	}
	_, hidden := nineRouterProviderPrefixes[strings.ToLower(parts[0])]
	return hidden
}

func modelIDSafe(model string) bool {
	model = strings.TrimSpace(model)
	if model == "" || len(model) > 160 || strings.Contains(model, "/") {
		return false
	}
	for _, char := range model {
		switch {
		case char >= 'a' && char <= 'z':
		case char >= 'A' && char <= 'Z':
		case char >= '0' && char <= '9':
		case strings.ContainsRune("-._:", char):
		default:
			return false
		}
	}
	return true
}

package aipool

import "strings"

var hiddenProviderPrefixes = map[string]struct{}{
	"cc": {}, "cx": {}, "gc": {}, "gh": {}, "glm": {},
	"minimax": {}, "kimi": {}, "if": {}, "qw": {}, "kr": {},
}

// CanonicalModelID removes 9Router transport/provider prefixes. The client-facing
// Pool API only exposes stable model names such as gpt-5.6-sol.
func CanonicalModelID(upstream string) string {
	upstream = strings.TrimSpace(upstream)
	parts := strings.SplitN(upstream, "/", 2)
	if len(parts) == 2 {
		if _, hidden := hiddenProviderPrefixes[strings.ToLower(parts[0])]; hidden {
			return strings.TrimSpace(parts[1])
		}
	}
	return upstream
}

func isProviderQualifiedModel(model string) bool {
	parts := strings.SplitN(strings.TrimSpace(model), "/", 2)
	if len(parts) != 2 {
		return false
	}
	_, hidden := hiddenProviderPrefixes[strings.ToLower(parts[0])]
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

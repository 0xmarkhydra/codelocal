package contextsurface

import (
	"fmt"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/projectbrain"
)

// ExperienceItems turns only promoted, verified Project Brain experience into
// bounded model-visible context. It intentionally stays in the active lane so
// experience can guide work without acquiring mandatory rule authority.
func ExperienceItems(decisions []projectbrain.ExperienceDecision, branch, trigger string, limit int) []Item {
	selected := projectbrain.SelectExperiences(decisions, branch, trigger, limit)
	out := make([]Item, 0, len(selected))
	for _, experience := range selected {
		text := strings.TrimSpace(experience.Statement)
		if len(experience.Preconditions) > 0 {
			text += " Preconditions: " + strings.Join(experience.Preconditions, "; ") + "."
		}
		if experience.Kind == projectbrain.ExperienceFailureRecovery && experience.RecoveryAction != "" {
			text += " Verified recovery: " + experience.RecoveryAction + "."
		}
		priority := 800 + int(experience.Confidence*100)
		out = append(out, Item{ID: "experience:" + experience.ID, Lane: LaneActive, Text: text, Source: fmt.Sprintf("projectbrain:%s", experience.Kind), Priority: priority, Trust: "verified"})
	}
	return out
}

// CompileWithExperiences is the canonical V2 bridge from verified learning back
// into Context Surface. Current task evidence still wins through normal lane
// sorting and mandatory project rules retain fail-closed authority.
func CompileWithExperiences(input Input, decisions []projectbrain.ExperienceDecision, branch, trigger string, experienceLimit, maxTokens int) Surface {
	experiences := ExperienceItems(decisions, branch, trigger, experienceLimit)
	input.Active = append(experiences, input.Active...)
	return Compile(input, maxTokens)
}

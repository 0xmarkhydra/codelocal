package usage

import (
	"encoding/json"
	"fmt"
	"unicode/utf8"
)

// EstimateTokens intentionally reports an approximation, not model billing tokens.
// MCP does not expose ChatGPT's full conversation/model token accounting to CodeLocal.
// We estimate only the payload crossing the MCP boundary using ~4 Unicode chars/token.
func EstimateTokens(value any) (bytes int, tokens int) {
	raw, err := json.Marshal(value)
	if err != nil {
		raw = []byte(fmt.Sprint(value))
	}
	bytes = len(raw)
	chars := utf8.RuneCount(raw)
	if chars == 0 {
		return bytes, 0
	}
	tokens = (chars + 3) / 4
	return bytes, tokens
}

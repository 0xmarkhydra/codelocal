package usage

import "testing"

func TestEstimateTokensUsesMCPPayloadOnly(t *testing.T) {
	bytes, tokens := EstimateTokens(map[string]any{"taskHint": "save avatar"})
	if bytes <= 0 || tokens <= 0 {
		t.Fatalf("expected positive estimate, got bytes=%d tokens=%d", bytes, tokens)
	}
	if tokens >= bytes {
		t.Fatalf("token estimate should be a compact approximation, got bytes=%d tokens=%d", bytes, tokens)
	}
}

func TestEstimateTokensHandlesUnicode(t *testing.T) {
	_, tokens := EstimateTokens(map[string]any{"message": "xin chào Việt Nam"})
	if tokens <= 0 {
		t.Fatalf("expected unicode payload to produce token estimate")
	}
}

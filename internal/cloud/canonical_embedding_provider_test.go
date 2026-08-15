package cloud

import "testing"

func TestCanonicalEmbeddingProviderConfigFailsClosed(t *testing.T) {
	t.Setenv("CODELOCAL_EMBEDDING_PROVIDER", "")
	t.Setenv("OPENROUTER_API_KEY", "")
	t.Setenv("GEMINI_API_KEY", "")

	t.Setenv("CODELOCAL_CANONICAL_EMBEDDINGS", "0")
	provider, err := canonicalEmbeddingProviderFromEnv()
	if err != nil || provider != nil {
		t.Fatalf("disabled canonical embeddings should not construct provider: provider=%T err=%v", provider, err)
	}

	t.Setenv("CODELOCAL_CANONICAL_EMBEDDINGS", "1")
	t.Setenv("CODELOCAL_CANONICAL_EMBEDDING_MODEL_VERSION", "")
	if _, err := canonicalEmbeddingProviderFromEnv(); err == nil {
		t.Fatal("enabled canonical embeddings accepted missing model version")
	}

	t.Setenv("CODELOCAL_CANONICAL_EMBEDDING_MODEL_VERSION", "2026-08")
	if _, err := canonicalEmbeddingProviderFromEnv(); err == nil {
		t.Fatal("enabled canonical embeddings accepted missing semantic provider")
	}
}

func TestCanonicalEmbeddingMaintenanceTimeoutIsBounded(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  string
	}{
		{value: "", want: "12s"},
		{value: "500ms", want: "12s"},
		{value: "2s", want: "2s"},
		{value: "31s", want: "12s"},
		{value: "invalid", want: "12s"},
	} {
		t.Setenv("CODELOCAL_CANONICAL_EMBEDDING_MAINTENANCE_TIMEOUT", tc.value)
		if got := canonicalEmbeddingMaintenanceTimeout().String(); got != tc.want {
			t.Fatalf("timeout for %q=%q want %q", tc.value, got, tc.want)
		}
	}
}

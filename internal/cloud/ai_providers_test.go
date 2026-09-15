package cloud

import (
	"bytes"
	"strings"
	"testing"
)

func TestAIProviderCredentialEnvelopeRoundTripAndTenantBinding(t *testing.T) {
	t.Setenv("CODELOCAL_PROVIDER_CREDENTIAL_KEK", strings.Repeat("k", 48))
	secret := []byte("sk-user-provider-secret")
	wrapNonce, wrappedDEK, secretNonce, ciphertext, err := encryptAIProviderCredential("user-a", "provider-a", secret)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, secret) || bytes.Contains(wrappedDEK, secret) {
		t.Fatal("provider credential leaked into encrypted database material")
	}
	plain, err := decryptAIProviderCredential("user-a", "provider-a", wrapNonce, wrappedDEK, secretNonce, ciphertext)
	if err != nil {
		t.Fatal(err)
	}
	defer zeroAIProviderBytes(plain)
	if string(plain) != string(secret) {
		t.Fatalf("round trip mismatch: %q", plain)
	}
	if _, err := decryptAIProviderCredential("user-b", "provider-a", wrapNonce, wrappedDEK, secretNonce, ciphertext); err == nil {
		t.Fatal("credential envelope must be cryptographically bound to the owning user")
	}
	if _, err := decryptAIProviderCredential("user-a", "provider-b", wrapNonce, wrappedDEK, secretNonce, ciphertext); err == nil {
		t.Fatal("credential envelope must be cryptographically bound to the provider id")
	}
}

func TestNormalizeAIProviderBaseURLRejectsLocalAndInsecureTargets(t *testing.T) {
	bad := []string{
		"http://api.example.com/v1",
		"https://localhost/v1",
		"https://127.0.0.1/v1",
		"https://10.0.0.1/v1",
		"https://192.168.1.10/v1",
		"https://user:pass@example.com/v1",
		"https://example.com/v1?key=secret",
	}
	for _, raw := range bad {
		if _, err := NormalizeAIProviderBaseURL(raw); err == nil {
			t.Fatalf("unsafe provider URL accepted: %s", raw)
		}
	}
	got, err := NormalizeAIProviderBaseURL(" https://api.example.com/v1/ ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://api.example.com/v1" {
		t.Fatalf("normalized URL=%q", got)
	}
}

func TestNormalizeAIProviderModelsIsBoundedAndSafe(t *testing.T) {
	models := normalizeAIProviderModels([]string{"gpt-5.6-sol", "gpt-5.6-sol", "bad model", "openai/gpt-5.6"})
	joined := strings.Join(models, ",")
	if strings.Count(joined, "gpt-5.6-sol") != 1 || !strings.Contains(joined, "openai/gpt-5.6") || strings.Contains(joined, "bad model") {
		t.Fatalf("unexpected model normalization: %#v", models)
	}
}

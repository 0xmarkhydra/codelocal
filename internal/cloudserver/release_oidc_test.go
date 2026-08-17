package cloudserver

import (
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func TestValidateGitHubOIDCClaims(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	claims := githubOIDCClaims{
		Issuer:      githubOIDCIssuer,
		Audience:    json.RawMessage(`"codelocal-release-notify"`),
		ExpiresAt:   now.Add(5 * time.Minute).Unix(),
		IssuedAt:    now.Add(-time.Minute).Unix(),
		NotBefore:   now.Add(-time.Minute).Unix(),
		Repository:  githubReleaseRepo,
		WorkflowRef: githubReleaseWorkflow,
		EventName:   "workflow_run",
	}
	if err := validateGitHubOIDCClaims(claims, now); err != nil {
		t.Fatalf("valid claims rejected: %v", err)
	}

	claims.Repository = "attacker/fork"
	if err := validateGitHubOIDCClaims(claims, now); err == nil {
		t.Fatal("unexpected repository was accepted")
	}
}

func TestOIDCAudienceContainsStringAndArray(t *testing.T) {
	if !oidcAudienceContains(json.RawMessage(`"codelocal-release-notify"`), githubOIDCAudience) {
		t.Fatal("single audience was not accepted")
	}
	if !oidcAudienceContains(json.RawMessage(`["other","codelocal-release-notify"]`), githubOIDCAudience) {
		t.Fatal("audience array was not accepted")
	}
	if oidcAudienceContains(json.RawMessage(`"other"`), githubOIDCAudience) {
		t.Fatal("wrong audience was accepted")
	}
}

func TestReleaseNotifySharedSecretFallback(t *testing.T) {
	const secret = "0123456789abcdef0123456789abcdef"
	old := os.Getenv("CODELOCAL_RELEASE_NOTIFY_SECRET")
	t.Cleanup(func() { _ = os.Setenv("CODELOCAL_RELEASE_NOTIFY_SECRET", old) })
	if err := os.Setenv("CODELOCAL_RELEASE_NOTIFY_SECRET", secret); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest("POST", "/internal/releases/notify", nil)
	request.Header.Set("Authorization", "Bearer "+secret)
	if !releaseNotifyAuthorized(request) {
		t.Fatal("shared secret fallback was rejected")
	}
}

func TestVerifyGitHubOIDCTokenWithCachedSigningKey(t *testing.T) {
	now := time.Now()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	header, _ := json.Marshal(githubOIDCHeader{Algorithm: "RS256", KeyID: "test-key"})
	claims, _ := json.Marshal(githubOIDCClaims{
		Issuer:      githubOIDCIssuer,
		Audience:    json.RawMessage(`"codelocal-release-notify"`),
		ExpiresAt:   now.Add(5 * time.Minute).Unix(),
		IssuedAt:    now.Add(-time.Minute).Unix(),
		NotBefore:   now.Add(-time.Minute).Unix(),
		Repository:  githubReleaseRepo,
		WorkflowRef: githubReleaseWorkflow,
		EventName:   "workflow_run",
	})
	encodedHeader := base64.RawURLEncoding.EncodeToString(header)
	encodedClaims := base64.RawURLEncoding.EncodeToString(claims)
	digest := sha256.Sum256([]byte(encodedHeader + "." + encodedClaims))
	signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, digest[:])
	if err != nil {
		t.Fatal(err)
	}

	githubOIDCKeys.Lock()
	oldKeys, oldExpiry := githubOIDCKeys.Keys, githubOIDCKeys.ExpiresAt
	githubOIDCKeys.Keys = map[string]*rsa.PublicKey{"test-key": &key.PublicKey}
	githubOIDCKeys.ExpiresAt = now.Add(time.Hour)
	githubOIDCKeys.Unlock()
	t.Cleanup(func() {
		githubOIDCKeys.Lock()
		githubOIDCKeys.Keys, githubOIDCKeys.ExpiresAt = oldKeys, oldExpiry
		githubOIDCKeys.Unlock()
	})

	token := encodedHeader + "." + encodedClaims + "." + base64.RawURLEncoding.EncodeToString(signature)
	if err := verifyGitHubOIDCToken(t.Context(), token); err != nil {
		t.Fatalf("valid signed token rejected: %v", err)
	}
}

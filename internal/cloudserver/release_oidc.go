package cloudserver

import (
	"context"
	"crypto"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

const (
	githubOIDCIssuer      = "https://token.actions.githubusercontent.com"
	githubOIDCJWKSURL     = "https://token.actions.githubusercontent.com/.well-known/jwks"
	githubOIDCAudience    = "codelocal-release-notify"
	githubReleaseRepo     = "0xmarkhydra/codelocal"
	githubReleaseWorkflow = "0xmarkhydra/codelocal/.github/workflows/release-email.yml@refs/heads/main"
)

type githubOIDCHeader struct {
	Algorithm string `json:"alg"`
	KeyID     string `json:"kid"`
}

type githubOIDCClaims struct {
	Issuer      string          `json:"iss"`
	Audience    json.RawMessage `json:"aud"`
	ExpiresAt   int64           `json:"exp"`
	IssuedAt    int64           `json:"iat"`
	NotBefore   int64           `json:"nbf"`
	Repository  string          `json:"repository"`
	WorkflowRef string          `json:"workflow_ref"`
	EventName   string          `json:"event_name"`
	Ref         string          `json:"ref"`
}

type githubJWK struct {
	KeyType string `json:"kty"`
	KeyID   string `json:"kid"`
	Use     string `json:"use"`
	N       string `json:"n"`
	E       string `json:"e"`
}

type githubJWKS struct {
	Keys []githubJWK `json:"keys"`
}

var githubOIDCKeys = struct {
	sync.Mutex
	ExpiresAt time.Time
	Keys      map[string]*rsa.PublicKey
}{Keys: map[string]*rsa.PublicKey{}}

func releaseNotifyAuthorized(r *http.Request) bool {
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if !strings.HasPrefix(auth, "Bearer ") {
		return false
	}
	token := strings.TrimSpace(strings.TrimPrefix(auth, "Bearer "))
	if token == "" {
		return false
	}
	if releaseNotifySharedSecretValid(token) {
		return true
	}
	return verifyGitHubOIDCToken(r.Context(), token) == nil
}

func releaseNotifySharedSecretValid(actual string) bool {
	expected := strings.TrimSpace(os.Getenv("CODELOCAL_RELEASE_NOTIFY_SECRET"))
	if expected == "" {
		return false
	}
	a, b := []byte(strings.TrimSpace(actual)), []byte(expected)
	return len(a) == len(b) && len(a) >= 32 && subtle.ConstantTimeCompare(a, b) == 1
}

func verifyGitHubOIDCToken(ctx context.Context, token string) error {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return errors.New("invalid GitHub OIDC token")
	}
	headerBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return errors.New("invalid GitHub OIDC header")
	}
	var header githubOIDCHeader
	if err := json.Unmarshal(headerBytes, &header); err != nil || header.Algorithm != "RS256" || strings.TrimSpace(header.KeyID) == "" {
		return errors.New("unsupported GitHub OIDC header")
	}
	key, err := githubOIDCPublicKey(ctx, header.KeyID)
	if err != nil {
		return err
	}
	signature, err := base64.RawURLEncoding.DecodeString(parts[2])
	if err != nil {
		return errors.New("invalid GitHub OIDC signature")
	}
	digest := sha256.Sum256([]byte(parts[0] + "." + parts[1]))
	if err := rsa.VerifyPKCS1v15(key, crypto.SHA256, digest[:], signature); err != nil {
		return errors.New("GitHub OIDC signature verification failed")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return errors.New("invalid GitHub OIDC payload")
	}
	var claims githubOIDCClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return errors.New("invalid GitHub OIDC claims")
	}
	return validateGitHubOIDCClaims(claims, time.Now())
}

func validateGitHubOIDCClaims(claims githubOIDCClaims, now time.Time) error {
	if claims.Issuer != githubOIDCIssuer {
		return errors.New("unexpected GitHub OIDC issuer")
	}
	if !oidcAudienceContains(claims.Audience, githubOIDCAudience) {
		return errors.New("unexpected GitHub OIDC audience")
	}
	if claims.Repository != githubReleaseRepo || claims.WorkflowRef != githubReleaseWorkflow {
		return errors.New("untrusted GitHub release workflow")
	}
	if claims.EventName != "workflow_run" {
		return errors.New("unexpected GitHub release workflow context")
	}
	unix := now.Unix()
	if claims.ExpiresAt <= unix || claims.ExpiresAt > unix+15*60 {
		return errors.New("expired or invalid GitHub OIDC token")
	}
	if claims.NotBefore > unix+60 || claims.IssuedAt > unix+60 || claims.IssuedAt < unix-15*60 {
		return errors.New("GitHub OIDC token is outside the accepted time window")
	}
	return nil
}

func oidcAudienceContains(raw json.RawMessage, wanted string) bool {
	var single string
	if json.Unmarshal(raw, &single) == nil {
		return single == wanted
	}
	var many []string
	if json.Unmarshal(raw, &many) != nil {
		return false
	}
	for _, item := range many {
		if item == wanted {
			return true
		}
	}
	return false
}

func githubOIDCPublicKey(ctx context.Context, keyID string) (*rsa.PublicKey, error) {
	githubOIDCKeys.Lock()
	if time.Now().Before(githubOIDCKeys.ExpiresAt) {
		key := githubOIDCKeys.Keys[keyID]
		githubOIDCKeys.Unlock()
		if key == nil {
			return nil, errors.New("GitHub OIDC signing key not found")
		}
		return key, nil
	}
	githubOIDCKeys.Unlock()

	request, err := http.NewRequestWithContext(ctx, http.MethodGet, githubOIDCJWKSURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{Timeout: 5 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("load GitHub OIDC keys: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("load GitHub OIDC keys: status %d", response.StatusCode)
	}
	var set githubJWKS
	if err := json.NewDecoder(io.LimitReader(response.Body, 1<<20)).Decode(&set); err != nil {
		return nil, fmt.Errorf("decode GitHub OIDC keys: %w", err)
	}
	keys := make(map[string]*rsa.PublicKey, len(set.Keys))
	for _, item := range set.Keys {
		key, parseErr := rsaKeyFromJWK(item)
		if parseErr == nil {
			keys[item.KeyID] = key
		}
	}
	githubOIDCKeys.Lock()
	githubOIDCKeys.Keys = keys
	githubOIDCKeys.ExpiresAt = time.Now().Add(time.Hour)
	key := githubOIDCKeys.Keys[keyID]
	githubOIDCKeys.Unlock()
	if key == nil {
		return nil, errors.New("GitHub OIDC signing key not found")
	}
	return key, nil
}

func rsaKeyFromJWK(item githubJWK) (*rsa.PublicKey, error) {
	if item.KeyType != "RSA" || item.KeyID == "" || item.N == "" || item.E == "" {
		return nil, errors.New("invalid RSA JWK")
	}
	modulus, err := base64.RawURLEncoding.DecodeString(item.N)
	if err != nil {
		return nil, err
	}
	exponentBytes, err := base64.RawURLEncoding.DecodeString(item.E)
	if err != nil || len(exponentBytes) == 0 || len(exponentBytes) > 4 {
		return nil, errors.New("invalid RSA exponent")
	}
	exponent := 0
	for _, value := range exponentBytes {
		exponent = exponent<<8 | int(value)
	}
	if exponent < 3 {
		return nil, errors.New("invalid RSA exponent")
	}
	return &rsa.PublicKey{N: new(big.Int).SetBytes(modulus), E: exponent}, nil
}

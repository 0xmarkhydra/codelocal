package cloud

import (
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/url"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

const aiProviderMigrationSQL = `
CREATE TABLE IF NOT EXISTS codelocal_ai_providers (
 id TEXT PRIMARY KEY,
 user_id TEXT NOT NULL REFERENCES codelocal_users(id) ON DELETE CASCADE,
 name TEXT NOT NULL,
 base_url TEXT NOT NULL,
 protocol TEXT NOT NULL DEFAULT 'openai_compatible' CHECK (protocol IN ('openai_compatible')),
 models JSONB NOT NULL DEFAULT '[]'::jsonb,
 default_model TEXT NOT NULL DEFAULT '',
 enabled BOOLEAN NOT NULL DEFAULT TRUE,
 key_backend TEXT NOT NULL DEFAULT '',
 wrap_nonce BYTEA,
 wrapped_dek BYTEA,
 secret_nonce BYTEA,
 secret_ciphertext BYTEA,
 last_test_status TEXT NOT NULL DEFAULT '',
 last_test_message TEXT NOT NULL DEFAULT '',
 last_test_at BIGINT NOT NULL DEFAULT 0,
 created_at BIGINT NOT NULL,
 updated_at BIGINT NOT NULL,
 UNIQUE(user_id,id),
 CHECK (
  (key_backend='' AND wrap_nonce IS NULL AND wrapped_dek IS NULL AND secret_nonce IS NULL AND secret_ciphertext IS NULL)
  OR
  (key_backend<>'' AND wrap_nonce IS NOT NULL AND wrapped_dek IS NOT NULL AND secret_nonce IS NOT NULL AND secret_ciphertext IS NOT NULL)
 )
);
CREATE INDEX IF NOT EXISTS idx_codelocal_ai_providers_user_updated
 ON codelocal_ai_providers(user_id, updated_at DESC);
`

const (
	AIProviderProtocolOpenAICompatible = "openai_compatible"
	aiProviderCredentialBackend        = "local-kek-aes-gcm-v1"
)

type AIProvider struct {
	ID              string   `json:"id"`
	UserID          string   `json:"-"`
	Name            string   `json:"name"`
	BaseURL         string   `json:"baseUrl"`
	Protocol        string   `json:"protocol"`
	Models          []string `json:"models"`
	DefaultModel    string   `json:"defaultModel,omitempty"`
	Enabled         bool     `json:"enabled"`
	HasCredential   bool     `json:"hasCredential"`
	LastTestStatus  string   `json:"lastTestStatus,omitempty"`
	LastTestMessage string   `json:"lastTestMessage,omitempty"`
	LastTestAt      int64    `json:"lastTestAt,omitempty"`
	CreatedAt       int64    `json:"createdAt"`
	UpdatedAt       int64    `json:"updatedAt"`
}

func normalizeAIProviderName(value string) string {
	value = strings.TrimSpace(value)
	runes := []rune(value)
	if len(runes) > 80 {
		value = string(runes[:80])
	}
	return value
}

func normalizeAIProviderProtocol(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "", "openai", "openai-compatible", AIProviderProtocolOpenAICompatible:
		return AIProviderProtocolOpenAICompatible
	default:
		return ""
	}
}

func NormalizeAIProviderBaseURL(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) == 0 || len(value) > 500 {
		return "", errors.New("invalid provider base URL")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
		return "", errors.New("provider base URL must be a public HTTPS URL without credentials, query, or fragment")
	}
	host := strings.ToLower(strings.TrimSpace(parsed.Hostname()))
	if host == "" || host == "localhost" || strings.HasSuffix(host, ".localhost") || strings.HasSuffix(host, ".local") {
		return "", errors.New("provider base URL cannot target a local host")
	}
	if ip := net.ParseIP(host); ip != nil && !publicAIProviderIP(ip) {
		return "", errors.New("provider base URL cannot target a private or reserved IP")
	}
	parsed.Path = strings.TrimRight(parsed.Path, "/")
	return strings.TrimRight(parsed.String(), "/"), nil
}

func publicAIProviderIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsUnspecified() || ip.IsMulticast() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() {
		return false
	}
	if ip4 := ip.To4(); ip4 != nil {
		if ip4[0] == 100 && ip4[1] >= 64 && ip4[1] <= 127 {
			return false
		}
		if (ip4[0] == 192 && ip4[1] == 0 && ip4[2] == 2) || (ip4[0] == 198 && ip4[1] == 51 && ip4[2] == 100) || (ip4[0] == 203 && ip4[1] == 0 && ip4[2] == 113) {
			return false
		}
		return ip4[0] < 224
	}
	return !(len(ip) == net.IPv6len && ip[0] == 0x20 && ip[1] == 0x01 && ip[2] == 0x0d && ip[3] == 0xb8)
}

func normalizeAIProviderModels(models []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(models))
	for _, model := range models {
		model = strings.TrimSpace(model)
		if model == "" || len(model) > 160 || seen[model] {
			continue
		}
		safe := true
		for _, ch := range model {
			if (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || strings.ContainsRune("-._:/", ch) {
				continue
			}
			safe = false
			break
		}
		if !safe {
			continue
		}
		seen[model] = true
		out = append(out, model)
		if len(out) >= 200 {
			break
		}
	}
	sort.SliceStable(out, func(i, j int) bool { return strings.ToLower(out[i]) < strings.ToLower(out[j]) })
	return out
}

func aiProviderCredentialMasterKey() ([]byte, error) {
	raw := strings.TrimSpace(os.Getenv("CODELOCAL_PROVIDER_CREDENTIAL_KEK"))
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("CODELOCAL_SECRET_ENCRYPTION_KEY"))
	}
	if raw == "" {
		raw = strings.TrimSpace(os.Getenv("MCP_AUTH_SECRET"))
	}
	if len(raw) < 32 {
		return nil, errors.New("AI provider credential encryption requires a server key with at least 32 characters")
	}
	sum := sha256.Sum256([]byte("codelocal-ai-provider-kek-v1\x00" + raw))
	key := make([]byte, len(sum))
	copy(key, sum[:])
	return key, nil
}

func newAIProviderAEAD(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

func aiProviderAAD(userID, providerID, purpose string) []byte {
	return []byte("codelocal-ai-provider-v1\x00" + userID + "\x00" + providerID + "\x00" + purpose)
}

func encryptAIProviderCredential(userID, providerID string, plaintext []byte) (wrapNonce, wrappedDEK, secretNonce, ciphertext []byte, err error) {
	master, err := aiProviderCredentialMasterKey()
	if err != nil {
		return nil, nil, nil, nil, err
	}
	defer zeroAIProviderBytes(master)
	kek, err := newAIProviderAEAD(master)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	dek := make([]byte, 32)
	if _, err = io.ReadFull(rand.Reader, dek); err != nil {
		return nil, nil, nil, nil, err
	}
	defer zeroAIProviderBytes(dek)
	dataAEAD, err := newAIProviderAEAD(dek)
	if err != nil {
		return nil, nil, nil, nil, err
	}
	secretNonce = make([]byte, dataAEAD.NonceSize())
	if _, err = io.ReadFull(rand.Reader, secretNonce); err != nil {
		return nil, nil, nil, nil, err
	}
	ciphertext = dataAEAD.Seal(nil, secretNonce, plaintext, aiProviderAAD(userID, providerID, "secret"))
	wrapNonce = make([]byte, kek.NonceSize())
	if _, err = io.ReadFull(rand.Reader, wrapNonce); err != nil {
		return nil, nil, nil, nil, err
	}
	wrappedDEK = kek.Seal(nil, wrapNonce, dek, aiProviderAAD(userID, providerID, "dek"))
	return wrapNonce, wrappedDEK, secretNonce, ciphertext, nil
}

func decryptAIProviderCredential(userID, providerID string, wrapNonce, wrappedDEK, secretNonce, ciphertext []byte) ([]byte, error) {
	master, err := aiProviderCredentialMasterKey()
	if err != nil {
		return nil, err
	}
	defer zeroAIProviderBytes(master)
	kek, err := newAIProviderAEAD(master)
	if err != nil {
		return nil, err
	}
	dek, err := kek.Open(nil, wrapNonce, wrappedDEK, aiProviderAAD(userID, providerID, "dek"))
	if err != nil {
		return nil, err
	}
	defer zeroAIProviderBytes(dek)
	dataAEAD, err := newAIProviderAEAD(dek)
	if err != nil {
		return nil, err
	}
	return dataAEAD.Open(nil, secretNonce, ciphertext, aiProviderAAD(userID, providerID, "secret"))
}

func zeroAIProviderBytes(value []byte) {
	for index := range value {
		value[index] = 0
	}
}

// ZeroAIProviderCredential performs best-effort zeroization after the short-lived
// plaintext credential leaves the provider call path. Full RAM confidentiality
// still requires a confidential-computing/TEE deployment; normal Go/http code
// must materialize an Authorization header briefly to call the upstream.
func ZeroAIProviderCredential(value []byte) {
	zeroAIProviderBytes(value)
}

func (s *Store) CreateAIProvider(ctx context.Context, userID, name, baseURL, protocol, apiKey string, models []string) (AIProvider, error) {
	if s == nil || s.DB == nil || strings.TrimSpace(userID) == "" {
		return AIProvider{}, errors.New("AI provider store unavailable")
	}
	name = normalizeAIProviderName(name)
	if name == "" {
		return AIProvider{}, errors.New("provider name is required")
	}
	var providerCount int
	if err := s.DB.QueryRow(ctx, `SELECT COUNT(*) FROM codelocal_ai_providers WHERE user_id=$1`, userID).Scan(&providerCount); err != nil {
		return AIProvider{}, err
	}
	if providerCount >= 20 {
		return AIProvider{}, errors.New("AI provider limit reached")
	}
	var err error
	baseURL, err = NormalizeAIProviderBaseURL(baseURL)
	if err != nil {
		return AIProvider{}, err
	}
	protocol = normalizeAIProviderProtocol(protocol)
	if protocol == "" {
		return AIProvider{}, errors.New("unsupported provider protocol")
	}
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" || len(apiKey) > 16384 {
		return AIProvider{}, errors.New("provider API key is required")
	}
	id := RandomHex(16)
	secret := []byte(apiKey)
	wrapNonce, wrappedDEK, secretNonce, ciphertext, err := encryptAIProviderCredential(userID, id, secret)
	zeroAIProviderBytes(secret)
	if err != nil {
		return AIProvider{}, err
	}
	models = normalizeAIProviderModels(models)
	defaultModel := ""
	if len(models) > 0 {
		defaultModel = models[0]
	}
	modelsRaw, _ := json.Marshal(models)
	now := time.Now().UnixMilli()
	_, err = s.DB.Exec(ctx, `INSERT INTO codelocal_ai_providers(id,user_id,name,base_url,protocol,models,default_model,enabled,key_backend,wrap_nonce,wrapped_dek,secret_nonce,secret_ciphertext,created_at,updated_at) VALUES($1,$2,$3,$4,$5,$6,$7,TRUE,$8,$9,$10,$11,$12,$13,$13)`, id, userID, name, baseURL, protocol, modelsRaw, defaultModel, aiProviderCredentialBackend, wrapNonce, wrappedDEK, secretNonce, ciphertext, now)
	if err != nil {
		return AIProvider{}, err
	}
	return AIProvider{ID: id, UserID: userID, Name: name, BaseURL: baseURL, Protocol: protocol, Models: models, DefaultModel: defaultModel, Enabled: true, HasCredential: true, CreatedAt: now, UpdatedAt: now}, nil
}

const aiProviderSelectColumns = `id,user_id,name,base_url,protocol,models,default_model,enabled,key_backend,secret_ciphertext,last_test_status,last_test_message,last_test_at,created_at,updated_at`

func scanAIProvider(row pgx.Row) (AIProvider, error) {
	var out AIProvider
	var modelsRaw []byte
	var keyBackend string
	var ciphertext []byte
	err := row.Scan(&out.ID, &out.UserID, &out.Name, &out.BaseURL, &out.Protocol, &modelsRaw, &out.DefaultModel, &out.Enabled, &keyBackend, &ciphertext, &out.LastTestStatus, &out.LastTestMessage, &out.LastTestAt, &out.CreatedAt, &out.UpdatedAt)
	if err != nil {
		return out, err
	}
	_ = json.Unmarshal(modelsRaw, &out.Models)
	out.Models = normalizeAIProviderModels(out.Models)
	out.HasCredential = keyBackend != "" && len(ciphertext) > 0
	return out, nil
}

func (s *Store) ListAIProviders(ctx context.Context, userID string) ([]AIProvider, error) {
	rows, err := s.DB.Query(ctx, `SELECT `+aiProviderSelectColumns+` FROM codelocal_ai_providers WHERE user_id=$1 ORDER BY updated_at DESC,name ASC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []AIProvider{}
	for rows.Next() {
		var provider AIProvider
		var modelsRaw []byte
		var keyBackend string
		var ciphertext []byte
		if err := rows.Scan(&provider.ID, &provider.UserID, &provider.Name, &provider.BaseURL, &provider.Protocol, &modelsRaw, &provider.DefaultModel, &provider.Enabled, &keyBackend, &ciphertext, &provider.LastTestStatus, &provider.LastTestMessage, &provider.LastTestAt, &provider.CreatedAt, &provider.UpdatedAt); err != nil {
			return nil, err
		}
		_ = json.Unmarshal(modelsRaw, &provider.Models)
		provider.Models = normalizeAIProviderModels(provider.Models)
		provider.HasCredential = keyBackend != "" && len(ciphertext) > 0
		out = append(out, provider)
	}
	return out, rows.Err()
}

func (s *Store) GetAIProvider(ctx context.Context, userID, providerID string) (AIProvider, error) {
	return scanAIProvider(s.DB.QueryRow(ctx, `SELECT `+aiProviderSelectColumns+` FROM codelocal_ai_providers WHERE user_id=$1 AND id=$2`, userID, strings.TrimSpace(providerID)))
}

func (s *Store) UpdateAIProvider(ctx context.Context, userID, providerID, name, baseURL, protocol string, models []string, enabled bool) error {
	name = normalizeAIProviderName(name)
	if name == "" {
		return errors.New("provider name is required")
	}
	var err error
	baseURL, err = NormalizeAIProviderBaseURL(baseURL)
	if err != nil {
		return err
	}
	protocol = normalizeAIProviderProtocol(protocol)
	if protocol == "" {
		return errors.New("unsupported provider protocol")
	}
	models = normalizeAIProviderModels(models)
	defaultModel := ""
	if len(models) > 0 {
		defaultModel = models[0]
	}
	modelsRaw, _ := json.Marshal(models)
	command, err := s.DB.Exec(ctx, `UPDATE codelocal_ai_providers SET name=$3,base_url=$4,protocol=$5,models=$6,default_model=$7,enabled=$8,updated_at=$9 WHERE user_id=$1 AND id=$2`, userID, providerID, name, baseURL, protocol, modelsRaw, defaultModel, enabled, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) RotateAIProviderCredential(ctx context.Context, userID, providerID, apiKey string) error {
	apiKey = strings.TrimSpace(apiKey)
	if apiKey == "" || len(apiKey) > 16384 {
		return errors.New("provider API key is required")
	}
	secret := []byte(apiKey)
	wrapNonce, wrappedDEK, secretNonce, ciphertext, err := encryptAIProviderCredential(userID, providerID, secret)
	zeroAIProviderBytes(secret)
	if err != nil {
		return err
	}
	command, err := s.DB.Exec(ctx, `UPDATE codelocal_ai_providers SET key_backend=$3,wrap_nonce=$4,wrapped_dek=$5,secret_nonce=$6,secret_ciphertext=$7,last_test_status='',last_test_message='',last_test_at=0,updated_at=$8 WHERE user_id=$1 AND id=$2`, userID, providerID, aiProviderCredentialBackend, wrapNonce, wrappedDEK, secretNonce, ciphertext, time.Now().UnixMilli())
	if err != nil {
		return err
	}
	if command.RowsAffected() == 0 {
		return pgx.ErrNoRows
	}
	return nil
}

func (s *Store) MaterializeAIProviderCredential(ctx context.Context, userID, providerID string) ([]byte, error) {
	var backend string
	var wrapNonce, wrappedDEK, secretNonce, ciphertext []byte
	err := s.DB.QueryRow(ctx, `SELECT key_backend,wrap_nonce,wrapped_dek,secret_nonce,secret_ciphertext FROM codelocal_ai_providers WHERE user_id=$1 AND id=$2 AND enabled=TRUE`, userID, providerID).Scan(&backend, &wrapNonce, &wrappedDEK, &secretNonce, &ciphertext)
	if err != nil {
		return nil, err
	}
	if backend != aiProviderCredentialBackend {
		return nil, errors.New("unsupported provider credential backend")
	}
	return decryptAIProviderCredential(userID, providerID, wrapNonce, wrappedDEK, secretNonce, ciphertext)
}

func (s *Store) UpdateAIProviderProbe(ctx context.Context, userID, providerID, status, message string, models []string) error {
	status = strings.TrimSpace(status)
	if status != "ok" && status != "error" {
		status = "error"
	}
	message = strings.TrimSpace(message)
	if len(message) > 240 {
		message = message[:240]
	}
	models = normalizeAIProviderModels(models)
	modelsRaw, _ := json.Marshal(models)
	defaultModel := ""
	if len(models) > 0 {
		defaultModel = models[0]
	}
	_, err := s.DB.Exec(ctx, `UPDATE codelocal_ai_providers SET models=CASE WHEN $5::jsonb='[]'::jsonb THEN models ELSE $5::jsonb END,default_model=CASE WHEN $6='' THEN default_model ELSE $6 END,last_test_status=$3,last_test_message=$4,last_test_at=$7,updated_at=$7 WHERE user_id=$1 AND id=$2`, userID, providerID, status, message, string(modelsRaw), defaultModel, time.Now().UnixMilli())
	return err
}

func (s *Store) DeleteAIProvider(ctx context.Context, userID, providerID string) error {
	_, err := s.DB.Exec(ctx, `DELETE FROM codelocal_ai_providers WHERE user_id=$1 AND id=$2`, userID, strings.TrimSpace(providerID))
	return err
}

package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

// OIDCAuthorizationCode is short-lived, single-use state for CodeLocal's
// first-party Penpot OIDC client. It lives in the existing Redis service; no
// separate identity database is introduced.
type OIDCAuthorizationCode struct {
	UserID        string `json:"userId"`
	ClientID      string `json:"clientId"`
	RedirectURI   string `json:"redirectUri"`
	Scope         string `json:"scope"`
	Nonce         string `json:"nonce,omitempty"`
	CodeChallenge string `json:"codeChallenge,omitempty"`
	AuthTime      int64  `json:"authTime"`
	ExpiresAt     int64  `json:"expiresAt"`
}

func oidcAuthorizationCodeKey(code string) string {
	return "codelocal:oidc-code:" + HashSecret(strings.TrimSpace(code))[:32]
}

func (s *Store) PutOIDCAuthorizationCode(ctx context.Context, code string, record OIDCAuthorizationCode, ttl time.Duration) error {
	if s == nil || s.Redis == nil || strings.TrimSpace(code) == "" || strings.TrimSpace(record.UserID) == "" || strings.TrimSpace(record.ClientID) == "" || strings.TrimSpace(record.RedirectURI) == "" || ttl <= 0 {
		return errors.New("invalid OIDC authorization code")
	}
	raw, err := json.Marshal(record)
	if err != nil {
		return err
	}
	created, err := s.Redis.SetNX(ctx, oidcAuthorizationCodeKey(code), raw, ttl).Result()
	if err != nil {
		return err
	}
	if !created {
		return errors.New("OIDC authorization code collision")
	}
	return nil
}

func (s *Store) ConsumeOIDCAuthorizationCode(ctx context.Context, code string) (*OIDCAuthorizationCode, error) {
	if s == nil || s.Redis == nil || strings.TrimSpace(code) == "" {
		return nil, nil
	}
	raw, err := s.Redis.GetDel(ctx, oidcAuthorizationCodeKey(code)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var record OIDCAuthorizationCode
	if err := json.Unmarshal(raw, &record); err != nil {
		return nil, err
	}
	return &record, nil
}

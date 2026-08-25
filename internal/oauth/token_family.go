package oauth

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

const refreshRetryGrace = 30 * time.Second

var (
	ErrTokenStateUnavailable = errors.New("oauth token state unavailable")
	ErrTokenRevoked          = errors.New("oauth token revoked")
)

func (s *Server) deriveTokenID(label string, parts ...string) string {
	mac := hmac.New(sha256.New, s.Secret)
	_, _ = mac.Write([]byte("codelocal-oauth-v2\x00" + label))
	for _, part := range parts {
		_, _ = mac.Write([]byte("\x00" + part))
	}
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil)[:18])
}

func oauthFamilyKey(userID, clientID, familyID string) string {
	return "codelocal:oauth-family:" + cloud.HashSecret(userID + "\x00" + clientID + "\x00" + familyID)[:32]
}

func validateTokenAgainstSecurityState(payload tokenPayload, state cloud.UserSecurityState) error {
	if payload.SecurityVersion > 0 {
		if payload.SecurityVersion != state.Version {
			return ErrTokenRevoked
		}
		return nil
	}
	if state.PasswordChangedAt > 0 && payload.IssuedAt*1000 < state.PasswordChangedAt {
		return ErrTokenRevoked
	}
	return nil
}

func (s *Server) userSecurityState(ctx context.Context, userID string) (cloud.UserSecurityState, error) {
	if s.Store == nil {
		return cloud.UserSecurityState{}, fmt.Errorf("%w: security store missing", ErrTokenStateUnavailable)
	}
	state, err := s.Store.SecurityStateForUser(ctx, userID)
	if err == nil {
		return state, nil
	}
	if err.Error() == "USER_NOT_FOUND" {
		return cloud.UserSecurityState{}, ErrTokenRevoked
	}
	return cloud.UserSecurityState{}, fmt.Errorf("%w: %v", ErrTokenStateUnavailable, err)
}

func (s *Server) validateTokenSecurity(ctx context.Context, payload tokenPayload) error {
	state, err := s.userSecurityState(ctx, payload.Subject)
	if err != nil {
		return err
	}
	return validateTokenAgainstSecurityState(payload, state)
}

func (s *Server) startTokenFamily(ctx context.Context, userID, clientID, familyID, refreshJTI string, issuedAt int64) error {
	if s.Store == nil || s.Store.Redis == nil {
		return ErrTokenStateUnavailable
	}
	key := oauthFamilyKey(userID, clientID, familyID)
	pipe := s.Store.Redis.TxPipeline()
	pipe.HSet(ctx, key, map[string]any{
		"family":    familyID,
		"current":   refreshJTI,
		"previous":  "",
		"issuedAt":  issuedAt,
		"rotatedAt": int64(0),
		"revoked":   0,
	})
	pipe.Expire(ctx, key, s.RefreshTTL)
	_, err := pipe.Exec(ctx)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrTokenStateUnavailable, err)
	}
	return nil
}

func (s *Server) issueInitial(ctx context.Context, userID, clientID, resource, scope string) (map[string]any, error) {
	state, err := s.userSecurityState(ctx, userID)
	if err != nil {
		return nil, err
	}
	familyID := randomURL(18)
	refreshJTI := randomURL(18)
	issuedAt := time.Now().Unix()
	if err := s.startTokenFamily(ctx, userID, clientID, familyID, refreshJTI, issuedAt); err != nil {
		return nil, err
	}
	return s.tokenPair(userID, clientID, resource, scope, familyID, state.Version, refreshJTI, issuedAt), nil
}

type refreshRotation struct {
	Status     string
	FamilyID   string
	RefreshJTI string
	IssuedAt   int64
}

func (s *Server) rotateRefreshState(ctx context.Context, payload tokenPayload, securityVersion int64) (refreshRotation, error) {
	if s.Store == nil || s.Store.Redis == nil {
		return refreshRotation{}, ErrTokenStateUnavailable
	}
	familyID := payload.FamilyID
	if familyID == "" {
		familyID = s.deriveTokenID("legacy-family", payload.Subject, payload.ClientID, payload.JTI)
	}
	nextJTI := randomURL(18)
	now := time.Now()
	const script = `
local key=KEYS[1]
local family=ARGV[1]
local presented=ARGV[2]
local next=ARGV[3]
local issued=ARGV[4]
local nowms=tonumber(ARGV[5])
local grace=tonumber(ARGV[6])
local ttl=tonumber(ARGV[7])
if redis.call('EXISTS',key)==0 then
 redis.call('HSET',key,'family',family,'current',presented,'previous','','issuedAt',issued,'rotatedAt','0','revoked','0')
 redis.call('PEXPIRE',key,ttl)
end
local storedFamily=redis.call('HGET',key,'family') or ''
if storedFamily~=family then return {'family_mismatch',storedFamily,'','0'} end
if (redis.call('HGET',key,'revoked') or '0')=='1' then return {'revoked',storedFamily,'','0'} end
local current=redis.call('HGET',key,'current') or ''
if current==presented then
 redis.call('HSET',key,'previous',current,'current',next,'issuedAt',issued,'rotatedAt',tostring(nowms))
 redis.call('PEXPIRE',key,ttl)
 return {'rotated',storedFamily,next,issued}
end
local previous=redis.call('HGET',key,'previous') or ''
local rotated=tonumber(redis.call('HGET',key,'rotatedAt') or '0')
if previous==presented and rotated>0 and (nowms-rotated)<=grace then
 return {'retry',storedFamily,current,redis.call('HGET',key,'issuedAt') or issued}
end
if previous==presented then
 return {'stale',storedFamily,'','0'}
end
redis.call('HSET',key,'revoked','1','replayAt',tostring(nowms))
redis.call('PEXPIRE',key,ttl)
return {'replay',storedFamily,'','0'}
`
	values, err := s.Store.Redis.Eval(ctx, script, []string{oauthFamilyKey(payload.Subject, payload.ClientID, familyID)}, familyID, payload.JTI, nextJTI, now.Unix(), now.UnixMilli(), refreshRetryGrace.Milliseconds(), s.RefreshTTL.Milliseconds()).Slice()
	if err != nil {
		return refreshRotation{}, fmt.Errorf("%w: %v", ErrTokenStateUnavailable, err)
	}
	if len(values) != 4 {
		return refreshRotation{}, fmt.Errorf("%w: invalid token family response", ErrTokenStateUnavailable)
	}
	rotation := refreshRotation{Status: fmt.Sprint(values[0]), FamilyID: fmt.Sprint(values[1]), RefreshJTI: fmt.Sprint(values[2])}
	rotation.IssuedAt, _ = strconv.ParseInt(strings.TrimSpace(fmt.Sprint(values[3])), 10, 64)
	return rotation, nil
}

func (s *Server) rotateRefresh(ctx context.Context, payload tokenPayload) (map[string]any, error) {
	state, err := s.userSecurityState(ctx, payload.Subject)
	if err != nil {
		return nil, err
	}
	if err := validateTokenAgainstSecurityState(payload, state); err != nil {
		return nil, err
	}
	rotation, err := s.rotateRefreshState(ctx, payload, state.Version)
	if err != nil {
		return nil, err
	}
	switch rotation.Status {
	case "rotated", "retry":
		if rotation.IssuedAt <= 0 || rotation.RefreshJTI == "" || rotation.FamilyID == "" {
			return nil, errors.New("invalid oauth refresh rotation")
		}
		return s.tokenPair(payload.Subject, payload.ClientID, payload.Resource, payload.Scope, rotation.FamilyID, state.Version, rotation.RefreshJTI, rotation.IssuedAt), nil
	case "replay", "revoked":
		return nil, ErrTokenRevoked
	default:
		return nil, ErrTokenRevoked
	}
}

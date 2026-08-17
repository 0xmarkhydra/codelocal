package cloud

import (
	"context"
	"strings"
	"time"
)

const deviceNonceTTL = 3 * time.Minute

func deviceNonceKey(credentialID string) string {
	return "codelocal:device-nonces:" + HashSecret(credentialID)[:32]
}

func (s *Store) ConsumeDeviceNonce(ctx context.Context, credentialID, nonce string, now time.Time) (bool, error) {
	if s == nil || s.Redis == nil || strings.TrimSpace(credentialID) == "" || strings.TrimSpace(nonce) == "" {
		return false, nil
	}
	const script = `
local key=KEYS[1]
local nonce=ARGV[1]
local nowms=tonumber(ARGV[2])
local ttl=tonumber(ARGV[3])
redis.call('ZREMRANGEBYSCORE',key,'-inf',nowms-ttl)
if redis.call('ZSCORE',key,nonce) then return 0 end
redis.call('ZADD',key,nowms,nonce)
redis.call('PEXPIRE',key,ttl)
return 1
`
	used, err := s.Redis.Eval(ctx, script, []string{deviceNonceKey(credentialID)}, strings.TrimSpace(nonce), now.UnixMilli(), deviceNonceTTL.Milliseconds()).Int()
	return used == 1, err
}

func (s *Store) ClearDeviceNonceState(ctx context.Context, credentialID string) error {
	if s == nil || s.Redis == nil || strings.TrimSpace(credentialID) == "" {
		return nil
	}
	return s.Redis.Del(ctx, deviceNonceKey(credentialID)).Err()
}

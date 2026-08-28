package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const defaultRuntimeAliasTTL = 35 * time.Minute

func runtimeAliasKey(sourceClientKey string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(sourceClientKey)))
	return "codelocal:runtime-alias:v1:" + hex.EncodeToString(digest[:])
}

func (c *Coordinator) BindRuntimeAlias(ctx context.Context, sourceClientKey, targetClientKey string, ttl time.Duration) error {
	if c == nil || c.Redis == nil {
		return errors.New("runtime alias coordinator unavailable")
	}
	sourceClientKey = strings.TrimSpace(sourceClientKey)
	targetClientKey = strings.TrimSpace(targetClientKey)
	if sourceClientKey == "" || targetClientKey == "" {
		return errors.New("runtime alias source and target are required")
	}
	if sourceClientKey == targetClientKey {
		return c.Redis.Del(ctx, runtimeAliasKey(sourceClientKey)).Err()
	}
	if ttl <= 0 {
		ttl = defaultRuntimeAliasTTL
	}
	return c.Redis.Set(ctx, runtimeAliasKey(sourceClientKey), targetClientKey, ttl).Err()
}

func (c *Coordinator) ResolveRuntimeAlias(ctx context.Context, clientKey string) (string, error) {
	clientKey = strings.TrimSpace(clientKey)
	if clientKey == "" || c == nil || c.Redis == nil {
		return clientKey, nil
	}
	target, err := c.Redis.Get(ctx, runtimeAliasKey(clientKey)).Result()
	if errors.Is(err, redis.Nil) {
		return clientKey, nil
	}
	if err != nil {
		return "", err
	}
	target = strings.TrimSpace(target)
	if target == "" || target == clientKey {
		return clientKey, nil
	}
	return target, nil
}

func (c *Coordinator) ReleaseRuntimeAlias(ctx context.Context, sourceClientKey string) error {
	if c == nil || c.Redis == nil || strings.TrimSpace(sourceClientKey) == "" {
		return nil
	}
	return c.Redis.Del(ctx, runtimeAliasKey(sourceClientKey)).Err()
}

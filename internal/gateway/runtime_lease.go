package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrRuntimeLeaseHeld = errors.New("runtime lease already held")
	ErrRuntimeLeaseLost = errors.New("runtime lease ownership lost")
)

const defaultRuntimeLeaseTTL = 30 * time.Second

type RuntimeLeaseScope struct {
	UserID       string
	WorkspaceKey string
	Provider     RuntimeProviderKind
	Profile      string
}

type RuntimeLeaseBackend interface {
	Acquire(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	Renew(ctx context.Context, key, token string, ttl time.Duration) (bool, error)
	Release(ctx context.Context, key, token string) (bool, error)
}

type RuntimeLeaseCoordinator struct {
	Backend RuntimeLeaseBackend
	TTL     time.Duration
}

type RuntimeLease struct {
	key     string
	token   string
	ttl     time.Duration
	backend RuntimeLeaseBackend
}

func NewRuntimeLeaseCoordinator(backend RuntimeLeaseBackend, ttl time.Duration) *RuntimeLeaseCoordinator {
	if ttl <= 0 {
		ttl = defaultRuntimeLeaseTTL
	}
	return &RuntimeLeaseCoordinator{Backend: backend, TTL: ttl}
}

func (c *RuntimeLeaseCoordinator) Acquire(ctx context.Context, scope RuntimeLeaseScope) (*RuntimeLease, error) {
	if c == nil || c.Backend == nil {
		return nil, errors.New("runtime lease backend unavailable")
	}
	if err := validateRuntimeLeaseScope(scope); err != nil {
		return nil, err
	}
	token, err := randomLeaseToken()
	if err != nil {
		return nil, fmt.Errorf("create runtime lease token: %w", err)
	}
	key := runtimeLeaseKey(scope)
	ok, err := c.Backend.Acquire(ctx, key, token, c.TTL)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, ErrRuntimeLeaseHeld
	}
	return &RuntimeLease{key: key, token: token, ttl: c.TTL, backend: c.Backend}, nil
}

func (l *RuntimeLease) Renew(ctx context.Context) error {
	if l == nil || l.backend == nil || l.key == "" || l.token == "" {
		return ErrRuntimeLeaseLost
	}
	ok, err := l.backend.Renew(ctx, l.key, l.token, l.ttl)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRuntimeLeaseLost
	}
	return nil
}

func (l *RuntimeLease) Release(ctx context.Context) error {
	if l == nil || l.backend == nil || l.key == "" || l.token == "" {
		return nil
	}
	ok, err := l.backend.Release(ctx, l.key, l.token)
	if err != nil {
		return err
	}
	if !ok {
		return ErrRuntimeLeaseLost
	}
	l.key = ""
	l.token = ""
	return nil
}

func validateRuntimeLeaseScope(scope RuntimeLeaseScope) error {
	if strings.TrimSpace(scope.UserID) == "" {
		return errors.New("runtime lease userId required")
	}
	if strings.TrimSpace(scope.WorkspaceKey) == "" {
		return errors.New("runtime lease workspaceKey required")
	}
	if scope.Provider == "" || scope.Provider == RuntimeProviderAuto {
		return errors.New("runtime lease concrete provider required")
	}
	if strings.TrimSpace(scope.Profile) == "" {
		return errors.New("runtime lease profile required")
	}
	return nil
}

func runtimeLeaseKey(scope RuntimeLeaseScope) string {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		strings.TrimSpace(scope.UserID),
		strings.TrimSpace(scope.WorkspaceKey),
		string(scope.Provider),
		strings.TrimSpace(scope.Profile),
	}, "\x00")))
	return "codelocal:runtime-lease:v1:" + hex.EncodeToString(digest[:])
}

func randomLeaseToken() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

type RedisRuntimeLeaseBackend struct {
	Client *redis.Client
}

func NewRedisRuntimeLeaseBackend(client *redis.Client) *RedisRuntimeLeaseBackend {
	return &RedisRuntimeLeaseBackend{Client: client}
}

func (b *RedisRuntimeLeaseBackend) Acquire(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	if b == nil || b.Client == nil {
		return false, errors.New("runtime lease Redis unavailable")
	}
	return b.Client.SetNX(ctx, key, token, ttl).Result()
}

func (b *RedisRuntimeLeaseBackend) Renew(ctx context.Context, key, token string, ttl time.Duration) (bool, error) {
	if b == nil || b.Client == nil {
		return false, errors.New("runtime lease Redis unavailable")
	}
	const script = `if redis.call('GET',KEYS[1])==ARGV[1] then redis.call('PEXPIRE',KEYS[1],ARGV[2]); return 1 else return 0 end`
	value, err := b.Client.Eval(ctx, script, []string{key}, token, ttl.Milliseconds()).Int()
	return value == 1, err
}

func (b *RedisRuntimeLeaseBackend) Release(ctx context.Context, key, token string) (bool, error) {
	if b == nil || b.Client == nil {
		return false, errors.New("runtime lease Redis unavailable")
	}
	const script = `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end`
	value, err := b.Client.Eval(ctx, script, []string{key}, token).Int()
	return value == 1, err
}

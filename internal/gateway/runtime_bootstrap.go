package gateway

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

var (
	ErrRuntimeBootstrapNotFound = errors.New("runtime bootstrap token not found")
	ErrRuntimeBootstrapConsumed = errors.New("runtime bootstrap token already consumed")
)

const defaultRuntimeBootstrapTTL = 2 * time.Minute

type RuntimeBootstrapState struct {
	UserID           string `json:"userId"`
	WorkspaceKey     string `json:"workspaceKey"`
	WorkspaceID      string `json:"workspaceId"`
	Profile          string `json:"profile"`
	RuntimeSessionID string `json:"runtimeSessionId"`
	DeviceID         string `json:"deviceId"`
	DeviceName       string `json:"deviceName"`
	CreatedAt        int64  `json:"createdAt"`
	ExpiresAt        int64  `json:"expiresAt"`
}

type RuntimeBootstrapBackend interface {
	Put(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error)
	Take(ctx context.Context, key string) ([]byte, bool, error)
}

type RuntimeBootstrapStore struct {
	Backend RuntimeBootstrapBackend
	TTL     time.Duration
	Now     func() time.Time
}

func NewRuntimeBootstrapStore(backend RuntimeBootstrapBackend, ttl time.Duration) *RuntimeBootstrapStore {
	if ttl <= 0 {
		ttl = defaultRuntimeBootstrapTTL
	}
	return &RuntimeBootstrapStore{Backend: backend, TTL: ttl, Now: time.Now}
}

func (s *RuntimeBootstrapStore) Issue(ctx context.Context, state RuntimeBootstrapState) (string, error) {
	if s == nil || s.Backend == nil {
		return "", errors.New("runtime bootstrap backend unavailable")
	}
	if err := validateRuntimeBootstrapState(state); err != nil {
		return "", err
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	createdAt := now().UTC()
	state.CreatedAt = createdAt.UnixMilli()
	state.ExpiresAt = createdAt.Add(s.TTL).UnixMilli()
	payload, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	for attempt := 0; attempt < 3; attempt++ {
		token, err := newRuntimeBootstrapToken()
		if err != nil {
			return "", err
		}
		ok, err := s.Backend.Put(ctx, runtimeBootstrapKey(token), payload, s.TTL)
		if err != nil {
			return "", err
		}
		if ok {
			return token, nil
		}
	}
	return "", errors.New("runtime bootstrap token allocation failed")
}

func (s *RuntimeBootstrapStore) Consume(ctx context.Context, token string) (RuntimeBootstrapState, error) {
	if s == nil || s.Backend == nil {
		return RuntimeBootstrapState{}, errors.New("runtime bootstrap backend unavailable")
	}
	if strings.TrimSpace(token) == "" {
		return RuntimeBootstrapState{}, ErrRuntimeBootstrapNotFound
	}
	raw, ok, err := s.Backend.Take(ctx, runtimeBootstrapKey(token))
	if err != nil {
		return RuntimeBootstrapState{}, err
	}
	if !ok {
		return RuntimeBootstrapState{}, ErrRuntimeBootstrapConsumed
	}
	var state RuntimeBootstrapState
	if err := json.Unmarshal(raw, &state); err != nil {
		return RuntimeBootstrapState{}, errors.New("invalid runtime bootstrap state")
	}
	if err := validateRuntimeBootstrapState(state); err != nil {
		return RuntimeBootstrapState{}, err
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	if state.ExpiresAt <= now().UTC().UnixMilli() {
		return RuntimeBootstrapState{}, ErrRuntimeBootstrapNotFound
	}
	return state, nil
}

func validateRuntimeBootstrapState(state RuntimeBootstrapState) error {
	if strings.TrimSpace(state.UserID) == "" {
		return errors.New("runtime bootstrap userId required")
	}
	if strings.TrimSpace(state.WorkspaceKey) == "" {
		return errors.New("runtime bootstrap workspaceKey required")
	}
	if strings.TrimSpace(state.WorkspaceID) == "" {
		return errors.New("runtime bootstrap workspaceId required")
	}
	if strings.TrimSpace(state.Profile) == "" {
		return errors.New("runtime bootstrap profile required")
	}
	if strings.TrimSpace(state.RuntimeSessionID) == "" {
		return errors.New("runtime bootstrap sessionId required")
	}
	if strings.TrimSpace(state.DeviceID) == "" {
		return errors.New("runtime bootstrap deviceId required")
	}
	return nil
}

func newRuntimeBootstrapToken() (string, error) {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}

func runtimeBootstrapKey(token string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return "codelocal:runtime-bootstrap:v1:" + hex.EncodeToString(digest[:])
}

type RedisRuntimeBootstrapBackend struct {
	Client *redis.Client
}

func NewRedisRuntimeBootstrapBackend(client *redis.Client) *RedisRuntimeBootstrapBackend {
	return &RedisRuntimeBootstrapBackend{Client: client}
}

func (b *RedisRuntimeBootstrapBackend) Put(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	if b == nil || b.Client == nil {
		return false, errors.New("runtime bootstrap Redis unavailable")
	}
	return b.Client.SetNX(ctx, key, value, ttl).Result()
}

func (b *RedisRuntimeBootstrapBackend) Take(ctx context.Context, key string) ([]byte, bool, error) {
	if b == nil || b.Client == nil {
		return nil, false, errors.New("runtime bootstrap Redis unavailable")
	}
	const script = `local value=redis.call('GET',KEYS[1]); if not value then return false end; redis.call('DEL',KEYS[1]); return value`
	value, err := b.Client.Eval(ctx, script, []string{key}).Result()
	if errors.Is(err, redis.Nil) || value == nil || value == false {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	switch typed := value.(type) {
	case string:
		return []byte(typed), true, nil
	case []byte:
		return typed, true, nil
	default:
		return nil, false, errors.New("invalid runtime bootstrap Redis value")
	}
}

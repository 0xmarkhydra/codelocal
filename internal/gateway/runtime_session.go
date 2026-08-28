package gateway

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	cloudRuntimeSessionPrefix = "codelocal:cloud-runtime-session:v1:"
	cloudRuntimeIdleIndex     = "codelocal:cloud-runtime-session:v1:idle"
	cloudRuntimeReapPrefix    = "codelocal:cloud-runtime-reap:v1:"
)

type CloudRuntimeSession struct {
	ID           string `json:"id"`
	UserID       string `json:"userId"`
	WorkspaceKey string `json:"workspaceKey"`
	TargetKey    string `json:"targetKey"`
	SandboxID    string `json:"sandboxId,omitempty"`
	SnapshotName string `json:"snapshotName"`
	Profile      string `json:"profile"`
	LastActiveAt int64  `json:"lastActiveAt"`
	UpdatedAt    int64  `json:"updatedAt"`
}

type CloudRuntimeSessionStore struct {
	Redis *redis.Client
	TTL   time.Duration
	Now   func() time.Time
}

func NewCloudRuntimeSessionStore(client *redis.Client, ttl time.Duration) *CloudRuntimeSessionStore {
	if ttl <= 0 {
		ttl = 30 * 24 * time.Hour
	}
	return &CloudRuntimeSessionStore{Redis: client, TTL: ttl, Now: time.Now}
}

func cloudRuntimeSessionID(workspaceKey, profile string) string {
	digest := sha256.Sum256([]byte(strings.TrimSpace(workspaceKey) + "\x00" + strings.TrimSpace(profile)))
	return hex.EncodeToString(digest[:16])
}

func cloudRuntimeSessionKey(id string) string { return cloudRuntimeSessionPrefix + id }

func (s *CloudRuntimeSessionStore) Touch(ctx context.Context, state CloudRuntimeSession) error {
	if s == nil || s.Redis == nil {
		return errors.New("cloud runtime session Redis unavailable")
	}
	if strings.TrimSpace(state.WorkspaceKey) == "" || strings.TrimSpace(state.Profile) == "" || strings.TrimSpace(state.UserID) == "" {
		return errors.New("cloud runtime session identity is incomplete")
	}
	if state.ID == "" {
		state.ID = cloudRuntimeSessionID(state.WorkspaceKey, state.Profile)
	}
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	at := now().UTC().UnixMilli()
	state.LastActiveAt = at
	state.UpdatedAt = at
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.Redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, cloudRuntimeSessionKey(state.ID), raw, s.TTL)
		pipe.ZAdd(ctx, cloudRuntimeIdleIndex, redis.Z{Score: float64(at), Member: state.ID})
		return nil
	})
	return err
}

func (s *CloudRuntimeSessionStore) Get(ctx context.Context, workspaceKey, profile string) (*CloudRuntimeSession, error) {
	if s == nil || s.Redis == nil {
		return nil, errors.New("cloud runtime session Redis unavailable")
	}
	id := cloudRuntimeSessionID(workspaceKey, profile)
	raw, err := s.Redis.Get(ctx, cloudRuntimeSessionKey(id)).Bytes()
	if errors.Is(err, redis.Nil) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	var state CloudRuntimeSession
	if json.Unmarshal(raw, &state) != nil || state.ID != id || state.WorkspaceKey == "" {
		return nil, errors.New("invalid cloud runtime session state")
	}
	return &state, nil
}

func (s *CloudRuntimeSessionStore) Due(ctx context.Context, before time.Time, limit int64) ([]CloudRuntimeSession, error) {
	if s == nil || s.Redis == nil {
		return nil, errors.New("cloud runtime session Redis unavailable")
	}
	if limit <= 0 || limit > 256 {
		limit = 64
	}
	ids, err := s.Redis.ZRangeByScore(ctx, cloudRuntimeIdleIndex, &redis.ZRangeBy{Min: "-inf", Max: strconv.FormatInt(before.UTC().UnixMilli(), 10), Offset: 0, Count: limit}).Result()
	if err != nil || len(ids) == 0 {
		return nil, err
	}
	keys := make([]string, len(ids))
	for i, id := range ids {
		keys[i] = cloudRuntimeSessionKey(id)
	}
	values, err := s.Redis.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, err
	}
	out := make([]CloudRuntimeSession, 0, len(values))
	for i, value := range values {
		if value == nil {
			_ = s.Redis.ZRem(ctx, cloudRuntimeIdleIndex, ids[i]).Err()
			continue
		}
		var state CloudRuntimeSession
		if json.Unmarshal([]byte(fmt.Sprint(value)), &state) == nil && state.ID == ids[i] && state.SandboxID != "" {
			out = append(out, state)
		}
	}
	return out, nil
}

func (s *CloudRuntimeSessionStore) MarkSnapshotted(ctx context.Context, state CloudRuntimeSession) error {
	if s == nil || s.Redis == nil || state.ID == "" {
		return nil
	}
	state.SandboxID = ""
	now := time.Now
	if s.Now != nil {
		now = s.Now
	}
	state.UpdatedAt = now().UTC().UnixMilli()
	raw, err := json.Marshal(state)
	if err != nil {
		return err
	}
	_, err = s.Redis.TxPipelined(ctx, func(pipe redis.Pipeliner) error {
		pipe.Set(ctx, cloudRuntimeSessionKey(state.ID), raw, s.TTL)
		pipe.ZRem(ctx, cloudRuntimeIdleIndex, state.ID)
		return nil
	})
	return err
}

func (s *CloudRuntimeSessionStore) ClaimReap(ctx context.Context, sessionID, owner string, ttl time.Duration) (bool, error) {
	if s == nil || s.Redis == nil || sessionID == "" || owner == "" {
		return false, errors.New("cloud runtime reap identity is incomplete")
	}
	if ttl <= 0 {
		ttl = 3 * time.Minute
	}
	return s.Redis.SetNX(ctx, cloudRuntimeReapPrefix+sessionID, owner, ttl).Result()
}

func (s *CloudRuntimeSessionStore) ReleaseReap(ctx context.Context, sessionID, owner string) error {
	if s == nil || s.Redis == nil || sessionID == "" || owner == "" {
		return nil
	}
	const script = `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end`
	return s.Redis.Eval(ctx, script, []string{cloudRuntimeReapPrefix + sessionID}, owner).Err()
}

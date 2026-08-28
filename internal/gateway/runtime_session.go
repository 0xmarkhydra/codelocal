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
	cloudRuntimeSessionPrefix  = "codelocal:cloud-runtime-session:v1:"
	cloudRuntimeIdleIndex      = "codelocal:cloud-runtime-session:v1:idle"
	cloudRuntimeReapPrefix     = "codelocal:cloud-runtime-reap:v1:"
	cloudRuntimeInFlightPrefix = "codelocal:cloud-runtime-inflight:v1:"
)

var ErrCloudRuntimeReapInProgress = errors.New("cloud runtime checkpoint in progress")

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
	return s.getByID(ctx, cloudRuntimeSessionID(workspaceKey, profile))
}

func (s *CloudRuntimeSessionStore) getByID(ctx context.Context, id string) (*CloudRuntimeSession, error) {
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

func (s *CloudRuntimeSessionStore) TouchExisting(ctx context.Context, workspaceKey, profile string) error {
	state, err := s.Get(ctx, workspaceKey, profile)
	if err != nil || state == nil {
		return err
	}
	return s.Touch(ctx, *state)
}

// BeginCall marks a cloud workspace in-flight without racing the idle reaper.
// Touch first makes already-selected idle candidates observe fresh activity;
// then a Redis script atomically refuses a new call if a reaper claim already
// exists, otherwise increments the in-flight counter. Therefore either the call
// starts first (and the reaper sees InFlight) or the reaper starts first (and
// the call waits/rebinds) — never both mutate compute concurrently.
func (s *CloudRuntimeSessionStore) BeginCall(ctx context.Context, workspaceKey, profile string) (func(), error) {
	state, err := s.Get(ctx, workspaceKey, profile)
	if err != nil || state == nil {
		return func() {}, err
	}
	if err := s.Touch(ctx, *state); err != nil {
		return func() {}, err
	}
	inFlightKey := cloudRuntimeInFlightPrefix + state.ID
	reapKey := cloudRuntimeReapPrefix + state.ID
	const begin = `if redis.call('EXISTS',KEYS[1])==1 then return 0 end; local n=redis.call('INCR',KEYS[2]); redis.call('PEXPIRE',KEYS[2],ARGV[1]); return n`
	value, err := s.Redis.Eval(ctx, begin, []string{reapKey, inFlightKey}, (2 * time.Hour).Milliseconds()).Int64()
	if err != nil {
		return func() {}, err
	}
	if value == 0 {
		return func() {}, ErrCloudRuntimeReapInProgress
	}
	var done bool
	return func() {
		if done {
			return
		}
		done = true
		finishCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		const decrement = `local n=tonumber(redis.call('GET',KEYS[1]) or '0'); if n<=1 then redis.call('DEL',KEYS[1]); return 0 end; return redis.call('DECR',KEYS[1])`
		_ = s.Redis.Eval(finishCtx, decrement, []string{inFlightKey}).Err()
		_ = s.TouchExisting(finishCtx, workspaceKey, profile)
	}, nil
}

func (s *CloudRuntimeSessionStore) InFlight(ctx context.Context, sessionID string) (bool, error) {
	if s == nil || s.Redis == nil || sessionID == "" {
		return false, nil
	}
	value, err := s.Redis.Get(ctx, cloudRuntimeInFlightPrefix+sessionID).Int64()
	if errors.Is(err, redis.Nil) {
		return false, nil
	}
	return value > 0, err
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
		pipe.Del(ctx, cloudRuntimeInFlightPrefix+state.ID)
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

func (s *CloudRuntimeSessionStore) ReapHeld(ctx context.Context, workspaceKey, profile string) (bool, error) {
	if s == nil || s.Redis == nil {
		return false, nil
	}
	count, err := s.Redis.Exists(ctx, cloudRuntimeReapPrefix+cloudRuntimeSessionID(workspaceKey, profile)).Result()
	return count > 0, err
}

func (s *CloudRuntimeSessionStore) WaitReapClear(ctx context.Context, workspaceKey, profile string) error {
	if s == nil || s.Redis == nil {
		return nil
	}
	ticker := time.NewTicker(200 * time.Millisecond)
	defer ticker.Stop()
	for {
		held, err := s.ReapHeld(ctx, workspaceKey, profile)
		if err != nil {
			return err
		}
		if !held {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func (s *CloudRuntimeSessionStore) ReleaseReap(ctx context.Context, sessionID, owner string) error {
	if s == nil || s.Redis == nil || sessionID == "" || owner == "" {
		return nil
	}
	const script = `if redis.call('GET',KEYS[1])==ARGV[1] then return redis.call('DEL',KEYS[1]) else return 0 end`
	return s.Redis.Eval(ctx, script, []string{cloudRuntimeReapPrefix + sessionID}, owner).Err()
}

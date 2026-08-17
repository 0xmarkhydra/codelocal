package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5"
)

const userSecurityCacheTTL = 30 * time.Second

type UserSecurityState struct {
	Version           int64 `json:"version"`
	PasswordChangedAt int64 `json:"passwordChangedAt"`
}

func userSecurityCacheKey(userID string) string {
	return "codelocal:user-security:" + HashSecret(userID)[:32]
}

func (s *Store) ClearUserSecurityCache(ctx context.Context, userID string) error {
	if s == nil || s.Redis == nil || userID == "" {
		return nil
	}
	return s.Redis.Del(ctx, userSecurityCacheKey(userID)).Err()
}

func (s *Store) cachedUserSecurityState(ctx context.Context, userID string) (UserSecurityState, bool) {
	if s == nil || s.Redis == nil {
		return UserSecurityState{}, false
	}
	raw, err := s.Redis.Get(ctx, userSecurityCacheKey(userID)).Bytes()
	if err != nil {
		return UserSecurityState{}, false
	}
	var state UserSecurityState
	if json.Unmarshal(raw, &state) != nil || state.Version <= 0 {
		return UserSecurityState{}, false
	}
	return state, true
}

func (s *Store) cacheUserSecurityState(ctx context.Context, userID string, state UserSecurityState) {
	if s == nil || s.Redis == nil || state.Version <= 0 {
		return
	}
	raw, err := json.Marshal(state)
	if err != nil {
		return
	}
	_ = s.Redis.Set(ctx, userSecurityCacheKey(userID), raw, userSecurityCacheTTL).Err()
}

func (s *Store) SecurityStateForUser(ctx context.Context, userID string) (UserSecurityState, error) {
	if s == nil || s.DB == nil || userID == "" {
		return UserSecurityState{}, errors.New("user security state unavailable")
	}
	if state, ok := s.cachedUserSecurityState(ctx, userID); ok {
		return state, nil
	}
	var state UserSecurityState
	err := s.DB.QueryRow(ctx, `SELECT security_version,password_changed_at FROM codelocal_users WHERE id=$1`, userID).Scan(&state.Version, &state.PasswordChangedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return UserSecurityState{}, errors.New("USER_NOT_FOUND")
	}
	if err != nil {
		return UserSecurityState{}, err
	}
	if state.Version <= 0 {
		state.Version = 1
	}
	s.cacheUserSecurityState(ctx, userID, state)
	return state, nil
}

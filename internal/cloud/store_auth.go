package cloud

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/redis/go-redis/v9"
)

func (s *Store) CreateUser(ctx context.Context, email, passwordHash, passwordSalt, referredByCode string) (User, error) {
	email = normalizeEmail(email)
	referredByCode = NormalizeReferralCode(referredByCode)
	if !ValidReferralCode(referredByCode) {
		return User{}, errors.New("REFERRAL_REQUIRED")
	}

	primaryAdmin := email == PrimaryAdminEmail()
	if referredByCode == "MMON" {
		if !primaryAdmin {
			return User{}, errors.New("REFERRAL_INVALID")
		}
	} else {
		inviter, err := s.UserByReferralCode(ctx, referredByCode)
		if err != nil {
			return User{}, err
		}
		if inviter == nil {
			return User{}, errors.New("REFERRAL_INVALID")
		}
	}

	for attempt := 0; attempt < 10; attempt++ {
		ownCode := RandomReferralCode()
		user := User{ID: RandomHex(16), Email: email, PasswordHash: passwordHash, PasswordSalt: passwordSalt, SecurityVersion: 1, ReferralCode: ownCode, ReferredByCode: referredByCode, CreatedAt: time.Now().UnixMilli()}
		_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_users(id,email,password_hash,password_salt,referral_code,referred_by_code,created_at) VALUES($1,$2,$3,$4,$5,NULLIF($6,''),$7)`, user.ID, user.Email, user.PasswordHash, user.PasswordSalt, user.ReferralCode, user.ReferredByCode, user.CreatedAt)
		if err == nil {
			return user, nil
		}
		if strings.Contains(err.Error(), "23505") || strings.Contains(strings.ToLower(err.Error()), "duplicate") {
			if existing, lookupErr := s.UserByEmail(ctx, email); lookupErr == nil && existing != nil {
				return User{}, errors.New("EMAIL_ALREADY_REGISTERED")
			}
			continue
		}
		return User{}, err
	}
	return User{}, errors.New("REFERRAL_CODE_GENERATION_FAILED")
}

func scanUser(row pgx.Row) (*User, error) {
	var u User
	var referredBy *string
	if err := row.Scan(&u.ID, &u.Email, &u.PasswordHash, &u.PasswordSalt, &u.PasswordChangedAt, &u.SecurityVersion, &u.ReferralCode, &referredBy, &u.CreatedAt); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, err
	}
	if referredBy != nil {
		u.ReferredByCode = *referredBy
	}
	return &u, nil
}

func (s *Store) UserByEmail(ctx context.Context, email string) (*User, error) {
	return scanUser(s.DB.QueryRow(ctx, `SELECT id,email,password_hash,password_salt,password_changed_at,security_version,referral_code,referred_by_code,created_at FROM codelocal_users WHERE email=$1`, normalizeEmail(email)))
}

func (s *Store) UserByID(ctx context.Context, id string) (*User, error) {
	return scanUser(s.DB.QueryRow(ctx, `SELECT id,email,password_hash,password_salt,password_changed_at,security_version,referral_code,referred_by_code,created_at FROM codelocal_users WHERE id=$1`, id))
}

func (s *Store) UserByReferralCode(ctx context.Context, code string) (*User, error) {
	code = NormalizeReferralCode(code)
	if !ValidReferralCode(code) {
		return nil, nil
	}
	return scanUser(s.DB.QueryRow(ctx, `SELECT id,email,password_hash,password_salt,password_changed_at,security_version,referral_code,referred_by_code,created_at FROM codelocal_users WHERE UPPER(referral_code)=$1`, code))
}

func (s *Store) UpdateUserPassword(ctx context.Context, userID, passwordHash, passwordSalt string, changedAt int64) error {
	_, err := s.UpdateUserPasswordAndVersion(ctx, userID, passwordHash, passwordSalt, changedAt)
	return err
}

func (s *Store) UpdateUserPasswordAndVersion(ctx context.Context, userID, passwordHash, passwordSalt string, changedAt int64) (int64, error) {
	var securityVersion int64
	err := s.DB.QueryRow(ctx, `UPDATE codelocal_users SET password_hash=$2,password_salt=$3,password_changed_at=$4,security_version=security_version+1 WHERE id=$1 RETURNING security_version`, userID, passwordHash, passwordSalt, changedAt).Scan(&securityVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, errors.New("USER_NOT_FOUND")
	}
	if err != nil {
		return 0, err
	}
	_ = s.ClearUserSecurityCache(ctx, userID)
	return securityVersion, nil
}

func (s *Store) CreateSession(ctx context.Context, userID, csrf string, ttl time.Duration) (string, error) {
	return s.CreateSessionWithSecurity(ctx, userID, csrf, ttl, SecuritySignal{})
}

func (s *Store) CreateSessionWithSecurity(ctx context.Context, userID, csrf string, ttl time.Duration, signal SecuritySignal) (string, error) {
	return s.CreateSessionWithSecurityVersion(ctx, userID, csrf, ttl, 0, signal)
}

func (s *Store) CreateSessionWithSecurityVersion(ctx context.Context, userID, csrf string, ttl time.Duration, securityVersion int64, signal SecuritySignal) (string, error) {
	id := RandomHex(40)
	state := SessionState{UserID: userID, CSRF: csrf, CreatedAt: time.Now().UnixMilli(), SecurityVersion: securityVersion}
	if signal.DeviceHash != "" || signal.AgentHash != "" || signal.NetworkHash != "" {
		state.Security = &signal
	}
	data, err := json.Marshal(state)
	if err != nil {
		return "", err
	}
	return id, s.Redis.Set(ctx, "codelocal:session:"+id, data, ttl).Err()
}

func (s *Store) UpdateSessionState(ctx context.Context, id string, state SessionState) error {
	if id == "" {
		return nil
	}
	key := "codelocal:session:" + id
	ttl, err := s.Redis.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		return err
	}
	data, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.Redis.Set(ctx, key, data, ttl).Err()
}

func (s *Store) ReadSessionState(ctx context.Context, id string) (SessionState, bool, error) {
	if id == "" {
		return SessionState{}, false, nil
	}
	raw, err := s.Redis.Get(ctx, "codelocal:session:"+id).Bytes()
	if errors.Is(err, redis.Nil) {
		return SessionState{}, false, nil
	}
	if err != nil {
		return SessionState{}, false, err
	}
	var state SessionState
	if err := json.Unmarshal(raw, &state); err != nil {
		return SessionState{}, false, nil
	}
	return state, state.UserID != "" && state.CSRF != "", nil
}

func (s *Store) ReadSession(ctx context.Context, id string) (userID, csrf string, ok bool, err error) {
	state, ok, err := s.ReadSessionState(ctx, id)
	return state.UserID, state.CSRF, ok, err
}

func (s *Store) DeleteSession(ctx context.Context, id string) error {
	if id == "" {
		return nil
	}
	return s.Redis.Del(ctx, "codelocal:session:"+id).Err()
}

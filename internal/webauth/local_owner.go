package webauth

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

// LocalOwnerEnv enables single-operator self-host mode.
//
// When set, the gateway treats the machine operator as the only account. This
// removes the account/signup/login layer for a gateway that is bound to
// loopback and owned by the operator. It deliberately does NOT weaken any
// workspace, path, approval or execution policy: those remain enforced by the
// local runtime exactly as before.
const LocalOwnerEnv = "CODELOCAL_LOCAL_OWNER"

func LocalOwnerMode() bool {
	switch strings.ToLower(strings.TrimSpace(os.Getenv(LocalOwnerEnv))) {
	case "1", "true", "yes", "on":
		return true
	}
	return false
}

func LocalOwnerEmail() string {
	if value := strings.ToLower(strings.TrimSpace(os.Getenv("CODELOCAL_LOCAL_OWNER_EMAIL"))); value != "" {
		return value
	}
	return "local-owner@codelocal.local"
}

// LocalOwnerIdentity returns the single local-owner identity, creating the
// account row on first use. It performs no password, cookie, session or CSRF
// validation by design.
func (m *Manager) LocalOwnerIdentity(ctx context.Context) (*Identity, error) {
	user, err := m.localOwnerUser(ctx)
	if err != nil || user == nil {
		return nil, err
	}
	return &Identity{User: *user, SessionID: "local-owner", CSRF: "local-owner"}, nil
}

func (m *Manager) localOwnerUser(ctx context.Context) (*cloud.User, error) {
	email := LocalOwnerEmail()
	if existing, err := m.Store.UserByEmail(ctx, email); err != nil || existing != nil {
		return existing, err
	}
	// Insert the operator account directly: self-host mode has no referral or
	// signup flow to satisfy. No usable password is issued, because local-owner
	// mode never authenticates with a password.
	hash, salt, err := HashPassword(cloud.RandomHex(24))
	if err != nil {
		return nil, err
	}
	user := cloud.User{
		ID:              cloud.RandomHex(16),
		Email:           email,
		PasswordHash:    hash,
		PasswordSalt:    salt,
		SecurityVersion: 1,
		ReferralCode:    cloud.RandomReferralCode(),
		CreatedAt:       time.Now().UnixMilli(),
	}
	_, err = m.Store.DB.Exec(ctx, `INSERT INTO codelocal_users(id,email,password_hash,password_salt,referral_code,created_at,password_changed_at,security_version) VALUES($1,$2,$3,$4,$5,$6,0,1) ON CONFLICT (email) DO NOTHING`, user.ID, user.Email, user.PasswordHash, user.PasswordSalt, user.ReferralCode, user.CreatedAt)
	if err != nil {
		return nil, err
	}
	return m.Store.UserByEmail(ctx, email)
}

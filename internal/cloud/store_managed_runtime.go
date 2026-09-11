package cloud

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/deviceauth"
	"github.com/jackc/pgx/v5"
)

type ManagedRuntimeCredential struct {
	CredentialID string `json:"credentialId"`
	Secret       string `json:"credentialSecret"`
	DeviceID     string `json:"deviceId"`
	DeviceName   string `json:"deviceName"`
	PublicKey    string `json:"publicKey"`
}

// CreateManagedRuntimeCredential creates or rotates a device credential for a
// CodeLocal-managed cloud runtime. The cleartext secret is returned exactly to
// the caller; only its hash is persisted using the existing device-auth model.
func (s *Store) CreateManagedRuntimeCredential(ctx context.Context, userID, deviceID, deviceName, publicKey string) (ManagedRuntimeCredential, error) {
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	deviceName = strings.TrimSpace(deviceName)
	publicKey = strings.TrimSpace(publicKey)
	if s == nil || s.DB == nil {
		return ManagedRuntimeCredential{}, errors.New("cloud store unavailable")
	}
	if userID == "" {
		return ManagedRuntimeCredential{}, errors.New("managed runtime userId required")
	}
	if deviceID == "" {
		return ManagedRuntimeCredential{}, errors.New("managed runtime deviceId required")
	}
	if deviceName == "" {
		deviceName = "CodeLocal Cloud Runtime"
	}
	if !deviceauth.ValidPublicKey(publicKey) {
		return ManagedRuntimeCredential{}, errors.New("managed runtime public key invalid")
	}

	credentialID := RandomHex(16)
	secret := RandomHex(32)
	secretHash := HashSecret(secret)
	now := time.Now().UnixMilli()

	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return ManagedRuntimeCredential{}, err
	}
	defer tx.Rollback(ctx)

	previousCredentialID := ""
	err = tx.QueryRow(ctx, `SELECT credential_id FROM codelocal_devices WHERE user_id=$1 AND device_id=$2 FOR UPDATE`, userID, deviceID).Scan(&previousCredentialID)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return ManagedRuntimeCredential{}, err
	}
	_, err = tx.Exec(ctx, `
INSERT INTO codelocal_devices(credential_id,user_id,device_id,device_name,public_key,secret_hash,created_at,last_seen_at,revoked_at)
VALUES($1,$2,$3,$4,$5,$6,$7,$7,NULL)
ON CONFLICT(user_id,device_id) DO UPDATE SET
 credential_id=EXCLUDED.credential_id,
 device_name=EXCLUDED.device_name,
 public_key=EXCLUDED.public_key,
 secret_hash=EXCLUDED.secret_hash,
 created_at=EXCLUDED.created_at,
 last_seen_at=EXCLUDED.last_seen_at,
 revoked_at=NULL`, credentialID, userID, deviceID, deviceName, publicKey, secretHash, now)
	if err != nil {
		return ManagedRuntimeCredential{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return ManagedRuntimeCredential{}, err
	}

	keys := []string{"codelocal:device:" + credentialID, "codelocal:device-touch:" + credentialID}
	if previousCredentialID != "" && previousCredentialID != credentialID {
		keys = append(keys, "codelocal:device:"+previousCredentialID, "codelocal:device-touch:"+previousCredentialID)
		_ = s.ClearSecurityState(ctx, "credential", previousCredentialID)
		_ = s.ClearDeviceNonceState(ctx, previousCredentialID)
	}
	s.invalidateDeviceCache(keys...)
	return ManagedRuntimeCredential{
		CredentialID: credentialID,
		Secret:       secret,
		DeviceID:     deviceID,
		DeviceName:   deviceName,
		PublicKey:    publicKey,
	}, nil
}

// RevokeManagedRuntimeCredential invalidates the current credential attached
// to a managed cloud runtime device. This is safe to call during teardown even
// if the credential was already rotated or revoked.
func (s *Store) RevokeManagedRuntimeCredential(ctx context.Context, userID, deviceID string) (bool, error) {
	userID = strings.TrimSpace(userID)
	deviceID = strings.TrimSpace(deviceID)
	if s == nil || s.DB == nil {
		return false, errors.New("cloud store unavailable")
	}
	if userID == "" || deviceID == "" {
		return false, errors.New("managed runtime userId and deviceId required")
	}
	var credentialID string
	err := s.DB.QueryRow(ctx, `
UPDATE codelocal_devices
SET revoked_at=$3
WHERE user_id=$1 AND device_id=$2 AND revoked_at IS NULL
RETURNING credential_id`, userID, deviceID, time.Now().UnixMilli()).Scan(&credentialID)
	if errors.Is(err, pgx.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	s.invalidateDeviceCache("codelocal:device:"+credentialID, "codelocal:device-touch:"+credentialID)
	_ = s.ClearSecurityState(ctx, "credential", credentialID)
	_ = s.ClearDeviceNonceState(ctx, credentialID)
	return true, nil
}

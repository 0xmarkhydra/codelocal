package cloud

import (
	"context"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreatePairing(ctx context.Context, deviceID, deviceName string, ttl time.Duration) (Pairing, error) {
	codeBytes := make([]byte, 4)
	_, _ = rand.Read(codeBytes)
	code := int(codeBytes[0])<<16 | int(codeBytes[1])<<8 | int(codeBytes[2])
	code = 100000 + code%900000
	pairing := Pairing{PairingID: RandomHex(16), Code: fmt.Sprintf("%06d", code), DeviceID: deviceID, DeviceName: deviceName, CreatedAt: time.Now().UnixMilli(), ExpiresAt: time.Now().Add(ttl).UnixMilli()}
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_pairings(pairing_id,code,device_id,device_name,created_at,expires_at) VALUES($1,$2,$3,$4,$5,$6)`, pairing.PairingID, pairing.Code, pairing.DeviceID, pairing.DeviceName, pairing.CreatedAt, pairing.ExpiresAt)
	return pairing, err
}

func scanPairing(row pgx.Row) (*Pairing, error) {
	var p Pairing
	var userID *string
	var approved, claimed *int64
	err := row.Scan(&p.PairingID, &p.Code, &p.DeviceID, &p.DeviceName, &userID, &p.CreatedAt, &p.ExpiresAt, &approved, &claimed)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if userID != nil {
		p.UserID = *userID
	}
	if approved != nil {
		p.ApprovedAt = *approved
	}
	if claimed != nil {
		p.ClaimedAt = *claimed
	}
	return &p, nil
}

func (s *Store) GetPairing(ctx context.Context, id string) (*Pairing, error) {
	return scanPairing(s.DB.QueryRow(ctx, `SELECT pairing_id,code,device_id,device_name,user_id,created_at,expires_at,approved_at,claimed_at FROM codelocal_pairings WHERE pairing_id=$1`, id))
}

func (s *Store) ApprovePairing(ctx context.Context, id, code, userID string) (*Pairing, error) {
	now := time.Now().UnixMilli()
	return scanPairing(s.DB.QueryRow(ctx, `UPDATE codelocal_pairings SET user_id=$3,approved_at=$4 WHERE pairing_id=$1 AND code=$2 AND expires_at>$4 AND approved_at IS NULL AND claimed_at IS NULL AND user_id IS NULL RETURNING pairing_id,code,device_id,device_name,user_id,created_at,expires_at,approved_at,claimed_at`, id, code, userID, now))
}

func (s *Store) ClaimPairing(ctx context.Context, id, code, credentialID, secretHash, publicKey string) (*Device, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	var p Pairing
	var userID string
	err = tx.QueryRow(ctx, `SELECT pairing_id,code,device_id,device_name,user_id,created_at,expires_at FROM codelocal_pairings WHERE pairing_id=$1 AND code=$2 AND expires_at>$3 AND approved_at IS NOT NULL AND claimed_at IS NULL FOR UPDATE`, id, code, time.Now().UnixMilli()).Scan(&p.PairingID, &p.Code, &p.DeviceID, &p.DeviceName, &userID, &p.CreatedAt, &p.ExpiresAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// A claim may have committed even when its HTTP response was lost. Allow a
		// retry only when the caller presents the exact credential ID and secret
		// hash that became current for this pairing's device.
		claimedErr := tx.QueryRow(ctx, `SELECT pairing_id,code,device_id,device_name,user_id,created_at,expires_at FROM codelocal_pairings WHERE pairing_id=$1 AND code=$2 AND approved_at IS NOT NULL AND claimed_at IS NOT NULL`, id, code).Scan(&p.PairingID, &p.Code, &p.DeviceID, &p.DeviceName, &userID, &p.CreatedAt, &p.ExpiresAt)
		if errors.Is(claimedErr, pgx.ErrNoRows) {
			return nil, nil
		}
		if claimedErr != nil {
			return nil, claimedErr
		}
		var d Device
		var revoked *int64
		deviceErr := tx.QueryRow(ctx, `SELECT credential_id,user_id,device_id,device_name,COALESCE(public_key,''),secret_hash,created_at,last_seen_at,revoked_at FROM codelocal_devices WHERE user_id=$1 AND device_id=$2 AND credential_id=$3`, userID, p.DeviceID, credentialID).Scan(&d.CredentialID, &d.UserID, &d.DeviceID, &d.DeviceName, &d.PublicKey, &d.SecretHash, &d.CreatedAt, &d.LastSeenAt, &revoked)
		if errors.Is(deviceErr, pgx.ErrNoRows) {
			return nil, nil
		}
		if deviceErr != nil {
			return nil, deviceErr
		}
		if revoked != nil || !EqualSecretHash(d.SecretHash, secretHash) || d.PublicKey != strings.TrimSpace(publicKey) {
			return nil, nil
		}
		return &d, nil
	}
	if err != nil {
		return nil, err
	}
	previousCredentialID := ""
	if existingErr := tx.QueryRow(ctx, `SELECT credential_id FROM codelocal_devices WHERE user_id=$1 AND device_id=$2 FOR UPDATE`, userID, p.DeviceID).Scan(&previousCredentialID); existingErr != nil && !errors.Is(existingErr, pgx.ErrNoRows) {
		return nil, existingErr
	}
	now := time.Now().UnixMilli()
	if _, err = tx.Exec(ctx, `UPDATE codelocal_pairings SET claimed_at=$2 WHERE pairing_id=$1`, id, now); err != nil {
		return nil, err
	}
	publicKey = strings.TrimSpace(publicKey)
	if _, err = tx.Exec(ctx, `INSERT INTO codelocal_devices(credential_id,user_id,device_id,device_name,public_key,secret_hash,created_at,last_seen_at) VALUES($1,$2,$3,$4,NULLIF($5,''),$6,$7,$7) ON CONFLICT(user_id,device_id) DO UPDATE SET credential_id=EXCLUDED.credential_id,device_name=EXCLUDED.device_name,public_key=EXCLUDED.public_key,secret_hash=EXCLUDED.secret_hash,created_at=EXCLUDED.created_at,last_seen_at=EXCLUDED.last_seen_at,revoked_at=NULL`, credentialID, userID, p.DeviceID, p.DeviceName, publicKey, secretHash, now); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	keys := []string{"codelocal:device:" + credentialID, "codelocal:device-touch:" + credentialID}
	if previousCredentialID != "" && previousCredentialID != credentialID {
		keys = append(keys, "codelocal:device:"+previousCredentialID, "codelocal:device-touch:"+previousCredentialID)
		_ = s.ClearSecurityState(ctx, "credential", previousCredentialID)
		_ = s.ClearDeviceNonceState(ctx, previousCredentialID)
	}
	s.invalidateDeviceCache(keys...)
	return &Device{CredentialID: credentialID, PreviousCredentialID: previousCredentialID, UserID: userID, DeviceID: p.DeviceID, DeviceName: p.DeviceName, PublicKey: publicKey, SecretHash: secretHash, CreatedAt: now, LastSeenAt: now}, nil
}

func (s *Store) AuthenticateDevice(ctx context.Context, credentialID, secretHash string) (*Device, error) {
	if credentialID == "" || secretHash == "" {
		return nil, nil
	}
	cacheKey := "codelocal:device:" + credentialID
	if raw, err := s.Redis.Get(ctx, cacheKey).Bytes(); err == nil {
		var cached Device
		if json.Unmarshal(raw, &cached) == nil && cached.RevokedAt == 0 && EqualSecretHash(cached.SecretHash, secretHash) {
			cached.LastSeenAt = time.Now().UnixMilli()
			return &cached, nil
		}
	}
	var d Device
	var revoked *int64
	err := s.DB.QueryRow(ctx, `SELECT credential_id,user_id,device_id,device_name,COALESCE(public_key,''),secret_hash,created_at,last_seen_at,revoked_at FROM codelocal_devices WHERE credential_id=$1`, credentialID).Scan(&d.CredentialID, &d.UserID, &d.DeviceID, &d.DeviceName, &d.PublicKey, &d.SecretHash, &d.CreatedAt, &d.LastSeenAt, &revoked)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if revoked != nil || !EqualSecretHash(d.SecretHash, secretHash) {
		return nil, nil
	}
	now := time.Now().UnixMilli()
	d.LastSeenAt = now
	payload, _ := json.Marshal(d)
	_ = s.Redis.Set(ctx, cacheKey, payload, 60*time.Second).Err()
	lastPersistKey := "codelocal:device-touch:" + credentialID
	if ok, _ := s.Redis.SetNX(ctx, lastPersistKey, "1", 60*time.Second).Result(); ok {
		go func() {
			_, _ = s.DB.Exec(context.Background(), `UPDATE codelocal_devices SET last_seen_at=$2 WHERE credential_id=$1`, credentialID, now)
		}()
	}
	return &d, nil
}

func (s *Store) ListDevices(ctx context.Context, userID string) ([]Device, error) {
	rows, err := s.DB.Query(ctx, `SELECT credential_id,user_id,device_id,device_name,COALESCE(public_key,''),secret_hash,created_at,last_seen_at,revoked_at FROM codelocal_devices WHERE user_id=$1 ORDER BY last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Device{}
	for rows.Next() {
		var d Device
		var revoked *int64
		if err := rows.Scan(&d.CredentialID, &d.UserID, &d.DeviceID, &d.DeviceName, &d.PublicKey, &d.SecretHash, &d.CreatedAt, &d.LastSeenAt, &revoked); err != nil {
			return nil, err
		}
		if revoked != nil {
			d.RevokedAt = *revoked
		}
		out = append(out, d)
	}
	return out, rows.Err()
}

func (s *Store) ActiveCredentialIDs(ctx context.Context, credentialIDs []string) (map[string]bool, error) {
	out := make(map[string]bool, len(credentialIDs))
	if len(credentialIDs) == 0 {
		return out, nil
	}
	rows, err := s.DB.Query(ctx, `SELECT credential_id FROM codelocal_devices WHERE credential_id = ANY($1::text[]) AND revoked_at IS NULL`, credentialIDs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var credentialID string
		if err := rows.Scan(&credentialID); err != nil {
			return nil, err
		}
		out[credentialID] = true
	}
	return out, rows.Err()
}

func (s *Store) RevokeDevice(ctx context.Context, userID, credentialID string) (bool, error) {
	result, err := s.DB.Exec(ctx, `UPDATE codelocal_devices SET revoked_at=$3 WHERE user_id=$1 AND credential_id=$2 AND revoked_at IS NULL`, userID, credentialID, time.Now().UnixMilli())
	if err != nil {
		return false, err
	}
	s.invalidateDeviceCache("codelocal:device:"+credentialID, "codelocal:device-touch:"+credentialID)
	_ = s.ClearSecurityState(ctx, "credential", credentialID)
	_ = s.ClearDeviceNonceState(ctx, credentialID)
	return result.RowsAffected() == 1, nil
}

func (s *Store) RenameDevice(ctx context.Context, userID, credentialID, name string) (bool, error) {
	result, err := s.DB.Exec(ctx, `UPDATE codelocal_devices SET device_name=$3 WHERE user_id=$1 AND credential_id=$2 AND revoked_at IS NULL`, userID, credentialID, name)
	if err != nil {
		return false, err
	}
	s.invalidateDeviceCache("codelocal:device:" + credentialID)
	return result.RowsAffected() == 1, nil
}

func (s *Store) UpsertWorkspace(ctx context.Context, w Workspace) error {
	now := time.Now().UnixMilli()
	caps, _ := json.Marshal(w.Capabilities)
	_, err := s.DB.Exec(ctx, `INSERT INTO codelocal_workspaces(user_id,device_id,workspace_id,workspace_name,project_root,protocol_version,capabilities,created_at,last_seen_at) VALUES($1,$2,$3,$4,NULL,$5,$6,$7,$7) ON CONFLICT(user_id,device_id,workspace_id) DO UPDATE SET workspace_name=EXCLUDED.workspace_name,project_root=NULL,protocol_version=EXCLUDED.protocol_version,capabilities=EXCLUDED.capabilities,last_seen_at=EXCLUDED.last_seen_at`, w.UserID, w.DeviceID, w.WorkspaceID, w.WorkspaceName, w.ProtocolVersion, caps, now)
	return err
}

func (s *Store) ListWorkspaceRecords(ctx context.Context, userID string) ([]Workspace, error) {
	rows, err := s.DB.Query(ctx, `
SELECT w.user_id,w.device_id,w.workspace_id,w.workspace_name,w.project_root,w.protocol_version,w.capabilities,w.created_at,w.last_seen_at,
 COALESCE(wp.project_id,''),COALESCE(p.name,''),COALESCE(wp.source,''),COALESCE(wp.confidence,0)
FROM codelocal_workspaces w
LEFT JOIN codelocal_workspace_projects wp ON wp.user_id=w.user_id AND wp.device_id=w.device_id AND wp.workspace_id=w.workspace_id
LEFT JOIN codelocal_projects p ON p.user_id=wp.user_id AND p.project_id=wp.project_id
WHERE w.user_id=$1
ORDER BY w.last_seen_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Workspace{}
	for rows.Next() {
		var w Workspace
		var projectRoot *string
		var protocolVersion *int
		var caps []byte
		if err := rows.Scan(&w.UserID, &w.DeviceID, &w.WorkspaceID, &w.WorkspaceName, &projectRoot, &protocolVersion, &caps, &w.CreatedAt, &w.LastSeenAt, &w.ProjectID, &w.ProjectName, &w.ProjectSource, &w.ProjectConfidence); err != nil {
			return nil, err
		}
		if projectRoot != nil {
			w.ProjectRoot = *projectRoot
		}
		if protocolVersion != nil {
			w.ProtocolVersion = *protocolVersion
		}
		_ = json.Unmarshal(caps, &w.Capabilities)
		if w.Capabilities == nil {
			w.Capabilities = map[string]any{}
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (s *Store) ReconcileWorkspaces(ctx context.Context, userID, deviceID string, authorized []string) ([]string, error) {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	rows, err := tx.Query(ctx, `SELECT workspace_id FROM codelocal_workspaces WHERE user_id=$1 AND device_id=$2`, userID, deviceID)
	if err != nil {
		return nil, err
	}
	allowed := map[string]struct{}{}
	for _, id := range authorized {
		allowed[id] = struct{}{}
	}
	removed := []string{}
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := allowed[id]; !ok {
			removed = append(removed, id)
		}
	}
	rows.Close()
	if len(removed) > 0 {
		if _, err := tx.Exec(ctx, `DELETE FROM codelocal_workspaces WHERE user_id=$1 AND device_id=$2 AND workspace_id = ANY($3)`, userID, deviceID, removed); err != nil {
			return nil, err
		}
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return removed, nil
}

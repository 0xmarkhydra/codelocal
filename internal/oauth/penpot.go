package oauth

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const penpotAudience = "https://design.codelocal.cloud/mcp/stream"

// Runtime settings refresh every five minutes, leaving one retry window.
const penpotGrantTTL = 10 * time.Minute

// PenpotGrant travels only over the authenticated runtime settings channel.
// It is a credential, not display metadata; never log or persist it in config.
type PenpotGrant struct {
	oidcTokenClaims
	UserToken    string `json:"penpot_key"`
	DeviceID     string `json:"device_id"`
	WorkspaceID  string `json:"workspace_id"`
	CredentialID string `json:"credential_id"`
	Cloud        bool   `json:"cloud,omitempty"`
	ProbeOnly    bool   `json:"probe_only,omitempty"`
}

func (s *Server) IssuePenpotCloudGrant(ctx context.Context, user, nativeToken string, probe bool) (string, error) {
	if user == "" || !validPenpotKey(nativeToken) {
		return "", errors.New("invalid Penpot identity")
	}
	state, err := s.userSecurityState(ctx, user)
	if err != nil {
		return "", err
	}
	now := time.Now()
	return s.OIDC.signToken(PenpotGrant{oidcTokenClaims: oidcTokenClaims{
		Issuer: s.OIDC.Issuer, Subject: user, Audience: penpotAudience, ClientID: s.OIDC.ClientID,
		TokenUse: "penpot_mcp_cloud", JWTID: randomURL(18), IssuedAt: now.Unix(), NotBefore: now.Unix(),
		ExpiresAt: now.Add(2 * time.Minute).Unix(), SecurityVersion: state.Version,
	}, UserToken: nativeToken, Cloud: true, ProbeOnly: probe})
}

func (s *Server) IssuePenpotGrant(ctx context.Context, userID, deviceID, workspaceID, nativeToken string) (string, error) {
	if userID == "" || deviceID == "" || workspaceID == "" || !validPenpotKey(nativeToken) {
		return "", errors.New("invalid Penpot identity")
	}
	state, err := s.userSecurityState(ctx, userID)
	if err != nil {
		return "", err
	}
	devices, err := s.Store.ListDevices(ctx, userID)
	if err != nil {
		return "", errors.New("Penpot device state unavailable")
	}
	credentialID := ""
	for _, device := range devices {
		if device.DeviceID == deviceID && device.RevokedAt == 0 {
			credentialID = device.CredentialID
			break
		}
	}
	if credentialID == "" {
		return "", errors.New("Penpot device is revoked")
	}
	now := time.Now()
	return s.OIDC.signToken(PenpotGrant{
		oidcTokenClaims: oidcTokenClaims{
			Issuer: s.OIDC.Issuer, Subject: userID, Audience: penpotAudience,
			ClientID: s.OIDC.ClientID, TokenUse: "penpot_mcp", JWTID: randomURL(18),
			IssuedAt: now.Unix(), NotBefore: now.Unix(), ExpiresAt: now.Add(penpotGrantTTL).Unix(),
			SecurityVersion: state.Version,
		},
		UserToken: nativeToken, DeviceID: deviceID, WorkspaceID: workspaceID, CredentialID: credentialID,
	})
}

func validPenpotKey(token string) bool {
	return token != "" && len(token) <= 8192 && strings.TrimSpace(token) == token && !strings.ContainsAny(token, "\r\n\x00")
}

func (s *Server) VerifyPenpotGrant(ctx context.Context, token string) (PenpotGrant, error) {
	grant, err := s.verifyPenpotGrant(token)
	if err != nil {
		return PenpotGrant{}, err
	}
	if err := s.validateTokenSecurity(ctx, tokenPayload{Subject: grant.Subject, IssuedAt: grant.IssuedAt, SecurityVersion: grant.SecurityVersion}); err != nil {
		return PenpotGrant{}, err
	}
	if grant.Cloud {
		return grant, nil
	}
	active, err := s.Store.ActiveCredentialIDs(ctx, []string{grant.CredentialID})
	if err != nil || !active[grant.CredentialID] {
		return PenpotGrant{}, errors.New("Penpot device is revoked")
	}
	return grant, nil
}

func (s *Server) verifyPenpotGrant(token string) (PenpotGrant, error) {
	var grant PenpotGrant
	raw, err := s.OIDC.verifySignedToken(token)
	if err != nil || json.Unmarshal(raw, &grant) != nil {
		return PenpotGrant{}, errors.New("invalid Penpot grant")
	}
	now := time.Now().Unix()
	identityValid := grant.TokenUse == "penpot_mcp" && grant.DeviceID != "" && grant.WorkspaceID != "" && grant.CredentialID != "" && !grant.Cloud && !grant.ProbeOnly
	if grant.Cloud {
		identityValid = grant.TokenUse == "penpot_mcp_cloud" && grant.DeviceID == "" && grant.WorkspaceID == "" && grant.CredentialID == ""
	}
	if grant.Issuer != s.OIDC.Issuer || grant.Audience != penpotAudience ||
		grant.ClientID != s.OIDC.ClientID || !identityValid ||
		grant.Subject == "" ||
		grant.JWTID == "" || grant.NotBefore > now || grant.IssuedAt > now ||
		grant.ExpiresAt <= now || grant.ExpiresAt <= grant.IssuedAt ||
		grant.ExpiresAt-grant.IssuedAt > int64(penpotGrantTTL.Seconds()) || !validPenpotKey(grant.UserToken) {
		return PenpotGrant{}, errors.New("expired or invalid Penpot grant")
	}
	return grant, nil
}

func penpotSessionBinding(grant PenpotGrant) string {
	binding := grant.Subject + "\x00" + grant.DeviceID + "\x00" + grant.CredentialID + "\x00" + grant.WorkspaceID + "\x00" + grant.UserToken
	if grant.Cloud {
		binding += "\x00cloud"
		if grant.ProbeOnly {
			binding += "\x00probe"
		}
	}
	digest := sha256.Sum256([]byte(binding))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

// Sessions bind to the native key and workspace, not the rotating grant.
func (s *Server) SealPenpotSession(grant PenpotGrant, sessionID string) (string, error) {
	if sessionID == "" || len(sessionID) > 256 || strings.ContainsAny(sessionID, "\r\n\x00") {
		return "", errors.New("invalid Penpot session")
	}
	now := time.Now()
	return s.OIDC.signToken(oidcTokenClaims{
		Issuer: s.OIDC.Issuer, Subject: grant.Subject, Audience: penpotAudience,
		TokenUse: "penpot_session", Nonce: penpotSessionBinding(grant), Scope: sessionID,
		IssuedAt: now.Unix(), ExpiresAt: now.Add(time.Hour).Unix(),
	})
}

func (s *Server) OpenPenpotSession(grant PenpotGrant, token string) (string, error) {
	raw, err := s.OIDC.verifySignedToken(token)
	var claims oidcTokenClaims
	if err != nil || json.Unmarshal(raw, &claims) != nil ||
		claims.Issuer != s.OIDC.Issuer || claims.Audience != penpotAudience ||
		claims.TokenUse != "penpot_session" || claims.Subject != grant.Subject ||
		claims.ExpiresAt <= time.Now().Unix() || claims.Nonce != penpotSessionBinding(grant) ||
		claims.Scope == "" || len(claims.Scope) > 256 || strings.ContainsAny(claims.Scope, "\r\n\x00") {
		return "", errors.New("invalid Penpot session")
	}
	return claims.Scope, nil
}

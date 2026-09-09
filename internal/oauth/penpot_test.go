package oauth

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func testPenpotGrant(s *Server) PenpotGrant {
	now := time.Now().Unix()
	return PenpotGrant{
		oidcTokenClaims: oidcTokenClaims{
			Issuer: s.OIDC.Issuer, Subject: "user-A", Audience: penpotAudience,
			ClientID: s.OIDC.ClientID, TokenUse: "penpot_mcp", JWTID: "grant-A",
			IssuedAt: now, NotBefore: now, ExpiresAt: now + 60, SecurityVersion: 1,
		},
		UserToken: "native-A", DeviceID: "device-A", WorkspaceID: "workspace-A", CredentialID: "credential-A",
	}
}

func TestPenpotGrantValidation(t *testing.T) {
	s := &Server{OIDC: testOIDCConfig()}
	grant := testPenpotGrant(s)
	token, err := s.OIDC.signToken(grant)
	if err != nil {
		t.Fatal(err)
	}
	actual, err := s.verifyPenpotGrant(token)
	if err != nil || actual.Subject != grant.Subject || actual.UserToken != grant.UserToken {
		t.Fatal("valid grant did not round trip")
	}
	if _, err := s.VerifyPenpotGrant(context.Background(), token); !errors.Is(err, ErrTokenStateUnavailable) {
		t.Fatal("unavailable revocation store must fail closed")
	}
	for _, modify := range []struct {
		name string
		edit func(*PenpotGrant)
	}{
		{"issuer", func(g *PenpotGrant) { g.Issuer = "https://other.test" }},
		{"audience", func(g *PenpotGrant) { g.Audience = s.OIDC.ClientID }},
		{"oidc-access", func(g *PenpotGrant) { g.TokenUse = "access" }},
		{"oidc-id", func(g *PenpotGrant) { g.TokenUse = "id" }},
		{"expired", func(g *PenpotGrant) { g.ExpiresAt = time.Now().Unix() }},
		{"future", func(g *PenpotGrant) { g.NotBefore = time.Now().Add(time.Minute).Unix() }},
		{"long-lived", func(g *PenpotGrant) { g.ExpiresAt = g.IssuedAt + 3600 }},
		{"missing-user", func(g *PenpotGrant) { g.Subject = "" }},
		{"missing-device", func(g *PenpotGrant) { g.DeviceID = "" }},
		{"missing-workspace", func(g *PenpotGrant) { g.WorkspaceID = "" }},
		{"missing-credential", func(g *PenpotGrant) { g.CredentialID = "" }},
		{"missing-key", func(g *PenpotGrant) { g.UserToken = "" }},
		{"header-injection", func(g *PenpotGrant) { g.UserToken = "secret\r\nInjected: yes" }},
	} {
		t.Run(modify.name, func(t *testing.T) {
			invalid := grant
			modify.edit(&invalid)
			signed, err := s.OIDC.signToken(invalid)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := s.verifyPenpotGrant(signed); err == nil {
				t.Fatal("invalid grant accepted")
			}
		})
	}
	for _, invalid := range []string{"", "native-A", token + "x", strings.Replace(token, ".", "..", 1)} {
		if _, err := s.verifyPenpotGrant(invalid); err == nil {
			t.Fatal("unsigned or tampered grant accepted")
		}
	}
	if _, err := s.OIDC.verifyAccessToken(token, time.Now()); err == nil {
		t.Fatal("Penpot grant accepted as OIDC access token")
	}
}

func TestPenpotSessionIsolationAndRestart(t *testing.T) {
	s := &Server{OIDC: testOIDCConfig()}
	grant := testPenpotGrant(s)
	sealed, err := s.SealPenpotSession(grant, "upstream-session-A")
	if err != nil {
		t.Fatal(err)
	}
	restarted := &Server{OIDC: testOIDCConfig()}
	if id, err := restarted.OpenPenpotSession(grant, sealed); err != nil || id != "upstream-session-A" {
		t.Fatal("own session rejected after restart")
	}
	for _, change := range []func(*PenpotGrant){
		func(g *PenpotGrant) { g.Subject = "user-B" },
		func(g *PenpotGrant) { g.DeviceID = "device-B" },
		func(g *PenpotGrant) { g.CredentialID = "credential-B" },
		func(g *PenpotGrant) { g.WorkspaceID = "workspace-B" },
		func(g *PenpotGrant) { g.UserToken = "rotated-native-key" },
	} {
		other := grant
		change(&other)
		if _, err := s.OpenPenpotSession(other, sealed); err == nil {
			t.Fatal("session accepted by another identity or credential")
		}
	}
	renewed := grant
	renewed.JWTID = "renewed-grant"
	renewed.ExpiresAt++
	if _, err := s.OpenPenpotSession(renewed, sealed); err != nil {
		t.Fatal("grant renewal incorrectly invalidated native session")
	}
	for _, invalid := range []string{"", "upstream-session-A", sealed + "x"} {
		if _, err := s.OpenPenpotSession(grant, invalid); err == nil {
			t.Fatal("unsigned session accepted")
		}
	}
}

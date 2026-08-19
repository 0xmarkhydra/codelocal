package cloudserver

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestBuildAccountResourceKeepsPasswordAndSessionInternalsOut(t *testing.T) {
	user := cloud.User{
		ID: "user-public-id", Email: "user@example.com", PasswordHash: "private-password-hash", PasswordSalt: "private-password-salt",
		PasswordChangedAt: 1234, SecurityVersion: 99, ReferralCode: "ABC123", ReferredByCode: "REF456", CreatedAt: 5678,
	}
	result := buildAccountResource(user, "csrf-public-token-abcdefghijklmnop", true)
	if result.Email != user.Email || result.UserID != user.ID || result.ReferralCode != "ABC123" || result.InvitedBy != "REF456" || !result.RequiresReauthentication {
		t.Fatalf("unexpected account DTO: %#v", result)
	}
	raw, err := json.Marshal(result)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{"private-password-hash", "private-password-salt", "securityVersion", "sessionId", "passwordHash", "passwordSalt"} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("account secret/internal state leaked (%s): %s", forbidden, serialized)
		}
	}
}

func TestBuildAccountResourceUsesStableDirectInviterLabel(t *testing.T) {
	result := buildAccountResource(cloud.User{Email: "user@example.com"}, "csrf-public-token-abcdefghijklmnop", false)
	if result.InvitedBy != "Direct / legacy account" {
		t.Fatalf("unexpected direct inviter label: %q", result.InvitedBy)
	}
}

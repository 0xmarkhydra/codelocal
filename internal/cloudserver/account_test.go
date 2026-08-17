package cloudserver

import (
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
)

func TestAccountInfoCardShowsCoreUserInformation(t *testing.T) {
	user := cloud.User{ID: "user-1", Email: "user@example.com", ReferralCode: "ABC123", ReferredByCode: "INV999", CreatedAt: 1_700_000_000_000, PasswordChangedAt: 1_700_000_100_000}
	html := accountInfoCard(user)
	for _, want := range []string{"user@example.com", "user-1", "ABC123", "INV999", "Member since", "Password updated"} {
		if !strings.Contains(html, want) {
			t.Fatalf("account info card missing %q", want)
		}
	}
}

func TestAccountPasswordCardPostsProtectedChangeForm(t *testing.T) {
	html := accountPasswordCard("csrf-value")
	for _, want := range []string{`action="/account/password"`, `name="currentPassword"`, `name="password"`, `name="confirmPassword"`, `value="csrf-value"`, `/forgot-password`} {
		if !strings.Contains(html, want) {
			t.Fatalf("account password card missing %q", want)
		}
	}
}

package webauth

import (
	"strings"
	"testing"
)

func TestPasswordHashAndVerify(t *testing.T) {
	hash, salt, err := HashPassword("correct-horse-battery-staple")
	if err != nil {
		t.Fatal(err)
	}
	if hash == "" || salt == "" {
		t.Fatalf("empty hash/salt: %q %q", hash, salt)
	}
	if !VerifyPassword("correct-horse-battery-staple", salt, hash) {
		t.Fatal("correct password did not verify")
	}
	if VerifyPassword("wrong-password-value", salt, hash) {
		t.Fatal("wrong password verified")
	}
}

func TestPasswordLengthPolicy(t *testing.T) {
	if _, _, err := HashPassword("short"); err == nil {
		t.Fatal("short password should be rejected")
	}
	tooLong := make([]byte, 257)
	for i := range tooLong {
		tooLong[i] = 'x'
	}
	if _, _, err := HashPassword(string(tooLong)); err == nil {
		t.Fatal("overlong password should be rejected")
	}
	if VerifyPassword(string(tooLong), "salt", "hash") {
		t.Fatal("overlong password should never verify")
	}
}

func TestValidEmail(t *testing.T) {
	if !validEmail("user@example.com") {
		t.Fatal("valid email rejected")
	}
	for _, value := range []string{"", "missing-at.example.com", "a@b", "a b@example.com"} {
		if validEmail(value) {
			t.Fatalf("invalid email accepted: %q", value)
		}
	}
}

func TestSignupFormRequiresReferralCode(t *testing.T) {
	manager := &Manager{}
	signup := manager.form("signup", "csrf-token", "/dashboard", "")
	if !strings.Contains(signup, `name="referralCode"`) || !strings.Contains(signup, `maxlength="6"`) || !strings.Contains(signup, "required") {
		t.Fatal("signup form must require a 6-character referral code")
	}
	login := manager.form("login", "csrf-token", "/dashboard", "")
	if strings.Contains(login, `name="referralCode"`) {
		t.Fatal("login form must not request referral code")
	}
	prefilled := manager.form("signup", "csrf-token", "/dashboard", "", "mmon6a")
	if !strings.Contains(prefilled, `name="referralCode" value="MMON6A"`) {
		t.Fatal("signup form must prefill a referral code supplied by an invite link")
	}
}

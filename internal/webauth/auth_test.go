package webauth

import (
	"strings"
	"testing"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
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

func TestAuthFormAvoidsImmediateIOSKeyboardAndUsesEmailInputHints(t *testing.T) {
	manager := &Manager{}
	login := manager.form("login", "csrf-token", "/dashboard", "")
	if strings.Contains(login, "autofocus") {
		t.Fatal("auth form must not autofocus an input because iOS would open the keyboard before the browser sheet settles")
	}
	for _, want := range []string{`inputmode="email"`, `autocapitalize="none"`, `spellcheck="false"`} {
		if !strings.Contains(login, want) {
			t.Fatalf("auth email input must include mobile hint %s", want)
		}
	}
	if !strings.Contains(login, `href="/forgot-password"`) {
		t.Fatal("login form must expose the forgot-password flow")
	}
}

func TestSessionInvalidForUserUsesSecurityVersionAndLegacyFallback(t *testing.T) {
	user := cloud.User{PasswordChangedAt: 200, SecurityVersion: 3}
	if sessionInvalidForUser(cloud.SessionState{CreatedAt: 250, SecurityVersion: 3}, user) {
		t.Fatal("matching security version must remain valid")
	}
	if !sessionInvalidForUser(cloud.SessionState{CreatedAt: 250, SecurityVersion: 2}, user) {
		t.Fatal("security version mismatch must revoke the session even when timestamps are newer")
	}
	if !sessionInvalidForUser(cloud.SessionState{CreatedAt: 199}, user) {
		t.Fatal("legacy session created before password change must be invalid")
	}
	if sessionInvalidForUser(cloud.SessionState{CreatedAt: 200}, user) {
		t.Fatal("legacy replacement session created at the password change timestamp must remain compatible")
	}
	if sessionInvalidForUser(cloud.SessionState{CreatedAt: 0}, cloud.User{}) {
		t.Fatal("legacy session must remain valid until a security-changing event occurs")
	}
}

func TestPasswordResetOTPIsScopedAwayFromSignup(t *testing.T) {
	t.Setenv("MCP_AUTH_SECRET", "test-secret")
	token := "abcdefghijklmnopqrstuvwxyzABCDEF"
	code := "123456"
	resetHash := passwordResetCodeHash(token, code)
	if resetHash == signupCodeHash(token, code) {
		t.Fatal("password reset and signup OTPs must use different hash namespaces")
	}
	if !validResetCode(token, code, resetHash) {
		t.Fatal("valid reset code was rejected")
	}
	if validResetCode(token, "654321", resetHash) {
		t.Fatal("wrong reset code was accepted")
	}
}

func TestPasswordResetDailySendPolicy(t *testing.T) {
	if passwordResetDailySendLimit != 2 {
		t.Fatalf("password reset daily send limit=%d want 2", passwordResetDailySendLimit)
	}
	if passwordResetDailyWindow != 24*time.Hour {
		t.Fatalf("password reset daily window=%s want 24h", passwordResetDailyWindow)
	}
	token := "abcdefghijklmnopqrstuvwxyzABCDEF"
	if got := passwordResetFinalizeLockKey(token); got != passwordResetKey(token)+":finalize" {
		t.Fatalf("unexpected finalize lock key %q", got)
	}
	if got := passwordResetAttemptKey(token); got != passwordResetKey(token)+":attempts" {
		t.Fatalf("unexpected attempt counter key %q", got)
	}
}

func TestPasswordResetDailyLimitPageExplainsLimitAndRetry(t *testing.T) {
	html := passwordResetDailyLimitPage(int(3 * time.Hour / time.Second))
	for _, want := range []string{"Reset email limit reached", "at most 2 password reset emails in 24 hours", "in about 3 hours", "Back to sign in"} {
		if !strings.Contains(html, want) {
			t.Fatalf("password reset limit page missing %q", want)
		}
	}
}

func TestIdentityReauthenticationWindow(t *testing.T) {
	if (&Identity{RiskUntil: time.Now().Add(time.Minute).UnixMilli()}).RequiresReauthentication() != true {
		t.Fatal("active security risk must require reauthentication")
	}
	if (&Identity{RiskUntil: time.Now().Add(-time.Minute).UnixMilli()}).RequiresReauthentication() {
		t.Fatal("expired security risk must not require reauthentication")
	}
}

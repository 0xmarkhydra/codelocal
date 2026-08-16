package webauth

import (
	"regexp"
	"strings"
	"testing"
)

func TestNewSignupCodeIsSixDigits(t *testing.T) {
	pattern := regexp.MustCompile(`^[0-9]{6}$`)
	for i := 0; i < 20; i++ {
		code, err := newSignupCode()
		if err != nil {
			t.Fatal(err)
		}
		if !pattern.MatchString(code) {
			t.Fatalf("unexpected verification code: %q", code)
		}
	}
}

func TestSignupCodeHashBindsTokenAndCode(t *testing.T) {
	first := signupCodeHash("token-one", "123456")
	if first == signupCodeHash("token-one", "654321") {
		t.Fatal("verification hash must change with code")
	}
	if first == signupCodeHash("token-two", "123456") {
		t.Fatal("verification hash must change with pending signup token")
	}
}

func TestVerificationFormUsesOneTimeCodeHints(t *testing.T) {
	manager := &Manager{}
	page := manager.verificationForm("csrf", "token", "someone@example.com", "")
	for _, want := range []string{`name="code"`, `inputmode="numeric"`, `autocomplete="one-time-code"`, `pattern="[0-9]{6}"`, `maxlength="6"`} {
		if !strings.Contains(page, want) {
			t.Fatalf("verification form missing %s", want)
		}
	}
	if !strings.Contains(page, "s******@example.com") {
		t.Fatal("verification page should mask the destination email")
	}
}

package webauth

import (
	"regexp"
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

func TestMaskEmail(t *testing.T) {
	if got := maskEmail("someone@example.com"); got != "s******@example.com" {
		t.Fatalf("maskEmail()=%q", got)
	}
}

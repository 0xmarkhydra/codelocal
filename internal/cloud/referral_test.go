package cloud

import "testing"

func TestAdminEmailsFallbackAndOverride(t *testing.T) {
	t.Setenv("CODELOCAL_ADMIN_EMAILS", "")
	t.Setenv("CODELOCAL_ADMIN_EMAIL", "")
	if got := PrimaryAdminEmail(); got != "monglv36@gmail.com" {
		t.Fatalf("fallback admin = %q", got)
	}
	if !IsAdminEmail(" MONGlv36@GMAIL.COM ") {
		t.Fatal("fallback admin should be case-insensitive")
	}

	t.Setenv("CODELOCAL_ADMIN_EMAILS", "owner@example.com, ops@example.com, OWNER@example.com")
	if got := PrimaryAdminEmail(); got != "owner@example.com" {
		t.Fatalf("primary admin = %q", got)
	}
	if IsAdminEmail("monglv36@gmail.com") {
		t.Fatal("fallback admin must not remain active when env overrides it")
	}
	if !IsAdminEmail("ops@example.com") {
		t.Fatal("secondary configured admin not recognized")
	}
}

func TestReferralCodeNormalizationAndGeneration(t *testing.T) {
	if got := NormalizeReferralCode("  mMon  "); got != "MMON" {
		t.Fatalf("normalized code = %q", got)
	}
	for _, code := range []string{"MMON", "CL7K9AB2QZ", "USER2026"} {
		if !ValidReferralCode(code) {
			t.Fatalf("valid code rejected: %q", code)
		}
	}
	for _, code := range []string{"", "ABC", "HAS SPACE", "BAD-CODE", "CODE!"} {
		if ValidReferralCode(code) {
			t.Fatalf("invalid code accepted: %q", code)
		}
	}
	for i := 0; i < 20; i++ {
		code := RandomReferralCode()
		if !ValidReferralCode(code) || len(code) != 10 || code[:2] != "CL" {
			t.Fatalf("generated invalid code: %q", code)
		}
	}
}

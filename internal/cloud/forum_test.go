package cloud

import "testing"

func TestNormalizeForumAssetIDsDeduplicatesAndLimits(t *testing.T) {
	got := normalizeForumAssetIDs([]string{" media_one ", "media_one", "bad", "media_two", "media_three"}, 2)
	if len(got) != 2 || got[0] != "media_one" || got[1] != "media_two" {
		t.Fatalf("unexpected asset ids: %#v", got)
	}
}

func TestNormalizeForumDeleteReasonRequiresAuditReason(t *testing.T) {
	if _, err := normalizeForumDeleteReason("   "); err != ErrForumInvalid {
		t.Fatalf("empty delete reason err=%v want ErrForumInvalid", err)
	}
	got, err := normalizeForumDeleteReason("  spam / abuse  ")
	if err != nil || got != "spam / abuse" {
		t.Fatalf("normalized delete reason=(%q,%v)", got, err)
	}
	if _, err := normalizeForumDeleteReason(string(make([]byte, 501))); err != ErrForumInvalid {
		t.Fatalf("oversized delete reason err=%v want ErrForumInvalid", err)
	}
}

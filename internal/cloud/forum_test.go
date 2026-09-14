package cloud

import "testing"

func TestNormalizeForumAssetIDsDeduplicatesAndLimits(t *testing.T) {
	got := normalizeForumAssetIDs([]string{" media_one ", "media_one", "bad", "media_two", "media_three"}, 2)
	if len(got) != 2 || got[0] != "media_one" || got[1] != "media_two" {
		t.Fatalf("unexpected asset ids: %#v", got)
	}
}

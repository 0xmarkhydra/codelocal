package orchestration

import (
	"strings"
	"testing"
)

func TestNormalizeFeedbackDedupeKeyIsStable(t *testing.T) {
	a := NormalizeFeedbackDedupeKey("bug", "Payment lỗi khi thanh toán!")
	b := NormalizeFeedbackDedupeKey("bug", "  payment   lỗi khi thanh toán ")
	if a != b {
		t.Fatalf("dedupe key unstable: %q vs %q", a, b)
	}
	c := NormalizeFeedbackDedupeKey("feature", "Payment lỗi khi thanh toán!")
	if a == c {
		t.Fatal("different kinds must produce different dedupe keys")
	}
	if !strings.HasPrefix(a, "bug:") {
		t.Fatalf("dedupe key should carry kind prefix, got %q", a)
	}
}

func TestClusterFeedbackSignalsThreshold(t *testing.T) {
	key := NormalizeFeedbackDedupeKey("bug", "App crash on login")
	items := []FeedbackSignalItem{
		{FeedbackID: "fb_1", DedupeKey: key, Kind: "bug", Title: "App crash on login"},
		{FeedbackID: "fb_2", DedupeKey: key, Kind: "bug", Title: "App crash on login"},
		{FeedbackID: "fb_3", DedupeKey: "other:abc", Kind: "feature", Title: "Dark mode"},
	}
	clusters := ClusterFeedbackSignals(items, 3)
	if len(clusters) != 0 {
		t.Fatalf("below-threshold groups must not signal, got %v", clusters)
	}
	items = append(items, FeedbackSignalItem{FeedbackID: "fb_4", DedupeKey: key, Kind: "bug", Title: "App crash on login"})
	clusters = ClusterFeedbackSignals(items, 3)
	if len(clusters) != 1 || clusters[0].Count != 3 {
		t.Fatalf("threshold cluster missing: %v", clusters)
	}
	if len(clusters[0].SampleIDs) != 3 {
		t.Fatalf("samples should be bounded representatives, got %v", clusters[0].SampleIDs)
	}
}

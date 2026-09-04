package orchestration

import (
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
)

// NormalizeFeedbackDedupeKey builds a stable clustering key from kind + normalized
// title so duplicate widget/chat reports of the same issue collapse into one signal.
// It is intentionally lexical (no ML): P0 clustering is dedupe + counts.
func NormalizeFeedbackDedupeKey(kind, title string) string {
	kind = strings.ToLower(strings.TrimSpace(kind))
	switch kind {
	case "bug", "feature", "praise", "other":
	default:
		kind = "other"
	}
	title = strings.Join(strings.Fields(strings.ToLower(title)), " ")
	// Strip punctuation so "Payment lỗi!" and "payment loi" still differ only by
	// diacritics (kept) — exact-enough for P1, full normalization later.
	title = strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r >= 0x80 {
			return r
		}
		if r == ' ' || r == '-' || r == '_' {
			return ' '
		}
		return -1
	}, title)
	title = strings.Join(strings.Fields(title), " ")
	if title == "" {
		title = "untitled"
	}
	sum := sha256.Sum256([]byte(kind + "\x00" + title))
	return kind + ":" + hex.EncodeToString(sum[:])[:24]
}

type FeedbackSignalItem struct {
	FeedbackID string
	DedupeKey  string
	Kind       string
	Title      string
}

type FeedbackCluster struct {
	DedupeKey string   `json:"dedupeKey"`
	Kind      string   `json:"kind"`
	Title     string   `json:"title"`
	Count     int      `json:"count"`
	SampleIDs []string `json:"sampleIds"`
}

// ClusterFeedbackSignals groups items by dedupe key and returns only clusters at or
// above threshold, sorted by count desc. Below-threshold groups stay visible in the
// raw list but never page anyone.
func ClusterFeedbackSignals(items []FeedbackSignalItem, threshold int) []FeedbackCluster {
	if threshold <= 0 {
		threshold = 3
	}
	groups := map[string][]FeedbackSignalItem{}
	for _, item := range items {
		if strings.TrimSpace(item.DedupeKey) == "" {
			continue
		}
		groups[item.DedupeKey] = append(groups[item.DedupeKey], item)
	}
	out := []FeedbackCluster{}
	for key, members := range groups {
		if len(members) < threshold {
			continue
		}
		kind := members[0].Kind
		title := members[0].Title
		samples := []string{}
		for i, m := range members {
			if i >= 5 {
				break
			}
			samples = append(samples, m.FeedbackID)
		}
		out = append(out, FeedbackCluster{DedupeKey: key, Kind: kind, Title: title, Count: len(members), SampleIDs: samples})
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Count != out[j].Count {
			return out[i].Count > out[j].Count
		}
		return out[i].DedupeKey < out[j].DedupeKey
	})
	return out
}

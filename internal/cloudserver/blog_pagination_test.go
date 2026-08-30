package cloudserver

import "testing"

func TestPublicBlogPageLimit(t *testing.T) {
	cases := map[string]int{
		"":      100,
		"0":     100,
		"bad":   100,
		"1":     1,
		"100":   100,
		"200":   200,
		"201":   200,
		"10000": 200,
	}
	for raw, want := range cases {
		if got := publicBlogPageLimit(raw); got != want {
			t.Fatalf("limit(%q)=%d want %d", raw, got, want)
		}
	}
}

func TestPublicBlogPageOffset(t *testing.T) {
	cases := map[string]int{
		"":      0,
		"-1":    0,
		"bad":   0,
		"0":     0,
		"100":   100,
		"10000": 10000,
	}
	for raw, want := range cases {
		if got := publicBlogPageOffset(raw); got != want {
			t.Fatalf("offset(%q)=%d want %d", raw, got, want)
		}
	}
}

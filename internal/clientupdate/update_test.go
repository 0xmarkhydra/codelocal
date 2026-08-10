package clientupdate

import (
	"strings"
	"testing"
)

func TestCompareSemverPrereleaseOrdering(t *testing.T) {
	cases := []struct {
		left, right string
		want        int
	}{
		{"1.5.0-beta.2", "1.5.0-beta.3", -1},
		{"1.5.0-beta.10", "1.5.0-beta.3", 1},
		{"1.5.0-beta.3", "1.5.0", -1},
		{"1.5.0", "1.5.0-beta.3", 1},
		{"v1.5.0-beta.3", "1.5.0-beta.3", 0},
	}
	for _, tc := range cases {
		got, ok := Compare(tc.left, tc.right)
		if !ok || got != tc.want {
			t.Fatalf("Compare(%q,%q)=(%d,%v), want (%d,true)", tc.left, tc.right, got, ok, tc.want)
		}
	}
}

func TestManifestDefaultsToBetaChannel(t *testing.T) {
	t.Setenv("CODELOCAL_RELEASE_CHANNEL", "")
	t.Setenv("CODELOCAL_UPDATE_COMMAND", "")
	manifest := ManifestFromEnv()
	if manifest.Channel != "beta" {
		t.Fatalf("channel=%q", manifest.Channel)
	}
	if manifest.UpdateCommand != "npm i -g codelocal@beta" {
		t.Fatalf("update command=%q", manifest.UpdateCommand)
	}
}

func TestEvaluateUpdateLevels(t *testing.T) {
	manifest := Manifest{
		LatestVersion:  "1.5.0-beta.4",
		MinimumVersion: "1.5.0-beta.2",
		Channel:        "beta",
		UpdateCommand:  "npm i -g codelocal@beta",
		RestartCommand: "codelocal",
		Message:        "Update available.",
	}
	if notice := Evaluate("1.5.0-beta.4", manifest); notice != nil {
		t.Fatalf("up-to-date client received notice: %#v", notice)
	}
	if notice := Evaluate("1.5.0-beta.3", manifest); notice == nil || notice.Level != Recommended {
		t.Fatalf("expected recommended update, got %#v", notice)
	}
	if notice := Evaluate("1.5.0-beta.1", manifest); notice == nil || notice.Level != Required {
		t.Fatalf("expected required update, got %#v", notice)
	}
	if notice := Evaluate("", manifest); notice == nil || notice.Level != Recommended {
		t.Fatalf("legacy client should get recommended update, got %#v", notice)
	}
}

func TestRenderNoticeIncludesSafeBetaCommand(t *testing.T) {
	notice := Notice{Key: "client-update:1.5.0-beta.4", Level: Recommended, InstalledVersion: "1.5.0-beta.2", LatestVersion: "1.5.0-beta.4", MinimumVersion: "1.5.0-beta.2", Channel: "beta", UpdateCommand: "npm i -g codelocal@beta", RestartCommand: "codelocal", Message: "Update available."}
	text := Render(notice)
	for _, want := range []string{"[CODELOCAL_UPDATE_NOTICE]", "npm i -g codelocal@beta", "Installed: 1.5.0-beta.2", "Latest: 1.5.0-beta.4"} {
		if !strings.Contains(text, want) {
			t.Fatalf("notice missing %q: %s", want, text)
		}
	}
}

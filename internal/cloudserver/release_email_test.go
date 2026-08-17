package cloudserver

import (
	"strings"
	"testing"
)

func TestReleaseEmailContentIncludesUpdateAndChatGPTRefreshSteps(t *testing.T) {
	text, html := releaseEmailContent("1.6.0")

	for _, want := range []string{
		"npm install -g codelocal@latest",
		"hash -r",
		"codelocal --version",
		"Settings -> Plugins -> CodeLocal -> Refresh",
		"1.6.0",
	} {
		if !strings.Contains(text, want) {
			t.Fatalf("release text missing %q:\n%s", want, text)
		}
	}

	for _, want := range []string{
		"npm install -g codelocal@latest",
		"hash -r",
		"codelocal --version",
		"Settings → Plugins → CodeLocal → Refresh",
		"1.6.0",
	} {
		if !strings.Contains(html, want) {
			t.Fatalf("release HTML missing %q:\n%s", want, html)
		}
	}
}

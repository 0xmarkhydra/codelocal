package cloud

import "testing"

func TestSafeCloudRepositoryRemote(t *testing.T) {
	if got := safeCloudRepositoryRemote("https://github.com/acme/project.git#main"); got != "https://github.com/acme/project.git" {
		t.Fatalf("safe remote = %q", got)
	}
	for _, value := range []string{
		"https://token@example.com/acme/project.git",
		"git@github.com:acme/project.git",
		"ssh://git@github.com/acme/project.git",
		"file:///tmp/project",
	} {
		if got := safeCloudRepositoryRemote(value); got != "" {
			t.Fatalf("unsafe remote %q accepted as %q", value, got)
		}
	}
}

func TestSafeCloudRepositoryPath(t *testing.T) {
	for input, want := range map[string]string{".": ".", "apps/web": "apps/web", "apps/../api": "api"} {
		if got := safeCloudRepositoryPath(input); got != want {
			t.Fatalf("path %q = %q, want %q", input, got, want)
		}
	}
	for _, value := range []string{"../secret", "/etc", "../../tmp"} {
		if got := safeCloudRepositoryPath(value); got != "" {
			t.Fatalf("unsafe path %q accepted as %q", value, got)
		}
	}
}

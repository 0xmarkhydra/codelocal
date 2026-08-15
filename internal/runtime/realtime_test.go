package runtime

import "testing"

func TestRuntimeControlURL(t *testing.T) {
	cases := map[string]string{
		"https://codelocal.cloud":      "wss://codelocal.cloud/api/client/runtime/ws",
		"http://localhost:3333":        "ws://localhost:3333/api/client/runtime/ws",
		"https://example.com/old/path": "wss://example.com/api/client/runtime/ws",
	}
	for input, expected := range cases {
		if actual := runtimeControlURL(input); actual != expected {
			t.Fatalf("runtimeControlURL(%q) = %q, want %q", input, actual, expected)
		}
	}
}

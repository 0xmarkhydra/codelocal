package mcpconfig

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestParseLocalCursorStyleConfig(t *testing.T) {
	raw := `{
	  "mcpServers": {
	    "playwright": {
	      "command": "npx",
	      "args": ["-y", "@playwright/mcp@latest"],
	      "env": {"API_KEY": "${API_KEY}"}
	    }
	  }
	}`
	servers, err := Parse(raw, ModeLocal)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Name != "playwright" || servers[0].Transport != "stdio" {
		t.Fatalf("unexpected servers: %#v", servers)
	}
	if servers[0].Env["API_KEY"].Source != "API_KEY" {
		t.Fatalf("unexpected env ref: %#v", servers[0].Env)
	}
}

func TestParseOnlineRemoteConfig(t *testing.T) {
	raw := `{
	  "github": {
	    "url": "https://example.com/mcp",
	    "headers": {"Authorization": "Bearer ${TOKEN}"}
	  }
	}`
	servers, err := Parse(raw, ModeOnline)
	if err != nil {
		t.Fatal(err)
	}
	if len(servers) != 1 || servers[0].Transport != "http" {
		t.Fatalf("unexpected servers: %#v", servers)
	}
	ref := servers[0].Headers["Authorization"]
	if ref.Source != "TOKEN" || ref.Prefix != "Bearer " {
		t.Fatalf("unexpected header ref: %#v", ref)
	}
}

func TestParseRejectsLiteralSecrets(t *testing.T) {
	for _, raw := range []string{
		`{"mcpServers":{"bad":{"command":"node","env":{"TOKEN":"literal-secret"}}}}`,
		`{"mcpServers":{"bad":{"url":"https://example.com/mcp","headers":{"Authorization":"Bearer literal-secret"}}}}`,
	} {
		if _, err := Parse(raw, ModeLocal); err == nil {
			t.Fatalf("expected secret rejection for %s", raw)
		}
	}
}

func TestParseOnlineRejectsStdio(t *testing.T) {
	if _, err := Parse(`{"mcpServers":{"bad":{"command":"node"}}}`, ModeOnline); err == nil {
		t.Fatal("expected online stdio rejection")
	}
}

func TestActualUsesCanonicalJSONName(t *testing.T) {
	raw, err := json.Marshal(Actual{Name: "playwright", State: "ready", ToolCount: 3})
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, `"name":"playwright"`) || strings.Contains(text, `"Name"`) {
		t.Fatalf("unexpected actual JSON: %s", text)
	}
}

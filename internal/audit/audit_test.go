package audit

import (
	"errors"
	"strings"
	"testing"
)

func TestSanitizeRedactsCredentialKeysAndFreeFormText(t *testing.T) {
	value := sanitize(map[string]any{
		"apiKey":  "credential-value",
		"api_key": "credential-value-2",
		"nested": map[string]any{
			"privateKey": "private-value",
			"text":       "one-time code 123456",
		},
	}, "detail")
	root, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("sanitize returned %T", value)
	}
	if root["apiKey"] != "[REDACTED]" || root["api_key"] != "[REDACTED]" {
		t.Fatal("API-key shaped fields must be redacted")
	}
	nested, ok := root["nested"].(map[string]any)
	if !ok {
		t.Fatalf("nested sanitize returned %T", root["nested"])
	}
	if nested["privateKey"] != "[REDACTED]" {
		t.Fatal("private-key shaped field must be redacted")
	}
	if nested["text"] != "[20 bytes]" {
		t.Fatalf("free-form text should retain size only, got %#v", nested["text"])
	}
}

func TestSanitizeRedactsSecretsInsideErrors(t *testing.T) {
	value := sanitize(errors.New("provider failed api_key=super-secret-value"), "error")
	root, ok := value.(map[string]any)
	if !ok {
		t.Fatalf("sanitize error returned %T", value)
	}
	message, _ := root["message"].(string)
	if strings.Contains(message, "super-secret-value") || !strings.Contains(message, "REDACTED") {
		t.Fatalf("error message leaked secret: %q", message)
	}
}

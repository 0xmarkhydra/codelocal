package cloud

import (
	"bytes"
	"testing"
)

func TestRuntimeSecretEncryptionRoundTrip(t *testing.T) {
	t.Setenv("CODELOCAL_SECRET_ENCRYPTION_KEY", "test-key-with-at-least-thirty-two-characters")
	nonce, ciphertext, err := encryptRuntimeSecret("super-secret")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(ciphertext, []byte("super-secret")) {
		t.Fatal("runtime secret ciphertext contains plaintext")
	}
	value, err := decryptRuntimeSecret(nonce, ciphertext)
	if err != nil || value != "super-secret" {
		t.Fatalf("round trip failed: value=%q err=%v", value, err)
	}
}

func TestRuntimeScopeValidation(t *testing.T) {
	valid := []struct {
		scope             RuntimeScope
		device, workspace string
	}{{RuntimeScopeGlobal, "", ""}, {RuntimeScopeDevice, "mac", ""}, {RuntimeScopeWorkspace, "mac", "media"}}
	for _, item := range valid {
		if _, _, _, err := normalizeRuntimeScope(item.scope, item.device, item.workspace); err != nil {
			t.Fatalf("valid scope rejected: %#v: %v", item, err)
		}
	}
	if _, _, _, err := normalizeRuntimeScope(RuntimeScopeSession, "mac", "media"); err == nil {
		t.Fatal("session scope must remain local-only")
	}
}

func TestRuntimeConfigMergeAndOpenMontageDefault(t *testing.T) {
	global := RuntimeConfigLayer{Scope: RuntimeScopeGlobal, Values: map[string]string{"TTS_PROVIDER": "vbee", "video.aspect": "16:9"}}
	device := RuntimeConfigLayer{Scope: RuntimeScopeDevice, Values: map[string]string{"FFMPEG_PATH": "ffmpeg"}}
	workspace := RuntimeConfigLayer{Scope: RuntimeScopeWorkspace, Values: map[string]string{"video.aspect": "9:16"}, Secrets: map[string]RuntimeSecretRef{"VBEE_API_KEY": {Configured: true}}}
	got := MergeRuntimeConfig(defaultRuntimeConfigLayer(), global, device, workspace)
	if got.Values["video.aspect"] != "9:16" || got.Values["FFMPEG_PATH"] != "ffmpeg" {
		t.Fatalf("unexpected merged values: %#v", got.Values)
	}
	if !got.Secrets["VBEE_API_KEY"].Configured {
		t.Fatalf("secret metadata missing: %#v", got.Secrets)
	}
	if len(got.SystemProjects) != 1 || got.SystemProjects[0].ID != OpenMontageSystemProjectID || got.SystemProjects[0].Name != OpenMontageName || !got.SystemProjects[0].SystemApp || !got.SystemProjects[0].Managed || got.SystemProjects[0].Hidden || got.SystemProjects[0].Enabled {
		t.Fatalf("unexpected OpenMontage defaults: %#v", got.SystemProjects)
	}
}

func TestValidRuntimeEnvKey(t *testing.T) {
	for _, key := range []string{"VBEE_API_KEY", "FFMPEG_PATH", "A1"} {
		if !ValidRuntimeEnvKey(key) {
			t.Fatalf("valid env key rejected: %q", key)
		}
	}
	for _, key := range []string{"", "1BAD", "video.aspect", "BAD-KEY"} {
		if ValidRuntimeEnvKey(key) {
			t.Fatalf("invalid env key accepted: %q", key)
		}
	}
}

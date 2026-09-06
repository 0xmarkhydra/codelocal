package mcphub

import "testing"

func TestMaterializeHeadersPrefersRuntimeSecretWithoutPersistingValue(t *testing.T) {
	hub, err := New(t.TempDir(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer hub.Close()
	hub.SetSecretResolver(func(name string) (string, bool) {
		if name == "CODELOCAL_PLUGIN_TOKEN" {
			return "server-decrypted-secret", true
		}
		return "", false
	})
	refs := map[string]HeaderReference{"Authorization": {Source: "CODELOCAL_PLUGIN_TOKEN", Prefix: "Bearer "}}
	headers, err := hub.materializeHeaders(refs)
	if err != nil {
		t.Fatal(err)
	}
	if headers.Get("Authorization") != "Bearer server-decrypted-secret" {
		t.Fatalf("authorization=%q", headers.Get("Authorization"))
	}
	if refs["Authorization"].Source == "server-decrypted-secret" {
		t.Fatal("secret value was written into persistent header references")
	}
}

package webutil

import (
	"net/http/httptest"
	"testing"
)

func TestNetworkPrefixMinimizesIPAddress(t *testing.T) {
	if got := NetworkPrefix("203.0.113.42"); got != "203.0.113.0/24" {
		t.Fatalf("ipv4 prefix=%q", got)
	}
	if got := NetworkPrefix("2001:db8:abcd:1234:5678::99"); got != "2001:db8:abcd:1234::/64" {
		t.Fatalf("ipv6 prefix=%q", got)
	}
	if got := NetworkPrefix("not-an-ip"); got != "" {
		t.Fatalf("invalid ip prefix=%q", got)
	}
}

func TestSecurityAgentIgnoresVersionNumberChurn(t *testing.T) {
	first := normalizeSecurityAgent("CodeLocal/1.5.19 Go-http-client/1.1")
	second := normalizeSecurityAgent("CodeLocal/1.5.23 Go-http-client/1.1")
	if first != second {
		t.Fatalf("version-only change altered normalized agent: %q vs %q", first, second)
	}
	if first == normalizeSecurityAgent("curl/8.7.1") {
		t.Fatal("different client families must not normalize together")
	}
}

func TestRequestSecuritySignalHashesSensitiveContext(t *testing.T) {
	t.Setenv("MCP_AUTH_SECRET", "test-secret")
	req := httptest.NewRequest("GET", "https://codelocal.cloud/dashboard", nil)
	req.RemoteAddr = "203.0.113.42:443"
	req.Header.Set("User-Agent", "Mozilla/5.0 ExampleBrowser/123")
	signal := RequestSecuritySignal(req, "browser-device-token")
	for name, value := range map[string]string{"device": signal.DeviceHash, "agent": signal.AgentHash, "network": signal.NetworkHash} {
		if len(value) != 64 {
			t.Fatalf("%s hash length=%d", name, len(value))
		}
	}
	if signal.NetworkHash == "203.0.113.0/24" || signal.DeviceHash == "browser-device-token" {
		t.Fatal("raw security context must never be stored in the signal")
	}
}

func TestRequestSecuritySignalSupportsStagedBindingSecretMigration(t *testing.T) {
	t.Setenv("MCP_AUTH_SECRET", "legacy-secret")
	req := httptest.NewRequest("GET", "https://codelocal.cloud/dashboard", nil)
	req.RemoteAddr = "203.0.113.42:443"
	req.Header.Set("User-Agent", "Mozilla/5.0 ExampleBrowser/123")

	t.Setenv("CODELOCAL_SECURITY_BINDING_SECRET", "dedicated-secret")
	migrating := RequestSecuritySignal(req, "browser-device-token")
	if migrating.DeviceHash == "" || migrating.LegacyDeviceHash == "" || migrating.DeviceHash == migrating.LegacyDeviceHash {
		t.Fatalf("expected distinct primary and legacy device hashes: %#v", migrating)
	}

	t.Setenv("CODELOCAL_SECURITY_BINDING_SECRET", "")
	legacy := RequestSecuritySignal(req, "browser-device-token")
	if legacy.DeviceHash != migrating.LegacyDeviceHash {
		t.Fatal("legacy alias must match the pre-migration security hash")
	}
}

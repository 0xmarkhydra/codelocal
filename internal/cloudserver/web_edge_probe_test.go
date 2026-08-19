package cloudserver

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
)

func TestWebEdgeProbeDigestIsStableAndKeyed(t *testing.T) {
	first := webEdgeProbeDigest("token-a", "198.51.100.7")
	if first == "" || len(first) != 24 {
		t.Fatalf("unexpected probe digest %q", first)
	}
	if second := webEdgeProbeDigest("token-a", "198.51.100.7"); second != first {
		t.Fatalf("probe digest is not stable: %q != %q", second, first)
	}
	if other := webEdgeProbeDigest("token-b", "198.51.100.7"); other == first {
		t.Fatal("probe digest must be keyed by the deployment secret")
	}
}

func TestWebEdgeProbeIsHiddenWithoutValidToken(t *testing.T) {
	t.Setenv("CODELOCAL_EDGE_PROBE_TOKEN", "probe-secret-123456789")
	s := &Server{}

	missing := httptest.NewRecorder()
	s.webEdgeProbe(missing, httptest.NewRequest("GET", "/internal/web-edge-probe", nil))
	if missing.Code != 404 {
		t.Fatalf("missing token status = %d, want 404", missing.Code)
	}

	wrongRequest := httptest.NewRequest("GET", "/internal/web-edge-probe", nil)
	wrongRequest.Header.Set("Authorization", "Bearer wrong-secret")
	wrong := httptest.NewRecorder()
	s.webEdgeProbe(wrong, wrongRequest)
	if wrong.Code != 404 {
		t.Fatalf("wrong token status = %d, want 404", wrong.Code)
	}
}

func TestWebEdgeProbeReturnsOnlyOpaqueSecuritySignals(t *testing.T) {
	const token = "probe-secret-123456789"
	t.Setenv("CODELOCAL_EDGE_PROBE_TOKEN", token)
	request := httptest.NewRequest("GET", "/internal/web-edge-probe", nil)
	request.RemoteAddr = "198.51.100.7:43210"
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("User-Agent", "CodeLocal-Edge-Probe/test")
	request.Header.Set("X-Forwarded-Proto", "https")
	request.Header.Set("X-Railway-Edge", "edge-test")
	request.Header.Set("X-Railway-Request-Id", "request-test")

	response := httptest.NewRecorder()
	(&Server{}).webEdgeProbe(response, request)
	if response.Code != 200 {
		t.Fatalf("probe status = %d, body=%s", response.Code, response.Body.String())
	}
	var value webEdgeProbeDTO
	if err := json.Unmarshal(response.Body.Bytes(), &value); err != nil {
		t.Fatal(err)
	}
	if value.ClientIPDigest != webEdgeProbeDigest(token, "198.51.100.7") || value.UserAgentDigest != webEdgeProbeDigest(token, "CodeLocal-Edge-Probe/test") {
		t.Fatalf("unexpected opaque probe values: %#v", value)
	}
	if value.ForwardedProto != "https" || !value.RailwayEdgePresent || !value.RailwayRequestIDPresent {
		t.Fatalf("expected Railway forwarding evidence: %#v", value)
	}
	if response.Body.String() == "198.51.100.7" {
		t.Fatal("probe must not expose the raw client IP")
	}
}

package deviceauth

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestSignedRequestBindsMethodPathAndBody(t *testing.T) {
	publicKey, privateKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	body := []byte(`{"workspaceIds":["demo"]}`)
	req := httptest.NewRequest(http.MethodPost, "https://codelocal.test/api/client/runtime/poll?x=1", strings.NewReader(string(body)))
	if err := SignRequest(req, body, privateKey, now); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRequest(req, publicKey, now.Add(30*time.Second)); err != nil {
		t.Fatalf("valid signed request rejected: %v", err)
	}
	raw, _ := io.ReadAll(req.Body)
	if string(raw) != string(body) {
		t.Fatal("verification must restore the request body")
	}
}

func TestSignedRequestRejectsTamperingAndClockSkew(t *testing.T) {
	publicKey, privateKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	body := []byte(`{"ok":true}`)
	req := httptest.NewRequest(http.MethodPost, "https://codelocal.test/api/client/auth/check", strings.NewReader(string(body)))
	if err := SignRequest(req, body, privateKey, now); err != nil {
		t.Fatal(err)
	}
	req.Body = io.NopCloser(strings.NewReader(`{"ok":false}`))
	if err := VerifyRequest(req, publicKey, now); err == nil {
		t.Fatal("tampered body must fail signature verification")
	}

	req = httptest.NewRequest(http.MethodPost, "https://codelocal.test/api/client/auth/check", strings.NewReader(string(body)))
	if err := SignRequest(req, body, privateKey, now); err != nil {
		t.Fatal(err)
	}
	if err := VerifyRequest(req, publicKey, now.Add(MaxClockSkew+time.Second)); err == nil {
		t.Fatal("stale signed request must fail")
	}
}

func TestSignatureUsesUniqueBoundNonce(t *testing.T) {
	publicKey, privateKey, err := GenerateKeyPair()
	if err != nil {
		t.Fatal(err)
	}
	now := time.Unix(1_700_000_000, 0)
	first, err := SignatureHeaders(http.MethodGet, "wss://codelocal.test/client", nil, privateKey, now)
	if err != nil {
		t.Fatal(err)
	}
	second, err := SignatureHeaders(http.MethodGet, "wss://codelocal.test/client", nil, privateKey, now)
	if err != nil {
		t.Fatal(err)
	}
	if first.Get(HeaderNonce) == "" || first.Get(HeaderNonce) == second.Get(HeaderNonce) {
		t.Fatal("each signed request must use a unique nonce")
	}
	req := httptest.NewRequest(http.MethodGet, "https://codelocal.test/client", nil)
	for key, values := range first {
		req.Header[key] = append([]string(nil), values...)
	}
	if RequestNonce(req) != first.Get(HeaderNonce) {
		t.Fatal("request nonce header was not preserved")
	}
	if err := VerifyRequest(req, publicKey, now); err != nil {
		t.Fatalf("nonce-bound signature rejected: %v", err)
	}
}

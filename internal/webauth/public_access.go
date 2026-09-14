package webauth

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net"
	"net/http"
	"os"
	"strings"
)

// PublicAccessTokenEnv gates every request that reaches the gateway from
// outside the loopback interface.
//
// Local-owner mode removes the account/login layer for a gateway owned by the
// machine operator. That is safe while the gateway only listens on 127.0.0.1,
// but it becomes unsafe the moment the same gateway is exposed to the internet
// through a tunnel or reverse proxy: an anonymous caller could reach the MCP
// tool surface and execute work on this machine.
//
// When this variable is set, remote requests must prove possession of the
// token. Loopback requests are unaffected, so the local CLI, Codex, the
// dashboard and the runtime keep working without any credential.
const PublicAccessTokenEnv = "CODELOCAL_PUBLIC_ACCESS_TOKEN"

// PublicPathPrefix is the URL prefix that carries the remote token, for example
// /t/<token>/mcp. A path prefix is used because MCP clients that negotiate "no
// authentication" cannot set custom headers on the connect request, but they
// can always be given an arbitrary URL.
const PublicPathPrefix = "/t/"

// PublicAccessToken returns the configured token, or "" when remote access is
// not gated.
func PublicAccessToken() string { return strings.TrimSpace(os.Getenv(PublicAccessTokenEnv)) }

// RemoteAccessGated reports whether remote (non-loopback) requests must present
// the public access token.
func RemoteAccessGated() bool { return PublicAccessToken() != "" }

// IsLoopbackRequest reports whether the request originated on the local
// machine.
//
// This check is deliberately fail-closed. A tunnel or reverse proxy that
// terminates on this host connects over loopback, so a loopback peer address
// proves nothing on its own. Forwarding headers can also be injected by the
// remote caller, so their values are never trusted either: the mere presence of
// any forwarding header means the request crossed a proxy and is therefore
// treated as remote.
func IsLoopbackRequest(r *http.Request) bool {
	if r == nil {
		return false
	}
	for _, header := range []string{"X-Forwarded-For", "X-Forwarded-Host", "X-Forwarded-Proto", "X-Real-Ip", "Forwarded", "Cf-Connecting-Ip", "Cf-Ray", "Cdn-Loop"} {
		if strings.TrimSpace(r.Header.Get(header)) != "" {
			return false
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err != nil {
		host = strings.TrimSpace(r.RemoteAddr)
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}

// LocalHostHeader returns the loopback authority the gateway listens on. It is
// used to normalize the Host header of proxied requests.
func LocalHostHeader() string {
	host := strings.TrimSpace(os.Getenv("HOST"))
	if host == "" || host == "0.0.0.0" || host == "::" {
		host = "127.0.0.1"
	}
	port := strings.TrimSpace(os.Getenv("PORT"))
	if port == "" {
		port = "3333"
	}
	if host == "127.0.0.1" {
		return host + ":" + port
	}
	return net.JoinHostPort(host, port)
}

// NormalizeProxiedHost rewrites the Host header of a request that arrived
// through a tunnel or reverse proxy so the MCP transport's DNS-rebinding guard
// sees the address the gateway actually listens on.
//
// The guard must stay enabled: a malicious page in the local browser can point
// a hostile hostname at 127.0.0.1 and reach the gateway over loopback. Proxied
// traffic is different — it has already passed the remote-access token gate —
// so its authority is replaced here rather than disabling the guard globally.
func NormalizeProxiedHost(r *http.Request) {
	if r == nil || IsLoopbackRequest(r) {
		return
	}
	if host := LocalHostHeader(); host != "" {
		r.Host = host
	}
}

// PublicAccessTokenMatches compares a presented token with the configured one
// in constant time.
func PublicAccessTokenMatches(presented string) bool {
	expected := PublicAccessToken()
	if expected == "" || presented == "" {
		return false
	}
	a := sha256.Sum256([]byte(presented))
	b := sha256.Sum256([]byte(expected))
	return subtle.ConstantTimeCompare(a[:], b[:]) == 1
}

// SplitPublicPrefix removes a leading /t/<token>/ segment from the request path
// when it carries a valid token, and reports whether the request is authorized.
//
// Returns (newPath, ok). When the prefix is absent the original path is
// returned unchanged with ok=false so the caller can try other credential
// forms.
func SplitPublicPrefix(path string) (string, bool) {
	if !strings.HasPrefix(path, PublicPathPrefix) {
		return path, false
	}
	rest := strings.TrimPrefix(path, PublicPathPrefix)
	segment, remainder, found := strings.Cut(rest, "/")
	if segment == "" {
		return path, false
	}
	if !PublicAccessTokenMatches(segment) {
		return path, false
	}
	if !found {
		return "/", true
	}
	return "/" + remainder, true
}

// presentedPublicToken extracts a caller-supplied token from headers or query.
func presentedPublicToken(r *http.Request) string {
	if header := strings.TrimSpace(r.Header.Get("Authorization")); strings.HasPrefix(header, "Bearer ") {
		if value := strings.TrimSpace(strings.TrimPrefix(header, "Bearer ")); value != "" {
			return value
		}
	}
	if value := strings.TrimSpace(r.Header.Get("X-CodeLocal-Access-Token")); value != "" {
		return value
	}
	return strings.TrimSpace(r.URL.Query().Get("access_token"))
}

// AuthorizeRemoteRequest is the single entry point for the remote-access gate.
// It returns true when the request may proceed. When it returns true and the
// rewritten path differs from the original, the caller must dispatch the
// rewritten path.
func AuthorizeRemoteRequest(w http.ResponseWriter, r *http.Request) (string, bool) {
	if IsLoopbackRequest(r) {
		return r.URL.Path, true
	}
	if !RemoteAccessGated() {
		// No account layer would otherwise protect the tool surface.
		if LocalOwnerMode() {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Content-Type-Options", "nosniff")
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("{\"error\":\"remote_access_disabled\"}\n"))
			return "", false
		}
		return r.URL.Path, true
	}
	if rewritten, ok := SplitPublicPrefix(r.URL.Path); ok {
		return rewritten, true
	}
	if PublicAccessTokenMatches(presentedPublicToken(r)) {
		return r.URL.Path, true
	}
	w.Header().Set("WWW-Authenticate", `Bearer realm="codelocal-remote"`)
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.WriteHeader(http.StatusUnauthorized)
	_, _ = w.Write([]byte("{\"error\":\"remote_access_token_required\"}\n"))
	return "", false
}

// GeneratePublicAccessToken returns a fresh token suitable for
// CODELOCAL_PUBLIC_ACCESS_TOKEN.
func GeneratePublicAccessToken() string {
	buf := make([]byte, 32)
	if _, err := rand.Read(buf); err != nil {
		return ""
	}
	return hex.EncodeToString(buf)
}

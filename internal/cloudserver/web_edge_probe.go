package cloudserver

import (
	"crypto/hmac"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"net/http"
	"os"
	"strings"

	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type webEdgeProbeDTO struct {
	ClientIPDigest          string `json:"clientIpDigest"`
	UserAgentDigest         string `json:"userAgentDigest"`
	ForwardedProto          string `json:"forwardedProto"`
	RailwayEdgePresent      bool   `json:"railwayEdgePresent"`
	RailwayRequestIDPresent bool   `json:"railwayRequestIdPresent"`
}

func webEdgeProbeToken() string {
	return strings.TrimSpace(os.Getenv("CODELOCAL_EDGE_PROBE_TOKEN"))
}

func webEdgeProbeAuthorized(r *http.Request, token string) bool {
	provided := strings.TrimSpace(strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	if token == "" || len(provided) != len(token) {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(provided), []byte(token)) == 1
}

func webEdgeProbeDigest(token, value string) string {
	mac := hmac.New(sha256.New, []byte(token))
	_, _ = mac.Write([]byte(value))
	return hex.EncodeToString(mac.Sum(nil))[:24]
}

func (s *Server) webEdgeProbe(w http.ResponseWriter, r *http.Request) {
	token := webEdgeProbeToken()
	if !webEdgeProbeAuthorized(r, token) {
		http.NotFound(w, r)
		return
	}
	webutil.JSON(w, http.StatusOK, webEdgeProbeDTO{
		ClientIPDigest:          webEdgeProbeDigest(token, webutil.ClientIP(r)),
		UserAgentDigest:         webEdgeProbeDigest(token, r.UserAgent()),
		ForwardedProto:          strings.TrimSpace(r.Header.Get("X-Forwarded-Proto")),
		RailwayEdgePresent:      strings.TrimSpace(r.Header.Get("X-Railway-Edge")) != "",
		RailwayRequestIDPresent: strings.TrimSpace(r.Header.Get("X-Railway-Request-Id")) != "",
	})
}

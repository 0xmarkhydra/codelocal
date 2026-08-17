package cloudserver

import (
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) credentialSecurityDecision(r *http.Request, credentialID string) (cloud.SecurityDecision, error) {
	signal := webutil.RequestSecuritySignal(r, "")
	return s.Store.ObserveSecurityState(r.Context(), "credential", credentialID, signal, 30*24*time.Hour)
}

package cloudserver

import (
	"net/http"

	"github.com/0xmarkhydra/codelocal/internal/webauth"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

func (s *Server) authenticatedAPIIdentity(w http.ResponseWriter, r *http.Request) (*webauth.Identity, bool) {
	identity, err := s.WebAuth.Identity(r)
	if err != nil {
		webutil.JSON(w, http.StatusInternalServerError, map[string]string{"error": "identity_unavailable"})
		return nil, false
	}
	if identity == nil {
		webutil.JSON(w, http.StatusUnauthorized, map[string]string{"error": "unauthorized"})
		return nil, false
	}
	return identity, true
}

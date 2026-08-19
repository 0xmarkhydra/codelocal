package cloudserver

import (
	"net/http"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
)

type accountResourceDTO struct {
	Email                    string `json:"email"`
	UserID                   string `json:"userId"`
	ReferralCode             string `json:"referralCode"`
	InvitedBy                string `json:"invitedBy"`
	CreatedAt                int64  `json:"createdAt"`
	PasswordChangedAt        int64  `json:"passwordChangedAt"`
	IsAdmin                  bool   `json:"isAdmin"`
	RequiresReauthentication bool   `json:"requiresReauthentication"`
	CSRF                     string `json:"csrf"`
}

func buildAccountResource(user cloud.User, csrf string, requiresReauthentication bool) accountResourceDTO {
	invitedBy := user.ReferredByCode
	if invitedBy == "" {
		invitedBy = "Direct / legacy account"
	}
	return accountResourceDTO{
		Email: user.Email, UserID: user.ID, ReferralCode: user.ReferralCode, InvitedBy: invitedBy,
		CreatedAt: user.CreatedAt, PasswordChangedAt: user.PasswordChangedAt,
		IsAdmin: cloud.IsAdminEmail(user.Email), RequiresReauthentication: requiresReauthentication, CSRF: csrf,
	}
}

func (s *Server) accountResourceAPI(w http.ResponseWriter, r *http.Request) {
	identity, ok := s.authenticatedAPIIdentity(w, r)
	if !ok {
		return
	}
	user, err := s.Store.UserByID(r.Context(), identity.User.ID)
	if err != nil || user == nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]string{"error": "account_unavailable"})
		return
	}
	webutil.JSON(w, http.StatusOK, buildAccountResource(*user, identity.CSRF, identity.RequiresReauthentication()))
}

package cloudserver

import (
	"net/http"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
)

func accountTime(value int64, fallback string) string {
	if value <= 0 {
		return fallback
	}
	return time.UnixMilli(value).UTC().Format("02 Jan 2006, 15:04 UTC")
}

func accountValue(label, value string) string {
	return `<div class="kv"><div class="kv-key">` + ui.Escape(label) + `</div><div class="kv-value">` + ui.Escape(value) + `</div></div>`
}

func accountInfoCard(user cloud.User) string {
	inviter := user.ReferredByCode
	if inviter == "" {
		inviter = "Direct / legacy account"
	}
	passwordChanged := accountTime(user.PasswordChangedAt, "Not changed since account creation")
	return `<div class="card span6"><div class="section-head"><div><div class="title">Account information</div><div class="label">Identity and membership details stored by CodeLocal Cloud.</div></div></div><div class="divider"></div>` +
		accountValue("Email", user.Email) +
		accountValue("User ID", user.ID) +
		accountValue("Referral code", user.ReferralCode) +
		accountValue("Invited by", inviter) +
		accountValue("Member since", accountTime(user.CreatedAt, "Unknown")) +
		accountValue("Password updated", passwordChanged) + `</div>`
}

func accountPasswordCard(csrf string) string {
	return `<div class="card span6"><div class="section-head"><div><div class="title">Change password</div><div class="label">Changing your password revokes every other signed-in web session.</div></div></div><div class="divider"></div><form class="form" method="post" action="/account/password">` +
		ui.Hidden(map[string]string{"csrf": csrf}) +
		`<div class="field"><label>Current password</label><input class="input" type="password" name="currentPassword" autocomplete="current-password" maxlength="256" required></div>` +
		`<div class="field"><label>New password</label><input class="input" type="password" name="password" autocomplete="new-password" minlength="10" maxlength="256" required></div>` +
		`<div class="field"><label>Confirm new password</label><input class="input" type="password" name="confirmPassword" autocomplete="new-password" minlength="10" maxlength="256" required></div>` +
		`<button class="btn primary" type="submit">Update password</button></form><div class="divider"></div><div class="label">Locked out? The <a href="/forgot-password">forgot password</a> flow sends a 6-digit reset code to your verified email.</div></div>`
}

func (s *Server) mainAccount(w http.ResponseWriter, r *http.Request, identity *webauth.Identity) {
	user, err := s.Store.UserByID(r.Context(), identity.User.ID)
	if err != nil || user == nil {
		http.Error(w, "Unable to load account", http.StatusInternalServerError)
		return
	}
	body := mainFlash(r) + `<div class="grid">` + accountInfoCard(*user) + accountPasswordCard(identity.CSRF) + `</div>`
	writeHTML(w, ui.DashboardPage(ui.DashboardOptions{Title: "Account", Active: "account", Email: user.Email, CSRF: identity.CSRF, Subtitle: "Manage your CodeLocal identity and password security.", Body: body, IsAdmin: cloud.IsAdminEmail(user.Email)}))
}

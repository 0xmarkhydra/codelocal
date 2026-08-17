package webauth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/mailer"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
	"github.com/redis/go-redis/v9"
)

const (
	passwordResetTTL            = 10 * time.Minute
	passwordResetMaxAttempts    = 5
	passwordResetDailySendLimit = 2
	passwordResetDailyWindow    = 24 * time.Hour
)

var (
	passwordResetTokenRE = regexp.MustCompile(`^[A-Za-z0-9_-]{24,128}$`)
	passwordResetCodeRE  = regexp.MustCompile(`^[0-9]{6}$`)
)

type pendingPasswordReset struct {
	UserID    string `json:"userId"`
	Email     string `json:"email"`
	CodeHash  string `json:"codeHash"`
	Attempts  int    `json:"attempts"`
	CreatedAt int64  `json:"createdAt"`
}

func passwordResetKey(token string) string { return "codelocal:password-reset:" + token }

func passwordResetCodeHash(token, code string) string {
	secret := os.Getenv("MCP_AUTH_SECRET")
	return cloud.HashSecret(secret + "\x00password-reset-otp-v1\x00" + token + "\x00" + code)
}

func (m *Manager) savePasswordReset(ctx context.Context, token string, pending pendingPasswordReset, ttl time.Duration) error {
	raw, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return m.Store.Redis.Set(ctx, passwordResetKey(token), raw, ttl).Err()
}

func (m *Manager) loadPasswordReset(ctx context.Context, token string) (pendingPasswordReset, time.Duration, bool, error) {
	if !passwordResetTokenRE.MatchString(token) {
		return pendingPasswordReset{}, 0, false, nil
	}
	key := passwordResetKey(token)
	raw, err := m.Store.Redis.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return pendingPasswordReset{}, 0, false, nil
	}
	if err != nil {
		return pendingPasswordReset{}, 0, false, err
	}
	var pending pendingPasswordReset
	if json.Unmarshal(raw, &pending) != nil {
		_ = m.Store.Redis.Del(ctx, key).Err()
		return pendingPasswordReset{}, 0, false, nil
	}
	ttl, err := m.Store.Redis.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		_ = m.Store.Redis.Del(ctx, key).Err()
		return pendingPasswordReset{}, 0, false, err
	}
	return pending, ttl, true, nil
}

func (m *Manager) sendPasswordResetCode(ctx context.Context, pending pendingPasswordReset, token, code string) error {
	client, err := mailer.FromEnv()
	if err != nil {
		return err
	}
	resetURL := m.PublicBaseURL + "/reset-password?token=" + url.QueryEscape(token)
	subject := "Reset your CodeLocal password"
	text := "Use code " + code + " to reset your CodeLocal password. It expires in 10 minutes. Open " + resetURL + " to continue. If you did not request this, ignore this email."
	html := `<div style="font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;max-width:520px;margin:auto;padding:32px">` +
		`<h2 style="margin:0 0 12px">Reset your CodeLocal password</h2>` +
		`<p style="color:#555;line-height:1.6">Enter this 6-digit code to choose a new password.</p>` +
		`<div style="font-size:34px;font-weight:700;letter-spacing:8px;margin:28px 0">` + code + `</div>` +
		`<p style="margin:0 0 22px"><a href="` + ui.Escape(resetURL) + `" style="display:inline-block;padding:11px 16px;border-radius:10px;background:#246bfd;color:#fff;text-decoration:none;font-weight:650">Reset password</a></p>` +
		`<p style="color:#777;font-size:14px;line-height:1.6">This code expires in 10 minutes. If you did not request a password reset, you can ignore this email.</p></div>`
	key := "password-reset-" + token + "-" + passwordResetCodeHash(token, code)[:16]
	return client.Send(ctx, mailer.Message{To: pending.Email, Subject: subject, HTML: html, Text: text}, key)
}

func passwordResetNotice() string {
	return ui.Page("Check your email", "If a CodeLocal account exists for that email, we sent password reset instructions.", `<div class="stack"><div class="row"><div class="row-title">Open the email from CodeLocal</div><div class="row-meta">The reset code expires after 10 minutes.</div></div><div class="actions"><a class="btn" href="/login">Back to sign in</a></div></div>`)
}

func passwordResetRetryLabel(retry int) string {
	if retry <= 0 {
		return "later"
	}
	d := time.Duration(retry) * time.Second
	if d >= time.Hour {
		n := int((d + time.Hour - 1) / time.Hour)
		unit := "hours"
		if n == 1 {
			unit = "hour"
		}
		return fmt.Sprintf("in about %d %s", n, unit)
	}
	n := int((d + time.Minute - 1) / time.Minute)
	unit := "minutes"
	if n == 1 {
		unit = "minute"
	}
	return fmt.Sprintf("in about %d %s", n, unit)
}

func passwordResetDailyLimitPage(retry int) string {
	copy := "You can request at most 2 password reset emails in 24 hours. Try again " + passwordResetRetryLabel(retry) + "."
	return ui.Page("Reset email limit reached", copy, `<div class="actions"><a class="btn" href="/login">Back to sign in</a></div>`)
}

func passwordResetNetworkLimitPage(retry int) string {
	copy := "Too many password reset requests came from this network. Try again " + passwordResetRetryLabel(retry) + "."
	return ui.Page("Please wait before trying again", copy, `<div class="actions"><a class="btn" href="/login">Back to sign in</a></div>`)
}

func writePasswordResetRateLimit(w http.ResponseWriter, html string) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(html))
}

func (m *Manager) forgotPasswordForm(csrf, errorMessage string) string {
	alert := ""
	if errorMessage != "" {
		alert = `<div class="alert">` + ui.Escape(errorMessage) + `</div>`
	}
	body := alert + `<form class="form" method="post" action="/forgot-password"><input type="hidden" name="csrf" value="` + ui.Escape(csrf) + `"><div class="field"><label>Email</label><input class="input" type="email" name="email" autocomplete="email" inputmode="email" autocapitalize="none" spellcheck="false" maxlength="254" required></div><button class="btn primary" type="submit">Send reset instructions</button></form><div class="auth-switch"><a href="/login">Back to sign in</a></div>`
	return ui.Page("Forgot your password?", "Enter the email used for your CodeLocal account.", body)
}

func (m *Manager) forgotPasswordGet(w http.ResponseWriter, r *http.Request) {
	csrf := m.EnsureCSRF(w, r)
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(m.forgotPasswordForm(csrf, "")))
}

func (m *Manager) forgotPasswordPost(w http.ResponseWriter, r *http.Request) {
	csrf := m.EnsureCSRF(w, r)
	if !m.VerifyCSRF(r) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(m.forgotPasswordForm(csrf, "Security token expired. Please try again.")))
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if !validEmail(email) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(m.forgotPasswordForm(csrf, "Enter a valid email address.")))
		return
	}
	user, err := m.Store.UserByEmail(r.Context(), email)
	if err != nil {
		http.Error(w, "Unable to start password reset", http.StatusInternalServerError)
		return
	}
	if user != nil {
		m.startPasswordReset(r.Context(), *user)
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write([]byte(passwordResetNotice()))
}

func (m *Manager) startPasswordReset(ctx context.Context, user cloud.User) {
	code, err := newSignupCode()
	if err != nil {
		return
	}
	token := randomURL(32)
	pending := pendingPasswordReset{UserID: user.ID, Email: user.Email, CodeHash: passwordResetCodeHash(token, code), CreatedAt: time.Now().UnixMilli()}
	if m.savePasswordReset(ctx, token, pending, passwordResetTTL) != nil {
		return
	}
	if m.sendPasswordResetCode(ctx, pending, token, code) != nil {
		_ = m.Store.Redis.Del(ctx, passwordResetKey(token)).Err()
		m.Store.Audit(cloud.AuditEvent{UserID: user.ID, Event: "auth.password_reset_delivery_failed"})
		return
	}
	m.Store.Audit(cloud.AuditEvent{UserID: user.ID, Event: "auth.password_reset_requested"})
}

func (m *Manager) resetPasswordForm(csrf, token, email, errorMessage string) string {
	alert := ""
	if errorMessage != "" {
		alert = `<div class="alert">` + ui.Escape(errorMessage) + `</div>`
	}
	body := alert + `<form class="form" method="post" action="/reset-password"><input type="hidden" name="csrf" value="` + ui.Escape(csrf) + `"><input type="hidden" name="token" value="` + ui.Escape(token) + `"><div class="field"><label>Reset code</label><input class="input mono" type="text" name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}" minlength="6" maxlength="6" required></div><div class="hint">We sent a 6-digit code to ` + ui.Escape(maskEmail(email)) + `.</div><div class="field"><label>New password</label><input class="input" type="password" name="password" autocomplete="new-password" minlength="10" maxlength="256" required></div><div class="field"><label>Confirm new password</label><input class="input" type="password" name="confirmPassword" autocomplete="new-password" minlength="10" maxlength="256" required></div><button class="btn primary" type="submit">Reset password</button></form>`
	return ui.Page("Choose a new password", "The reset code expires after 10 minutes.", body)
}

func resetExpiredPage() string {
	return ui.Page("Reset link expired", "That password reset request is no longer valid.", `<div class="actions"><a class="btn primary" href="/forgot-password">Request a new reset</a><a class="btn" href="/login">Back to sign in</a></div>`)
}

func (m *Manager) resetPasswordGet(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	pending, _, ok, err := m.loadPasswordReset(r.Context(), token)
	if err != nil {
		http.Error(w, "Unable to load password reset", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(resetExpiredPage()))
		return
	}
	csrf := m.EnsureCSRF(w, r)
	_, _ = w.Write([]byte(m.resetPasswordForm(csrf, token, pending.Email, "")))
}

func validResetCode(token, code, expected string) bool {
	if !passwordResetCodeRE.MatchString(code) {
		return false
	}
	a, b := []byte(passwordResetCodeHash(token, code)), []byte(expected)
	return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}

func (m *Manager) rejectResetAttempt(w http.ResponseWriter, r *http.Request, csrf, token string, pending pendingPasswordReset, ttl time.Duration) {
	pending.Attempts++
	if pending.Attempts >= passwordResetMaxAttempts {
		_ = m.Store.Redis.Del(r.Context(), passwordResetKey(token)).Err()
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(resetExpiredPage()))
		return
	}
	_ = m.savePasswordReset(r.Context(), token, pending, ttl)
	w.WriteHeader(http.StatusBadRequest)
	_, _ = w.Write([]byte(m.resetPasswordForm(csrf, token, pending.Email, "The reset code is incorrect.")))
}

func (m *Manager) resetPasswordPost(w http.ResponseWriter, r *http.Request) {
	csrf := m.EnsureCSRF(w, r)
	if !m.VerifyCSRF(r) {
		http.Error(w, "Invalid security token.", http.StatusForbidden)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	pending, ttl, ok, err := m.loadPasswordReset(r.Context(), token)
	if err != nil {
		http.Error(w, "Unable to load password reset", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(resetExpiredPage()))
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	if !validResetCode(token, code, pending.CodeHash) {
		m.rejectResetAttempt(w, r, csrf, token, pending, ttl)
		return
	}
	password := r.FormValue("password")
	if password != r.FormValue("confirmPassword") {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(m.resetPasswordForm(csrf, token, pending.Email, "Passwords do not match.")))
		return
	}
	hash, salt, err := HashPassword(password)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(m.resetPasswordForm(csrf, token, pending.Email, err.Error())))
		return
	}
	user, err := m.Store.UserByID(r.Context(), pending.UserID)
	if err != nil || user == nil {
		http.Error(w, "Unable to reset password", http.StatusInternalServerError)
		return
	}
	if VerifyPassword(password, user.PasswordSalt, user.PasswordHash) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(m.resetPasswordForm(csrf, token, pending.Email, "Choose a password you have not already been using.")))
		return
	}
	changedAt := time.Now().UnixMilli()
	if err := m.Store.UpdateUserPassword(r.Context(), user.ID, hash, salt, changedAt); err != nil {
		http.Error(w, "Unable to reset password", http.StatusInternalServerError)
		return
	}
	_ = m.Store.Redis.Del(r.Context(), passwordResetKey(token)).Err()
	m.setCookie(w, SessionCookie, "", -1, true)
	m.Store.Audit(cloud.AuditEvent{UserID: user.ID, Event: "auth.password_reset_completed"})
	_, _ = w.Write([]byte(ui.Page("Password reset", "Your CodeLocal password has been updated. Existing signed-in sessions were revoked.", `<div class="actions"><a class="btn primary" href="/login">Sign in</a></div>`)))
}

func (m *Manager) changePasswordPost(w http.ResponseWriter, r *http.Request) {
	identity, err := m.Identity(r)
	if err != nil || identity == nil {
		http.Redirect(w, r, "/login?next=%2Fdashboard%2Faccount", http.StatusFound)
		return
	}
	if !m.VerifyCSRF(r) {
		http.Redirect(w, r, "/dashboard/account?error="+url.QueryEscape("Security token expired. Please try again."), http.StatusSeeOther)
		return
	}
	current := r.FormValue("currentPassword")
	password := r.FormValue("password")
	if !VerifyPassword(current, identity.User.PasswordSalt, identity.User.PasswordHash) {
		http.Redirect(w, r, "/dashboard/account?error="+url.QueryEscape("Current password is incorrect."), http.StatusSeeOther)
		return
	}
	if password != r.FormValue("confirmPassword") {
		http.Redirect(w, r, "/dashboard/account?error="+url.QueryEscape("New passwords do not match."), http.StatusSeeOther)
		return
	}
	if VerifyPassword(password, identity.User.PasswordSalt, identity.User.PasswordHash) {
		http.Redirect(w, r, "/dashboard/account?error="+url.QueryEscape("Choose a different password."), http.StatusSeeOther)
		return
	}
	hash, salt, err := HashPassword(password)
	if err != nil {
		http.Redirect(w, r, "/dashboard/account?error="+url.QueryEscape(err.Error()), http.StatusSeeOther)
		return
	}
	changedAt := time.Now().UnixMilli()
	if err := m.Store.UpdateUserPassword(r.Context(), identity.User.ID, hash, salt, changedAt); err != nil {
		http.Error(w, "Unable to change password", http.StatusInternalServerError)
		return
	}
	newSessionID, err := m.createBrowserSession(w, r, identity.User.ID, identity.CSRF)
	if err != nil {
		http.Error(w, "Password changed, but unable to refresh session", http.StatusInternalServerError)
		return
	}
	_ = m.Store.DeleteSession(r.Context(), identity.SessionID)
	m.setCookie(w, SessionCookie, newSessionID, int(m.SessionTTL.Seconds()), true)
	m.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "auth.password_changed"})
	http.Redirect(w, r, "/dashboard/account?ok="+url.QueryEscape("Password updated. Other signed-in sessions were revoked."), http.StatusSeeOther)
}

func (m *Manager) registerPasswordReset(mux *http.ServeMux) {
	mux.HandleFunc("GET /forgot-password", m.forgotPasswordGet)
	forgot := webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-password-reset-account", Limit: passwordResetDailySendLimit, Window: passwordResetDailyWindow, Subject: func(r *http.Request) string {
		_ = r.ParseForm()
		return strings.ToLower(strings.TrimSpace(r.Form.Get("email")))
	}, OnLimit: func(w http.ResponseWriter, _ *http.Request, retry int) {
		writePasswordResetRateLimit(w, passwordResetDailyLimitPage(retry))
	}}, http.HandlerFunc(m.forgotPasswordPost))
	mux.Handle("POST /forgot-password", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-password-reset-ip", Limit: 20, Window: time.Hour, OnLimit: func(w http.ResponseWriter, _ *http.Request, retry int) {
		writePasswordResetRateLimit(w, passwordResetNetworkLimitPage(retry))
	}}, forgot))
	mux.HandleFunc("GET /reset-password", m.resetPasswordGet)
	reset := webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-password-reset-token", Limit: 12, Window: 10 * time.Minute, Subject: func(r *http.Request) string {
		_ = r.ParseForm()
		return strings.TrimSpace(r.Form.Get("token"))
	}}, http.HandlerFunc(m.resetPasswordPost))
	mux.Handle("POST /reset-password", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-password-reset-verify-ip", Limit: 60, Window: 10 * time.Minute}, reset))
}

func (m *Manager) registerAccountSecurity(mux *http.ServeMux) {
	handler := m.Require(http.HandlerFunc(m.changePasswordPost))
	mux.Handle("POST /account/password", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-password-change", Limit: 8, Window: 10 * time.Minute}, handler))
}

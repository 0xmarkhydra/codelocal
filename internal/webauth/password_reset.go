package webauth

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"html"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/mailer"
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
	UserID   string `json:"userId"`
	Email    string `json:"email"`
	CodeHash string `json:"codeHash"`
}

func passwordResetKey(token string) string             { return "codelocal:password-reset:" + token }
func passwordResetFinalizeLockKey(token string) string { return passwordResetKey(token) + ":finalize" }
func passwordResetAttemptKey(token string) string      { return passwordResetKey(token) + ":attempts" }

func (m *Manager) incrementPasswordResetAttempts(ctx context.Context, token string, ttl time.Duration) (int64, error) {
	if !passwordResetTokenRE.MatchString(token) || ttl <= 0 {
		return 0, nil
	}
	const script = `local count=redis.call('INCR',KEYS[1]); redis.call('PEXPIRE',KEYS[1],ARGV[1]); return count`
	return m.Store.Redis.Eval(ctx, script, []string{passwordResetAttemptKey(token)}, ttl.Milliseconds()).Int64()
}

func (m *Manager) acquirePasswordResetFinalizeLock(ctx context.Context, token string, ttl time.Duration) (bool, error) {
	if !passwordResetTokenRE.MatchString(token) || ttl <= 0 {
		return false, nil
	}
	return m.Store.Redis.SetNX(ctx, passwordResetFinalizeLockKey(token), "1", ttl).Result()
}

func (m *Manager) releasePasswordResetFinalizeLock(ctx context.Context, token string) {
	_ = m.Store.Redis.Del(ctx, passwordResetFinalizeLockKey(token)).Err()
}

func passwordResetCodeHash(token, code string) string {
	return cloud.HashSecret(os.Getenv("MCP_AUTH_SECRET") + "\x00password-reset-otp-v1\x00" + token + "\x00" + code)
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
	htmlBody := `<div style="font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;max-width:520px;margin:auto;padding:32px">` +
		`<h2 style="margin:0 0 12px">Reset your CodeLocal password</h2>` +
		`<p style="color:#555;line-height:1.6">Enter this 6-digit code to choose a new password.</p>` +
		`<div style="font-size:34px;font-weight:700;letter-spacing:8px;margin:28px 0">` + code + `</div>` +
		`<p style="margin:0 0 22px"><a href="` + html.EscapeString(resetURL) + `" style="display:inline-block;padding:11px 16px;border-radius:10px;background:#246bfd;color:#fff;text-decoration:none;font-weight:650">Reset password</a></p>` +
		`<p style="color:#777;font-size:14px;line-height:1.6">This code expires in 10 minutes. If you did not request a password reset, you can ignore this email.</p></div>`
	key := "password-reset-" + token + "-" + passwordResetCodeHash(token, code)[:16]
	return client.Send(ctx, mailer.Message{To: pending.Email, Subject: subject, HTML: htmlBody, Text: text}, key)
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

func (m *Manager) forgotPasswordPost(w http.ResponseWriter, r *http.Request) {
	m.EnsureCSRF(w, r)
	if !m.VerifyCSRF(r) {
		authUIRedirect(w, r, "/forgot-password", url.Values{"error": {"Security token expired. Please try again."}})
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	if !validEmail(email) {
		authUIRedirect(w, r, "/forgot-password", url.Values{"error": {"Enter a valid email address."}})
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
	authUIRedirect(w, r, "/forgot-password", url.Values{"sent": {"1"}})
}

func (m *Manager) startPasswordReset(ctx context.Context, user cloud.User) {
	code, err := newSignupCode()
	if err != nil {
		return
	}
	token := randomURL(32)
	pending := pendingPasswordReset{UserID: user.ID, Email: user.Email, CodeHash: passwordResetCodeHash(token, code)}
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

func validResetCode(token, code, expected string) bool {
	if !passwordResetCodeRE.MatchString(code) {
		return false
	}
	a, b := []byte(passwordResetCodeHash(token, code)), []byte(expected)
	return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}

func (m *Manager) rejectResetAttempt(w http.ResponseWriter, r *http.Request, token string, ttl time.Duration) {
	attempts, err := m.incrementPasswordResetAttempts(r.Context(), token, ttl)
	if err != nil {
		http.Error(w, "Unable to verify reset code", http.StatusServiceUnavailable)
		return
	}
	if attempts >= passwordResetMaxAttempts {
		_ = m.Store.Redis.Del(r.Context(), passwordResetKey(token), passwordResetAttemptKey(token), passwordResetFinalizeLockKey(token)).Err()
		authUIRedirect(w, r, "/reset-password", url.Values{"token": {token}, "expired": {"1"}})
		return
	}
	authUIRedirect(w, r, "/reset-password", url.Values{"token": {token}, "error": {"The reset code is incorrect."}})
}

func (m *Manager) resetPasswordPost(w http.ResponseWriter, r *http.Request) {
	m.EnsureCSRF(w, r)
	token := strings.TrimSpace(r.FormValue("token"))
	if !m.VerifyCSRF(r) {
		authUIRedirect(w, r, "/reset-password", url.Values{"token": {token}, "error": {"Invalid security token. Please try again."}})
		return
	}
	pending, ttl, ok, err := m.loadPasswordReset(r.Context(), token)
	if err != nil {
		http.Error(w, "Unable to load password reset", http.StatusInternalServerError)
		return
	}
	if !ok {
		authUIRedirect(w, r, "/reset-password", url.Values{"token": {token}, "expired": {"1"}})
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	if !validResetCode(token, code, pending.CodeHash) {
		m.rejectResetAttempt(w, r, token, ttl)
		return
	}
	password := r.FormValue("password")
	if password != r.FormValue("confirmPassword") {
		authUIRedirect(w, r, "/reset-password", url.Values{"token": {token}, "error": {"Passwords do not match."}})
		return
	}
	hash, salt, err := HashPassword(password)
	if err != nil {
		authUIRedirect(w, r, "/reset-password", url.Values{"token": {token}, "error": {err.Error()}})
		return
	}
	user, err := m.Store.UserByID(r.Context(), pending.UserID)
	if err != nil || user == nil {
		http.Error(w, "Unable to reset password", http.StatusInternalServerError)
		return
	}
	if VerifyPassword(password, user.PasswordSalt, user.PasswordHash) {
		authUIRedirect(w, r, "/reset-password", url.Values{"token": {token}, "error": {"Choose a password you have not already been using."}})
		return
	}
	locked, err := m.acquirePasswordResetFinalizeLock(r.Context(), token, ttl)
	if err != nil {
		http.Error(w, "Unable to reset password", http.StatusInternalServerError)
		return
	}
	if !locked {
		authUIRedirect(w, r, "/reset-password", url.Values{"token": {token}, "error": {"This password reset is already being finalized. If it does not complete, request a new reset code."}})
		return
	}
	changedAt := time.Now().UnixMilli()
	if err := m.Store.UpdateUserPassword(r.Context(), user.ID, hash, salt, changedAt); err != nil {
		m.releasePasswordResetFinalizeLock(r.Context(), token)
		http.Error(w, "Unable to reset password", http.StatusInternalServerError)
		return
	}
	_ = m.Store.Redis.Del(r.Context(), passwordResetKey(token), passwordResetAttemptKey(token), passwordResetFinalizeLockKey(token)).Err()
	m.setCookie(w, SessionCookie, "", -1, true)
	m.Store.Audit(cloud.AuditEvent{UserID: user.ID, Event: "auth.password_reset_completed"})
	authUIRedirect(w, r, "/reset-password", url.Values{"done": {"1"}})
}

func passwordChangeReturnPath(_ *http.Request) string { return "/dashboard/account" }

func passwordChangeRedirect(path, key, message string) string {
	return path + "?" + key + "=" + url.QueryEscape(message)
}

func (m *Manager) changePasswordPost(w http.ResponseWriter, r *http.Request) {
	returnPath := passwordChangeReturnPath(r)
	identity, err := m.Identity(r)
	if err != nil || identity == nil {
		http.Redirect(w, r, "/login?next="+url.QueryEscape(returnPath), http.StatusFound)
		return
	}
	if !m.RequireFreshSecurityContext(w, r, identity, returnPath) {
		return
	}
	if !m.VerifyCSRF(r) {
		http.Redirect(w, r, passwordChangeRedirect(returnPath, "error", "Security token expired. Please try again."), http.StatusSeeOther)
		return
	}
	current := r.FormValue("currentPassword")
	password := r.FormValue("password")
	if !VerifyPassword(current, identity.User.PasswordSalt, identity.User.PasswordHash) {
		http.Redirect(w, r, passwordChangeRedirect(returnPath, "error", "Current password is incorrect."), http.StatusSeeOther)
		return
	}
	if password != r.FormValue("confirmPassword") {
		http.Redirect(w, r, passwordChangeRedirect(returnPath, "error", "New passwords do not match."), http.StatusSeeOther)
		return
	}
	if VerifyPassword(password, identity.User.PasswordSalt, identity.User.PasswordHash) {
		http.Redirect(w, r, passwordChangeRedirect(returnPath, "error", "Choose a different password."), http.StatusSeeOther)
		return
	}
	hash, salt, err := HashPassword(password)
	if err != nil {
		http.Redirect(w, r, passwordChangeRedirect(returnPath, "error", err.Error()), http.StatusSeeOther)
		return
	}
	changedAt := time.Now().UnixMilli()
	securityVersion, err := m.Store.UpdateUserPasswordAndVersion(r.Context(), identity.User.ID, hash, salt, changedAt)
	if err != nil {
		http.Error(w, "Unable to change password", http.StatusInternalServerError)
		return
	}
	newSessionID, err := m.createBrowserSession(w, r, identity.User.ID, identity.CSRF, securityVersion)
	if err != nil {
		http.Error(w, "Password changed, but unable to refresh session", http.StatusInternalServerError)
		return
	}
	_ = m.Store.DeleteSession(r.Context(), identity.SessionID)
	m.setCookie(w, SessionCookie, newSessionID, int(m.SessionTTL.Seconds()), true)
	m.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "auth.password_changed"})
	http.Redirect(w, r, passwordChangeRedirect(returnPath, "ok", "Password updated. Other signed-in sessions were revoked."), http.StatusSeeOther)
}

func (m *Manager) passwordResetContext(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	pending, _, ok, err := m.loadPasswordReset(r.Context(), token)
	w.Header().Set("Cache-Control", "no-store")
	if err != nil {
		webutil.JSON(w, http.StatusServiceUnavailable, map[string]any{"valid": false, "error": "reset_unavailable"})
		return
	}
	if !ok {
		webutil.JSON(w, http.StatusGone, map[string]any{"valid": false})
		return
	}
	webutil.JSON(w, http.StatusOK, map[string]any{"valid": true, "emailMasked": maskEmail(pending.Email)})
}

func (m *Manager) registerPasswordReset(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/v1/auth/password-reset", m.passwordResetContext)
	forgot := webutil.RateLimit(m.Store, webutil.RateLimitOptions{
		Scope: "auth-password-reset-account", Limit: passwordResetDailySendLimit, Window: passwordResetDailyWindow,
		Subject: func(r *http.Request) string {
			_ = r.ParseForm()
			return strings.ToLower(strings.TrimSpace(r.Form.Get("email")))
		},
		OnLimit: func(w http.ResponseWriter, r *http.Request, retry int) {
			authUIRedirect(w, r, "/forgot-password", url.Values{"error": {"Reset email limit reached. Try again " + passwordResetRetryLabel(retry) + "."}})
		},
	}, http.HandlerFunc(m.forgotPasswordPost))
	mux.Handle("POST /forgot-password", webutil.RateLimit(m.Store, webutil.RateLimitOptions{
		Scope: "auth-password-reset-ip", Limit: 20, Window: time.Hour,
		OnLimit: func(w http.ResponseWriter, r *http.Request, retry int) {
			authUIRedirect(w, r, "/forgot-password", url.Values{"error": {"Too many reset requests from this network. Try again " + passwordResetRetryLabel(retry) + "."}})
		},
	}, forgot))
	resetLimit := func(w http.ResponseWriter, r *http.Request, _ int) {
		authUIRedirect(w, r, "/reset-password", url.Values{"token": {strings.TrimSpace(r.FormValue("token"))}, "error": {"Too many reset attempts. Request a new code or try again later."}})
	}
	reset := webutil.RateLimit(m.Store, webutil.RateLimitOptions{
		Scope: "auth-password-reset-token", Limit: 12, Window: 10 * time.Minute,
		Subject: func(r *http.Request) string {
			_ = r.ParseForm()
			return strings.TrimSpace(r.Form.Get("token"))
		}, OnLimit: resetLimit,
	}, http.HandlerFunc(m.resetPasswordPost))
	mux.Handle("POST /reset-password", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-password-reset-verify-ip", Limit: 60, Window: 10 * time.Minute, OnLimit: resetLimit}, reset))
}

func (m *Manager) registerAccountSecurity(mux *http.ServeMux) {
	handler := m.Require(http.HandlerFunc(m.changePasswordPost))
	mux.Handle("POST /account/password", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-password-change", Limit: 8, Window: 10 * time.Minute}, handler))
}

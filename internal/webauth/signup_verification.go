package webauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"fmt"
	"math/big"
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
	signupVerificationTTL         = 10 * time.Minute
	signupVerificationMaxAttempts = 5
)

var (
	signupTokenRE = regexp.MustCompile(`^[A-Za-z0-9_-]{24,128}$`)
	signupCodeRE  = regexp.MustCompile(`^[0-9]{6}$`)
)

type pendingSignup struct {
	Email          string `json:"email"`
	PasswordHash   string `json:"passwordHash"`
	PasswordSalt   string `json:"passwordSalt"`
	ReferralCode   string `json:"referralCode"`
	Next           string `json:"next"`
	CodeHash       string `json:"codeHash"`
	Attempts       int    `json:"attempts"`
	VerificationAt int64  `json:"verificationAt"`
}

func signupPendingKey(token string) string { return "codelocal:signup-verification:" + token }

func newSignupCode() (string, error) {
	value, err := rand.Int(rand.Reader, big.NewInt(1_000_000))
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%06d", value.Int64()), nil
}

func signupCodeHash(token, code string) string {
	return cloud.HashSecret(os.Getenv("MCP_AUTH_SECRET") + "\x00signup-email-otp-v1\x00" + token + "\x00" + code)
}

func maskEmail(email string) string {
	parts := strings.SplitN(email, "@", 2)
	if len(parts) != 2 || parts[0] == "" {
		return email
	}
	local := parts[0]
	if len(local) == 1 {
		local = local[:1] + "***"
	} else {
		local = local[:1] + strings.Repeat("*", minInt(len(local)-1, 6))
	}
	return local + "@" + parts[1]
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (m *Manager) savePendingSignup(ctx context.Context, token string, pending pendingSignup, ttl time.Duration) error {
	raw, err := json.Marshal(pending)
	if err != nil {
		return err
	}
	return m.Store.Redis.Set(ctx, signupPendingKey(token), raw, ttl).Err()
}

func (m *Manager) loadPendingSignup(ctx context.Context, token string) (pendingSignup, time.Duration, bool, error) {
	if !signupTokenRE.MatchString(token) {
		return pendingSignup{}, 0, false, nil
	}
	key := signupPendingKey(token)
	raw, err := m.Store.Redis.Get(ctx, key).Bytes()
	if errors.Is(err, redis.Nil) {
		return pendingSignup{}, 0, false, nil
	}
	if err != nil {
		return pendingSignup{}, 0, false, err
	}
	var pending pendingSignup
	if err := json.Unmarshal(raw, &pending); err != nil {
		_ = m.Store.Redis.Del(ctx, key).Err()
		return pendingSignup{}, 0, false, nil
	}
	ttl, err := m.Store.Redis.TTL(ctx, key).Result()
	if err != nil || ttl <= 0 {
		_ = m.Store.Redis.Del(ctx, key).Err()
		return pendingSignup{}, 0, false, err
	}
	return pending, ttl, true, nil
}

func (m *Manager) sendSignupCode(ctx context.Context, email, token, code string) error {
	client, err := mailer.FromEnv()
	if err != nil {
		return err
	}
	subject := "Your CodeLocal verification code"
	text := "Your CodeLocal verification code is " + code + ". It expires in 10 minutes. If you did not request this, you can ignore this email."
	html := `<div style="font-family:-apple-system,BlinkMacSystemFont,Segoe UI,sans-serif;max-width:520px;margin:auto;padding:32px">` +
		`<h2 style="margin:0 0 12px">Verify your CodeLocal email</h2>` +
		`<p style="color:#555">Enter this 6-digit code to finish creating your account.</p>` +
		`<div style="font-size:34px;font-weight:700;letter-spacing:8px;margin:28px 0">` + code + `</div>` +
		`<p style="color:#777;font-size:14px">This code expires in 10 minutes. If you did not request it, you can ignore this email.</p></div>`
	key := "signup-otp-" + token + "-" + signupCodeHash(token, code)[:16]
	return client.Send(ctx, mailer.Message{To: email, Subject: subject, HTML: html, Text: text}, key)
}

func (m *Manager) referralAllowed(ctx context.Context, email, referralCode string) (bool, error) {
	referralCode = cloud.NormalizeReferralCode(referralCode)
	if !cloud.ValidReferralCode(referralCode) {
		return false, nil
	}
	if referralCode == "MMON" {
		return strings.EqualFold(strings.TrimSpace(email), cloud.PrimaryAdminEmail()), nil
	}
	inviter, err := m.Store.UserByReferralCode(ctx, referralCode)
	return inviter != nil, err
}

func signupErrorMessage(err error) string {
	if err == nil {
		return ""
	}
	switch err.Error() {
	case "EMAIL_ALREADY_REGISTERED":
		return "An account with this email already exists."
	case "REFERRAL_REQUIRED", "REFERRAL_INVALID":
		return "Referral code is invalid. Ask an existing CodeLocal member for a valid invite code."
	case "REFERRAL_CODE_GENERATION_FAILED":
		return "Unable to allocate a referral code. Please try again."
	default:
		return "Unable to create account. Please try again."
	}
}

func (m *Manager) signupStart(w http.ResponseWriter, r *http.Request) {
	csrf := m.EnsureCSRF(w, r)
	next := webutil.SafeNext(r.FormValue("next"))
	referralCode := cloud.NormalizeReferralCode(r.FormValue("referralCode"))
	if !m.VerifyCSRF(r) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = w.Write([]byte(m.form("signup", csrf, next, "Security token expired. Please try again.", referralCode)))
		return
	}
	email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
	password := r.FormValue("password")
	if !validEmail(email) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(m.form("signup", csrf, next, "Enter a valid email address.", referralCode)))
		return
	}
	if existing, err := m.Store.UserByEmail(r.Context(), email); err != nil {
		http.Error(w, "Unable to check account", http.StatusInternalServerError)
		return
	} else if existing != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(m.form("signup", csrf, next, "An account with this email already exists.", referralCode)))
		return
	}
	allowed, err := m.referralAllowed(r.Context(), email, referralCode)
	if err != nil {
		http.Error(w, "Unable to validate referral code", http.StatusInternalServerError)
		return
	}
	if !allowed {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(m.form("signup", csrf, next, "Referral code is invalid. Ask an existing CodeLocal member for a valid invite code.", referralCode)))
		return
	}
	hash, salt, err := HashPassword(password)
	if err != nil {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(m.form("signup", csrf, next, err.Error(), referralCode)))
		return
	}
	code, err := newSignupCode()
	if err != nil {
		http.Error(w, "Unable to create verification code", http.StatusInternalServerError)
		return
	}
	token := randomURL(32)
	pending := pendingSignup{Email: email, PasswordHash: hash, PasswordSalt: salt, ReferralCode: referralCode, Next: next, CodeHash: signupCodeHash(token, code), VerificationAt: time.Now().UnixMilli()}
	if err := m.savePendingSignup(r.Context(), token, pending, signupVerificationTTL); err != nil {
		http.Error(w, "Unable to start email verification", http.StatusInternalServerError)
		return
	}
	if err := m.sendSignupCode(r.Context(), email, token, code); err != nil {
		_ = m.Store.Redis.Del(r.Context(), signupPendingKey(token)).Err()
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(m.form("signup", csrf, next, "We could not send the verification email. Please try again.", referralCode)))
		return
	}
	http.Redirect(w, r, "/signup/verify?token="+url.QueryEscape(token), http.StatusSeeOther)
}

func (m *Manager) verificationForm(csrf, token, email, errorMessage string) string {
	alert := ""
	if errorMessage != "" {
		alert = `<div class="alert">` + ui.Escape(errorMessage) + `</div>`
	}
	body := alert + `<form class="form" method="post" action="/signup/verify">` +
		`<input type="hidden" name="csrf" value="` + ui.Escape(csrf) + `">` +
		`<input type="hidden" name="token" value="` + ui.Escape(token) + `">` +
		`<div class="field"><label>Verification code</label><input class="input mono" type="text" name="code" inputmode="numeric" autocomplete="one-time-code" pattern="[0-9]{6}" minlength="6" maxlength="6" required></div>` +
		`<div class="hint">We sent a 6-digit code to ` + ui.Escape(maskEmail(email)) + `. The code expires in 10 minutes.</div>` +
		`<button class="btn primary" type="submit">Verify email</button></form>` +
		`<div class="auth-switch"><a href="/signup">Use a different email</a></div>`
	return ui.Page("Check your email", "Verify your email before your CodeLocal account is created.", body)
}

func (m *Manager) verificationExpiredPage() string {
	return ui.Page("Verification expired", "That verification request is no longer valid.", `<div class="stack"><div class="row"><div class="row-title">Start again</div><div class="row-meta">Verification codes expire after 10 minutes for security.</div></div><div class="actions"><a class="btn primary" href="/signup">Back to sign up</a></div></div>`)
}

func (m *Manager) signupVerifyGet(w http.ResponseWriter, r *http.Request) {
	token := strings.TrimSpace(r.URL.Query().Get("token"))
	pending, _, ok, err := m.loadPendingSignup(r.Context(), token)
	if err != nil {
		http.Error(w, "Unable to load verification", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(m.verificationExpiredPage()))
		return
	}
	csrf := m.EnsureCSRF(w, r)
	_, _ = w.Write([]byte(m.verificationForm(csrf, token, pending.Email, "")))
}

func (m *Manager) signupVerifyPost(w http.ResponseWriter, r *http.Request) {
	csrf := m.EnsureCSRF(w, r)
	if !m.VerifyCSRF(r) {
		http.Error(w, "Invalid security token.", http.StatusForbidden)
		return
	}
	token := strings.TrimSpace(r.FormValue("token"))
	pending, ttl, ok, err := m.loadPendingSignup(r.Context(), token)
	if err != nil {
		http.Error(w, "Unable to load verification", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if !ok {
		w.WriteHeader(http.StatusGone)
		_, _ = w.Write([]byte(m.verificationExpiredPage()))
		return
	}
	code := strings.TrimSpace(r.FormValue("code"))
	valid := signupCodeRE.MatchString(code)
	if valid {
		a, b := []byte(signupCodeHash(token, code)), []byte(pending.CodeHash)
		valid = len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
	}
	if !valid {
		pending.Attempts++
		if pending.Attempts >= signupVerificationMaxAttempts {
			_ = m.Store.Redis.Del(r.Context(), signupPendingKey(token)).Err()
			w.WriteHeader(http.StatusTooManyRequests)
			_, _ = w.Write([]byte(m.verificationExpiredPage()))
			return
		}
		if ttl > 0 {
			if err := m.savePendingSignup(r.Context(), token, pending, ttl); err != nil {
				http.Error(w, "Unable to update verification attempt", http.StatusInternalServerError)
				return
			}
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(m.verificationForm(csrf, token, pending.Email, "The verification code is incorrect.")))
		return
	}
	user, err := m.Store.CreateUser(r.Context(), pending.Email, pending.PasswordHash, pending.PasswordSalt, pending.ReferralCode)
	if err != nil {
		_ = m.Store.Redis.Del(r.Context(), signupPendingKey(token)).Err()
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(ui.Page("Unable to create account", signupErrorMessage(err), `<div class="actions"><a class="btn primary" href="/signup">Back to sign up</a><a class="btn" href="/login">Sign in</a></div>`)))
		return
	}
	_ = m.Store.Redis.Del(r.Context(), signupPendingKey(token)).Err()
	sessionID, err := m.createBrowserSession(w, r, user.ID, csrf)
	if err != nil {
		http.Error(w, "Unable to create session", http.StatusInternalServerError)
		return
	}
	m.setCookie(w, SessionCookie, sessionID, int(m.SessionTTL.Seconds()), true)
	m.Store.Audit(cloud.AuditEvent{UserID: user.ID, Event: "auth.register", Detail: map[string]any{"emailVerified": true}})
	http.Redirect(w, r, pending.Next, http.StatusSeeOther)
}

func (m *Manager) registerSignupVerification(mux *http.ServeMux) {
	mux.HandleFunc("GET /signup/verify", m.signupVerifyGet)
	var verify http.Handler = http.HandlerFunc(m.signupVerifyPost)
	verify = webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-signup-verify-token", Limit: 12, Window: 10 * time.Minute, Subject: func(r *http.Request) string {
		_ = r.ParseForm()
		return strings.TrimSpace(r.Form.Get("token"))
	}}, verify)
	mux.Handle("POST /signup/verify", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-signup-verify-ip", Limit: 60, Window: 10 * time.Minute}, verify))
}

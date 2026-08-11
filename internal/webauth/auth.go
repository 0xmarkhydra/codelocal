package webauth

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/ui"
	"github.com/0xmarkhydra/codelocal/internal/webutil"
	"golang.org/x/crypto/scrypt"
)

const (
	SessionCookie = "codelocal_session"
	CSRFCookie    = "codelocal_csrf"
)

var emailRE = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type Identity struct {
	User      cloud.User
	SessionID string
	CSRF      string
}

type contextKey string

const identityKey contextKey = "codelocal-web-identity"

type Manager struct {
	Store         *cloud.Store
	PublicBaseURL string
	SessionTTL    time.Duration
}

func New(store *cloud.Store, publicBaseURL string) *Manager {
	return &Manager{Store: store, PublicBaseURL: strings.TrimRight(publicBaseURL, "/"), SessionTTL: 30 * 24 * time.Hour}
}

func randomURL(bytes int) string {
	buf := make([]byte, bytes)
	_, _ = rand.Read(buf)
	return base64.RawURLEncoding.EncodeToString(buf)
}
func (m *Manager) secureCookies() bool {
	return strings.HasPrefix(m.PublicBaseURL, "https://") || os.Getenv("NODE_ENV") == "production" || os.Getenv("APP_ENV") == "production"
}
func (m *Manager) setCookie(w http.ResponseWriter, name, value string, maxAge int, httpOnly bool) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: "/", HttpOnly: httpOnly, Secure: m.secureCookies(), SameSite: http.SameSiteLaxMode, MaxAge: maxAge})
}

func (m *Manager) EnsureCSRF(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(CSRFCookie); err == nil && regexp.MustCompile(`^[A-Za-z0-9_-]{24,128}$`).MatchString(cookie.Value) {
		return cookie.Value
	}
	value := randomURL(32)
	m.setCookie(w, CSRFCookie, value, int(m.SessionTTL.Seconds()), true)
	return value
}

func (m *Manager) VerifyCSRF(r *http.Request) bool {
	cookie, err := r.Cookie(CSRFCookie)
	if err != nil {
		return false
	}
	if err := r.ParseForm(); err != nil {
		return false
	}
	body := r.Form.Get("csrf")
	a, b := []byte(cookie.Value), []byte(body)
	return len(a) > 0 && len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}

func derivePassword(password, salt string) (string, error) {
	key, err := scrypt.Key([]byte(password), []byte(salt), 1<<14, 8, 1, 64)
	if err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(key), nil
}
func HashPassword(password string) (hash, salt string, err error) {
	if len(password) < 10 {
		return "", "", errText("Password must be at least 10 characters.")
	}
	if len(password) > 256 {
		return "", "", errText("Password is too long.")
	}
	salt = randomURL(24)
	hash, err = derivePassword(password, salt)
	return
}

type errText string

func (e errText) Error() string { return string(e) }
func VerifyPassword(password, salt, expected string) bool {
	if len(password) > 256 {
		return false
	}
	actual, err := derivePassword(password, salt)
	if err != nil {
		return false
	}
	a, b := []byte(actual), []byte(expected)
	return len(a) == len(b) && subtle.ConstantTimeCompare(a, b) == 1
}

func (m *Manager) Identity(r *http.Request) (*Identity, error) {
	if cached, ok := r.Context().Value(identityKey).(*Identity); ok {
		return cached, nil
	}
	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		return nil, nil
	}
	userID, csrf, ok, err := m.Store.ReadSession(r.Context(), cookie.Value)
	if err != nil || !ok {
		return nil, err
	}
	user, err := m.Store.UserByID(r.Context(), userID)
	if err != nil || user == nil {
		return nil, err
	}
	return &Identity{User: *user, SessionID: cookie.Value, CSRF: csrf}, nil
}
func WithIdentity(r *http.Request, identity *Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey, identity))
}

func (m *Manager) Require(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		identity, err := m.Identity(r)
		if err != nil {
			http.Error(w, "Internal error", 500)
			return
		}
		if identity == nil {
			nextPath := webutil.SafeNext(r.URL.RequestURI())
			http.Redirect(w, r, "/login?next="+url.QueryEscape(nextPath), http.StatusFound)
			return
		}
		next.ServeHTTP(w, WithIdentity(r, identity))
	})
}

func validEmail(value string) bool { return len(value) <= 254 && emailRE.MatchString(value) }
func (m *Manager) form(mode, csrf, next, errorMessage string, referralCodes ...string) string {
	signup := mode == "signup"
	referralCode := ""
	if len(referralCodes) > 0 {
		referralCode = cloud.NormalizeReferralCode(referralCodes[0])
	}
	title := "Welcome back"
	subtitle := "Sign in to manage your devices, workspaces and MCP extensions."
	button := "Sign in"
	autocomplete := "current-password"
	switcher := `New to CodeLocal? <a href="/signup?next=` + url.QueryEscape(next) + `">Create an account</a>`
	if signup {
		title = "Create your CodeLocal account"
		subtitle = "One account connects ChatGPT to your development machines."
		button = "Create account"
		autocomplete = "new-password"
		switcher = `Already have an account? <a href="/login?next=` + url.QueryEscape(next) + `">Sign in</a>`
	}
	alert := ""
	if errorMessage != "" {
		alert = `<div class="alert">` + ui.Escape(errorMessage) + `</div>`
	}
	referralField := ""
	if signup {
		referralField = `<div class="field"><label>Referral code</label><input class="input mono" type="text" name="referralCode" value="` + ui.Escape(referralCode) + `" autocomplete="off" minlength="4" maxlength="6" pattern="[A-Za-z0-9]+" required></div><div class="hint">Enter the 6-character invite code from an existing CodeLocal member.</div>`
	}
	body := alert + `<form class="form" method="post" action="/` + mode + `"><input type="hidden" name="csrf" value="` + ui.Escape(csrf) + `"><input type="hidden" name="next" value="` + ui.Escape(next) + `"><div class="field"><label>Email</label><input class="input" type="email" name="email" autocomplete="email" inputmode="email" autocapitalize="none" spellcheck="false" maxlength="254" required></div><div class="field"><label>Password</label><input class="input" type="password" name="password" autocomplete="` + autocomplete + `" minlength="10" maxlength="256" required></div>` + referralField + `<button class="btn primary" type="submit">` + button + `</button></form><div class="auth-switch">` + switcher + `</div>`
	return ui.Page(title, subtitle, body)
}

func (m *Manager) Register(mux *http.ServeMux) {
	mux.HandleFunc("GET /login", func(w http.ResponseWriter, r *http.Request) {
		identity, _ := m.Identity(r)
		if identity != nil {
			http.Redirect(w, r, webutil.SafeNext(r.URL.Query().Get("next")), http.StatusFound)
			return
		}
		csrf := m.EnsureCSRF(w, r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(m.form("login", csrf, webutil.SafeNext(r.URL.Query().Get("next")), "")))
	})
	login := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csrf := m.EnsureCSRF(w, r)
		next := webutil.SafeNext(r.FormValue("next"))
		if !m.VerifyCSRF(r) {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(m.form("login", csrf, next, "Security token expired. Please try again.")))
			return
		}
		email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
		password := r.FormValue("password")
		user, _ := m.Store.UserByEmail(r.Context(), email)
		valid := false
		if user != nil {
			valid = VerifyPassword(password, user.PasswordSalt, user.PasswordHash)
		} else if len(password) <= 256 {
			_, _ = derivePassword(password, "codelocal-login-timing-padding-v1")
		}
		if user == nil || !valid {
			m.Store.Audit(cloud.AuditEvent{UserID: func() string {
				if user != nil {
					return user.ID
				}
				return ""
			}(), Event: "auth.login_failed", Detail: map[string]any{"email": email}})
			w.WriteHeader(401)
			_, _ = w.Write([]byte(m.form("login", csrf, next, "Email or password is incorrect.")))
			return
		}
		sessionID, err := m.Store.CreateSession(r.Context(), user.ID, csrf, m.SessionTTL)
		if err != nil {
			http.Error(w, "Unable to create session", 500)
			return
		}
		m.setCookie(w, SessionCookie, sessionID, int(m.SessionTTL.Seconds()), true)
		m.Store.Audit(cloud.AuditEvent{UserID: user.ID, Event: "auth.login", Detail: map[string]any{"method": "password"}})
		http.Redirect(w, r, next, http.StatusSeeOther)
	})
	mux.Handle("POST /login", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-login-ip", Limit: 40, Window: 10 * time.Minute}, webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-login-account", Limit: 12, Window: 10 * time.Minute, Subject: func(r *http.Request) string {
		_ = r.ParseForm()
		return strings.ToLower(strings.TrimSpace(r.Form.Get("email")))
	}}, login)))
	mux.HandleFunc("GET /register", func(w http.ResponseWriter, r *http.Request) {
		params := url.Values{}
		params.Set("next", webutil.SafeNext(r.URL.Query().Get("next")))
		if ref := cloud.NormalizeReferralCode(r.URL.Query().Get("ref")); cloud.ValidReferralCode(ref) {
			params.Set("ref", ref)
		}
		http.Redirect(w, r, "/signup?"+params.Encode(), http.StatusFound)
	})
	mux.HandleFunc("GET /signup", func(w http.ResponseWriter, r *http.Request) {
		identity, _ := m.Identity(r)
		if identity != nil {
			http.Redirect(w, r, webutil.SafeNext(r.URL.Query().Get("next")), http.StatusFound)
			return
		}
		csrf := m.EnsureCSRF(w, r)
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte(m.form("signup", csrf, webutil.SafeNext(r.URL.Query().Get("next")), "", r.URL.Query().Get("ref"))))
	})
	signup := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		csrf := m.EnsureCSRF(w, r)
		next := webutil.SafeNext(r.FormValue("next"))
		referralCode := cloud.NormalizeReferralCode(r.FormValue("referralCode"))
		if !m.VerifyCSRF(r) {
			w.WriteHeader(403)
			_, _ = w.Write([]byte(m.form("signup", csrf, next, "Security token expired. Please try again.", referralCode)))
			return
		}
		email := strings.ToLower(strings.TrimSpace(r.FormValue("email")))
		password := r.FormValue("password")
		if !validEmail(email) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(m.form("signup", csrf, next, "Enter a valid email address.", referralCode)))
			return
		}
		if !cloud.ValidReferralCode(referralCode) {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(m.form("signup", csrf, next, "Enter a valid referral code from an existing member.", referralCode)))
			return
		}
		hash, salt, err := HashPassword(password)
		if err != nil {
			w.WriteHeader(400)
			_, _ = w.Write([]byte(m.form("signup", csrf, next, err.Error(), referralCode)))
			return
		}
		user, err := m.Store.CreateUser(r.Context(), email, hash, salt, referralCode)
		if err != nil {
			message := err.Error()
			switch message {
			case "EMAIL_ALREADY_REGISTERED":
				message = "An account with this email already exists."
			case "REFERRAL_REQUIRED", "REFERRAL_INVALID":
				message = "Referral code is invalid. Ask an existing CodeLocal member for a valid invite code."
			case "REFERRAL_CODE_GENERATION_FAILED":
				message = "Unable to allocate a referral code. Please try again."
			}
			w.WriteHeader(400)
			_, _ = w.Write([]byte(m.form("signup", csrf, next, message, referralCode)))
			return
		}
		sessionID, err := m.Store.CreateSession(r.Context(), user.ID, csrf, m.SessionTTL)
		if err != nil {
			http.Error(w, "Unable to create session", 500)
			return
		}
		m.setCookie(w, SessionCookie, sessionID, int(m.SessionTTL.Seconds()), true)
		m.Store.Audit(cloud.AuditEvent{UserID: user.ID, Event: "auth.register"})
		http.Redirect(w, r, next, http.StatusSeeOther)
	})
	mux.Handle("POST /signup", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-signup-ip", Limit: 20, Window: time.Hour}, webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-signup-account", Limit: 4, Window: time.Hour, Subject: func(r *http.Request) string {
		_ = r.ParseForm()
		return strings.ToLower(strings.TrimSpace(r.Form.Get("email")))
	}}, signup)))
	mux.HandleFunc("POST /logout", func(w http.ResponseWriter, r *http.Request) {
		identity, _ := m.Identity(r)
		if !m.VerifyCSRF(r) {
			http.Error(w, "Invalid security token.", 403)
			return
		}
		next := webutil.SafeNext(r.FormValue("next"))
		if identity != nil {
			_ = m.Store.DeleteSession(r.Context(), identity.SessionID)
			m.Store.Audit(cloud.AuditEvent{UserID: identity.User.ID, Event: "auth.logout"})
		}
		m.setCookie(w, SessionCookie, "", -1, true)
		if next != "/dashboard" {
			http.Redirect(w, r, "/login?next="+url.QueryEscape(next), http.StatusSeeOther)
			return
		}
		http.Redirect(w, r, "/login", http.StatusSeeOther)
	})
}

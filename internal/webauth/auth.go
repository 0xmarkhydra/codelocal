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
	"github.com/0xmarkhydra/codelocal/internal/webutil"
	"golang.org/x/crypto/scrypt"
)

const (
	SessionCookie        = "codelocal_session"
	CSRFCookie           = "codelocal_csrf"
	SecurityDeviceCookie = "codelocal_device"
)

var emailRE = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

type Identity struct {
	User      cloud.User
	SessionID string
	CSRF      string
	RiskUntil int64
}

func (i *Identity) RequiresReauthentication() bool {
	return i != nil && i.RiskUntil > time.Now().UnixMilli()
}

func (m *Manager) RequireFreshSecurityContext(w http.ResponseWriter, r *http.Request, identity *Identity, nextPaths ...string) bool {
	if identity == nil || !identity.RequiresReauthentication() {
		return true
	}
	_ = m.Store.DeleteSession(r.Context(), identity.SessionID)
	m.setCookie(w, SessionCookie, "", -1, true)
	next := r.URL.RequestURI()
	if len(nextPaths) > 0 && strings.TrimSpace(nextPaths[0]) != "" {
		next = nextPaths[0]
	}
	http.Redirect(w, r, "/login?next="+url.QueryEscape(webutil.SafeNext(next)), http.StatusSeeOther)
	return false
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

func (m *Manager) EnsureSecurityDevice(w http.ResponseWriter, r *http.Request) string {
	if cookie, err := r.Cookie(SecurityDeviceCookie); err == nil && regexp.MustCompile(`^[A-Za-z0-9_-]{24,128}$`).MatchString(cookie.Value) {
		return cookie.Value
	}
	value := randomURL(32)
	m.setCookie(w, SecurityDeviceCookie, value, int((365 * 24 * time.Hour).Seconds()), true)
	return value
}

func (m *Manager) createBrowserSession(w http.ResponseWriter, r *http.Request, userID, csrf string, securityVersion int64) (string, error) {
	deviceToken := m.EnsureSecurityDevice(w, r)
	signal := webutil.RequestSecuritySignal(r, deviceToken)
	return m.Store.CreateSessionWithSecurityVersion(r.Context(), userID, csrf, m.SessionTTL, securityVersion, signal)
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

func sessionInvalidForUser(state cloud.SessionState, user cloud.User) bool {
	if state.SecurityVersion > 0 && user.SecurityVersion > 0 {
		return state.SecurityVersion != user.SecurityVersion
	}
	return user.PasswordChangedAt > 0 && state.CreatedAt < user.PasswordChangedAt
}

func (m *Manager) Identity(r *http.Request) (*Identity, error) {
	if cached, ok := r.Context().Value(identityKey).(*Identity); ok {
		return cached, nil
	}
	cookie, err := r.Cookie(SessionCookie)
	if err != nil {
		return nil, nil
	}
	state, ok, err := m.Store.ReadSessionState(r.Context(), cookie.Value)
	if err != nil || !ok {
		return nil, err
	}
	user, err := m.Store.UserByID(r.Context(), state.UserID)
	if err != nil || user == nil {
		return nil, err
	}
	if sessionInvalidForUser(state, *user) {
		_ = m.Store.DeleteSession(r.Context(), cookie.Value)
		return nil, nil
	}
	if state.Security != nil {
		deviceToken := ""
		if deviceCookie, cookieErr := r.Cookie(SecurityDeviceCookie); cookieErr == nil {
			deviceToken = deviceCookie.Value
		}
		signal := webutil.RequestSecuritySignal(r, deviceToken)
		decision := cloud.EvaluateSecuritySignals(*state.Security, signal)
		if decision.HighRisk {
			_ = m.Store.DeleteSession(r.Context(), cookie.Value)
			return nil, nil
		}
		if decision.AgentChanged || decision.NetworkChanged || decision.HashUpgrade {
			state.Security = &signal
			if decision.AgentChanged || decision.NetworkChanged {
				state.RiskUntil = time.Now().Add(10 * time.Minute).UnixMilli()
			}
			_ = m.Store.UpdateSessionState(r.Context(), cookie.Value, state)
		}
	}
	riskUntil := state.RiskUntil
	if state.Security == nil {
		riskUntil = time.Now().Add(10 * time.Minute).UnixMilli()
	}
	return &Identity{User: *user, SessionID: cookie.Value, CSRF: state.CSRF, RiskUntil: riskUntil}, nil
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

func authUIRedirect(w http.ResponseWriter, r *http.Request, path string, values url.Values) {
	if values == nil {
		values = url.Values{}
	}
	target := path
	if encoded := values.Encode(); encoded != "" {
		target += "?" + encoded
	}
	http.Redirect(w, r, target, http.StatusSeeOther)
}

func loginRateSubject(r *http.Request) string {
	_ = r.ParseForm()
	return strings.ToLower(strings.TrimSpace(r.Form.Get("email")))
}

func (m *Manager) loginPost(w http.ResponseWriter, r *http.Request) {
	csrf := m.EnsureCSRF(w, r)
	next := webutil.SafeNext(r.FormValue("next"))
	if !m.VerifyCSRF(r) {
		authUIRedirect(w, r, "/login", url.Values{"next": {next}, "error": {"Security token expired. Please try again."}})
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
		userID := ""
		if user != nil {
			userID = user.ID
		}
		m.Store.Audit(cloud.AuditEvent{UserID: userID, Event: "auth.login_failed", Detail: map[string]any{"email": email}})
		authUIRedirect(w, r, "/login", url.Values{"next": {next}, "error": {"Email or password is incorrect."}})
		return
	}
	sessionID, err := m.createBrowserSession(w, r, user.ID, csrf, user.SecurityVersion)
	if err != nil {
		http.Error(w, "Unable to create session", http.StatusInternalServerError)
		return
	}
	m.setCookie(w, SessionCookie, sessionID, int(m.SessionTTL.Seconds()), true)
	m.Store.Audit(cloud.AuditEvent{UserID: user.ID, Event: "auth.login", Detail: map[string]any{"method": "password"}})
	http.Redirect(w, r, next, http.StatusSeeOther)
}

func (m *Manager) logoutPost(w http.ResponseWriter, r *http.Request) {
	identity, _ := m.Identity(r)
	if !m.VerifyCSRF(r) {
		http.Error(w, "Invalid security token.", http.StatusForbidden)
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
}

func (m *Manager) Register(mux *http.ServeMux) {
	loginLimit := func(w http.ResponseWriter, r *http.Request, _ int) {
		authUIRedirect(w, r, "/login", url.Values{"next": {webutil.SafeNext(r.FormValue("next"))}, "error": {"Too many sign-in attempts. Please try again later."}})
	}
	login := webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-login-account", Limit: 12, Window: 10 * time.Minute, Subject: loginRateSubject, OnLimit: loginLimit}, http.HandlerFunc(m.loginPost))
	mux.Handle("POST /login", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-login-ip", Limit: 40, Window: 10 * time.Minute, OnLimit: loginLimit}, login))
	signupLimit := func(w http.ResponseWriter, r *http.Request, _ int) {
		authUIRedirect(w, r, "/signup", url.Values{"next": {webutil.SafeNext(r.FormValue("next"))}, "ref": {cloud.NormalizeReferralCode(r.FormValue("referralCode"))}, "error": {"Too many sign-up attempts. Please try again later."}})
	}
	signup := webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-signup-account", Limit: 4, Window: time.Hour, Subject: loginRateSubject, OnLimit: signupLimit}, http.HandlerFunc(m.signupStart))
	mux.Handle("POST /signup", webutil.RateLimit(m.Store, webutil.RateLimitOptions{Scope: "auth-signup-ip", Limit: 20, Window: time.Hour, OnLimit: signupLimit}, signup))
	m.registerSignupVerification(mux)
	m.registerPasswordReset(mux)
	m.registerAccountSecurity(mux)
	mux.HandleFunc("POST /logout", m.logoutPost)
}

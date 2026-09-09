// Package penpot authenticates CodeLocal's hosted Penpot MCP boundary without
// modifying upstream Penpot. Upstream HTTP and WS ports must stay private.
package penpot

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"sync"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/oauth"
	"github.com/0xmarkhydra/codelocal/internal/plugins"
	"github.com/coder/websocket"
)

type Adapter struct {
	Auth    *oauth.Server
	backend *url.URL
	mcp     *url.URL
	ws      *url.URL
	client  *http.Client
}

func New(auth *oauth.Server, backend, mcp, ws string) (*Adapter, error) {
	targets := make([]*url.URL, 0, 3)
	for index, value := range []string{backend, mcp, ws} {
		target, err := url.Parse(value)
		validScheme := target != nil && (target.Scheme == "http" || target.Scheme == "https")
		if index == 2 {
			validScheme = target != nil && (target.Scheme == "ws" || target.Scheme == "wss")
		}
		if err != nil || !validScheme || target.Host == "" || target.User != nil ||
			target.RawQuery != "" || target.Fragment != "" || (target.Path != "" && target.Path != "/") {
			return nil, errors.New("invalid Penpot private upstream configuration")
		}
		targets = append(targets, target)
	}
	if auth == nil {
		return nil, errors.New("Penpot auth is unavailable")
	}
	return &Adapter{Auth: auth, backend: targets[0], mcp: targets[1], ws: targets[2],
		client: &http.Client{Timeout: 5 * time.Second, CheckRedirect: func(*http.Request, []*http.Request) error {
			return errors.New("Penpot credential redirects are forbidden")
		}}}, nil
}

type profile struct {
	ID      string `json:"id"`
	Email   string `json:"email"`
	Active  bool   `json:"is-active"`
	Blocked bool   `json:"is-blocked"`
}

// Native Penpot validates both the JWE signature and DB expiry/revocation.
// get-profile is public and returns an anonymous zero UUID on invalid tokens.
func (a *Adapter) nativeProfile(ctx context.Context, token string) (profile, error) {
	if token == "" || len(token) > 8192 || strings.TrimSpace(token) != token || strings.ContainsAny(token, "\r\n\x00") {
		return profile{}, errors.New("invalid Penpot credential")
	}
	target := *a.backend
	target.Path = "/api/rpc/command/get-profile"
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, target.String(), strings.NewReader("{}"))
	if err != nil {
		return profile{}, errors.New("Penpot authentication unavailable")
	}
	request.Header.Set("Authorization", "Bearer "+token)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	response, err := a.client.Do(request)
	if err != nil {
		return profile{}, errors.New("Penpot authentication unavailable")
	}
	defer response.Body.Close()
	var result profile
	if response.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(response.Body, 64<<10)).Decode(&result) != nil ||
		result.ID == "" || result.ID == "00000000-0000-0000-0000-000000000000" ||
		!result.Active || result.Blocked || strings.TrimSpace(result.Email) == "" {
		return profile{}, errors.New("invalid or expired Penpot credential")
	}
	return result, nil
}

func (a *Adapter) ValidateOwner(ctx context.Context, token, email string) error {
	result, err := a.nativeProfile(ctx, token)
	if err != nil {
		return err
	}
	// Hosted Penpot is SSO-only; verified CodeLocal email is the OIDC profile
	// projection. No client-supplied profile ID or email is trusted.
	if strings.TrimSpace(email) == "" || !strings.EqualFold(strings.TrimSpace(result.Email), strings.TrimSpace(email)) {
		return errors.New("Penpot credential belongs to another account")
	}
	return nil
}

func (a *Adapter) authenticate(ctx context.Context, token string) (oauth.PenpotGrant, error) {
	grant, err := a.Auth.VerifyPenpotGrant(ctx, token)
	if err != nil {
		return oauth.PenpotGrant{}, err
	}
	user, err := a.Auth.Store.UserByID(ctx, grant.Subject)
	if err != nil || user == nil {
		return oauth.PenpotGrant{}, errors.New("Penpot account unavailable")
	}
	workspaces, err := a.Auth.Store.ListWorkspaceRecords(ctx, grant.Subject)
	if err != nil {
		return oauth.PenpotGrant{}, errors.New("Penpot workspace unavailable")
	}
	authorized := false
	for _, workspace := range workspaces {
		if workspace.DeviceID == grant.DeviceID && workspace.WorkspaceID == grant.WorkspaceID {
			authorized, _ = workspace.Capabilities["authorized"].(bool)
			break
		}
	}
	if !authorized {
		return oauth.PenpotGrant{}, errors.New("Penpot workspace revoked")
	}
	secrets, err := a.Auth.Store.MaterializeRuntimeSecrets(ctx, grant.Subject, grant.DeviceID, grant.WorkspaceID)
	if err != nil || subtle.ConstantTimeCompare([]byte(secrets[plugins.ManagedCredentialReference("penpot")]), []byte(grant.UserToken)) != 1 {
		return oauth.PenpotGrant{}, errors.New("Penpot connection revoked")
	}
	if err := a.ValidateOwner(ctx, grant.UserToken, user.Email); err != nil {
		return oauth.PenpotGrant{}, err
	}
	return grant, nil
}

func reject(w http.ResponseWriter, status int) {
	w.Header().Set("Cache-Control", "no-store")
	http.Error(w, http.StatusText(status), status)
}

func (a *Adapter) ServeMCP(w http.ResponseWriter, r *http.Request) {
	if a == nil {
		reject(w, http.StatusServiceUnavailable)
		return
	}
	if r.Method != http.MethodPost && r.Method != http.MethodGet && r.Method != http.MethodDelete {
		reject(w, http.StatusMethodNotAllowed)
		return
	}
	// Public credential query forms and duplicate headers are never accepted.
	headers := r.Header.Values("Authorization")
	if r.URL.RawQuery != "" || len(headers) != 1 || !strings.HasPrefix(headers[0], "Bearer ") || len(headers[0]) > 16384 {
		reject(w, http.StatusUnauthorized)
		return
	}
	grant, err := a.authenticate(r.Context(), strings.TrimPrefix(headers[0], "Bearer "))
	if err != nil {
		reject(w, http.StatusUnauthorized)
		return
	}
	a.serveAuthenticatedMCP(w, r, grant)
}

func (a *Adapter) serveAuthenticatedMCP(w http.ResponseWriter, r *http.Request, grant oauth.PenpotGrant) {
	sessionID := ""
	if values := r.Header.Values("Mcp-Session-Id"); len(values) > 0 {
		if len(values) != 1 {
			reject(w, http.StatusUnauthorized)
			return
		}
		var err error
		sessionID, err = a.Auth.OpenPenpotSession(grant, values[0])
		if err != nil {
			reject(w, http.StatusUnauthorized)
			return
		}
	}
	ctx, cancel := context.WithDeadline(r.Context(), time.Unix(grant.ExpiresAt, 0))
	defer cancel()
	r.Body = http.MaxBytesReader(w, r.Body, 4<<20)
	proxy := &httputil.ReverseProxy{
		Rewrite: func(request *httputil.ProxyRequest) {
			request.SetURL(a.mcp)
			request.Out.URL.Path, request.Out.URL.RawPath = "/mcp", ""
			request.Out.URL.RawQuery = url.Values{"userToken": {grant.UserToken}}.Encode()
			// Allow only MCP protocol headers. Never forward browser/device auth.
			request.Out.Header = http.Header{}
			for _, name := range []string{"Accept", "Content-Type", "Mcp-Protocol-Version", "Last-Event-Id"} {
				if value := request.In.Header.Get(name); value != "" {
					request.Out.Header.Set(name, value)
				}
			}
			if sessionID != "" {
				request.Out.Header.Set("Mcp-Session-Id", sessionID)
			}
		},
		ModifyResponse: func(response *http.Response) error {
			response.Header.Del("Set-Cookie")
			response.Header.Del("Location")
			response.Header.Set("Cache-Control", "no-store")
			if id := response.Header.Get("Mcp-Session-Id"); id != "" {
				sealed, err := a.Auth.SealPenpotSession(grant, id)
				if err != nil {
					return err
				}
				response.Header.Set("Mcp-Session-Id", sealed)
			}
			return nil
		},
		ErrorHandler:  func(w http.ResponseWriter, _ *http.Request, _ error) { reject(w, http.StatusBadGateway) },
		FlushInterval: -1,
	}
	proxy.ServeHTTP(w, r.WithContext(ctx))
}

func (a *Adapter) ServeWS(w http.ResponseWriter, r *http.Request) {
	if a == nil {
		reject(w, http.StatusServiceUnavailable)
		return
	}
	query, err := url.ParseQuery(r.URL.RawQuery)
	if err != nil || len(query) != 1 || len(query["userToken"]) != 1 ||
		r.Header.Get("Origin") != "https://design.codelocal.cloud" {
		reject(w, http.StatusUnauthorized)
		return
	}
	token := query.Get("userToken")
	owner, err := a.nativeProfile(r.Context(), token)
	if err != nil {
		reject(w, http.StatusUnauthorized)
		return
	}
	target := *a.ws
	target.RawQuery = url.Values{"userToken": {token}}.Encode()
	upstream, _, err := websocket.Dial(r.Context(), target.String(), &websocket.DialOptions{HTTPClient: a.client})
	if err != nil {
		reject(w, http.StatusBadGateway)
		return
	}
	defer upstream.CloseNow()
	conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"design.codelocal.cloud"}})
	if err != nil {
		return
	}
	defer conn.CloseNow()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	conn.SetReadLimit(16 << 20)
	upstream.SetReadLimit(16 << 20)
	valid := func() bool {
		current, err := a.nativeProfile(ctx, token)
		return err == nil && current.ID == owner.ID
	}
	// No auth cache: validate both directions before forwarding tasks/results.
	// Idle connections are also revalidated so revoked credentials disconnect.
	go func() {
		ticker := time.NewTicker(30 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				if !valid() {
					cancel()
					return
				}
			}
		}
	}()
	var pendingMu sync.Mutex
	pending := map[string]struct{}{}
	acceptMessage := func(data []byte, fromPlugin bool) bool {
		var message struct {
			ID string `json:"id"`
		}
		if json.Unmarshal(data, &message) != nil || message.ID == "" || len(message.ID) > 256 {
			return false
		}
		pendingMu.Lock()
		defer pendingMu.Unlock()
		_, exists := pending[message.ID]
		if fromPlugin {
			if !exists {
				return false
			}
			delete(pending, message.ID)
			return true
		}
		// ponytail: cap unresolved tasks per socket; reconnect on overflow.
		// Upstream owns task timeouts, so a future cancel message can release IDs.
		if exists || len(pending) >= 1024 {
			return false
		}
		pending[message.ID] = struct{}{}
		return true
	}
	copyMessages := func(dst, src *websocket.Conn, fromPlugin bool) {
		defer cancel()
		for {
			kind, data, err := src.Read(ctx)
			if err != nil || !valid() || !acceptMessage(data, fromPlugin) || dst.Write(ctx, kind, data) != nil {
				return
			}
		}
	}
	done := make(chan struct{})
	go func() { defer close(done); copyMessages(upstream, conn, true) }()
	copyMessages(conn, upstream, false)
	<-done
}

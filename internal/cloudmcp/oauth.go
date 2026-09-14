package cloudmcp

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"time"

	"github.com/modelcontextprotocol/go-sdk/oauthex"
	"golang.org/x/oauth2"
)

type OAuthFlow struct {
	Endpoint         string `json:"endpoint"`
	Issuer           string `json:"issuer"`
	RequireIssuer    bool   `json:"requireIssuer"`
	TokenEndpoint    string `json:"tokenEndpoint"`
	ClientID         string `json:"clientId"`
	RedirectURI      string `json:"redirectUri"`
	Verifier         string `json:"verifier"`
	AuthorizationURL string `json:"authorizationUrl"`
}

type OAuthToken struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	RefreshToken string `json:"refresh_token"`
	ExpiresIn    int64  `json:"expires_in"`
}

func DiscoverOAuth(ctx context.Context, client *http.Client, endpoint, redirectURI, metadataClientID, state string) (OAuthFlow, error) {
	var flow OAuthFlow
	endpoint, err := ValidateEndpoint(endpoint)
	if err != nil {
		return flow, err
	}
	request, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-11-25","capabilities":{},"clientInfo":{"name":"codelocal","version":"2"}}}`))
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	response, err := client.Do(request)
	if err != nil {
		return flow, errors.New("MCP authorization discovery failed")
	}
	challenges, parseErr := oauthex.ParseWWWAuthenticate(response.Header.Values("WWW-Authenticate"))
	response.Body.Close()
	if parseErr != nil {
		return flow, errors.New("invalid MCP authorization challenge")
	}
	metadataURL := ""
	scope := ""
	for _, c := range challenges {
		if c.Scheme == "bearer" {
			metadataURL = c.Params["resource_metadata"]
			scope = c.Params["scope"]
			break
		}
	}
	base, _ := url.Parse(endpoint)
	candidates := []string{}
	if metadataURL != "" {
		candidates = append(candidates, metadataURL)
	} else {
		candidates = append(candidates, base.Scheme+"://"+base.Host+"/.well-known/oauth-protected-resource"+base.Path, base.Scheme+"://"+base.Host+"/.well-known/oauth-protected-resource")
	}
	var resource *oauthex.ProtectedResourceMetadata
	for _, candidate := range candidates {
		resource, err = oauthex.GetProtectedResourceMetadata(ctx, candidate, endpoint, client)
		if err == nil {
			break
		}
	}
	if resource == nil || err != nil || len(resource.AuthorizationServers) == 0 {
		return flow, errors.New("MCP server has no usable OAuth metadata")
	}
	if resource.DPOPBoundAccessTokensRequired || resource.TLSClientCertificateBoundAccessTokens {
		return flow, errors.New("this MCP authorization method is not supported")
	}
	// Multiple issuers require an explicit choice, not arbitrary credential routing.
	if len(resource.AuthorizationServers) != 1 {
		return flow, errors.New("multiple OAuth issuers require administrator configuration")
	}
	issuer := resource.AuthorizationServers[0]
	if _, err = ValidateEndpoint(issuer); err != nil {
		return flow, err
	}
	u, _ := url.Parse(issuer)
	origin := u.Scheme + "://" + u.Host
	path := strings.TrimRight(u.Path, "/")
	var meta *oauthex.AuthServerMeta
	for _, candidate := range []string{origin + "/.well-known/oauth-authorization-server" + path, origin + "/.well-known/openid-configuration" + path, strings.TrimRight(issuer, "/") + "/.well-known/openid-configuration"} {
		meta, err = oauthex.GetAuthServerMeta(ctx, candidate, issuer, client)
		if err == nil && meta != nil {
			break
		}
	}
	if meta == nil || err != nil || meta.Issuer != issuer || !slices.Contains(meta.CodeChallengeMethodsSupported, "S256") {
		return flow, errors.New("OAuth metadata must have a matching issuer and PKCE S256")
	}
	for _, endpoint := range []string{meta.AuthorizationEndpoint, meta.TokenEndpoint} {
		if _, err = ValidateEndpoint(endpoint); err != nil {
			return flow, err
		}
	}
	clientID := metadataClientID
	if !meta.ClientIDMetadataDocumentSupported {
		if _, err = ValidateEndpoint(meta.RegistrationEndpoint); err != nil {
			return flow, errors.New("OAuth server requires a pre-registered client")
		}
		registration, err := oauthex.RegisterClient(ctx, meta.RegistrationEndpoint, &oauthex.ClientRegistrationMetadata{RedirectURIs: []string{redirectURI}, TokenEndpointAuthMethod: "none", GrantTypes: []string{"authorization_code", "refresh_token"}, ResponseTypes: []string{"code"}, ClientName: "CodeLocal"}, client)
		if err != nil || registration.ClientID == "" || registration.ClientSecret != "" {
			return flow, errors.New("OAuth public client registration failed")
		}
		clientID = registration.ClientID
	}
	if scope == "" {
		scope = strings.Join(resource.ScopesSupported, " ")
	}
	verifier := oauth2.GenerateVerifier()
	cfg := oauth2.Config{ClientID: clientID, RedirectURL: redirectURI, Scopes: strings.Fields(scope), Endpoint: oauth2.Endpoint{AuthURL: meta.AuthorizationEndpoint, TokenURL: meta.TokenEndpoint}}
	authURL := cfg.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oauth2.SetAuthURLParam("resource", endpoint))
	return OAuthFlow{Endpoint: endpoint, Issuer: issuer, RequireIssuer: meta.AuthorizationResponseIssParameterSupported, TokenEndpoint: meta.TokenEndpoint, ClientID: clientID, RedirectURI: redirectURI, Verifier: verifier, AuthorizationURL: authURL}, nil
}

func ExchangeOAuth(ctx context.Context, client *http.Client, flow OAuthFlow, code, issuer string) (OAuthToken, error) {
	if (flow.RequireIssuer && issuer == "") || (issuer != "" && issuer != flow.Issuer) {
		return OAuthToken{}, errors.New("OAuth issuer mismatch")
	}
	if code == "" || len(code) > 8192 {
		return OAuthToken{}, errors.New("missing or invalid authorization code")
	}
	return tokenRequest(ctx, client, flow.TokenEndpoint, url.Values{"grant_type": {"authorization_code"}, "code": {code}, "client_id": {flow.ClientID}, "redirect_uri": {flow.RedirectURI}, "code_verifier": {flow.Verifier}, "resource": {flow.Endpoint}})
}
func RefreshOAuth(ctx context.Context, client *http.Client, flow OAuthFlow, refresh string) (OAuthToken, error) {
	return tokenRequest(ctx, client, flow.TokenEndpoint, url.Values{"grant_type": {"refresh_token"}, "refresh_token": {refresh}, "client_id": {flow.ClientID}, "resource": {flow.Endpoint}})
}
func tokenRequest(ctx context.Context, client *http.Client, endpoint string, form url.Values) (OAuthToken, error) {
	var token OAuthToken
	if _, err := ValidateEndpoint(endpoint); err != nil {
		return token, err
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")
	response, err := client.Do(req)
	if err != nil {
		return token, errors.New("OAuth token request failed")
	}
	defer response.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(response.Body, 64<<10))
	if err != nil || response.StatusCode != 200 || json.Unmarshal(raw, &token) != nil || !strings.EqualFold(token.TokenType, "Bearer") || token.AccessToken == "" || len(token.AccessToken) > 16384 || strings.ContainsAny(token.AccessToken, "\r\n\x00") {
		return OAuthToken{}, errors.New("OAuth authorization expired or invalid; sign in again")
	}
	return token, nil
}

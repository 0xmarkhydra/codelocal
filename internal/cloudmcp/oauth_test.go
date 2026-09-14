package cloudmcp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"golang.org/x/oauth2"
)

func jsonResponse(body any) *http.Response {
	data, _ := json.Marshal(body)
	return &http.Response{StatusCode: 200, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(data)))}
}

func TestOAuthDiscoveryPKCEIssuerAndResource(t *testing.T) {
	ctx := context.Background()
	seenToken := 0
	metadataAttempts := []string{}
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Authorization") != "" {
			t.Fatal("token sent during discovery")
		}
		switch r.URL.String() {
		case "https://resource.example/mcp":
			return &http.Response{StatusCode: 401, Header: http.Header{"Www-Authenticate": []string{`Bearer resource_metadata="https://resource.example/meta", scope="read write"`}}, Body: io.NopCloser(strings.NewReader(""))}, nil
		case "https://resource.example/meta":
			return jsonResponse(map[string]any{"resource": "https://resource.example/mcp", "authorization_servers": []string{"https://auth.example/tenant"}}), nil
		case "https://auth.example/.well-known/oauth-authorization-server/tenant":
			metadataAttempts = append(metadataAttempts, r.URL.Path)
			return &http.Response{StatusCode: 404, Header: http.Header{}, Body: io.NopCloser(strings.NewReader(""))}, nil
		case "https://auth.example/.well-known/openid-configuration/tenant":
			metadataAttempts = append(metadataAttempts, r.URL.Path)
			return jsonResponse(map[string]any{"issuer": "https://auth.example/tenant", "authorization_endpoint": "https://auth.example/authorize", "token_endpoint": "https://auth.example/token", "code_challenge_methods_supported": []string{"S256"}, "client_id_metadata_document_supported": true, "authorization_response_iss_parameter_supported": true}), nil
		case "https://auth.example/token":
			seenToken++
			if err := r.ParseForm(); err != nil {
				t.Fatal(err)
			}
			if r.PostForm.Get("resource") != "https://resource.example/mcp" || r.PostForm.Get("code_verifier") == "" {
				t.Fatal("missing resource/PKCE")
			}
			return jsonResponse(map[string]any{"access_token": "fresh", "token_type": "Bearer", "refresh_token": "refresh", "expires_in": 3600}), nil
		default:
			t.Fatalf("unexpected endpoint %s", r.URL)
			return nil, nil
		}
	})}
	flow, err := DiscoverOAuth(ctx, client, "https://resource.example/mcp", "https://codelocal.example/callback", "https://codelocal.example/client", "random-state")
	if err != nil {
		t.Fatal(err)
	}
	authorize, _ := url.Parse(flow.AuthorizationURL)
	q := authorize.Query()
	if q.Get("resource") != flow.Endpoint || q.Get("state") != "random-state" || q.Get("code_challenge") != oauth2.S256ChallengeFromVerifier(flow.Verifier) || q.Get("code_challenge_method") != "S256" || q.Get("scope") != "read write" {
		t.Fatal("invalid authorization parameters", q)
	}
	if len(metadataAttempts) != 2 {
		t.Fatal("OIDC fallback not attempted")
	}
	for _, issuer := range []string{"", "https://other.example"} {
		if _, err := ExchangeOAuth(ctx, client, flow, "code", issuer); err == nil {
			t.Fatal("issuer mismatch accepted")
		}
	}
	if seenToken != 0 {
		t.Fatal("authorization code sent before issuer check")
	}
	token, err := ExchangeOAuth(ctx, client, flow, "code", flow.Issuer)
	if err != nil || token.AccessToken != "fresh" || seenToken != 1 {
		t.Fatal("exchange", err)
	}
}

func TestRefreshCarriesResourceAndDoesNotRetry(t *testing.T) {
	calls := 0
	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		calls++
		_ = r.ParseForm()
		if r.PostForm.Get("resource") != "https://resource.example/mcp" || r.PostForm.Get("grant_type") != "refresh_token" {
			t.Fatal("refresh missing resource")
		}
		return &http.Response{StatusCode: 400, Body: io.NopCloser(strings.NewReader(`{"error":"invalid_grant","detail":"secret"}`))}, nil
	})}
	_, err := RefreshOAuth(context.Background(), client, OAuthFlow{Endpoint: "https://resource.example/mcp", TokenEndpoint: "https://auth.example/token", ClientID: "client"}, "refresh")
	if err == nil || calls != 1 || strings.Contains(err.Error(), "secret") {
		t.Fatal("refresh retry or error leakage")
	}
}

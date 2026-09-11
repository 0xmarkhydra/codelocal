package identity

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/deviceauth"
)

const runtimeBootstrapExchangePath = "/api/client/runtime/bootstrap/exchange"

type RuntimeBootstrapConfig struct {
	ServerURL        string
	Token            string
	RuntimeSessionID string
	DeviceID         string
	DeviceName       string
	HTTPClient       *http.Client
}

type runtimeBootstrapExchangeRequest struct {
	Token            string `json:"token"`
	RuntimeSessionID string `json:"runtimeSessionId"`
	DeviceID         string `json:"deviceId"`
	PublicKey        string `json:"publicKey"`
}

type runtimeBootstrapExchangeResponse struct {
	CredentialID     string `json:"credentialId"`
	CredentialSecret string `json:"credentialSecret"`
	DeviceID         string `json:"deviceId"`
	DeviceName       string `json:"deviceName"`
	WorkspaceID      string `json:"workspaceId"`
	WorkspaceKey     string `json:"workspaceKey"`
	RuntimeSessionID string `json:"runtimeSessionId"`
}

// ExchangeRuntimeBootstrap generates the device signing keypair inside the
// runtime environment, then exchanges a one-time bootstrap token for the same
// credential shape used by Local Runtime. The private key never leaves the
// runtime process.
func ExchangeRuntimeBootstrap(ctx context.Context, config RuntimeBootstrapConfig) (Credential, string, string, error) {
	serverURL, err := normalizeRuntimeBootstrapServer(config.ServerURL)
	if err != nil {
		return Credential{}, "", "", err
	}
	token := strings.TrimSpace(config.Token)
	sessionID := strings.TrimSpace(config.RuntimeSessionID)
	deviceID := strings.TrimSpace(config.DeviceID)
	if token == "" || sessionID == "" || deviceID == "" {
		return Credential{}, "", "", errors.New("runtime bootstrap token, session and device are required")
	}
	publicKey, privateKey, err := deviceauth.GenerateKeyPair()
	if err != nil {
		return Credential{}, "", "", fmt.Errorf("generate runtime device keypair: %w", err)
	}
	requestBody := runtimeBootstrapExchangeRequest{
		Token:            token,
		RuntimeSessionID: sessionID,
		DeviceID:         deviceID,
		PublicKey:        publicKey,
	}
	raw, err := json.Marshal(requestBody)
	if err != nil {
		return Credential{}, "", "", err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, serverURL+runtimeBootstrapExchangePath, bytes.NewReader(raw))
	if err != nil {
		return Credential{}, "", "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	client := config.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 15 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return Credential{}, "", "", fmt.Errorf("exchange runtime bootstrap: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return Credential{}, "", "", fmt.Errorf("runtime bootstrap exchange failed (%d)", resp.StatusCode)
	}
	var output runtimeBootstrapExchangeResponse
	if err := json.NewDecoder(resp.Body).Decode(&output); err != nil {
		return Credential{}, "", "", fmt.Errorf("decode runtime bootstrap response: %w", err)
	}
	if output.CredentialID == "" || output.CredentialSecret == "" || output.DeviceID != deviceID || output.RuntimeSessionID != sessionID || output.WorkspaceID == "" {
		return Credential{}, "", "", errors.New("runtime bootstrap exchange returned an invalid binding")
	}
	deviceName := strings.TrimSpace(output.DeviceName)
	if deviceName == "" {
		deviceName = strings.TrimSpace(config.DeviceName)
	}
	credential := Credential{
		CredentialID:     output.CredentialID,
		CredentialSecret: output.CredentialSecret,
		DeviceID:         output.DeviceID,
		DeviceName:       deviceName,
		DevicePublicKey:  publicKey,
		DevicePrivateKey: privateKey,
		ServerURL:        serverURL,
		CreatedAt:        time.Now().UnixMilli(),
	}
	return credential, output.WorkspaceID, output.WorkspaceKey, nil
}

func normalizeRuntimeBootstrapServer(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", errors.New("runtime bootstrap server URL required")
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", errors.New("runtime bootstrap server URL invalid")
	}
	parsed.Path = ""
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.TrimRight(parsed.String(), "/"), nil
}

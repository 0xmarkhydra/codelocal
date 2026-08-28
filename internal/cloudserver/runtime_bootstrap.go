package cloudserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/0xmarkhydra/codelocal/internal/deviceauth"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
)

const maxRuntimeBootstrapBody = 64 << 10

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

// runtimeBootstrapExchange is deliberately unauthenticated by browser/device
// session: possession of the short-lived, one-time bootstrap token is the
// authentication factor. The token is bound to a runtime session/device and is
// atomically consumed before a managed device credential is created.
func (s *Server) runtimeBootstrapExchange(w http.ResponseWriter, r *http.Request) {
	if s == nil || s.Store == nil || s.Store.Redis == nil {
		http.Error(w, "runtime bootstrap unavailable", http.StatusServiceUnavailable)
		return
	}
	var body runtimeBootstrapExchangeRequest
	decoder := json.NewDecoder(http.MaxBytesReader(w, r.Body, maxRuntimeBootstrapBody))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&body); err != nil {
		http.Error(w, "invalid bootstrap request", http.StatusBadRequest)
		return
	}
	body.Token = strings.TrimSpace(body.Token)
	body.RuntimeSessionID = strings.TrimSpace(body.RuntimeSessionID)
	body.DeviceID = strings.TrimSpace(body.DeviceID)
	body.PublicKey = strings.TrimSpace(body.PublicKey)
	if body.Token == "" || body.RuntimeSessionID == "" || body.DeviceID == "" || !deviceauth.ValidPublicKey(body.PublicKey) {
		http.Error(w, "invalid bootstrap request", http.StatusBadRequest)
		return
	}

	bootstrap := gateway.NewRuntimeBootstrapStore(gateway.NewRedisRuntimeBootstrapBackend(s.Store.Redis), 2*time.Minute)
	state, err := bootstrap.Consume(r.Context(), body.Token)
	if err != nil {
		status := http.StatusUnauthorized
		if !errors.Is(err, gateway.ErrRuntimeBootstrapNotFound) && !errors.Is(err, gateway.ErrRuntimeBootstrapConsumed) {
			status = http.StatusServiceUnavailable
		}
		http.Error(w, "runtime bootstrap rejected", status)
		return
	}
	if state.RuntimeSessionID != body.RuntimeSessionID || state.DeviceID != body.DeviceID {
		http.Error(w, "runtime bootstrap binding mismatch", http.StatusForbidden)
		return
	}

	credential, err := s.Store.CreateManagedRuntimeCredential(r.Context(), state.UserID, state.DeviceID, state.DeviceName, body.PublicKey)
	if err != nil {
		http.Error(w, "runtime credential creation failed", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	_ = json.NewEncoder(w).Encode(runtimeBootstrapExchangeResponse{
		CredentialID:     credential.CredentialID,
		CredentialSecret: credential.Secret,
		DeviceID:         credential.DeviceID,
		DeviceName:       credential.DeviceName,
		WorkspaceID:      state.WorkspaceID,
		WorkspaceKey:     state.WorkspaceKey,
		RuntimeSessionID: state.RuntimeSessionID,
	})
}

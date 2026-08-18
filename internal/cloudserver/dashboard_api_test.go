package cloudserver

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/gateway"
	"github.com/0xmarkhydra/codelocal/internal/webauth"
)

func TestDashboardOverviewDTOIsMinimalAndNormalizesState(t *testing.T) {
	devices := []cloud.Device{
		{CredentialID: "credential-secret-1", UserID: "user-1", DeviceID: "device-1", DeviceName: "Mac", PublicKey: "public-key-secret", SecretHash: "secret-hash", LastSeenAt: 90},
		{CredentialID: "credential-secret-2", UserID: "user-1", DeviceID: "device-2", DeviceName: "Linux", SecretHash: "secret-hash-2", LastSeenAt: 80},
		{CredentialID: "credential-revoked", UserID: "user-1", DeviceID: "device-3", DeviceName: "Old", SecretHash: "secret-hash-3", RevokedAt: 1},
	}
	workspaces := []gateway.WorkspaceView{
		{Key: "private-key-1", DeviceID: "device-1", DeviceName: "Mac", WorkspaceID: "workspace-1", WorkspaceName: "Alpha", ProjectID: "project-private", ProjectRoot: "/Users/private/alpha", Capabilities: map[string]any{"shell": true}, Status: "active", RuntimeOnline: true, LastSeenAt: 100},
		{Key: "private-key-2", DeviceID: "device-1", DeviceName: "Mac", WorkspaceID: "workspace-2", WorkspaceName: "Beta", ProjectRoot: "/Users/private/beta", Status: "sleeping", RuntimeOnline: true, LastSeenAt: 90},
		{Key: "private-key-3", DeviceID: "device-2", DeviceName: "Linux", WorkspaceID: "workspace-3", WorkspaceName: "Gamma", Status: "device_offline", RuntimeOnline: false, LastSeenAt: 80},
		{DeviceID: "device-2", DeviceName: "Linux", WorkspaceID: "workspace-4", WorkspaceName: "Delta", Status: "unknown", LastSeenAt: 70},
		{DeviceID: "device-2", DeviceName: "Linux", WorkspaceID: "workspace-5", WorkspaceName: "Epsilon", Status: "sleeping", LastSeenAt: 60},
		{DeviceID: "device-2", DeviceName: "Linux", WorkspaceID: "workspace-6", WorkspaceName: "Zeta", Status: "active", RuntimeOnline: true, LastSeenAt: 50},
	}

	payload := buildDashboardOverviewDTO(dashboardOverviewSource{
		Email:          "user@example.com",
		IsAdmin:        true,
		Devices:        devices,
		DeviceOnline:   map[string]bool{"device-1": true, "device-2": false, "device-3": true},
		Workspaces:     workspaces,
		UsageAvailable: true,
		Usage24h:       cloud.MCPUsageSummary{Calls: 7, InputTokensEst: 100, OutputTokensEst: 50, TotalTokensEst: 150},
		Usage30d:       cloud.MCPUsageSummary{Calls: 20, TotalTokensEst: 900},
		UsageAll:       cloud.MCPUsageSummary{Calls: 30, TotalTokensEst: 1400},
	})

	if payload.User.Email != "user@example.com" || !payload.User.IsAdmin {
		t.Fatalf("unexpected user payload: %#v", payload.User)
	}
	if payload.Devices.Paired != 2 || payload.Devices.Online != 1 {
		t.Fatalf("unexpected device summary: %#v", payload.Devices)
	}
	if payload.Workspaces.Total != 6 || payload.Workspaces.Active != 2 || payload.Workspaces.Sleeping != 2 || payload.Workspaces.Offline != 2 {
		t.Fatalf("unexpected workspace summary: %#v", payload.Workspaces)
	}
	if len(payload.Workspaces.Recent) != 5 {
		t.Fatalf("recent workspace count=%d want 5", len(payload.Workspaces.Recent))
	}
	if payload.Workspaces.Recent[2].Status != "offline" || payload.Workspaces.Recent[3].Status != "offline" {
		t.Fatalf("workspace statuses were not normalized: %#v", payload.Workspaces.Recent)
	}
	if !payload.Usage.Available || !payload.Usage.Estimated || payload.Usage.Scope != dashboardUsageScope {
		t.Fatalf("unexpected usage metadata: %#v", payload.Usage)
	}
	if payload.Usage.Last24h.Calls != 7 || payload.Usage.Last24h.TotalTokensEstimated != 150 {
		t.Fatalf("unexpected usage window: %#v", payload.Usage.Last24h)
	}

	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	serialized := string(raw)
	for _, forbidden := range []string{
		"credential-secret", "public-key-secret", "secret-hash", "private-key", "/Users/private",
		`"credentialId"`, `"publicKey"`, `"secretHash"`, `"projectRoot"`, `"capabilities"`, `"projectId"`, `"key"`,
	} {
		if strings.Contains(serialized, forbidden) {
			t.Fatalf("dashboard API leaked internal field/value %q: %s", forbidden, serialized)
		}
	}
}

func TestDashboardOverviewDTOFailsUsageClosedWithoutHidingCoreState(t *testing.T) {
	payload := buildDashboardOverviewDTO(dashboardOverviewSource{
		Email:      "user@example.com",
		Workspaces: []gateway.WorkspaceView{{WorkspaceID: "workspace-1", WorkspaceName: "Alpha", Status: "sleeping"}},
	})
	if payload.Usage.Available || !payload.Usage.Estimated {
		t.Fatalf("usage availability did not fail closed: %#v", payload.Usage)
	}
	if payload.Workspaces.Total != 1 || payload.Workspaces.Sleeping != 1 {
		t.Fatalf("usage failure hid core workspace state: %#v", payload.Workspaces)
	}
}

func TestDashboardOverviewAPIUnauthenticatedReturnsJSON401WithoutRedirect(t *testing.T) {
	server := &Server{WebAuth: &webauth.Manager{}}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/v1/dashboard/overview", nil)

	server.dashboardOverviewAPI(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status=%d want %d body=%s", recorder.Code, http.StatusUnauthorized, recorder.Body.String())
	}
	if location := recorder.Header().Get("Location"); location != "" {
		t.Fatalf("API must not redirect unauthenticated clients: Location=%q", location)
	}
	if got := recorder.Header().Get("Content-Type"); !strings.HasPrefix(got, "application/json") {
		t.Fatalf("content-type=%q want application/json", got)
	}
	if got := recorder.Header().Get("Cache-Control"); got != "no-store" {
		t.Fatalf("cache-control=%q want no-store", got)
	}
	if !strings.Contains(recorder.Body.String(), `"error":"unauthorized"`) {
		t.Fatalf("unexpected JSON error body: %s", recorder.Body.String())
	}
}

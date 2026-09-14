package penpot

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/0xmarkhydra/codelocal/internal/cloud"
	"github.com/0xmarkhydra/codelocal/internal/oauth"
)

func TestPenpotCloudGrantProbeAndRevocationIntegration(t *testing.T) {
	store := penpotTestStore(t)
	ctx := context.Background()
	auth, err := oauth.New(store, nil, "https://codelocal.test", "isolated-signing-secret")
	if err != nil {
		t.Fatal(err)
	}
	var upstream atomic.Int32
	backend := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Token native-A" {
			t.Error("wrong native key")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"A","email":"a@example.test","isActive":true}`)
	}))
	defer backend.Close()
	mcp := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstream.Add(1)
		fmt.Fprint(w, `{"jsonrpc":"2.0","id":1,"result":{}}`)
	}))
	defer mcp.Close()
	adapter, err := New(auth, backend.URL, mcp.URL, "ws://127.0.0.1:1")
	if err != nil {
		t.Fatal(err)
	}
	// No active device or workspace is required for Cloud grants.
	if _, err := store.DB.Exec(ctx, `DELETE FROM codelocal_workspaces; DELETE FROM codelocal_devices;`); err != nil {
		t.Fatal(err)
	}
	probe, err := auth.IssuePenpotCloudGrant(ctx, "A", "native-A", true)
	if err != nil {
		t.Fatal(err)
	}
	invoke := func(token, method string) int {
		request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"`+method+`"}`))
		request.Header.Set("Authorization", "Bearer "+token)
		response := httptest.NewRecorder()
		adapter.ServeMCP(response, request)
		return response.Code
	}
	if status := invoke("native-A", "tools/list"); status != 401 {
		t.Fatalf("raw key accepted: %d", status)
	}
	if status := invoke(probe, "tools/list"); status != 200 {
		t.Fatalf("probe failed: %d", status)
	}
	if status := invoke(probe, "tools/call"); status != 403 {
		t.Fatalf("probe executed tool: %d", status)
	}
	_, err = store.SavePluginCloudConnection(ctx, cloud.PluginConnection{UserID: "A", PluginID: "penpot", ServerName: "penpot", Endpoint: "https://design.codelocal.cloud/mcp/stream", State: cloud.PluginConnectionReady}, cloud.PluginCloudCredential{Kind: "bearer", Token: "native-A"})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := auth.IssuePenpotCloudGrant(ctx, "A", "native-A", false)
	if err != nil {
		t.Fatal(err)
	}
	if status := invoke(grant, "tools/call"); status != 200 {
		t.Fatalf("cloud call failed: %d", status)
	}
	if _, err = store.DeletePluginCloudConnection(ctx, "A", "penpot"); err != nil {
		t.Fatal(err)
	}
	if status := invoke(grant, "tools/call"); status != 401 {
		t.Fatalf("revoked cloud call accepted: %d", status)
	}
	if upstream.Load() != 2 {
		t.Fatalf("unexpected upstream calls: %d", upstream.Load())
	}
}

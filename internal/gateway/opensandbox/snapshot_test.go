package opensandbox

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSnapshotLifecycleUsesOfficialPathsAndFilters(t *testing.T) {
	calls := []string{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls = append(calls, r.Method+" "+r.URL.RequestURI())
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/sandboxes/sb-1/snapshots":
			var body CreateSnapshotRequest
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Name != "workspace-snapshot" {
				t.Errorf("snapshot body = %#v err=%v", body, err)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = w.Write([]byte(`{"id":"snap-1","sandboxId":"sb-1","name":"workspace-snapshot","status":{"state":"Creating"},"createdAt":"2026-08-29T00:00:00Z"}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/snapshots":
			if r.URL.Query().Get("name") != "workspace-snapshot" {
				t.Errorf("name query = %q", r.URL.Query().Get("name"))
			}
			states := r.URL.Query()["state"]
			if len(states) != 1 || states[0] != "Ready" {
				t.Errorf("state query = %#v", states)
			}
			_, _ = w.Write([]byte(`{"items":[{"id":"snap-1","sandboxId":"sb-1","name":"workspace-snapshot","status":{"state":"Ready"},"createdAt":"2026-08-29T00:00:00Z"}]}`))
		case r.Method == http.MethodGet && r.URL.Path == "/v1/snapshots/snap-1":
			_, _ = w.Write([]byte(`{"id":"snap-1","sandboxId":"sb-1","name":"workspace-snapshot","status":{"state":"Ready"},"createdAt":"2026-08-29T00:00:00Z"}`))
		case r.Method == http.MethodDelete && r.URL.Path == "/v1/snapshots/snap-1":
			w.WriteHeader(http.StatusNoContent)
		default:
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	client, err := NewClient(Config{BaseURL: server.URL, HTTPClient: server.Client()})
	if err != nil {
		t.Fatal(err)
	}
	ctx := context.Background()
	created, err := client.CreateSnapshot(ctx, "sb-1", "workspace-snapshot")
	if err != nil || created.ID != "snap-1" {
		t.Fatalf("CreateSnapshot() result=%#v err=%v", created, err)
	}
	listed, err := client.ListSnapshots(ctx, SnapshotListOptions{Name: "workspace-snapshot", States: []string{"Ready"}, PageSize: 20})
	if err != nil || len(listed.Items) != 1 {
		t.Fatalf("ListSnapshots() result=%#v err=%v", listed, err)
	}
	if _, err := client.GetSnapshot(ctx, "snap-1"); err != nil {
		t.Fatal(err)
	}
	if err := client.DeleteSnapshot(ctx, "snap-1"); err != nil {
		t.Fatal(err)
	}
	if len(calls) != 4 {
		t.Fatalf("calls=%#v", calls)
	}
}

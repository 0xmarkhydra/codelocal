package cloudserver

import (
	"net/http/httptest"
	"testing"
)

func TestDashboardChatRequestIDKeepsValidClientID(t *testing.T) {
	const requestID = "chat-mobile-retry-12345678"
	if got := dashboardChatRequestID(requestID); got != requestID {
		t.Fatalf("dashboardChatRequestID() = %q, want %q", got, requestID)
	}
	if got := dashboardChatRequestID("bad id with spaces"); got == "" || got == "bad id with spaces" {
		t.Fatalf("dashboardChatRequestID() did not replace invalid id: %q", got)
	}
}

func TestDashboardRuntimeRequestIDStableAcrossReconnect(t *testing.T) {
	const requestID = "chat-mobile-retry-12345678"
	args := map[string]any{"path": "README.md", "oldText": "before", "newText": "after"}

	firstRequest := dashboardWithExecutionState(httptest.NewRequest("POST", "/api/v1/dashboard/chat", nil), "user-1", requestID, nil)
	firstState := dashboardExecutionStateFromRequest(firstRequest)
	first := dashboardRuntimeRequestID(firstState, "edit_file", args)
	secondOccurrence := dashboardRuntimeRequestID(firstState, "edit_file", args)
	if first == secondOccurrence {
		t.Fatalf("two occurrences in one agent run reused idempotency key %q", first)
	}

	retryRequest := dashboardWithExecutionState(httptest.NewRequest("POST", "/api/v1/dashboard/chat", nil), "user-1", requestID, nil)
	retryState := dashboardExecutionStateFromRequest(retryRequest)
	replayed := dashboardRuntimeRequestID(retryState, "edit_file", args)
	if replayed != first {
		t.Fatalf("reconnect idempotency key = %q, want %q", replayed, first)
	}

	if got, want := dashboardChatMessageID(retryRequest, "user-1", "user"), dashboardChatMessageID(firstRequest, "user-1", "user"); got != want {
		t.Fatalf("reconnect user message id = %q, want %q", got, want)
	}
	if dashboardChatMessageID(firstRequest, "user-1", "assistant") == dashboardChatMessageID(firstRequest, "user-1", "user") {
		t.Fatal("assistant and user messages must not share the same deterministic id")
	}
}

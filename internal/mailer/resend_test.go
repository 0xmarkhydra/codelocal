package mailer

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSendUsesResendEmailEndpointAndIdempotencyKey(t *testing.T) {
	var got resendMessage
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/emails" {
			t.Fatalf("unexpected path: %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer re_test" {
			t.Fatalf("missing bearer authorization: %q", r.Header.Get("Authorization"))
		}
		if r.Header.Get("Idempotency-Key") != "otp-1" {
			t.Fatalf("unexpected idempotency key: %q", r.Header.Get("Idempotency-Key"))
		}
		if err := json.NewDecoder(r.Body).Decode(&got); err != nil {
			t.Fatal(err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"mail_1"}`))
	}))
	defer server.Close()

	client := &Client{APIKey: "re_test", From: "CodeLocal <updates@example.com>", APIBaseURL: server.URL, HTTPClient: server.Client()}
	err := client.Send(context.Background(), Message{To: "user@example.com", Subject: "Verify", Text: "123456"}, "otp-1")
	if err != nil {
		t.Fatal(err)
	}
	if got.From != client.From || got.To != "user@example.com" || got.Subject != "Verify" {
		t.Fatalf("unexpected payload: %+v", got)
	}
}

func TestSendBatchRejectsMoreThanResendLimit(t *testing.T) {
	client := &Client{APIKey: "re_test", From: "CodeLocal <updates@example.com>"}
	messages := make([]Message, 101)
	for i := range messages {
		messages[i] = Message{To: "user@example.com", Subject: "Release"}
	}
	if err := client.SendBatch(context.Background(), messages, "release-1"); err == nil {
		t.Fatal("expected batch size error")
	}
}

package gateway

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

type fakeRuntimeBootstrapBackend struct {
	values map[string][]byte
}

func newFakeRuntimeBootstrapBackend() *fakeRuntimeBootstrapBackend {
	return &fakeRuntimeBootstrapBackend{values: map[string][]byte{}}
}

func (b *fakeRuntimeBootstrapBackend) Put(_ context.Context, key string, value []byte, _ time.Duration) (bool, error) {
	if _, exists := b.values[key]; exists {
		return false, nil
	}
	b.values[key] = append([]byte(nil), value...)
	return true, nil
}

func (b *fakeRuntimeBootstrapBackend) Take(_ context.Context, key string) ([]byte, bool, error) {
	value, exists := b.values[key]
	if !exists {
		return nil, false, nil
	}
	delete(b.values, key)
	return append([]byte(nil), value...), true, nil
}

func testRuntimeBootstrapState() RuntimeBootstrapState {
	return RuntimeBootstrapState{
		UserID:           "user-1",
		WorkspaceKey:     "user-1::device-1::workspace-1",
		WorkspaceID:      "workspace-1",
		Profile:          "general-small",
		RuntimeSessionID: "session-1",
		DeviceID:         "cloud-session-1",
		DeviceName:       "CodeLocal Cloud Runtime",
	}
}

func TestRuntimeBootstrapIsOneTime(t *testing.T) {
	backend := newFakeRuntimeBootstrapBackend()
	store := NewRuntimeBootstrapStore(backend, time.Minute)
	store.Now = func() time.Time { return time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC) }

	token, err := store.Issue(context.Background(), testRuntimeBootstrapState())
	if err != nil {
		t.Fatalf("Issue() error = %v", err)
	}
	if len(token) != 64 {
		t.Fatalf("token length = %d, want 64", len(token))
	}
	state, err := store.Consume(context.Background(), token)
	if err != nil {
		t.Fatalf("Consume() error = %v", err)
	}
	if state.UserID != "user-1" || state.WorkspaceID != "workspace-1" || state.ExpiresAt <= state.CreatedAt {
		t.Fatalf("Consume() state = %#v", state)
	}
	if _, err := store.Consume(context.Background(), token); !errors.Is(err, ErrRuntimeBootstrapConsumed) {
		t.Fatalf("second Consume() error = %v, want consumed", err)
	}
}

func TestRuntimeBootstrapKeyDoesNotContainRawToken(t *testing.T) {
	token := strings.Repeat("a", 64)
	key := runtimeBootstrapKey(token)
	if strings.Contains(key, token) {
		t.Fatalf("bootstrap key leaks token: %q", key)
	}
}

func TestRuntimeBootstrapRejectsExpiredStateEvenIfBackendReturnsIt(t *testing.T) {
	backend := newFakeRuntimeBootstrapBackend()
	issuedAt := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	store := NewRuntimeBootstrapStore(backend, time.Second)
	store.Now = func() time.Time { return issuedAt }
	token, err := store.Issue(context.Background(), testRuntimeBootstrapState())
	if err != nil {
		t.Fatal(err)
	}
	store.Now = func() time.Time { return issuedAt.Add(2 * time.Second) }
	if _, err := store.Consume(context.Background(), token); !errors.Is(err, ErrRuntimeBootstrapNotFound) {
		t.Fatalf("Consume() error = %v, want expired/not found", err)
	}
}

func TestRuntimeBootstrapRequiresBoundSessionAndWorkspace(t *testing.T) {
	store := NewRuntimeBootstrapStore(newFakeRuntimeBootstrapBackend(), time.Minute)
	state := testRuntimeBootstrapState()
	state.RuntimeSessionID = ""
	if _, err := store.Issue(context.Background(), state); err == nil {
		t.Fatal("Issue() error = nil, want session validation")
	}
	state = testRuntimeBootstrapState()
	state.WorkspaceID = ""
	if _, err := store.Issue(context.Background(), state); err == nil {
		t.Fatal("Issue() error = nil, want workspace validation")
	}
}

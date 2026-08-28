package gateway

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeRuntimeLeaseBackend struct {
	ownerByKey map[string]string
	acquires   int
	renews     int
	releases   int
	acquireErr error
	renewErr   error
	releaseErr error
}

func newFakeRuntimeLeaseBackend() *fakeRuntimeLeaseBackend {
	return &fakeRuntimeLeaseBackend{ownerByKey: map[string]string{}}
}

func (b *fakeRuntimeLeaseBackend) Acquire(_ context.Context, key, token string, _ time.Duration) (bool, error) {
	b.acquires++
	if b.acquireErr != nil {
		return false, b.acquireErr
	}
	if _, exists := b.ownerByKey[key]; exists {
		return false, nil
	}
	b.ownerByKey[key] = token
	return true, nil
}

func (b *fakeRuntimeLeaseBackend) Renew(_ context.Context, key, token string, _ time.Duration) (bool, error) {
	b.renews++
	if b.renewErr != nil {
		return false, b.renewErr
	}
	return b.ownerByKey[key] == token, nil
}

func (b *fakeRuntimeLeaseBackend) Release(_ context.Context, key, token string) (bool, error) {
	b.releases++
	if b.releaseErr != nil {
		return false, b.releaseErr
	}
	if b.ownerByKey[key] != token {
		return false, nil
	}
	delete(b.ownerByKey, key)
	return true, nil
}

func testRuntimeLeaseScope() RuntimeLeaseScope {
	return RuntimeLeaseScope{
		UserID:       "user-1",
		WorkspaceKey: "device-1::workspace-1",
		Provider:     RuntimeProviderCloud,
		Profile:      "general-small",
	}
}

func TestRuntimeLeaseAllowsOnlyOneOwnerPerScope(t *testing.T) {
	backend := newFakeRuntimeLeaseBackend()
	coordinator := NewRuntimeLeaseCoordinator(backend, 20*time.Second)
	ctx := context.Background()

	first, err := coordinator.Acquire(ctx, testRuntimeLeaseScope())
	if err != nil {
		t.Fatalf("first Acquire() error = %v", err)
	}
	if _, err := coordinator.Acquire(ctx, testRuntimeLeaseScope()); !errors.Is(err, ErrRuntimeLeaseHeld) {
		t.Fatalf("second Acquire() error = %v, want ErrRuntimeLeaseHeld", err)
	}
	if err := first.Release(ctx); err != nil {
		t.Fatalf("Release() error = %v", err)
	}
	if _, err := coordinator.Acquire(ctx, testRuntimeLeaseScope()); err != nil {
		t.Fatalf("Acquire() after release error = %v", err)
	}
}

func TestRuntimeLeaseRenewFailsAfterOwnershipIsLost(t *testing.T) {
	backend := newFakeRuntimeLeaseBackend()
	coordinator := NewRuntimeLeaseCoordinator(backend, 20*time.Second)
	lease, err := coordinator.Acquire(context.Background(), testRuntimeLeaseScope())
	if err != nil {
		t.Fatal(err)
	}
	backend.ownerByKey[lease.key] = "different-owner"
	if err := lease.Renew(context.Background()); !errors.Is(err, ErrRuntimeLeaseLost) {
		t.Fatalf("Renew() error = %v, want ErrRuntimeLeaseLost", err)
	}
}

func TestRuntimeLeaseReleaseCannotDeleteDifferentOwner(t *testing.T) {
	backend := newFakeRuntimeLeaseBackend()
	coordinator := NewRuntimeLeaseCoordinator(backend, 20*time.Second)
	lease, err := coordinator.Acquire(context.Background(), testRuntimeLeaseScope())
	if err != nil {
		t.Fatal(err)
	}
	backend.ownerByKey[lease.key] = "different-owner"
	if err := lease.Release(context.Background()); !errors.Is(err, ErrRuntimeLeaseLost) {
		t.Fatalf("Release() error = %v, want ErrRuntimeLeaseLost", err)
	}
	if backend.ownerByKey[lease.key] != "different-owner" {
		t.Fatal("Release() deleted another owner's lease")
	}
}

func TestRuntimeLeaseKeysDoNotExposeRawIdentity(t *testing.T) {
	scope := testRuntimeLeaseScope()
	key := runtimeLeaseKey(scope)
	if key == "" {
		t.Fatal("runtimeLeaseKey() returned empty key")
	}
	for _, raw := range []string{scope.UserID, scope.WorkspaceKey, scope.Profile} {
		if containsRuntimeLeaseKey(key, raw) {
			t.Fatalf("runtime lease key leaks raw scope value %q: %q", raw, key)
		}
	}
}

func containsRuntimeLeaseKey(key, value string) bool {
	if value == "" || len(value) > len(key) {
		return false
	}
	for i := 0; i+len(value) <= len(key); i++ {
		if key[i:i+len(value)] == value {
			return true
		}
	}
	return false
}

func TestRuntimeLeaseRequiresConcreteProvider(t *testing.T) {
	backend := newFakeRuntimeLeaseBackend()
	coordinator := NewRuntimeLeaseCoordinator(backend, 0)
	scope := testRuntimeLeaseScope()
	scope.Provider = RuntimeProviderAuto
	if _, err := coordinator.Acquire(context.Background(), scope); err == nil {
		t.Fatal("Acquire() error = nil, want concrete provider validation")
	}
}

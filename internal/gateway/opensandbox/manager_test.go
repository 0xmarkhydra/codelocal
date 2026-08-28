package opensandbox

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeLifecycle struct {
	listed          []SandboxInfo
	getSequence     []SandboxInfo
	created         *SandboxInfo
	createRequest   CreateSandboxRequest
	resumeIDs       []string
	renewIDs        []string
	listOptions     ListOptions
	createCount     int
	getCount        int
	listErr         error
	createErr       error
	resumeErr       error
	renewErr        error
	renewExpiration time.Time
}

func (f *fakeLifecycle) CreateSandbox(_ context.Context, request CreateSandboxRequest) (*SandboxInfo, error) {
	f.createCount++
	f.createRequest = request
	if f.createErr != nil {
		return nil, f.createErr
	}
	return f.created, nil
}

func (f *fakeLifecycle) GetSandbox(context.Context, string) (*SandboxInfo, error) {
	if len(f.getSequence) == 0 {
		return nil, errors.New("no fake sandbox state")
	}
	index := f.getCount
	if index >= len(f.getSequence) {
		index = len(f.getSequence) - 1
	}
	f.getCount++
	item := f.getSequence[index]
	return &item, nil
}

func (f *fakeLifecycle) ListSandboxes(_ context.Context, options ListOptions) (*ListSandboxesResponse, error) {
	f.listOptions = options
	if f.listErr != nil {
		return nil, f.listErr
	}
	return &ListSandboxesResponse{Items: append([]SandboxInfo(nil), f.listed...)}, nil
}

func (f *fakeLifecycle) ResumeSandbox(_ context.Context, id string) error {
	f.resumeIDs = append(f.resumeIDs, id)
	return f.resumeErr
}

func (f *fakeLifecycle) RenewExpiration(_ context.Context, id string, expiresAt time.Time) (*RenewExpirationResponse, error) {
	f.renewIDs = append(f.renewIDs, id)
	f.renewExpiration = expiresAt
	if f.renewErr != nil {
		return nil, f.renewErr
	}
	return &RenewExpirationResponse{ExpiresAt: &expiresAt}, nil
}

func testAcquireSpec() AcquireSpec {
	return AcquireSpec{
		OwnerID:        "user-1",
		WorkspaceKey:   "user-1::device-1::workspace-1",
		WorkspaceID:    "workspace-1",
		Profile:        "general-small",
		Image:          "codelocal/runtime:v1",
		Entrypoint:     []string{"codelocal", "cloud-runtime"},
		ResourceLimits: map[string]string{"cpu": "2", "memory": "4Gi"},
		TTL:            30 * time.Minute,
		WaitTimeout:    100 * time.Millisecond,
		PollInterval:   time.Millisecond,
	}
}

func TestManagerAcquireReusesRunningSandbox(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	expires := now.Add(2 * time.Minute)
	fake := &fakeLifecycle{listed: []SandboxInfo{{ID: "sb-running", Status: SandboxStatus{State: "Running"}, ExpiresAt: &expires}}}
	manager := NewManager(fake)
	manager.Now = func() time.Time { return now }

	spec := testAcquireSpec()
	result, err := manager.Acquire(context.Background(), spec)
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if result.ID != "sb-running" {
		t.Fatalf("sandbox = %q", result.ID)
	}
	if fake.createCount != 0 {
		t.Fatalf("createCount = %d, want 0", fake.createCount)
	}
	if len(fake.renewIDs) != 1 || fake.renewIDs[0] != "sb-running" {
		t.Fatalf("renewIDs = %#v", fake.renewIDs)
	}
	if got := fake.listOptions.Metadata[metadataWorkspaceKey]; got != stableScopeHash(spec.WorkspaceKey) {
		t.Fatalf("workspace metadata = %q, want key hash", got)
	}
	if fake.listOptions.Metadata[metadataWorkspaceKey] == spec.WorkspaceKey {
		t.Fatal("workspace metadata must not expose raw workspace key")
	}
	if fake.listOptions.Metadata[metadataOwnerKey] == "user-1" {
		t.Fatal("owner metadata must be hashed, not raw user identity")
	}
}

func TestManagerAcquireResumesPausedSandbox(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	fake := &fakeLifecycle{
		listed:      []SandboxInfo{{ID: "sb-paused", Status: SandboxStatus{State: "Paused"}}},
		getSequence: []SandboxInfo{{ID: "sb-paused", Status: SandboxStatus{State: "Running"}}},
	}
	manager := NewManager(fake)
	manager.Now = func() time.Time { return now }

	result, err := manager.Acquire(context.Background(), testAcquireSpec())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if result.ID != "sb-paused" || len(fake.resumeIDs) != 1 {
		t.Fatalf("result=%#v resumeIDs=%#v", result, fake.resumeIDs)
	}
}

func TestManagerAcquireCreatesWhenNoReusableSandbox(t *testing.T) {
	now := time.Date(2026, 8, 29, 0, 0, 0, 0, time.UTC)
	fake := &fakeLifecycle{
		created:     &SandboxInfo{ID: "sb-new", Status: SandboxStatus{State: "Creating"}},
		getSequence: []SandboxInfo{{ID: "sb-new", Status: SandboxStatus{State: "Running"}}},
	}
	manager := NewManager(fake)
	manager.Now = func() time.Time { return now }

	result, err := manager.Acquire(context.Background(), testAcquireSpec())
	if err != nil {
		t.Fatalf("Acquire() error = %v", err)
	}
	if result.ID != "sb-new" || fake.createCount != 1 {
		t.Fatalf("result=%#v createCount=%d", result, fake.createCount)
	}
	if fake.createRequest.Metadata[metadataProfileKey] != "general-small" {
		t.Fatalf("metadata = %#v", fake.createRequest.Metadata)
	}
	if fake.createRequest.Timeout == nil || *fake.createRequest.Timeout != 1800 {
		t.Fatalf("timeout = %#v", fake.createRequest.Timeout)
	}
}

func TestBestReusableSandboxRejectsUnexpectedState(t *testing.T) {
	if got := bestReusableSandbox([]SandboxInfo{{ID: "bad", Status: SandboxStatus{State: "Error"}}}); got != nil {
		t.Fatalf("bestReusableSandbox() = %#v, want nil", got)
	}
}

func TestManagerAcquireRejectsMissingIsolationLimits(t *testing.T) {
	spec := testAcquireSpec()
	spec.ResourceLimits = nil
	manager := NewManager(&fakeLifecycle{})
	if _, err := manager.Acquire(context.Background(), spec); err == nil {
		t.Fatal("Acquire() error = nil, want resource limit validation")
	}
}

func TestManagerAcquireRejectsMissingWorkspaceKey(t *testing.T) {
	spec := testAcquireSpec()
	spec.WorkspaceKey = ""
	manager := NewManager(&fakeLifecycle{})
	if _, err := manager.Acquire(context.Background(), spec); err == nil {
		t.Fatal("Acquire() error = nil, want workspace key validation")
	}
}
